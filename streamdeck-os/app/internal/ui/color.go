package ui

import (
	"fmt"
	"math"
	"strings"
)

// Swatch is a named preset colour.
type Swatch struct {
	Name string
	Hex  string
}

// ColorOpts opens the colour wheel.
type ColorOpts struct {
	ID      string
	Title   string
	Value   string // #rrggbb
	Presets []Swatch
}

// HSV is a colour as hue (0-360), saturation and value (0-1).
type HSV struct{ H, S, V float64 }

// ParseHex reads #rrggbb (false if it isn't one).
func ParseHex(s string) (r, g, b uint8, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	var v uint32
	if _, err := fmt.Sscanf(s, "%06x", &v); err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), true
}

func Hex(r, g, b uint8) string { return fmt.Sprintf("#%02x%02x%02x", r, g, b) }

// RGB converts to 8-bit RGB.
func (c HSV) RGB() (r, g, b uint8) {
	h := math.Mod(c.H, 360)
	if h < 0 {
		h += 360
	}
	s, v := clamp01(c.S), clamp01(c.V)
	ch := v * s
	x := ch * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - ch
	var rf, gf, bf float64
	switch {
	case h < 60:
		rf, gf, bf = ch, x, 0
	case h < 120:
		rf, gf, bf = x, ch, 0
	case h < 180:
		rf, gf, bf = 0, ch, x
	case h < 240:
		rf, gf, bf = 0, x, ch
	case h < 300:
		rf, gf, bf = x, 0, ch
	default:
		rf, gf, bf = ch, 0, x
	}
	to := func(f float64) uint8 { return uint8(math.Round((f + m) * 255)) }
	return to(rf), to(gf), to(bf)
}

func (c HSV) Hex() string { r, g, b := c.RGB(); return Hex(r, g, b) }

// ToHSV converts from 8-bit RGB.
func ToHSV(r, g, b uint8) HSV {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	hi, lo := math.Max(rf, math.Max(gf, bf)), math.Min(rf, math.Min(gf, bf))
	d := hi - lo
	var h float64
	switch {
	case d == 0:
		h = 0
	case hi == rf:
		h = 60 * math.Mod((gf-bf)/d, 6)
	case hi == gf:
		h = 60 * ((bf-rf)/d + 2)
	default:
		h = 60 * ((rf-gf)/d + 4)
	}
	if h < 0 {
		h += 360
	}
	s := 0.0
	if hi > 0 {
		s = d / hi
	}
	return HSV{h, s, hi}
}

// HexToHSV parses #rrggbb (white if it isn't valid).
func HexToHSV(s string) HSV {
	r, g, b, ok := ParseHex(s)
	if !ok {
		return HSV{0, 0, 1}
	}
	return ToHSV(r, g, b)
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// Adjust applies a colour-wheel key: left/right turn the hue, up/down the
// saturation, pgup/pgdn (or + / -) the brightness. ok=false: not a colour key.
func (c HSV) Adjust(k Key) (HSV, bool) {
	switch {
	case k.Name == "left":
		c.H = math.Mod(c.H+355, 360)
	case k.Name == "right":
		c.H = math.Mod(c.H+5, 360)
	case k.Name == "up":
		c.S = clamp01(c.S + 0.05)
	case k.Name == "down":
		c.S = clamp01(c.S - 0.05)
	case k.Name == "pgup" || k.R == '+' || k.R == '=':
		c.V = clamp01(c.V + 0.05)
	case k.Name == "pgdn" || k.R == '-':
		c.V = clamp01(c.V - 0.05)
	default:
		return c, false
	}
	return c, true
}
