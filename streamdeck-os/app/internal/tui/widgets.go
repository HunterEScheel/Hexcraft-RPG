package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Frame lays out a full screen: title bar, body, help line at the bottom.
func Frame(s Screen, title string, body []string, help string) {
	w, h := s.Size()
	lines := make([]string, 0, h)
	lines = append(lines, Reverse+Bold+Fit(" "+title, w)+Reset, "")
	for _, l := range body {
		if len(lines) >= h-2 {
			break
		}
		lines = append(lines, l)
	}
	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	lines = append(lines, Dim+Fit(" "+help, w)+Reset)
	s.Draw(lines)
}

// Item is one row of a menu.
type Item struct {
	Label  string
	Detail string // shown dimmed after the label
	Swatch string // optional background escape for a small colour block
	Header bool   // section heading, not selectable
}

// MenuOpts configures Menu.
type MenuOpts struct {
	Title    string
	Intro    []string // lines above the list
	Items    []Item
	Selected int
	// Filter: typing letters narrows the list (for long lists).
	Filter bool
	Help   string
	// Keys lets the caller handle extra keys (e.g. "delete"); returning true
	// ends the menu with that key.
	Keys func(k Key, index int) bool
}

// MenuResult says how a menu ended.
type MenuResult struct {
	Index int  // index into Items (-1 if none)
	Key   Key  // the key that ended it (enter, esc, or one accepted by Keys)
	OK    bool // false when cancelled with Esc
}

// Menu shows a list and returns the chosen item.
func Menu(s Screen, o MenuOpts) MenuResult {
	sel := o.Selected
	filter := ""
	top := 0
	for {
		// Visible items after filtering.
		var vis []int
		for i, it := range o.Items {
			if filter != "" && (it.Header || !strings.Contains(strings.ToLower(it.Label+" "+it.Detail), strings.ToLower(filter))) {
				continue
			}
			vis = append(vis, i)
		}
		// Keep the selection on a selectable, visible item.
		pos := -1
		for p, i := range vis {
			if i == sel {
				pos = p
			}
		}
		if pos < 0 || o.Items[vis[pos]].Header {
			pos = -1
			for p, i := range vis {
				if !o.Items[i].Header {
					pos = p
					sel = i
					break
				}
			}
		}

		w, h := s.Size()
		body := append([]string{}, o.Intro...)
		if len(o.Intro) > 0 {
			body = append(body, "")
		}
		if o.Filter {
			if filter == "" {
				body = append(body, Dim+"  Type to search"+Reset)
			} else {
				body = append(body, "  Search: "+Bold+filter+Reset+Dim+"_"+Reset)
			}
		}
		room := h - 4 - len(body)
		if room < 3 {
			room = 3
		}
		if pos >= 0 {
			if pos < top {
				top = pos
			}
			if pos >= top+room {
				top = pos - room + 1
			}
		}
		if top > len(vis)-room {
			top = max(0, len(vis)-room)
		}
		if len(vis) == 0 {
			body = append(body, Dim+"  (nothing matches)"+Reset)
		}
		for p := top; p < len(vis) && p < top+room; p++ {
			it := o.Items[vis[p]]
			if it.Header {
				body = append(body, " "+Bold+Cyan+it.Label+Reset)
				continue
			}
			line := it.Label
			if it.Detail != "" {
				line += "  " + Dim + it.Detail + Reset
			}
			if it.Swatch != "" {
				line += "  [" + it.Swatch + "    " + Reset + "]"
			}
			if p == pos {
				// Plain text on the highlight: mixing dim/colour codes with
				// reverse video leaves gaps on the Linux console.
				plain := it.Label
				if it.Detail != "" {
					plain += "   " + it.Detail
				}
				sw := ""
				if it.Swatch != "" {
					sw = "  [" + it.Swatch + "    " + Reset + "]"
				}
				body = append(body, Reverse+Fit(" > "+plain, min(w, Width(" > "+plain)+2))+Reset+sw)
			} else {
				body = append(body, "   "+line)
			}
		}
		if len(vis) > room {
			body = append(body, Dim+fmt.Sprintf("   (%d-%d of %d)", top+1, min(top+room, len(vis)), len(vis))+Reset)
		}
		help := o.Help
		if help == "" {
			help = "Up/Down: choose   Enter: select   Esc: back"
		}
		Frame(s, o.Title, body, help)

		k, ok := <-s.Keys()
		if !ok {
			return MenuResult{Index: -1, Key: Key{Name: "esc"}}
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
			return MenuResult{Index: sel, Key: k, OK: true}
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
		case k.Name == "home":
			pos = -1
			move(1)
		case k.Name == "end":
			pos = len(vis)
			move(-1)
		case k.Name == "enter":
			if pos >= 0 {
				return MenuResult{Index: sel, Key: k, OK: true}
			}
		case k.Name == "esc":
			if filter != "" {
				filter = ""
				continue
			}
			return MenuResult{Index: -1, Key: k}
		case k.Name == "backspace":
			if o.Filter && filter != "" {
				filter = string([]rune(filter)[:len([]rune(filter))-1])
			}
		case k.Name == "" && o.Filter && unicode.IsPrint(k.R):
			filter += string(k.R)
		}
	}
}

// Input asks for one line of text. ok is false when cancelled.
func Input(s Screen, title string, intro []string, prompt, initial string) (string, bool) {
	buf := []rune(initial)
	cur := len(buf)
	for {
		w, _ := s.Size()
		body := append([]string{}, intro...)
		if len(intro) > 0 {
			body = append(body, "")
		}
		field := string(buf[:cur]) + Reverse + " " + Reset
		if cur < len(buf) {
			field = string(buf[:cur]) + Reverse + string(buf[cur]) + Reset + string(buf[cur+1:])
		}
		body = append(body, "  "+prompt, "  "+Fit(field, w-4))
		Frame(s, title, body, "Type, then Enter to accept   Esc: cancel")
		k, ok := <-s.Keys()
		if !ok {
			return "", false
		}
		switch k.Name {
		case "enter":
			return strings.TrimSpace(string(buf)), true
		case "esc":
			return "", false
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

// Number picks a value with Left/Right (or typing digits).
func Number(s Screen, title, prompt string, val, lo, hi, step int, unit string) (int, bool) {
	typed := ""
	for {
		w, _ := s.Size()
		barW := min(50, w-20)
		filled := 0
		if hi > lo {
			filled = (val - lo) * barW / (hi - lo)
		}
		bar := Reverse + strings.Repeat(" ", filled) + Reset + Dim + strings.Repeat("·", barW-filled) + Reset
		shown := strconv.Itoa(val) + unit
		if typed != "" {
			shown = typed + "_"
		}
		Frame(s, title, []string{"  " + prompt, "", "  " + bar + "  " + Bold + shown + Reset},
			"Left/Right: change   or type a number   Enter: accept   Esc: cancel")
		k, ok := <-s.Keys()
		if !ok {
			return val, false
		}
		switch {
		case k.Name == "enter":
			if typed != "" {
				n, _ := strconv.Atoi(typed)
				val = max(lo, min(hi, n))
			}
			return val, true
		case k.Name == "esc":
			return val, false
		case k.Name == "left" || k.Name == "down":
			typed = ""
			val = max(lo, val-step)
		case k.Name == "right" || k.Name == "up":
			typed = ""
			val = min(hi, val+step)
		case k.Name == "backspace" && typed != "":
			typed = typed[:len(typed)-1]
		case k.R >= '0' && k.R <= '9' && len(typed) < 6:
			typed += string(k.R)
		}
	}
}

// Message shows text until a key is pressed.
func Message(s Screen, title string, lines []string) {
	w, _ := s.Size()
	var body []string
	for _, l := range lines {
		if Width(l) > w-4 && !strings.Contains(l, "\x1b") {
			for _, wl := range Wrap(l, w-4) {
				body = append(body, "  "+wl)
			}
		} else {
			body = append(body, "  "+l)
		}
	}
	Frame(s, title, body, "Press any key")
	<-s.Keys()
}

// Confirm asks a yes/no question; default is No.
func Confirm(s Screen, title, question string) bool {
	r := Menu(s, MenuOpts{Title: title, Intro: []string{"  " + question}, Items: []Item{{Label: "No"}, {Label: "Yes"}}})
	return r.OK && r.Index == 1
}

// Busy shows a "please wait" screen (no input).
func Busy(s Screen, title, text string) {
	Frame(s, title, []string{"  " + text}, "Please wait…")
}
