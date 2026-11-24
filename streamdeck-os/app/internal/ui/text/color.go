package text

import (
	"fmt"
	"math"
	"strings"

	"streamdeckd/internal/tui"
	"streamdeckd/internal/ui"
)

// Color is the colour wheel's console version: the same keys, shown as
// hue/strength/brightness bars.
func (u *UI) Color(o ui.ColorOpts) (string, bool) {
	cur := ui.HexToHSV(o.Value)
	trace := func() { u.T.Printf("color %s value=%s", o.ID, cur.Hex()) }
	trace()
	bar := func(f float64) string {
		n := int(math.Round(f * 30))
		return tui.Reverse + strings.Repeat(" ", n) + tui.Reset + tui.Dim + strings.Repeat("·", 30-n) + tui.Reset
	}
	for {
		hex := cur.Hex()
		body := []string{
			"  " + ColorEsc(hex, true) + "        " + tui.Reset + "  " + tui.Bold + hex + tui.Reset,
			"",
			fmt.Sprintf("  Colour      %s %3.0f°", bar(cur.H/360), cur.H),
			fmt.Sprintf("  Strength    %s %3.0f%%", bar(cur.S), cur.S*100),
			fmt.Sprintf("  Brightness  %s %3.0f%%", bar(cur.V), cur.V*100),
			"",
		}
		var ps []string
		for _, s := range o.Presets {
			mark := " "
			if strings.EqualFold(s.Hex, hex) {
				mark = ">"
			}
			ps = append(ps, mark+ColorEsc(s.Hex, true)+"  "+tui.Reset+" "+s.Name)
		}
		for i := 0; i < len(ps); i += 4 {
			body = append(body, "  "+strings.Join(ps[i:min(i+4, len(ps))], "   "))
		}
		u.frame(o.Title, body, "←→ colour  ↑↓ strength  PgUp/PgDn brightness  Tab: next preset  #: type a code  Enter OK  Esc cancel")
		k, ok := u.key()
		if !ok {
			return "", false
		}
		before := cur
		if nc, ok := cur.Adjust(k); ok {
			cur = nc
		} else {
			switch {
			case k.Name == "enter":
				return hex, true
			case k.Name == "esc":
				return "", false
			case k.Name == "tab":
				next := 0
				for i, s := range o.Presets {
					if strings.EqualFold(s.Hex, hex) {
						next = i + 1
					}
				}
				if len(o.Presets) > 0 {
					cur = ui.HexToHSV(o.Presets[next%len(o.Presets)].Hex)
				}
			case k.R == '#':
				if v, ok := u.Input(ui.InputOpts{ID: "color-code", Title: o.Title, Label: "Colour code (#rrggbb):", Initial: "#"}); ok {
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
