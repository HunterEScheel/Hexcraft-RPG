// Package render draws key images: background colour, optional icon, label.
package render

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"streamdeckd/internal/config"
)

// State tints a key to give press feedback.
type State int

const (
	Normal State = iota
	Busy
	OK
	Failed
)

var (
	fontOnce sync.Once
	fontData *opentype.Font
	faces    sync.Map // size -> font.Face
	// font.Face values are not safe for concurrent use.
	renderMu sync.Mutex
)

func face(size float64) font.Face {
	fontOnce.Do(func() {
		f, err := opentype.Parse(gobold.TTF)
		if err != nil {
			panic(err)
		}
		fontData = f
	})
	if f, ok := faces.Load(size); ok {
		return f.(font.Face)
	}
	f, err := opentype.NewFace(fontData, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	faces.Store(size, f)
	return f
}

// Key renders one button to a JPEG of px×px pixels, mirrored as requested.
// KeyImage draws a key at px×px (unmirrored), e.g. for on-screen previews.
func KeyImage(px int, b config.Button, baseDir string, st State) *image.RGBA {
	renderMu.Lock()
	defer renderMu.Unlock()
	img := image.NewRGBA(image.Rect(0, 0, px, px))

	bg := color.RGBA{0x20, 0x20, 0x20, 0xff}
	if b.Color != "" {
		bg, _ = config.ParseColor(b.Color)
	}
	if st == Failed {
		bg = color.RGBA{0xb0, 0x10, 0x10, 0xff}
	}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	label := b.Label
	if st == Failed {
		label = "ERROR"
	}

	iconArea := img.Bounds()
	if b.Icon != "" && st != Failed {
		if label != "" {
			iconArea.Max.Y = px * 7 / 10
		}
		drawIcon(img, iconArea.Inset(px/12), resolve(baseDir, b.Icon))
	}

	if label != "" {
		fg := color.RGBA{0xff, 0xff, 0xff, 0xff}
		if b.TextColor != "" {
			fg, _ = config.ParseColor(b.TextColor)
		}
		if b.Icon != "" && st != Failed {
			drawText(img, label, fg, image.Rect(0, px*7/10, px, px), float64(px)/6.5)
		} else {
			drawText(img, label, fg, img.Bounds(), float64(px)/5.5)
		}
	}

	switch st {
	case Busy:
		border(img, max(2, px/24), color.RGBA{0xff, 0xc8, 0x00, 0xff})
	case OK:
		border(img, max(3, px/16), color.RGBA{0x20, 0xe0, 0x60, 0xff})
	}
	return img
}

func Key(px int, b config.Button, baseDir string, st State, flipH, flipV bool) ([]byte, error) {
	img := KeyImage(px, b, baseDir, st)
	out := image.Image(img)
	if flipH || flipV {
		out = flip(img, flipH, flipV)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Blank renders an empty (black) key.
func Blank(px int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	return buf.Bytes()
}

func resolve(base, p string) string {
	if filepath.IsAbs(p) || base == "" {
		return p
	}
	return filepath.Join(base, p)
}

var iconCache sync.Map // path -> image.Image

func drawIcon(dst *image.RGBA, area image.Rectangle, path string) {
	var src image.Image
	if v, ok := iconCache.Load(path); ok {
		src = v.(image.Image)
	} else {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		src, _, err = image.Decode(f)
		f.Close()
		if err != nil {
			return
		}
		iconCache.Store(path, src)
	}
	sb := src.Bounds()
	if sb.Dx() == 0 || sb.Dy() == 0 {
		return
	}
	// Fit inside area, keep aspect ratio, centre.
	w, h := area.Dx(), area.Dy()
	if sb.Dx()*h > sb.Dy()*w {
		h = sb.Dy() * w / sb.Dx()
	} else {
		w = sb.Dx() * h / sb.Dy()
	}
	x := area.Min.X + (area.Dx()-w)/2
	y := area.Min.Y + (area.Dy()-h)/2
	xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+w, y+h), src, sb, xdraw.Over, nil)
}

// drawText word-wraps and centres text in r, shrinking the font if needed.
func drawText(dst *image.RGBA, text string, fg color.Color, r image.Rectangle, size float64) {
	pad := r.Dx() / 16
	maxW := fixed.I(r.Dx() - 2*pad)
	var lines []string
	var fc font.Face
	for ; size >= 7; size-- {
		fc = face(size)
		lines = wrap(fc, text, maxW)
		lineH := fc.Metrics().Height.Ceil()
		fits := lineH*len(lines) <= r.Dy()-pad
		for _, l := range lines {
			if font.MeasureString(fc, l) > maxW {
				fits = false
			}
		}
		if fits {
			break
		}
	}
	m := fc.Metrics()
	lineH := m.Height.Ceil()
	y := r.Min.Y + (r.Dy()-lineH*len(lines))/2 + m.Ascent.Ceil()
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(fg), Face: fc}
	for _, l := range lines {
		w := font.MeasureString(fc, l)
		d.Dot = fixed.Point26_6{X: fixed.I(r.Min.X) + (fixed.I(r.Dx())-w)/2, Y: fixed.I(y)}
		d.DrawString(l)
		y += lineH
	}
}

func wrap(fc font.Face, text string, maxW fixed.Int26_6) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		cur := ""
		for _, w := range words {
			try := w
			if cur != "" {
				try = cur + " " + w
			}
			if cur != "" && font.MeasureString(fc, try) > maxW {
				lines = append(lines, cur)
				cur = w
			} else {
				cur = try
			}
		}
		lines = append(lines, cur)
	}
	return lines
}

func border(img *image.RGBA, t int, c color.Color) {
	b := img.Bounds()
	u := &image.Uniform{c}
	draw.Draw(img, image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+t), u, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(b.Min.X, b.Max.Y-t, b.Max.X, b.Max.Y), u, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(b.Min.X, b.Min.Y, b.Min.X+t, b.Max.Y), u, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(b.Max.X-t, b.Min.Y, b.Max.X, b.Max.Y), u, image.Point{}, draw.Src)
}

func flip(src *image.RGBA, h, v bool) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			sx, sy := x, y
			if h {
				sx = b.Max.X - 1 - (x - b.Min.X)
			}
			if v {
				sy = b.Max.Y - 1 - (y - b.Min.Y)
			}
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}
