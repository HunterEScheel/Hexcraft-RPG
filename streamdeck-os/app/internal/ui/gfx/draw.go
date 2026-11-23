package gfx

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// ---- theme ----

var (
	cBgTop     = rgb(0x0e, 0x11, 0x16)
	cBgBottom  = rgb(0x17, 0x1c, 0x26)
	cSurface   = rgb(0x1b, 0x20, 0x29)
	cSurface2  = rgb(0x24, 0x2b, 0x36)
	cBorder    = rgb(0x2e, 0x36, 0x43)
	cText      = rgb(0xec, 0xef, 0xf4)
	cMuted     = rgb(0x93, 0x9f, 0xb0)
	cAccent    = rgb(0x4f, 0x8c, 0xff)
	cAccentDim = color.RGBA{0x24, 0x34, 0x50, 0xff}
	cSuccess   = rgb(0x34, 0xc7, 0x59)
	cWarning   = rgb(0xff, 0xb0, 0x20)
	cDanger    = rgb(0xff, 0x5a, 0x52)
	cBlack     = rgb(0, 0, 0)
	cShadow    = color.RGBA{0, 0, 0, 0x50}
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 0xff} }

func hexColor(s string, def color.RGBA) color.RGBA {
	if len(s) == 7 && s[0] == '#' {
		var v [3]uint8
		for i := 0; i < 3; i++ {
			h, l := unhex(s[1+2*i]), unhex(s[2+2*i])
			if h < 0 || l < 0 {
				return def
			}
			v[i] = uint8(h<<4 | l)
		}
		return rgb(v[0], v[1], v[2])
	}
	return def
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// ---- fonts ----

type fontKind int

const (
	regular fontKind = iota
	medium
	bold
	mono
)

var (
	fontOnce  sync.Once
	fontFiles [4]*opentype.Font
	faceMu    sync.Mutex
	faceCache = map[[2]int]font.Face{}
)

func face(k fontKind, size int) font.Face {
	fontOnce.Do(func() {
		for i, ttf := range [][]byte{goregular.TTF, gomedium.TTF, gobold.TTF, gomonobold.TTF} {
			f, err := opentype.Parse(ttf)
			if err != nil {
				panic(err)
			}
			fontFiles[i] = f
		}
	})
	faceMu.Lock()
	defer faceMu.Unlock()
	key := [2]int{int(k), size}
	if f, ok := faceCache[key]; ok {
		return f
	}
	f, err := opentype.NewFace(fontFiles[k], &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	faceCache[key] = f
	return f
}

func textWidth(f font.Face, s string) int { return font.MeasureString(f, s).Ceil() }

// text draws s with its baseline at y (align: 0 left, 1 centre, 2 right of x).
func text(dst *image.RGBA, f font.Face, s string, x, y int, c color.Color, align int) {
	w := textWidth(f, s)
	switch align {
	case 1:
		x -= w / 2
	case 2:
		x -= w
	}
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

// textMid draws text vertically centred on cy.
func textMid(dst *image.RGBA, f font.Face, s string, x, cy int, c color.Color, align int) {
	m := f.Metrics()
	base := cy + (m.Ascent.Ceil()-m.Descent.Ceil())/2
	text(dst, f, s, x, base, c, align)
}

// ellipsize shortens s to fit w pixels.
func ellipsize(f font.Face, s string, w int) string {
	if textWidth(f, s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && textWidth(f, string(r)+"…") > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// wrap breaks s into lines no wider than w pixels.
func wrap(f font.Face, s string, w int) []string {
	var out []string
	for _, para := range splitLines(s) {
		cur := ""
		for _, word := range fields(para) {
			try := word
			if cur != "" {
				try = cur + " " + word
			}
			if cur != "" && textWidth(f, try) > w {
				out = append(out, cur)
				cur = word
			} else {
				cur = try
			}
		}
		out = append(out, cur)
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func fields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// ---- shapes (anti-aliased with x/image/vector) ----

type painter struct {
	img *image.RGBA
	ras *vector.Rasterizer
}

func (p *painter) fill(r image.Rectangle, c color.Color, path func(z *vector.Rasterizer, ox, oy float32)) {
	r = r.Intersect(p.img.Bounds())
	if r.Empty() {
		return
	}
	if p.ras == nil {
		p.ras = vector.NewRasterizer(r.Dx(), r.Dy())
	} else {
		p.ras.Reset(r.Dx(), r.Dy())
	}
	p.ras.DrawOp = draw.Over
	path(p.ras, float32(r.Min.X), float32(r.Min.Y))
	p.ras.Draw(p.img, r, image.NewUniform(c), image.Point{})
}

func roundPath(z *vector.Rasterizer, x0, y0, x1, y1, rad float32, reverse bool) {
	rad = min(rad, (x1-x0)/2, (y1-y0)/2)
	const k = 0.5523 // circle approximation with cubic Béziers
	if !reverse {
		z.MoveTo(x0+rad, y0)
		z.LineTo(x1-rad, y0)
		z.CubeTo(x1-rad+rad*k, y0, x1, y0+rad-rad*k, x1, y0+rad)
		z.LineTo(x1, y1-rad)
		z.CubeTo(x1, y1-rad+rad*k, x1-rad+rad*k, y1, x1-rad, y1)
		z.LineTo(x0+rad, y1)
		z.CubeTo(x0+rad-rad*k, y1, x0, y1-rad+rad*k, x0, y1-rad)
		z.LineTo(x0, y0+rad)
		z.CubeTo(x0, y0+rad-rad*k, x0+rad-rad*k, y0, x0+rad, y0)
	} else {
		z.MoveTo(x0+rad, y0)
		z.CubeTo(x0+rad-rad*k, y0, x0, y0+rad-rad*k, x0, y0+rad)
		z.LineTo(x0, y1-rad)
		z.CubeTo(x0, y1-rad+rad*k, x0+rad-rad*k, y1, x0+rad, y1)
		z.LineTo(x1-rad, y1)
		z.CubeTo(x1-rad+rad*k, y1, x1, y1-rad+rad*k, x1, y1-rad)
		z.LineTo(x1, y0+rad)
		z.CubeTo(x1, y0+rad-rad*k, x1-rad+rad*k, y0, x1-rad, y0)
	}
	z.ClosePath()
}

// roundRect fills r with rounded corners.
func (p *painter) roundRect(r image.Rectangle, rad int, c color.Color) {
	p.fill(r, c, func(z *vector.Rasterizer, ox, oy float32) {
		roundPath(z, float32(r.Min.X)-ox, float32(r.Min.Y)-oy, float32(r.Max.X)-ox, float32(r.Max.Y)-oy, float32(rad), false)
	})
}

// ring draws a rounded outline of width w inside r.
func (p *painter) ring(r image.Rectangle, rad, w int, c color.Color) {
	p.fill(r, c, func(z *vector.Rasterizer, ox, oy float32) {
		x0, y0, x1, y1 := float32(r.Min.X)-ox, float32(r.Min.Y)-oy, float32(r.Max.X)-ox, float32(r.Max.Y)-oy
		roundPath(z, x0, y0, x1, y1, float32(rad), false)
		fw := float32(w)
		roundPath(z, x0+fw, y0+fw, x1-fw, y1-fw, float32(rad)-fw, true)
	})
}

// shadow fakes a soft drop shadow under r.
func (p *painter) shadow(r image.Rectangle, rad, spread int) {
	for i := spread; i > 0; i -= max(1, spread/3) {
		a := uint8(0x18)
		p.roundRect(r.Add(image.Pt(0, spread/2)).Inset(-i), rad+i, color.RGBA{0, 0, 0, a})
	}
}

func (p *painter) circle(cx, cy, rad int, c color.Color) {
	p.roundRect(image.Rect(cx-rad, cy-rad, cx+rad, cy+rad), rad, c)
}

// poly fills a polygon given absolute points.
func (p *painter) poly(pts [][2]float32, c color.Color) {
	if len(pts) < 3 {
		return
	}
	minX, minY, maxX, maxY := pts[0][0], pts[0][1], pts[0][0], pts[0][1]
	for _, q := range pts {
		minX, minY, maxX, maxY = min(minX, q[0]), min(minY, q[1]), max(maxX, q[0]), max(maxY, q[1])
	}
	r := image.Rect(int(minX)-1, int(minY)-1, int(maxX)+2, int(maxY)+2)
	p.fill(r, c, func(z *vector.Rasterizer, ox, oy float32) {
		z.MoveTo(pts[0][0]-ox, pts[0][1]-oy)
		for _, q := range pts[1:] {
			z.LineTo(q[0]-ox, q[1]-oy)
		}
		z.ClosePath()
	})
}

// line draws a thick line segment.
func (p *painter) line(x0, y0, x1, y1, w float32, c color.Color) {
	dx, dy := x1-x0, y1-y0
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l == 0 {
		return
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	p.poly([][2]float32{{x0 + nx, y0 + ny}, {x1 + nx, y1 + ny}, {x1 - nx, y1 - ny}, {x0 - nx, y0 - ny}}, c)
}

// arc draws a thick circular arc (angles in radians, 0 = right, clockwise).
func (p *painter) arc(cx, cy, rad, w, a0, a1 float32, c color.Color) {
	n := int(max(8, (a1-a0)*rad/4))
	var pts [][2]float32
	for i := 0; i <= n; i++ {
		a := float64(a0 + (a1-a0)*float32(i)/float32(n))
		pts = append(pts, [2]float32{cx + (rad+w/2)*float32(math.Cos(a)), cy + (rad+w/2)*float32(math.Sin(a))})
	}
	for i := n; i >= 0; i-- {
		a := float64(a0 + (a1-a0)*float32(i)/float32(n))
		pts = append(pts, [2]float32{cx + (rad-w/2)*float32(math.Cos(a)), cy + (rad-w/2)*float32(math.Sin(a))})
	}
	p.poly(pts, c)
}

// gradient fills r with a vertical gradient.
func gradient(img *image.RGBA, r image.Rectangle, top, bottom color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		t := float64(y-r.Min.Y) / float64(max(1, r.Dy()-1))
		c := color.RGBA{
			uint8(float64(top.R) + (float64(bottom.R)-float64(top.R))*t),
			uint8(float64(top.G) + (float64(bottom.G)-float64(top.G))*t),
			uint8(float64(top.B) + (float64(bottom.B)-float64(top.B))*t), 0xff}
		draw.Draw(img, image.Rect(r.Min.X, y, r.Max.X, y+1), image.NewUniform(c), image.Point{}, draw.Src)
	}
}

// ---- icons (drawn in a size×size box at x,y) ----

func (p *painter) icon(name string, x, y, size int, c color.Color) {
	s := float32(size)
	X, Y := float32(x), float32(y)
	at := func(fx, fy float32) (float32, float32) { return X + fx*s, Y + fy*s }
	lw := max(1.5, s/11)
	switch name {
	case "wifi":
		cx, cy := at(0.5, 0.78)
		for i, r := range []float32{0.18, 0.34, 0.5} {
			_ = i
			p.arc(cx, cy, r*s, lw, -math.Pi*0.78, -math.Pi*0.22, c)
		}
		p.circle(int(cx), int(cy), int(max(2, s/12)), c)
	case "light":
		cx, cy := at(0.5, 0.4)
		p.circle(int(cx), int(cy), int(s*0.26), c)
		x0, y0 := at(0.36, 0.66)
		x1, y1 := at(0.64, 0.86)
		p.roundRect(image.Rect(int(x0), int(y0), int(x1), int(y1)), int(s*0.05), c)
	case "music":
		hx, hy := at(0.34, 0.76)
		p.circle(int(hx), int(hy), int(s*0.14), c)
		sx, sy := at(0.46, 0.76)
		tx, ty := at(0.46, 0.14)
		p.line(sx, sy, tx, ty, lw, c)
		fx, fy := at(0.74, 0.3)
		p.line(tx, ty, fx, fy, lw, c)
	case "gear":
		cx, cy := at(0.5, 0.5)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			p.line(cx+float32(math.Cos(a))*s*0.24, cy+float32(math.Sin(a))*s*0.24,
				cx+float32(math.Cos(a))*s*0.44, cy+float32(math.Sin(a))*s*0.44, s*0.14, c)
		}
		p.circle(int(cx), int(cy), int(s*0.3), c)
		p.circle(int(cx), int(cy), int(s*0.12), cSurface)
	case "deck":
		for r := 0; r < 2; r++ {
			for col := 0; col < 3; col++ {
				x0, y0 := at(0.1+float32(col)*0.28, 0.24+float32(r)*0.28)
				p.roundRect(image.Rect(int(x0), int(y0), int(x0+s*0.22), int(y0+s*0.22)), int(s*0.05), c)
			}
		}
	case "key":
		cx, cy := at(0.32, 0.5)
		p.arc(cx, cy, s*0.16, lw, 0, 2*math.Pi, c)
		x0, y0 := at(0.48, 0.5)
		x1, _ := at(0.9, 0.5)
		p.line(x0, y0, x1, y0, lw, c)
		tx, ty := at(0.8, 0.5)
		p.line(tx, ty, tx, ty+s*0.16, lw, c)
		ux, _ := at(0.68, 0.5)
		p.line(ux, ty, ux, ty+s*0.12, lw, c)
	case "folder":
		x0, y0 := at(0.1, 0.3)
		x1, y1 := at(0.9, 0.82)
		p.roundRect(image.Rect(int(x0), int(y0), int(x1), int(y1)), int(s*0.06), c)
		tx0, ty0 := at(0.1, 0.2)
		tx1, ty1 := at(0.45, 0.36)
		p.roundRect(image.Rect(int(tx0), int(ty0), int(tx1), int(ty1)), int(s*0.05), c)
	case "check":
		a, b := at(0.2, 0.52)
		c2, d := at(0.42, 0.74)
		e, f := at(0.82, 0.28)
		p.line(a, b, c2, d, lw*1.3, c)
		p.line(c2, d, e, f, lw*1.3, c)
	case "plus":
		a, b := at(0.5, 0.18)
		_, d := at(0.5, 0.82)
		p.line(a, b, a, d, lw*1.2, c)
		e, f := at(0.18, 0.5)
		g, _ := at(0.82, 0.5)
		p.line(e, f, g, f, lw*1.2, c)
	case "trash":
		x0, y0 := at(0.26, 0.32)
		x1, y1 := at(0.74, 0.86)
		p.roundRect(image.Rect(int(x0), int(y0), int(x1), int(y1)), int(s*0.05), c)
		a, b := at(0.16, 0.24)
		e, _ := at(0.84, 0.24)
		p.line(a, b, e, b, lw, c)
		hx0, hy0 := at(0.4, 0.14)
		hx1, _ := at(0.6, 0.14)
		p.line(hx0, hy0, hx1, hy0, lw, c)
	case "play":
		a, b := at(0.3, 0.18)
		e, f := at(0.3, 0.82)
		g, h := at(0.82, 0.5)
		p.poly([][2]float32{{a, b}, {e, f}, {g, h}}, c)
	case "pause":
		x0, y0 := at(0.28, 0.2)
		p.roundRect(image.Rect(int(x0), int(y0), int(x0+s*0.15), int(y0+s*0.6)), 2, c)
		x1, _ := at(0.57, 0.2)
		p.roundRect(image.Rect(int(x1), int(y0), int(x1+s*0.15), int(y0+s*0.6)), 2, c)
	case "next":
		a, b := at(0.2, 0.2)
		e, f := at(0.2, 0.8)
		g, h := at(0.66, 0.5)
		p.poly([][2]float32{{a, b}, {e, f}, {g, h}}, c)
		x0, y0 := at(0.68, 0.2)
		p.roundRect(image.Rect(int(x0), int(y0), int(x0+s*0.12), int(y0+s*0.6)), 2, c)
	case "volume":
		a, b := at(0.14, 0.38)
		p.roundRect(image.Rect(int(a), int(b), int(a+s*0.18), int(b+s*0.24)), 2, c)
		e, f := at(0.3, 0.38)
		g, h := at(0.52, 0.18)
		i, j := at(0.52, 0.82)
		k, l := at(0.3, 0.62)
		p.poly([][2]float32{{e, f}, {g, h}, {i, j}, {k, l}}, c)
		cx, cy := at(0.52, 0.5)
		p.arc(cx, cy, s*0.2, lw, -0.9, 0.9, c)
		p.arc(cx, cy, s*0.34, lw, -0.9, 0.9, c)
	case "back":
		a, b := at(0.62, 0.18)
		e, f := at(0.3, 0.5)
		g, h := at(0.62, 0.82)
		p.line(a, b, e, f, lw*1.3, c)
		p.line(e, f, g, h, lw*1.3, c)
	case "chevron":
		a, b := at(0.38, 0.2)
		e, f := at(0.66, 0.5)
		g, h := at(0.38, 0.8)
		p.line(a, b, e, f, lw, c)
		p.line(e, f, g, h, lw, c)
	case "disk":
		x0, y0 := at(0.12, 0.26)
		x1, y1 := at(0.88, 0.74)
		p.ring(image.Rect(int(x0), int(y0), int(x1), int(y1)), int(s*0.08), int(lw), c)
		cx, cy := at(0.72, 0.5)
		p.circle(int(cx), int(cy), int(max(2, s/14)), c)
	case "usb":
		x0, y0 := at(0.3, 0.1)
		x1, y1 := at(0.7, 0.45)
		p.ring(image.Rect(int(x0), int(y0), int(x1), int(y1)), 2, int(lw), c)
		x2, y2 := at(0.22, 0.45)
		x3, y3 := at(0.78, 0.9)
		p.roundRect(image.Rect(int(x2), int(y2), int(x3), int(y3)), int(s*0.08), c)
	case "wait":
		cx, cy := at(0.5, 0.5)
		p.arc(cx, cy, s*0.38, lw, 0, 2*math.Pi, c)
		p.line(cx, cy, cx, cy-s*0.24, lw, c)
		p.line(cx, cy, cx+s*0.17, cy, lw, c)
	case "info", "warn", "error":
		cx, cy := at(0.5, 0.5)
		if name == "warn" {
			a, b := at(0.5, 0.1)
			e, f := at(0.94, 0.88)
			g, h := at(0.06, 0.88)
			p.poly([][2]float32{{a, b}, {e, f}, {g, h}}, c)
		} else {
			p.circle(int(cx), int(cy), int(s*0.46), c)
		}
		mark := cBlack
		if name == "error" {
			a, b := at(0.34, 0.34)
			e, f := at(0.66, 0.66)
			p.line(a, b, e, f, lw*1.3, mark)
			p.line(e, b, a, f, lw*1.3, mark)
		} else {
			top, bot := 0.26, 0.6
			if name == "warn" {
				top, bot = 0.36, 0.68
			}
			ax, ay := at(0.5, float32(top))
			_, by := at(0.5, float32(bot))
			if name == "info" {
				ax, ay = at(0.5, 0.44)
				_, by = at(0.5, 0.74)
				dx, dy := at(0.5, 0.29)
				p.circle(int(dx), int(dy), int(max(2, s/14)), mark)
			} else {
				_, dy := at(0.5, float32(bot)+0.1)
				p.circle(int(ax), int(dy), int(max(2, s/16)), mark)
			}
			p.line(ax, ay, ax, by, lw*1.2, mark)
		}
	case "page":
		x0, y0 := at(0.24, 0.12)
		x1, y1 := at(0.76, 0.88)
		p.ring(image.Rect(int(x0), int(y0), int(x1), int(y1)), int(s*0.06), int(lw), c)
	default: // a dot
		cx, cy := at(0.5, 0.5)
		p.circle(int(cx), int(cy), int(s*0.2), c)
	}
}

// cursorShape returns the mouse arrow outline (in a 16×24 box).
func cursorShape() [][2]float32 {
	return [][2]float32{{0, 0}, {0, 17}, {4.5, 13}, {7.5, 20}, {10.5, 19}, {7.5, 12.5}, {13, 12.5}}
}

// tint is c at the given opacity (premultiplied, as image/draw expects).
func tint(c color.RGBA, a uint8) color.RGBA {
	m := func(v uint8) uint8 { return uint8(uint16(v) * uint16(a) / 255) }
	return color.RGBA{m(c.R), m(c.G), m(c.B), a}
}
