package gfx

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"

	"streamdeckd/internal/ui"
)

// wheelImage draws the hue/saturation disc at brightness v (cached).
func (g *GUI) wheelImage(size int, v float64) *image.RGBA {
	key := "wheel|" + strconv.Itoa(size) + "|" + strconv.FormatFloat(v, 'f', 3, 64)
	if im, ok := g.keyCache[key]; ok {
		return im
	}
	im := image.NewRGBA(image.Rect(0, 0, size, size))
	c := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-c, float64(y)+0.5-c
			d := math.Hypot(dx, dy) / c
			if d > 1.0 {
				continue
			}
			h := math.Atan2(-dy, dx) * 180 / math.Pi
			r, gg, b := ui.HSV{H: h, S: d, V: v}.RGB()
			a := 1.0
			if d > 1-1.5/c { // smooth the rim
				a = (1 - d) * c / 1.5
			}
			al := uint8(math.Round(a * 255))
			im.SetRGBA(x, y, tint(color.RGBA{r, gg, b, 0xff}, al))
		}
	}
	g.keyCache[key] = im
	return im
}

// Color is the colour wheel: click or drag on the wheel (hue and
// saturation) and the brightness bar, or pick a preset.
func (g *GUI) Color(o ui.ColorOpts) (string, bool) {
	cur := ui.HexToHSV(o.Value)
	start := cur.Hex()
	var wheel, bar image.Rectangle
	trace := func() { g.T.Printf("color %s value=%s", o.ID, cur.Hex()) }
	trace()
	for {
		hex := cur.Hex()
		g.frame(o.Title, true, "Click or drag the wheel · ←→ colour · ↑↓ strength · PgUp/PgDn brightness · Enter OK · Esc cancel", func(top, bottom int) {
			r := g.dialog(top, bottom, g.px(1000), g.px(640))
			ws := g.px(400)
			wheel = image.Rect(r.Min.X+g.px(40), r.Min.Y+g.px(60), r.Min.X+g.px(40)+ws, r.Min.Y+g.px(60)+ws)
			wi := g.wheelImage(ws, 1) // brightness has its own bar
			draw.Draw(g.img, wheel, wi, image.Point{}, draw.Over)
			g.addHit(wheel, "wheel")
			// Marker.
			cx := float64(wheel.Min.X) + float64(ws)/2 + math.Cos(cur.H*math.Pi/180)*cur.S*float64(ws)/2
			cy := float64(wheel.Min.Y) + float64(ws)/2 - math.Sin(cur.H*math.Pi/180)*cur.S*float64(ws)/2
			g.p.circle(int(cx), int(cy), g.px(13), cBlack)
			g.p.circle(int(cx), int(cy), g.px(11), cText)
			mr, mg, mb := ui.HSV{H: cur.H, S: cur.S, V: 1}.RGB()
			g.p.circle(int(cx), int(cy), g.px(8), color.RGBA{mr, mg, mb, 0xff})

			// Brightness bar.
			bar = image.Rect(wheel.Max.X+g.px(36), wheel.Min.Y, wheel.Max.X+g.px(80), wheel.Max.Y)
			for y := bar.Min.Y; y < bar.Max.Y; y++ {
				v := 1 - float64(y-bar.Min.Y)/float64(bar.Dy())
				rr, gg, bb := ui.HSV{H: cur.H, S: cur.S, V: v}.RGB()
				draw.Draw(g.img, image.Rect(bar.Min.X, y, bar.Max.X, y+1), image.NewUniform(color.RGBA{rr, gg, bb, 0xff}), image.Point{}, draw.Src)
			}
			g.p.ring(bar, g.px(6), g.px(2), cBorder)
			ky := bar.Min.Y + int((1-cur.V)*float64(bar.Dy()))
			g.p.roundRect(image.Rect(bar.Min.X-g.px(6), ky-g.px(4), bar.Max.X+g.px(6), ky+g.px(4)), g.px(4), cText)
			g.addHit(bar.Inset(-g.px(8)), "value")
			textMid(g.img, face(regular, g.fs(15)), "Brightness", bar.Min.X+bar.Dx()/2, wheel.Max.Y+g.px(24), cMuted, 1)

			// Preview, code, presets, buttons.
			px := bar.Max.X + g.px(48)
			pw := r.Max.X - g.px(36) - px
			pr := image.Rect(px, wheel.Min.Y, px+pw, wheel.Min.Y+g.px(120))
			pc := color.RGBA{}
			pc.R, pc.G, pc.B, _ = ui.ParseHex(hex)
			pc.A = 0xff
			g.p.roundRect(pr, g.px(16), pc)
			g.p.ring(pr, g.px(16), g.px(2), cBorder)
			old := color.RGBA{A: 0xff}
			old.R, old.G, old.B, _ = ui.ParseHex(start)
			or := image.Rect(pr.Max.X-g.px(56), pr.Min.Y+g.px(12), pr.Max.X-g.px(12), pr.Min.Y+g.px(56))
			g.p.roundRect(or, g.px(10), old)
			g.p.ring(or, g.px(10), g.px(2), cText)
			textMid(g.img, face(mono, g.fs(26)), strings.ToUpper(hex), px, pr.Max.Y+g.px(30), cText, 0)
			cr := image.Rect(pr.Max.X-g.px(150), pr.Max.Y+g.px(10), pr.Max.X, pr.Max.Y+g.px(50))
			g.button(cr, "Type a code", 0, false)

			sy := pr.Max.Y + g.px(78)
			text(g.img, face(bold, g.fs(14)), "PRESETS", px, sy, cMuted, 0)
			cols := 5
			gap := g.px(10)
			ss := min(g.px(40), (pw-gap*(cols-1))/cols)
			for i, s := range o.Presets {
				x := px + (i%cols)*(ss+gap)
				y := sy + g.px(14) + (i/cols)*(ss+gap)
				sr := image.Rect(x, y, x+ss, y+ss)
				c := color.RGBA{A: 0xff}
				c.R, c.G, c.B, _ = ui.ParseHex(s.Hex)
				g.p.circle(x+ss/2, y+ss/2, ss/2, c)
				ring := cBorder
				if strings.EqualFold(s.Hex, hex) {
					ring = cText
				}
				g.p.ring(sr, ss/2, g.px(2), ring)
				if g.hover == "swatch:"+strconv.Itoa(i) {
					textMid(g.img, face(regular, g.fs(15)), s.Name, px, r.Max.Y-g.px(54), cMuted, 0)
				}
				g.addHit(sr, "swatch:"+strconv.Itoa(i))
			}
			by := r.Max.Y - g.px(80)
			g.button(image.Rect(r.Max.X-g.px(352), by, r.Max.X-g.px(192), by+g.px(52)), "Cancel", 0, false)
			g.button(image.Rect(r.Max.X-g.px(176), by, r.Max.X-g.px(36), by+g.px(52)), "OK", 1, false)
		})
		ev, ok := g.next()
		if !ok {
			return "", false
		}
		before := cur
		switch ev.Kind {
		case ui.ClickEvent:
			switch {
			case ev.Target == "back" || ev.Target == "btn:cancel":
				return "", false
			case ev.Target == "btn:ok":
				return cur.Hex(), true
			case ev.Target == "btn:type a code":
				if v, ok := g.Input(ui.InputOpts{ID: "color-code", Title: o.Title, Label: "Colour code", Initial: cur.Hex(), Intro: []string{"Six hex digits, like #ff8800."}}); ok {
					if !strings.HasPrefix(v, "#") {
						v = "#" + v
					}
					if _, _, _, valid := ui.ParseHex(v); valid {
						cur = ui.HexToHSV(v)
					}
				}
			case ev.Target == "wheel":
				c := wheel.Min.Add(wheel.Max).Div(2)
				dx, dy := float64(ev.X-c.X), float64(ev.Y-c.Y)
				cur.H = math.Mod(math.Atan2(-dy, dx)*180/math.Pi+360, 360)
				cur.S = math.Min(1, math.Hypot(dx, dy)/(float64(wheel.Dx())/2))
				if cur.V < 0.15 {
					cur.V = 1 // picking a colour on a black swatch: show it
				}
			case ev.Target == "value":
				cur.V = math.Max(0, math.Min(1, 1-float64(ev.Y-bar.Min.Y)/float64(bar.Dy())))
			case strings.HasPrefix(ev.Target, "swatch:"):
				i, _ := strconv.Atoi(strings.TrimPrefix(ev.Target, "swatch:"))
				if i < len(o.Presets) {
					cur = ui.HexToHSV(o.Presets[i].Hex)
				}
			}
		case ui.KeyEvent:
			k := ev.Key
			if nc, ok := cur.Adjust(k); ok {
				cur = nc
				break
			}
			switch {
			case k.Name == "enter":
				return cur.Hex(), true
			case k.Name == "esc":
				return "", false
			case k.Name == "tab": // next preset
				next := 0
				for i, s := range o.Presets {
					if strings.EqualFold(s.Hex, cur.Hex()) {
						next = i + 1
					}
				}
				if len(o.Presets) > 0 {
					cur = ui.HexToHSV(o.Presets[next%len(o.Presets)].Hex)
				}
			case k.R == '#':
				if v, ok := g.Input(ui.InputOpts{ID: "color-code", Title: o.Title, Label: "Colour code", Initial: "#"}); ok {
					if _, _, _, valid := ui.ParseHex(v); valid {
						cur = ui.HexToHSV(v)
					}
				}
			}
		}
		if cur != before {
			trace()
		}
	}
}
