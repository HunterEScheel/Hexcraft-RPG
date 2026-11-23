// Package gfx is the graphical renderer of the ui toolkit: it draws on the
// framebuffer with anti-aliased shapes and real fonts, and takes input from
// the keyboard (through the console, so the Dvorak keymap applies) and from
// mice/touchpads.
package gfx

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"streamdeckd/internal/config"
	"streamdeckd/internal/fb"
	"streamdeckd/internal/render"
	"streamdeckd/internal/tui"
	"streamdeckd/internal/ui"
)

type hit struct {
	r      image.Rectangle
	target string
}

// GUI implements ui.UI on a framebuffer.
type GUI struct {
	dev  *fb.Device
	img  *image.RGBA
	W, H int
	s    float64
	T    ui.Tracer

	mu       sync.Mutex
	p        painter
	hits     []hit
	hover    string
	cx, cy   float64
	cursorOn bool
	paint    func() // redraws the current screen (spinner animation)
	spinning bool
	spin     float64
	held     int

	events   chan ui.Event
	keyCache map[string]*image.RGBA
}

// New starts the renderer. keys come from the console in raw mode; mice may be nil.
func New(dev *fb.Device, keys <-chan tui.Key, mice <-chan fb.MouseEvent, t ui.Tracer) *GUI {
	g := &GUI{dev: dev, W: dev.W, H: dev.H, T: t, events: make(chan ui.Event, 256), keyCache: map[string]*image.RGBA{}}
	g.img = image.NewRGBA(image.Rect(0, 0, g.W, g.H))
	g.p.img = g.img
	g.s = math.Min(float64(g.W)/1280, float64(g.H)/800)
	g.cx, g.cy = float64(g.W)/2, float64(g.H)/2
	go g.loop(keys, mice)
	return g
}

func (g *GUI) Events() <-chan ui.Event { return g.events }
func (g *GUI) Close()                  {}

func (g *GUI) px(v float64) int { return int(math.Round(v * g.s)) }
func (g *GUI) fs(v float64) int { return max(10, int(math.Round(v*g.s))) }

func (g *GUI) emit(ev ui.Event, mustDeliver bool) {
	if mustDeliver {
		g.events <- ev
		return
	}
	select {
	case g.events <- ev:
	default:
	}
}

func (g *GUI) loop(keys <-chan tui.Key, mice <-chan fb.MouseEvent) {
	tick := time.NewTicker(120 * time.Millisecond)
	defer tick.Stop()
	n := 0
	for {
		select {
		case k, ok := <-keys:
			if !ok {
				close(g.events)
				return
			}
			g.emit(ui.Event{Kind: ui.KeyEvent, Key: k}, true)
		case m, ok := <-mice:
			if !ok {
				mice = nil
				continue
			}
			g.mouse(m)
		case <-tick.C:
			n++
			g.mu.Lock()
			spinning, paint := g.spinning, g.paint
			g.mu.Unlock()
			if spinning && paint != nil {
				g.spin += 0.45
				paint()
			}
			if n%8 == 0 {
				g.emit(ui.Event{Kind: ui.TickEvent}, false)
			}
		}
	}
}

func (g *GUI) mouse(m fb.MouseEvent) {
	d := math.Max(math.Abs(float64(m.DX)), math.Abs(float64(m.DY)))
	f := 1.0
	if d > 4 { // gentle acceleration for big swipes
		f = 1 + (d-4)*0.12
	}
	g.mu.Lock()
	old := g.cursorRect()
	g.cx = math.Max(0, math.Min(float64(g.W-1), g.cx+float64(m.DX)*f))
	g.cy = math.Max(0, math.Min(float64(g.H-1), g.cy+float64(m.DY)*f))
	wasOn := g.cursorOn
	g.cursorOn = true
	if wasOn {
		g.dev.Flush(g.img, old)
	}
	g.drawCursor()
	target := g.hitAt(int(g.cx), int(g.cy))
	changed := target != g.hover
	g.hover = target
	prev := g.held
	g.held = m.Buttons
	moved := m.DX != 0 || m.DY != 0
	g.mu.Unlock()

	if changed {
		g.emit(ui.Event{Kind: ui.HoverEvent, Target: target}, false)
	}
	if m.Buttons&1 != 0 && (prev&1 == 0 || (moved && strings.HasPrefix(target, "slider"))) {
		g.emit(ui.Event{Kind: ui.ClickEvent, Target: target, Delta: int(g.cx)}, true)
	}
	if m.Wheel != 0 {
		g.emit(ui.Event{Kind: ui.WheelEvent, Delta: m.Wheel, Target: target}, false)
	}
}

func (g *GUI) cursorRect() image.Rectangle {
	x, y := int(g.cx), int(g.cy)
	return image.Rect(x-1, y-1, x+g.px(18)+2, y+g.px(26)+2)
}

func (g *GUI) drawCursor() {
	r := g.cursorRect().Intersect(g.img.Bounds())
	if r.Empty() {
		return
	}
	tmp := image.NewRGBA(r)
	draw.Draw(tmp, r, g.img, r.Min, draw.Src)
	tp := painter{img: tmp}
	sc := float32(g.s * 1.15)
	shape := func(grow float32) [][2]float32 {
		var pts [][2]float32
		for _, q := range cursorShape() {
			pts = append(pts, [2]float32{float32(g.cx) + q[0]*sc*grow, float32(g.cy) + q[1]*sc*grow})
		}
		return pts
	}
	tp.poly(shape(1.12), cBlack)
	tp.poly(shape(1.0), cText)
	g.dev.Flush(tmp, r)
}

func (g *GUI) hitAt(x, y int) string {
	for i := len(g.hits) - 1; i >= 0; i-- {
		if (image.Point{x, y}).In(g.hits[i].r) {
			return g.hits[i].target
		}
	}
	return ""
}

func (g *GUI) addHit(r image.Rectangle, target string) { g.hits = append(g.hits, hit{r, target}) }

// frame draws a full screen: background, top bar, content, hint bar.
func (g *GUI) frame(title string, back bool, hints string, content func(top, bottom int)) {
	paint := func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.hits = g.hits[:0]
		gradient(g.img, g.img.Bounds(), cBgTop, cBgBottom)
		barH := g.px(72)
		x := g.px(32)
		if back {
			br := image.Rect(x-g.px(10), g.px(14), x+g.px(110), barH-g.px(14))
			if g.hover == "back" {
				g.p.roundRect(br, g.px(12), cSurface2)
			}
			g.p.icon("back", x-g.px(4), barH/2-g.px(13), g.px(26), cText)
			textMid(g.img, face(medium, g.fs(19)), "Back", x+g.px(26), barH/2, cText, 0)
			g.addHit(br, "back")
			x += g.px(130)
		}
		textMid(g.img, face(bold, g.fs(26)), ellipsize(face(bold, g.fs(26)), title, g.W-x-g.px(220)), x, barH/2, cText, 0)
		textMid(g.img, face(medium, g.fs(20)), time.Now().Format("15:04"), g.W-g.px(32), barH/2, cMuted, 2)
		draw.Draw(g.img, image.Rect(0, barH, g.W, barH+1), image.NewUniform(cBorder), image.Point{}, draw.Src)
		hintH := g.px(46)
		draw.Draw(g.img, image.Rect(0, g.H-hintH, g.W, g.H), image.NewUniform(color.RGBA{0x0c, 0x0e, 0x12, 0xff}), image.Point{}, draw.Src)
		textMid(g.img, face(regular, g.fs(17)), hints, g.W/2, g.H-hintH/2, cMuted, 1)
		content(barH, g.H-hintH)
		g.dev.Flush(g.img, g.img.Bounds())
		if g.cursorOn {
			g.drawCursor()
		}
		// Hover may have changed under a still pointer.
		g.hover = g.hitAt(int(g.cx), int(g.cy))
	}
	g.mu.Lock()
	g.paint = paint
	g.mu.Unlock()
	paint()
}

// ---- building blocks ----

func (g *GUI) card(r image.Rectangle) {
	g.p.shadow(r, g.px(18), g.px(10))
	g.p.roundRect(r, g.px(18), cSurface)
}

// button draws a clickable button; kind: 0 secondary, 1 primary, 2 danger.
func (g *GUI) button(r image.Rectangle, label string, kind int, focused bool) {
	bg := cSurface2
	fg := cText
	switch kind {
	case 1:
		bg = cAccent
	case 2:
		bg = cDanger
	}
	target := "btn:" + strings.ToLower(label)
	if g.hover == target && kind == 0 {
		bg = cBorder
	}
	g.p.roundRect(r, g.px(12), bg)
	if focused {
		g.p.ring(r.Inset(-g.px(4)), g.px(15), g.px(3), cText)
	}
	textMid(g.img, face(bold, g.fs(19)), label, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, fg, 1)
	g.addHit(r, target)
}

func (g *GUI) badge(name string, cx, cy, size int, bg, fg color.Color) {
	g.p.circle(cx, cy, size/2, bg)
	is := size * 56 / 100
	g.p.icon(name, cx-is/2, cy-is/2, is, fg)
}

func (g *GUI) paragraph(lines []string, x, y, w int, f fontKind, size float64, c color.Color) int {
	fc := face(f, g.fs(size))
	lh := g.px(size * 1.45)
	for _, l := range lines {
		for _, wl := range wrap(fc, l, w) {
			text(g.img, fc, wl, x, y+fc.Metrics().Ascent.Ceil(), c, 0)
			y += lh
		}
	}
	return y
}

func (g *GUI) column() (x0, w int) {
	w = min(g.W-g.px(64), g.px(880))
	return (g.W - w) / 2, w
}

// nextEvent returns the next event, ignoring hovers the caller doesn't need.
func (g *GUI) next() (ui.Event, bool) {
	ev, ok := <-g.events
	return ev, ok
}

// ---- Menu ----

func (g *GUI) Menu(o ui.MenuOpts) ui.MenuResult {
	sel, filter, top := o.Selected, "", 0
	lastTrace := -2
	for {
		var vis []int
		for i, it := range o.Items {
			if filter != "" && (it.Header || !strings.Contains(strings.ToLower(it.Label+" "+it.Detail+" "+it.Value), strings.ToLower(filter))) {
				continue
			}
			vis = append(vis, i)
		}
		pos := -1
		for p, i := range vis {
			if i == sel && !o.Items[i].Header {
				pos = p
			}
		}
		if pos < 0 {
			for p, i := range vis {
				if !o.Items[i].Header {
					pos, sel = p, i
					break
				}
			}
		}
		if sel != lastTrace {
			g.T.TraceMenu(o, sel)
			lastTrace = sel
		}
		hasDetail := false
		for _, it := range o.Items {
			if it.Detail != "" {
				hasDetail = true
			}
		}
		rowH := g.px(60)
		if hasDetail {
			rowH = g.px(72)
		}
		hdrH := g.px(44)
		var room int
		g.frame(o.Title, true, "↑↓ choose · Enter select · Esc back"+map[bool]string{true: " · type to search", false: ""}[o.Filter], func(top0, bottom int) {
			x0, w := g.column()
			y := top0 + g.px(28)
			if o.Subtitle != "" {
				y = g.paragraph([]string{o.Subtitle}, x0, y, w, medium, 20, cText) + g.px(6)
			}
			if len(o.Intro) > 0 {
				y = g.paragraph(o.Intro, x0, y, w, regular, 18, cMuted) + g.px(10)
			}
			if o.Filter {
				fr := image.Rect(x0, y, x0+w, y+g.px(52))
				g.p.roundRect(fr, g.px(14), cSurface2)
				if filter == "" {
					textMid(g.img, face(regular, g.fs(19)), "Type to search…", x0+g.px(20), fr.Min.Y+fr.Dy()/2, cMuted, 0)
				} else {
					textMid(g.img, face(medium, g.fs(20)), filter, x0+g.px(20), fr.Min.Y+fr.Dy()/2, cText, 0)
					g.p.ring(fr, g.px(14), g.px(2), cAccent)
				}
				y = fr.Max.Y + g.px(14)
			}
			listTop := y
			avail := bottom - g.px(24) - listTop
			// How many rows fit from 'top'.
			room = 0
			hgt := 0
			for p := top; p < len(vis); p++ {
				h := rowH
				if o.Items[vis[p]].Header {
					h = hdrH
				}
				if hgt+h > avail {
					break
				}
				hgt += h
				room++
			}
			if pos >= 0 && pos >= top+room && room > 0 {
				top = pos - room + 1
			}
			if pos >= 0 && pos < top {
				top = pos
			}
			cardR := image.Rect(x0, listTop, x0+w, listTop+min(avail, hgt+g.px(16)))
			if len(vis) == 0 {
				cardR.Max.Y = listTop + g.px(80)
			}
			g.card(cardR)
			if len(vis) == 0 {
				textMid(g.img, face(regular, g.fs(19)), "Nothing matches", x0+w/2, cardR.Min.Y+cardR.Dy()/2, cMuted, 1)
			}
			y = listTop + g.px(8)
			for p := top; p < len(vis) && y < cardR.Max.Y-g.px(8); p++ {
				it := o.Items[vis[p]]
				if it.Header {
					textMid(g.img, face(bold, g.fs(15)), strings.ToUpper(it.Label), x0+g.px(24), y+hdrH/2+g.px(4), cMuted, 0)
					y += hdrH
					continue
				}
				r := image.Rect(x0+g.px(8), y, x0+w-g.px(8), y+rowH)
				if r.Max.Y > cardR.Max.Y-g.px(4) {
					break
				}
				selected := p == pos
				if selected {
					g.p.roundRect(r, g.px(12), cAccentDim)
					g.p.roundRect(image.Rect(r.Min.X, r.Min.Y+g.px(12), r.Min.X+g.px(5), r.Max.Y-g.px(12)), g.px(3), cAccent)
				}
				tx := r.Min.X + g.px(22)
				fg := cText
				if it.Danger {
					fg = cDanger
				}
				if it.Icon != "" {
					bg := cSurface2
					if selected {
						bg = cAccent
					}
					g.badge(it.Icon, tx+g.px(20), r.Min.Y+rowH/2, g.px(40), bg, cText)
					tx += g.px(56)
				}
				right := r.Max.X - g.px(44)
				g.p.icon("chevron", r.Max.X-g.px(36), r.Min.Y+rowH/2-g.px(11), g.px(22), cMuted)
				if it.Swatch != "" {
					sw := image.Rect(right-g.px(34), r.Min.Y+rowH/2-g.px(15), right-g.px(4), r.Min.Y+rowH/2+g.px(15))
					g.p.roundRect(sw, g.px(8), hexColor(it.Swatch, cBlack))
					g.p.ring(sw, g.px(8), g.px(2), cBorder)
					right = sw.Min.X - g.px(12)
				}
				if it.Value != "" {
					vf := face(regular, g.fs(18))
					val := ellipsize(vf, it.Value, w/3)
					textMid(g.img, vf, val, right, r.Min.Y+rowH/2, cMuted, 2)
					right -= textWidth(vf, val) + g.px(16)
				}
				lf := face(medium, g.fs(21))
				if it.Detail != "" {
					text(g.img, lf, ellipsize(lf, it.Label, right-tx), tx, r.Min.Y+rowH/2-g.px(4), fg, 0)
					df := face(regular, g.fs(16))
					text(g.img, df, ellipsize(df, it.Detail, right-tx), tx, r.Min.Y+rowH/2+g.px(20), cMuted, 0)
				} else {
					textMid(g.img, lf, ellipsize(lf, it.Label, right-tx), tx, r.Min.Y+rowH/2, fg, 0)
				}
				g.addHit(r, "row:"+strconv.Itoa(vis[p]))
				y += rowH
			}
			if len(vis) > room && room > 0 { // scrollbar
				tr := image.Rect(x0+w-g.px(6), cardR.Min.Y+g.px(12), x0+w-g.px(2), cardR.Max.Y-g.px(12))
				g.p.roundRect(tr, g.px(2), cSurface2)
				th := max(g.px(30), tr.Dy()*room/len(vis))
				ty := tr.Min.Y + (tr.Dy()-th)*top/max(1, len(vis)-room)
				g.p.roundRect(image.Rect(tr.Min.X, ty, tr.Max.X, ty+th), g.px(2), cMuted)
			}
		})
		ev, ok := g.next()
		if !ok {
			return ui.MenuResult{Index: -1, Key: ui.Key{Name: "esc"}}
		}
		move := func(d int) {
			for p := pos + d; p >= 0 && p < len(vis); p += d {
				if !o.Items[vis[p]].Header {
					sel = vis[p]
					return
				}
			}
		}
		switch ev.Kind {
		case ui.HoverEvent:
			if i, ok := strings.CutPrefix(ev.Target, "row:"); ok {
				sel, _ = strconv.Atoi(i)
			}
		case ui.WheelEvent:
			for i := 0; i < 2; i++ {
				move(-sign(ev.Delta))
			}
		case ui.ClickEvent:
			if ev.Target == "back" {
				return ui.MenuResult{Index: -1, Key: ui.Key{Name: "esc"}}
			}
			if i, ok := strings.CutPrefix(ev.Target, "row:"); ok {
				sel, _ = strconv.Atoi(i)
				return ui.MenuResult{Index: sel, Key: ui.Key{Name: "enter"}, OK: true}
			}
		case ui.KeyEvent:
			k := ev.Key
			switch {
			case o.Keys != nil && pos >= 0 && o.Keys(k, sel):
				return ui.MenuResult{Index: sel, Key: k, OK: true}
			case k.Name == "up":
				move(-1)
			case k.Name == "down":
				move(1)
			case k.Name == "pgup":
				for i := 0; i < max(1, room-1); i++ {
					move(-1)
				}
			case k.Name == "pgdn":
				for i := 0; i < max(1, room-1); i++ {
					move(1)
				}
			case k.Name == "enter":
				if pos >= 0 {
					return ui.MenuResult{Index: sel, Key: k, OK: true}
				}
			case k.Name == "esc":
				if filter != "" {
					filter = ""
					continue
				}
				return ui.MenuResult{Index: -1, Key: k}
			case k.Name == "backspace" && o.Filter && filter != "":
				filter = string([]rune(filter)[:len([]rune(filter))-1])
			case k.Name == "" && o.Filter && unicode.IsPrint(k.R):
				filter += string(k.R)
			}
		}
	}
}

func sign(v int) int {
	if v < 0 {
		return -1
	}
	return 1
}

// dialog draws a centred card and returns its rectangle.
func (g *GUI) dialog(top, bottom, w, h int) image.Rectangle {
	w = min(w, g.W-g.px(48))
	x0 := (g.W - w) / 2
	y0 := top + max(g.px(24), (bottom-top-h)/2)
	r := image.Rect(x0, y0, x0+w, y0+h)
	g.card(r)
	return r
}

// ---- Input ----

func (g *GUI) Input(o ui.InputOpts) (string, bool) {
	g.T.Printf("input %s", o.ID)
	buf := []rune(o.Initial)
	cur := len(buf)
	show := !o.Secret
	focus := 0 // 0 field, 1 cancel, 2 ok
	for {
		hints := "Type, then Enter · Esc cancel"
		if o.Secret {
			hints += " · Tab show/hide"
		}
		g.frame(o.Title, true, hints, func(top, bottom int) {
			w := g.px(760)
			introH := 0
			fc := face(regular, g.fs(18))
			for _, l := range o.Intro {
				introH += len(wrap(fc, l, w-g.px(64))) * g.px(26)
			}
			r := g.dialog(top, bottom, w, g.px(250)+introH)
			x := r.Min.X + g.px(32)
			y := r.Min.Y + g.px(28)
			y = g.paragraph(o.Intro, x, y, r.Dx()-g.px(64), regular, 18, cMuted)
			if len(o.Intro) > 0 {
				y += g.px(10)
			}
			text(g.img, face(medium, g.fs(18)), o.Label, x, y+g.px(18), cText, 0)
			y += g.px(32)
			fr := image.Rect(x, y, r.Max.X-g.px(32), y+g.px(60))
			g.p.roundRect(fr, g.px(12), cSurface2)
			g.p.ring(fr, g.px(12), g.px(2), map[bool]color.RGBA{true: cAccent, false: cBorder}[focus == 0])
			g.addHit(fr, "field")
			shown := string(buf)
			if !show {
				shown = strings.Repeat("•", len(buf))
			}
			tf := face(regular, g.fs(24))
			avail := fr.Dx() - g.px(40)
			if o.Secret {
				avail -= g.px(80)
				sr := image.Rect(fr.Max.X-g.px(84), fr.Min.Y+g.px(10), fr.Max.X-g.px(10), fr.Max.Y-g.px(10))
				g.p.roundRect(sr, g.px(8), cBorder)
				textMid(g.img, face(medium, g.fs(16)), map[bool]string{true: "Hide", false: "Show"}[show], sr.Min.X+sr.Dx()/2, sr.Min.Y+sr.Dy()/2, cText, 1)
				g.addHit(sr, "btn:show")
			}
			before := string([]rune(shown)[:cur])
			full := shown
			offset := 0
			for textWidth(tf, string([]rune(full)[offset:])) > avail && offset < cur {
				offset++
			}
			vis := string([]rune(full)[offset:])
			for textWidth(tf, vis) > avail {
				vis = string([]rune(vis)[:len([]rune(vis))-1])
			}
			if shown == "" && o.Placeholder != "" {
				textMid(g.img, tf, o.Placeholder, fr.Min.X+g.px(18), fr.Min.Y+fr.Dy()/2, cMuted, 0)
			}
			textMid(g.img, tf, vis, fr.Min.X+g.px(18), fr.Min.Y+fr.Dy()/2, cText, 0)
			cx := fr.Min.X + g.px(18) + textWidth(tf, string([]rune(before)[min(offset, len([]rune(before))):]))
			if focus == 0 {
				draw.Draw(g.img, image.Rect(cx+1, fr.Min.Y+g.px(14), cx+g.px(3), fr.Max.Y-g.px(14)), image.NewUniform(cAccent), image.Point{}, draw.Src)
			}
			by := r.Max.Y - g.px(80)
			g.button(image.Rect(r.Max.X-g.px(352), by, r.Max.X-g.px(192), by+g.px(52)), "Cancel", 0, focus == 1)
			g.button(image.Rect(r.Max.X-g.px(176), by, r.Max.X-g.px(32), by+g.px(52)), "OK", 1, focus == 2)
		})
		ev, ok := g.next()
		if !ok {
			return "", false
		}
		switch ev.Kind {
		case ui.ClickEvent:
			switch ev.Target {
			case "back", "btn:cancel":
				return "", false
			case "btn:ok":
				return strings.TrimSpace(string(buf)), true
			case "btn:show":
				show = !show
			case "field":
				focus = 0
			}
		case ui.KeyEvent:
			k := ev.Key
			switch k.Name {
			case "enter":
				if focus == 1 {
					return "", false
				}
				return strings.TrimSpace(string(buf)), true
			case "esc":
				return "", false
			case "tab":
				if o.Secret {
					show = !show
				}
			case "left":
				cur = max(0, cur-1)
			case "right":
				cur = min(len(buf), cur+1)
			case "home":
				cur = 0
			case "end":
				cur = len(buf)
			case "backspace":
				if cur > 0 {
					buf = append(buf[:cur-1], buf[cur:]...)
					cur--
				}
			case "delete":
				if cur < len(buf) {
					buf = append(buf[:cur], buf[cur+1:]...)
				}
			case "":
				if unicode.IsPrint(k.R) && len(buf) < 300 {
					buf = append(buf[:cur], append([]rune{k.R}, buf[cur:]...)...)
					cur++
				}
			}
		}
	}
}

// ---- Number ----

func (g *GUI) Number(o ui.NumberOpts) (int, bool) {
	g.T.Printf("number %s", o.ID)
	val := o.Value
	var track image.Rectangle
	for {
		g.frame(o.Title, true, "←→ change · Enter OK · Esc cancel · drag the slider", func(top, bottom int) {
			r := g.dialog(top, bottom, g.px(720), g.px(330))
			x := r.Min.X + g.px(32)
			text(g.img, face(medium, g.fs(19)), o.Label, x, r.Min.Y+g.px(48), cMuted, 0)
			text(g.img, face(bold, g.fs(58)), strconv.Itoa(val)+o.Unit, r.Min.X+r.Dx()/2, r.Min.Y+g.px(128), cText, 1)
			track = image.Rect(x+g.px(64), r.Min.Y+g.px(168), r.Max.X-g.px(96), r.Min.Y+g.px(180))
			g.p.roundRect(track, g.px(6), cSurface2)
			frac := 0.0
			if o.Max > o.Min {
				frac = float64(val-o.Min) / float64(o.Max-o.Min)
			}
			kx := track.Min.X + int(frac*float64(track.Dx()))
			g.p.roundRect(image.Rect(track.Min.X, track.Min.Y, kx, track.Max.Y), g.px(6), cAccent)
			g.p.circle(kx, track.Min.Y+track.Dy()/2, g.px(15), cText)
			g.addHit(track.Inset(-g.px(18)), "slider")
			mr := image.Rect(x, track.Min.Y-g.px(20), x+g.px(48), track.Max.Y+g.px(20))
			g.button(mr, "−", 0, false)
			g.hits[len(g.hits)-1].target = "btn:minus"
			pr := image.Rect(r.Max.X-g.px(80), track.Min.Y-g.px(20), r.Max.X-g.px(32), track.Max.Y+g.px(20))
			g.button(pr, "+", 0, false)
			g.hits[len(g.hits)-1].target = "btn:plus"
			by := r.Max.Y - g.px(80)
			g.button(image.Rect(r.Max.X-g.px(352), by, r.Max.X-g.px(192), by+g.px(52)), "Cancel", 0, false)
			g.button(image.Rect(r.Max.X-g.px(176), by, r.Max.X-g.px(32), by+g.px(52)), "OK", 1, false)
		})
		ev, ok := g.next()
		if !ok {
			return val, false
		}
		step := max(1, o.Step)
		switch ev.Kind {
		case ui.ClickEvent:
			switch ev.Target {
			case "back", "btn:cancel":
				return o.Value, false
			case "btn:ok":
				return val, true
			case "btn:minus":
				val = max(o.Min, val-step)
			case "btn:plus":
				val = min(o.Max, val+step)
			case "slider":
				f := float64(ev.Delta-track.Min.X) / float64(max(1, track.Dx()))
				v := o.Min + int(math.Round(f*float64(o.Max-o.Min)/float64(step)))*step
				val = max(o.Min, min(o.Max, v))
			}
		case ui.WheelEvent:
			val = max(o.Min, min(o.Max, val+sign(ev.Delta)*step))
		case ui.KeyEvent:
			switch ev.Key.Name {
			case "enter":
				return val, true
			case "esc":
				return o.Value, false
			case "left", "down":
				val = max(o.Min, val-step)
			case "right", "up":
				val = min(o.Max, val+step)
			}
		}
	}
}

// ---- Message / Confirm / Info ----

func (g *GUI) Message(n ui.Note) {
	g.T.Printf("message %s", n.ID)
	icon, col := "info", cAccent
	switch n.Kind {
	case ui.SuccessNote:
		icon, col = "check", cSuccess
	case ui.ErrorNote:
		icon, col = "error", cDanger
	case ui.WarningNote:
		icon, col = "warn", cWarning
	}
	for {
		g.frame(n.Title, false, "Enter or click OK", func(top, bottom int) {
			w := g.px(760)
			fc := face(regular, g.fs(19))
			h := g.px(210)
			for _, l := range n.Lines {
				h += len(wrap(fc, l, w-g.px(140))) * g.px(28)
			}
			r := g.dialog(top, bottom, w, h)
			if n.Kind == ui.SuccessNote {
				g.p.circle(r.Min.X+g.px(60), r.Min.Y+g.px(64), g.px(28), col)
				g.p.icon("check", r.Min.X+g.px(42), r.Min.Y+g.px(46), g.px(36), cBlack)
			} else {
				g.p.icon(icon, r.Min.X+g.px(32), r.Min.Y+g.px(36), g.px(56), col)
			}
			text(g.img, face(bold, g.fs(25)), n.Title, r.Min.X+g.px(112), r.Min.Y+g.px(74), cText, 0)
			g.paragraph(n.Lines, r.Min.X+g.px(112), r.Min.Y+g.px(100), w-g.px(150), regular, 19, cMuted)
			by := r.Max.Y - g.px(80)
			g.button(image.Rect(r.Max.X-g.px(176), by, r.Max.X-g.px(32), by+g.px(52)), "OK", 1, true)
		})
		ev, ok := g.next()
		if !ok || (ev.Kind == ui.KeyEvent && (ev.Key.Name == "enter" || ev.Key.Name == "esc" || ev.Key.Name == " " || ev.Key.R == ' ')) ||
			(ev.Kind == ui.ClickEvent && ev.Target == "btn:ok") {
			return
		}
	}
}

func (g *GUI) Confirm(c ui.ConfirmOpts) bool {
	g.T.TraceMenu(ui.MenuOpts{ID: c.ID, Items: []ui.Item{{Label: or(c.No, "No")}, {Label: or(c.Yes, "Yes")}}}, 0)
	yes, no := or(c.Yes, "Yes"), or(c.No, "No")
	focus := 0 // 0 = No (safe default), 1 = Yes
	for {
		g.frame(c.Title, false, "←→ choose · Enter confirm · Esc cancel", func(top, bottom int) {
			w := g.px(720)
			fc := face(regular, g.fs(20))
			h := g.px(200) + len(wrap(fc, c.Question, w-g.px(64)))*g.px(30)
			r := g.dialog(top, bottom, w, h)
			text(g.img, face(bold, g.fs(25)), c.Title, r.Min.X+g.px(32), r.Min.Y+g.px(56), cText, 0)
			g.paragraph([]string{c.Question}, r.Min.X+g.px(32), r.Min.Y+g.px(82), w-g.px(64), regular, 20, cMuted)
			by := r.Max.Y - g.px(80)
			kind := 1
			if c.Danger {
				kind = 2
			}
			g.button(image.Rect(r.Max.X-g.px(400), by, r.Max.X-g.px(224), by+g.px(52)), no, 0, focus == 0)
			g.hits[len(g.hits)-1].target = "btn:no"
			g.button(image.Rect(r.Max.X-g.px(208), by, r.Max.X-g.px(32), by+g.px(52)), yes, kind, focus == 1)
			g.hits[len(g.hits)-1].target = "btn:yes"
		})
		ev, ok := g.next()
		if !ok {
			return false
		}
		switch ev.Kind {
		case ui.ClickEvent:
			if ev.Target == "btn:yes" {
				return true
			}
			if ev.Target == "btn:no" {
				return false
			}
		case ui.HoverEvent:
			if ev.Target == "btn:yes" {
				focus = 1
			} else if ev.Target == "btn:no" {
				focus = 0
			}
		case ui.KeyEvent:
			switch ev.Key.Name {
			case "left", "up":
				focus = 0
			case "right", "down", "tab":
				focus = 1 - focus
			case "enter":
				if focus == 1 {
					g.T.TraceMenu(ui.MenuOpts{ID: c.ID, Items: []ui.Item{{Label: no}, {Label: yes}}}, 1)
				}
				return focus == 1
			case "esc":
				return false
			}
		}
	}
}

func or(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (g *GUI) Info(v ui.InfoView) {
	g.T.Printf("info %s", v.ID)
	g.mu.Lock()
	g.spinning = v.Spinner
	g.mu.Unlock()
	hint := v.Hint
	if hint == "" && v.Spinner {
		hint = "Please wait…"
	}
	g.frame(v.Title, false, hint, func(top, bottom int) {
		w := g.px(820)
		fc := face(regular, g.fs(20))
		h := g.px(150)
		for _, l := range v.Lines {
			h += len(wrap(fc, l, w-g.px(64))) * g.px(30)
		}
		if v.Big != "" {
			h += g.px(110)
		}
		if len(v.Buttons) > 0 {
			h += g.px(80)
		}
		r := g.dialog(top, bottom, w, h)
		y := r.Min.Y + g.px(32)
		x := r.Min.X + g.px(32)
		if v.Spinner {
			g.p.arc(float32(r.Max.X-g.px(60)), float32(r.Min.Y+g.px(58)), float32(g.px(20)), float32(g.px(5)), float32(g.spin), float32(g.spin+4.2), cAccent)
		}
		text(g.img, face(bold, g.fs(26)), v.Title, x, y+g.px(28), cText, 0)
		y += g.px(60)
		y = g.paragraph(v.Lines, x, y, w-g.px(64), regular, 20, cMuted)
		if v.Big != "" {
			br := image.Rect(x, y+g.px(16), r.Max.X-g.px(32), y+g.px(96))
			g.p.roundRect(br, g.px(14), cSurface2)
			bf := face(mono, g.fs(36))
			textMid(g.img, bf, ellipsize(bf, v.Big, br.Dx()-g.px(20)), br.Min.X+br.Dx()/2, br.Min.Y+br.Dy()/2, cAccent, 1)
			y = br.Max.Y
		}
		if len(v.Buttons) > 0 {
			bx := r.Max.X - g.px(32)
			by := r.Max.Y - g.px(80)
			for i := len(v.Buttons) - 1; i >= 0; i-- {
				bw := max(g.px(150), textWidth(face(bold, g.fs(19)), v.Buttons[i])+g.px(48))
				kind := 0
				if i == len(v.Buttons)-1 {
					kind = 1
				}
				g.button(image.Rect(bx-bw, by, bx, by+g.px(52)), v.Buttons[i], kind, false)
				bx -= bw + g.px(16)
			}
		}
	})
}

// ---- Home ----

func (g *GUI) Home(v ui.HomeView) {
	g.mu.Lock()
	g.spinning = false
	g.mu.Unlock()
	ids := make([]string, len(v.Tiles))
	for i, t := range v.Tiles {
		ids[i] = t.ID
	}
	g.T.Printf("home sel=%d tiles=%s", v.Selected, strings.Join(ids, ","))
	g.frame(v.Title, false, "Arrows choose · Enter open · or click", func(top, bottom int) {
		m := g.px(36)
		x0, w := m, g.W-2*m
		y := top + g.px(26)
		if v.Subtitle != "" {
			text(g.img, face(regular, g.fs(19)), v.Subtitle, x0, y+g.px(18), cMuted, 0)
			y += g.px(40)
		}
		// Status cards.
		n := max(1, len(v.Cards))
		gap := g.px(16)
		cw := (w - gap*(n-1)) / n
		ch := g.px(96)
		for i, c := range v.Cards {
			r := image.Rect(x0+i*(cw+gap), y, x0+i*(cw+gap)+cw, y+ch)
			g.card(r)
			col := cMuted
			switch c.State {
			case ui.CardOK:
				col = cSuccess
			case ui.CardWarn:
				col = cWarning
			}
			g.badge(c.Icon, r.Min.X+g.px(42), r.Min.Y+ch/2, g.px(48), tint(col, 0x40), col)
			tx := r.Min.X + g.px(80)
			text(g.img, face(regular, g.fs(16)), c.Title, tx, r.Min.Y+g.px(40), cMuted, 0)
			vf := face(medium, g.fs(20))
			text(g.img, vf, ellipsize(vf, c.Value, r.Max.X-tx-g.px(14)), tx, r.Min.Y+g.px(70), cText, 0)
		}
		y += ch + g.px(22)
		if len(v.Steps) > 0 {
			r := image.Rect(x0, y, x0+w, y+g.px(86))
			g.card(r)
			text(g.img, face(bold, g.fs(18)), "Getting started", r.Min.X+g.px(24), r.Min.Y+g.px(34), cText, 0)
			sx := r.Min.X + g.px(24)
			for i, s := range v.Steps {
				cy := r.Min.Y + g.px(62)
				if s.Done {
					g.p.circle(sx+g.px(12), cy, g.px(12), cSuccess)
					g.p.icon("check", sx+g.px(3), cy-g.px(9), g.px(18), cBlack)
				} else {
					g.p.ring(image.Rect(sx, cy-g.px(12), sx+g.px(24), cy+g.px(12)), g.px(12), g.px(2), cMuted)
					textMid(g.img, face(bold, g.fs(13)), strconv.Itoa(i+1), sx+g.px(12), cy, cMuted, 1)
				}
				lf := face(medium, g.fs(17))
				col := cText
				if s.Done {
					col = cMuted
				}
				textMid(g.img, lf, s.Label, sx+g.px(32), cy, col, 0)
				sx += g.px(32) + textWidth(lf, s.Label) + g.px(34)
			}
			y = r.Max.Y + g.px(18)
		}
		if v.Banner != "" {
			r := image.Rect(x0, y, x0+w, y+g.px(64))
			g.p.roundRect(r, g.px(16), tint(cWarning, 0x26))
			g.p.ring(r, g.px(16), g.px(2), cWarning)
			g.p.icon("usb", r.Min.X+g.px(20), r.Min.Y+g.px(16), g.px(32), cWarning)
			textMid(g.img, face(medium, g.fs(19)), v.Banner, r.Min.X+g.px(66), r.Min.Y+r.Dy()/2, cText, 0)
			br := image.Rect(r.Max.X-g.px(170), r.Min.Y+g.px(10), r.Max.X-g.px(12), r.Max.Y-g.px(10))
			g.button(br, v.BannerOK, 1, false)
			g.hits[len(g.hits)-1].target = "banner"
			y = r.Max.Y + g.px(18)
		}
		// Tiles.
		cols := 3
		tgap := g.px(18)
		tw := (w - tgap*(cols-1)) / cols
		rows := (len(v.Tiles) + cols - 1) / cols
		th := min(g.px(170), (bottom-g.px(20)-y-tgap*(rows-1))/max(1, rows))
		for i, t := range v.Tiles {
			r := image.Rect(x0+(i%cols)*(tw+tgap), y+(i/cols)*(th+tgap), x0+(i%cols)*(tw+tgap)+tw, y+(i/cols)*(th+tgap)+th)
			g.card(r)
			sel := i == v.Selected
			if sel {
				g.p.roundRect(r, g.px(18), cSurface2)
				g.p.ring(r.Inset(-g.px(3)), g.px(21), g.px(3), cAccent)
			}
			g.badge(t.Icon, r.Min.X+g.px(50), r.Min.Y+th/2-g.px(8), g.px(64), map[bool]color.RGBA{true: cAccent, false: cSurface2}[sel], cText)
			tx := r.Min.X + g.px(100)
			lf := face(bold, g.fs(23))
			text(g.img, lf, ellipsize(lf, t.Label, r.Max.X-tx-g.px(16)), tx, r.Min.Y+th/2-g.px(4), cText, 0)
			df := face(regular, g.fs(16))
			lines := wrap(df, t.Detail, r.Max.X-tx-g.px(16))
			for li, l := range lines {
				if li > 1 {
					break
				}
				text(g.img, df, l, tx, r.Min.Y+th/2+g.px(22)+li*g.px(22), cMuted, 0)
			}
			g.addHit(r, "tile:"+t.ID)
		}
	})
}

// ---- Grid (button editor) ----

func (g *GUI) keyImage(b config.Button, dir string, size int) *image.RGBA {
	k := fmt.Sprintf("%d|%s|%#v", size, dir, b)
	if im, ok := g.keyCache[k]; ok {
		return im
	}
	if len(g.keyCache) > 400 {
		g.keyCache = map[string]*image.RGBA{}
	}
	im := render.KeyImage(size, b, dir, render.Normal)
	g.keyCache[k] = im
	return im
}

func (g *GUI) Grid(v ui.GridView) {
	g.mu.Lock()
	g.spinning = false
	g.mu.Unlock()
	g.T.Printf("grid page=%s sel=%d", v.Page, v.Selected)
	title := "Buttons"
	if len(v.Path) > 0 {
		title = "Buttons  ›  " + strings.Join(v.Path, "  ›  ")
	}
	g.frame(title, true, "Click a key or press it on the Stream Deck · Enter edit · press twice / double-click opens a folder · Esc back", func(top, bottom int) {
		m := g.px(32)
		panelW := g.px(360)
		deckR := image.Rect(m, top+g.px(28), g.W-m-panelW-g.px(24), bottom-g.px(24))
		// The deck: 5×3 keys in a dark body.
		const cols, rows = 5, 3
		gap := g.px(16)
		ks := min((deckR.Dx()-g.px(48)-gap*(cols-1))/cols, (deckR.Dy()-g.px(48)-gap*(rows-1))/rows)
		bw := ks*cols + gap*(cols-1) + g.px(48)
		bh := ks*rows + gap*(rows-1) + g.px(48)
		bmin := image.Pt(deckR.Min.X+(deckR.Dx()-bw)/2, deckR.Min.Y+(deckR.Dy()-bh)/2)
		body := image.Rectangle{Min: bmin, Max: bmin.Add(image.Pt(bw, bh))}
		g.p.shadow(body, g.px(28), g.px(14))
		g.p.roundRect(body, g.px(28), rgb(0x08, 0x09, 0x0c))
		g.p.ring(body, g.px(28), g.px(2), cBorder)
		if v.DeckLive {
			textMid(g.img, face(regular, g.fs(15)), "● live on your Stream Deck", body.Min.X+bw/2, body.Max.Y+g.px(18), cSuccess, 1)
		}
		for k := 0; k < cols*rows; k++ {
			x := body.Min.X + g.px(24) + (k%cols)*(ks+gap)
			y := body.Min.Y + g.px(24) + (k/cols)*(ks+gap)
			r := image.Rect(x, y, x+ks, y+ks)
			cell := ui.Cell{Empty: true}
			if k < len(v.Cells) {
				cell = v.Cells[k]
			}
			if cell.Empty {
				g.p.roundRect(r, g.px(14), rgb(0x16, 0x19, 0x1f))
				if g.hover == "cell:"+strconv.Itoa(k) || k == v.Selected {
					g.p.icon("plus", r.Min.X+ks/2-g.px(14), r.Min.Y+ks/2-g.px(14), g.px(28), cMuted)
				}
			} else {
				im := g.keyImage(cell.Button, v.Dir, ks)
				mask := image.NewAlpha(r)
				mp := painter{}
				_ = mp
				g.roundMask(mask, r, g.px(14))
				draw.DrawMask(g.img, r, im, im.Bounds().Min, mask, r.Min, draw.Over)
				if isFolder(cell.Button) {
					g.p.circle(r.Max.X-g.px(16), r.Min.Y+g.px(16), g.px(13), cAccent)
					g.p.icon("folder", r.Max.X-g.px(26), r.Min.Y+g.px(6), g.px(20), cText)
				}
				if cell.Warn {
					g.p.circle(r.Min.X+g.px(14), r.Min.Y+g.px(14), g.px(8), cDanger)
				}
			}
			if k == v.Selected {
				g.p.ring(r.Inset(-g.px(6)), g.px(18), g.px(4), cAccent)
			} else if g.hover == "cell:"+strconv.Itoa(k) {
				g.p.ring(r.Inset(-g.px(5)), g.px(17), g.px(2), cMuted)
			}
			g.addHit(r, "cell:"+strconv.Itoa(k))
		}
		// Side panel.
		pr := image.Rect(g.W-m-panelW, top+g.px(28), g.W-m, bottom-g.px(24))
		g.card(pr)
		x := pr.Min.X + g.px(24)
		pw := pr.Dx() - g.px(48)
		y := pr.Min.Y + g.px(26)
		tf := face(bold, g.fs(23))
		for _, l := range wrap(tf, v.KeyTitle, pw) {
			text(g.img, tf, l, x, y+g.px(22), cText, 0)
			y += g.px(32)
		}
		y += g.px(8)
		for _, d := range v.Details {
			y = g.paragraph([]string{"• " + d}, x, y, pw, regular, 17, cMuted) + g.px(4)
		}
		for _, d := range v.Warnings {
			y = g.paragraph([]string{"! " + d}, x, y, pw, medium, 17, cDanger) + g.px(4)
		}
		if v.Err != "" {
			y = g.paragraph([]string{"Not saved: " + v.Err}, x, y+g.px(6), pw, medium, 17, cDanger)
		}
		by := pr.Max.Y - g.px(24)
		for i := len(v.Buttons) - 1; i >= 0; i-- {
			kind := 0
			if i == 0 {
				kind = 1
			}
			r := image.Rect(x, by-g.px(50), pr.Max.X-g.px(24), by)
			g.button(r, v.Buttons[i], kind, false)
			by -= g.px(62)
		}
	})
}

func (g *GUI) roundMask(mask *image.Alpha, r image.Rectangle, rad int) {
	tmp := image.NewRGBA(r)
	tp := painter{img: tmp}
	tp.roundRect(r, rad, color.White)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			mask.SetAlpha(x, y, color.Alpha{tmp.RGBAAt(x, y).A})
		}
	}
}

func isFolder(b config.Button) bool {
	for _, a := range b.Actions {
		if a.Type == config.Page {
			return true
		}
	}
	return false
}
