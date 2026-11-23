// Package text renders the ui toolkit on a character console (tests, and
// PCs without a framebuffer).
package text

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"streamdeckd/internal/config"
	"streamdeckd/internal/tui"
	"streamdeckd/internal/ui"
)

type UI struct {
	S      tui.Screen
	T      ui.Tracer
	events chan ui.Event
}

func New(s tui.Screen, t ui.Tracer) *UI {
	u := &UI{S: s, T: t, events: make(chan ui.Event, 64)}
	go func() {
		for k := range s.Keys() {
			u.events <- ui.Event{Kind: ui.KeyEvent, Key: k}
		}
		close(u.events)
	}()
	return u
}

func (u *UI) Events() <-chan ui.Event { return u.events }
func (u *UI) Close()                  {}

func (u *UI) key() (ui.Key, bool) {
	for ev := range u.events {
		if ev.Kind == ui.KeyEvent {
			return ev.Key, true
		}
	}
	return ui.Key{Name: "esc"}, false
}

func (u *UI) frame(title string, body []string, help string) { tui.Frame(u.S, title, body, help) }

func (u *UI) Menu(o ui.MenuOpts) ui.MenuResult {
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
			u.T.TraceMenu(o, sel)
			lastTrace = sel
		}
		w, h := u.S.Size()
		var body []string
		if o.Subtitle != "" {
			body = append(body, "  "+o.Subtitle, "")
		}
		for _, l := range o.Intro {
			body = append(body, "  "+l)
		}
		if len(o.Intro) > 0 {
			body = append(body, "")
		}
		if o.Filter {
			if filter == "" {
				body = append(body, tui.Dim+"  Type to search"+tui.Reset)
			} else {
				body = append(body, "  Search: "+tui.Bold+filter+tui.Reset)
			}
		}
		room := max(3, h-4-len(body))
		if pos >= 0 {
			if pos < top {
				top = pos
			}
			if pos >= top+room {
				top = pos - room + 1
			}
		}
		if len(vis) == 0 {
			body = append(body, tui.Dim+"  (nothing matches)"+tui.Reset)
		}
		for p := top; p < len(vis) && p < top+room; p++ {
			it := o.Items[vis[p]]
			if it.Header {
				body = append(body, " "+tui.Bold+tui.Cyan+it.Label+tui.Reset)
				continue
			}
			plain := it.Label
			if it.Value != "" {
				plain += "   " + it.Value
			}
			if it.Detail != "" {
				plain += "   " + it.Detail
			}
			if p == pos {
				body = append(body, tui.Reverse+tui.Fit(" > "+plain, min(w, tui.Width(plain)+5))+tui.Reset)
			} else {
				body = append(body, "   "+plain)
			}
		}
		u.frame(o.Title, body, "Up/Down: choose   Enter: select   Esc: back")
		k, ok := u.key()
		if !ok {
			return ui.MenuResult{Index: -1, Key: k}
		}
		move := func(d int) {
			for p := pos + d; p >= 0 && p < len(vis); p += d {
				if !o.Items[vis[p]].Header {
					sel = vis[p]
					return
				}
			}
		}
		switch {
		case o.Keys != nil && pos >= 0 && o.Keys(k, sel):
			return ui.MenuResult{Index: sel, Key: k, OK: true}
		case k.Name == "up":
			move(-1)
		case k.Name == "down":
			move(1)
		case k.Name == "pgup":
			for i := 0; i < room-1; i++ {
				move(-1)
			}
		case k.Name == "pgdn":
			for i := 0; i < room-1; i++ {
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

func (u *UI) Input(o ui.InputOpts) (string, bool) {
	u.T.Printf("input %s", o.ID)
	buf := []rune(o.Initial)
	cur := len(buf)
	show := !o.Secret
	for {
		w, _ := u.S.Size()
		var body []string
		for _, l := range o.Intro {
			body = append(body, "  "+l)
		}
		if len(o.Intro) > 0 {
			body = append(body, "")
		}
		shown := buf
		if !show {
			shown = []rune(strings.Repeat("•", len(buf)))
		}
		field := string(shown[:cur]) + tui.Reverse + " " + tui.Reset
		if cur < len(shown) {
			field = string(shown[:cur]) + tui.Reverse + string(shown[cur]) + tui.Reset + string(shown[cur+1:])
		}
		body = append(body, "  "+o.Label, "  "+tui.Fit(field, w-4))
		help := "Type, then Enter   Esc: cancel"
		if o.Secret {
			help += "   Tab: show/hide"
		}
		u.frame(o.Title, body, help)
		k, ok := u.key()
		if !ok {
			return "", false
		}
		switch k.Name {
		case "enter":
			return strings.TrimSpace(string(buf)), true
		case "esc":
			return "", false
		case "tab":
			show = !show
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

func (u *UI) Number(o ui.NumberOpts) (int, bool) {
	u.T.Printf("number %s", o.ID)
	val, typed := o.Value, ""
	step := max(1, o.Step)
	for {
		w, _ := u.S.Size()
		barW := min(50, w-20)
		filled := 0
		if o.Max > o.Min {
			filled = (val - o.Min) * barW / (o.Max - o.Min)
		}
		bar := tui.Reverse + strings.Repeat(" ", filled) + tui.Reset + tui.Dim + strings.Repeat("·", barW-filled) + tui.Reset
		shown := strconv.Itoa(val) + o.Unit
		if typed != "" {
			shown = typed + "_"
		}
		u.frame(o.Title, []string{"  " + o.Label, "", "  " + bar + "  " + tui.Bold + shown + tui.Reset},
			"Left/Right: change   or type a number   Enter: accept   Esc: cancel")
		k, ok := u.key()
		if !ok {
			return o.Value, false
		}
		switch {
		case k.Name == "enter":
			if typed != "" {
				n, _ := strconv.Atoi(typed)
				val = max(o.Min, min(o.Max, n))
			}
			return val, true
		case k.Name == "esc":
			return o.Value, false
		case k.Name == "left" || k.Name == "down":
			typed = ""
			val = max(o.Min, val-step)
		case k.Name == "right" || k.Name == "up":
			typed = ""
			val = min(o.Max, val+step)
		case k.Name == "backspace" && typed != "":
			typed = typed[:len(typed)-1]
		case k.R >= '0' && k.R <= '9' && len(typed) < 6:
			typed += string(k.R)
		}
	}
}

func (u *UI) Message(n ui.Note) {
	u.T.Printf("message %s", n.ID)
	w, _ := u.S.Size()
	var body []string
	switch n.Kind {
	case ui.ErrorNote:
		body = append(body, "  "+tui.Red+tui.Bold+"Problem"+tui.Reset, "")
	case ui.SuccessNote:
		body = append(body, "  "+tui.Green+tui.Bold+"Done"+tui.Reset, "")
	}
	for _, l := range n.Lines {
		for _, wl := range tui.Wrap(l, w-4) {
			body = append(body, "  "+wl)
		}
	}
	u.frame(n.Title, body, "Press any key")
	u.key()
}

func (u *UI) Confirm(c ui.ConfirmOpts) bool {
	yes, no := c.Yes, c.No
	if yes == "" {
		yes = "Yes"
	}
	if no == "" {
		no = "No"
	}
	r := u.Menu(ui.MenuOpts{ID: c.ID, Title: c.Title, Intro: []string{c.Question}, Items: []ui.Item{{Label: no}, {Label: yes, Danger: c.Danger}}})
	return r.OK && r.Index == 1
}

func (u *UI) Info(v ui.InfoView) {
	u.T.Printf("info %s", v.ID)
	var body []string
	for _, l := range v.Lines {
		body = append(body, "  "+l)
	}
	if v.Big != "" {
		body = append(body, "", "        "+tui.Bold+v.Big+tui.Reset, "")
	}
	if len(v.Buttons) > 0 {
		body = append(body, "", "  "+strings.Join(v.Buttons, "   "))
	}
	help := v.Hint
	if v.Spinner && help == "" {
		help = "Please wait…"
	}
	u.frame(v.Title, body, help)
}

func (u *UI) Home(v ui.HomeView) {
	u.T.Printf("home sel=%d tiles=%s", v.Selected, tileIDs(v.Tiles))
	var body []string
	if v.Subtitle != "" {
		body = append(body, "  "+v.Subtitle, "")
	}
	for _, c := range v.Cards {
		mark := "  "
		switch c.State {
		case ui.CardOK:
			mark = tui.Green + "● " + tui.Reset
		case ui.CardWarn:
			mark = tui.Yellow + "● " + tui.Reset
		default:
			mark = tui.Dim + "○ " + tui.Reset
		}
		body = append(body, fmt.Sprintf("  %s%-12s %s", mark, c.Title, c.Value))
	}
	if len(v.Steps) > 0 {
		body = append(body, "", "  Getting started:")
		for i, s := range v.Steps {
			m := "[ ]"
			if s.Done {
				m = "[x]"
			}
			body = append(body, fmt.Sprintf("    %s %d. %s", m, i+1, s.Label))
		}
	}
	if v.Banner != "" {
		body = append(body, "", "  "+tui.Yellow+v.Banner+tui.Reset+"  ["+v.BannerOK+": B]")
	}
	body = append(body, "")
	for i, t := range v.Tiles {
		line := fmt.Sprintf("   %s   %s", t.Label, tui.Dim+t.Detail+tui.Reset)
		if i == v.Selected {
			line = tui.Reverse + " > " + t.Label + "   " + t.Detail + " " + tui.Reset
		}
		body = append(body, line)
	}
	u.frame(v.Title, body, "Up/Down: choose   Enter: open")
}

func tileIDs(t []ui.Tile) string {
	ids := make([]string, len(t))
	for i, x := range t {
		ids[i] = x.ID
	}
	return strings.Join(ids, ",")
}

const cols, rows = 5, 3

func (u *UI) Grid(v ui.GridView) {
	u.T.Printf("grid page=%s sel=%d", v.Page, v.Selected)
	w, h := u.S.Size()
	cw := min(18, max(10, (w-1)/cols-1))
	var lines []string
	title := " Buttons"
	for _, p := range v.Path {
		title += " > " + p
	}
	deck := "Stream Deck: live preview"
	if !v.DeckLive {
		deck = "Stream Deck not connected"
	}
	lines = append(lines, tui.Reverse+tui.Bold+tui.Fit(title, w-tui.Width(deck)-1)+deck+" "+tui.Reset, "")
	hline := func(l, m, r string) string {
		return " " + l + strings.Repeat(strings.Repeat("─", cw)+m, cols-1) + strings.Repeat("─", cw) + r
	}
	lines = append(lines, hline("┌", "┬", "┐"))
	for r := 0; r < rows; r++ {
		cells := make([][3]string, cols)
		for c := 0; c < cols; c++ {
			k := r*cols + c
			cell := ui.Cell{Empty: true}
			if k < len(v.Cells) {
				cell = v.Cells[k]
			}
			b := cell.Button
			style := ""
			if !cell.Empty {
				style = ColorEsc(orDef(b.Color, "#202020"), true) + ColorEsc(orDef(b.TextColor, "#ffffff"), false)
			}
			if k == v.Selected {
				style = "\x1b[47;30m"
			}
			label := b.Label
			if !cell.Empty && label == "" {
				label = "(no label)"
			}
			wrapped := tui.Wrap(label, cw-2)
			num := strconv.Itoa(k + 1)
			if k == v.Selected {
				num = "> " + num
			}
			if isFolder(b) {
				num += " [folder]"
			}
			for i := 0; i < 3; i++ {
				t := ""
				switch {
				case i == 0:
					t = tui.Fit(num, cw-2)
					if cell.Empty && k != v.Selected {
						t = tui.Dim + t + tui.Reset
					}
				case i-1 < len(wrapped):
					t = wrapped[i-1]
				}
				cells[c][i] = style + " " + tui.Fit(t, cw-2) + " " + tui.Reset
			}
		}
		for i := 0; i < 3; i++ {
			line := " │"
			for c := 0; c < cols; c++ {
				line += cells[c][i] + "│"
			}
			lines = append(lines, line)
		}
		if r < rows-1 {
			lines = append(lines, hline("├", "┼", "┤"))
		}
	}
	lines = append(lines, hline("└", "┴", "┘"), "", " "+v.KeyTitle)
	for _, d := range v.Details {
		lines = append(lines, "   "+d)
	}
	for _, d := range v.Warnings {
		lines = append(lines, "   "+tui.Red+"! "+d+tui.Reset)
	}
	if v.Err != "" {
		lines = append(lines, "", tui.Red+" Not saved: "+strings.ReplaceAll(v.Err, "\n", "; ")+tui.Reset)
	}
	help := []string{
		" Arrows: move  Enter: edit  or press a key on the Stream Deck (twice opens a folder)",
		" T: try  Del: clear  C/V: copy/paste  F: folders  D: deck  Esc: back",
	}
	for len(lines) < h-len(help) {
		lines = append(lines, "")
	}
	lines = lines[:min(len(lines), h-len(help))]
	for _, hl := range help {
		lines = append(lines, tui.Dim+hl+tui.Reset)
	}
	u.S.Draw(lines)
}

func isFolder(b config.Button) bool {
	return len(b.Actions) == 1 && b.Actions[0].Type == config.Page
}

func orDef(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// ColorEsc picks the closest of the console's 8 colours by hue.
func ColorEsc(hex string, bg bool) string {
	c, err := config.ParseColor(hex)
	if err != nil {
		return ""
	}
	n := ConsoleColor(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	if bg {
		return fmt.Sprintf("\x1b[%dm", 40+n)
	}
	if n == 0 {
		return "\x1b[30m"
	}
	return fmt.Sprintf("\x1b[1;%dm", 30+n)
}

// ConsoleColor returns an ANSI colour number 0-7 for an RGB colour.
func ConsoleColor(r, g, b float64) int {
	hi, lo := max(r, g, b), min(r, g, b)
	l := (hi + lo) / 2
	sat := 0.0
	if hi != lo {
		d := 2*l - 1
		if d < 0 {
			d = -d
		}
		sat = (hi - lo) / (1 - d)
	}
	if sat < 0.25 || hi < 0.12 {
		if l > 0.6 {
			return 7
		}
		return 0
	}
	var h float64
	switch hi {
	case r:
		h = 60 * (g - b) / (hi - lo)
		if h < 0 {
			h += 360
		}
	case g:
		h = 60 * ((b-r)/(hi-lo) + 2)
	default:
		h = 60 * ((r-g)/(hi-lo) + 4)
	}
	switch {
	case h < 20 || h >= 330:
		return 1
	case h < 70:
		return 3
	case h < 160:
		return 2
	case h < 200:
		return 6
	case h < 255:
		return 4
	default:
		return 5
	}
}
