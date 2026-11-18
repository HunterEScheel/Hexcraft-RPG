// Package tui is a small full-screen text UI for the Linux console: raw
// keyboard input (the kernel keymap, i.e. Dvorak, still applies), full-screen
// redraws, and a few widgets (menus with type-to-filter, text input, number
// slider, message).
package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Key is one key press. Name is set for special keys ("up", "down", "left",
// "right", "enter", "esc", "backspace", "delete", "home", "end", "pgup",
// "pgdn", "tab"); otherwise R holds the character.
type Key struct {
	Name string
	R    rune
}

func (k Key) String() string {
	if k.Name != "" {
		return k.Name
	}
	return string(k.R)
}

// Screen is what widgets draw on and read keys from.
type Screen interface {
	Size() (w, h int)
	Draw(lines []string)
	Keys() <-chan Key
}

// ParseKeys turns raw terminal bytes into keys. rest is a trailing partial
// escape sequence or character (the caller waits for more bytes).
func ParseKeys(buf []byte) (keys []Key, rest []byte) {
	for len(buf) > 0 {
		b := buf[0]
		switch {
		case b == 0x1b:
			if len(buf) == 1 {
				return keys, buf // maybe a lone Esc, maybe more coming
			}
			if buf[1] != '[' && buf[1] != 'O' {
				keys = append(keys, Key{Name: "esc"})
				buf = buf[1:]
				continue
			}
			// Linux console F1-F5: ESC [ [ A..E
			if buf[1] == '[' && len(buf) >= 3 && buf[2] == '[' {
				if len(buf) < 4 {
					return keys, buf
				}
				if name := csiNames["["+string(buf[3])]; name != "" {
					keys = append(keys, Key{Name: name})
				}
				buf = buf[4:]
				continue
			}
			// CSI / SS3 sequence: ESC [ params final
			i := 2
			for i < len(buf) && (buf[i] < 0x40 || buf[i] > 0x7e) {
				i++
			}
			if i >= len(buf) {
				return keys, buf
			}
			seq := string(buf[2 : i+1])
			buf = buf[i+1:]
			if name := csiNames[seq]; name != "" {
				keys = append(keys, Key{Name: name})
			}
		case b == '\r' || b == '\n':
			keys = append(keys, Key{Name: "enter"})
			buf = buf[1:]
		case b == 0x7f || b == 0x08:
			keys = append(keys, Key{Name: "backspace"})
			buf = buf[1:]
		case b == '\t':
			keys = append(keys, Key{Name: "tab"})
			buf = buf[1:]
		case b < 0x20:
			buf = buf[1:] // other control chars: ignore
		default:
			r, n := utf8.DecodeRune(buf)
			if r == utf8.RuneError && n <= 1 && !utf8.FullRune(buf) {
				return keys, buf
			}
			keys = append(keys, Key{R: r})
			buf = buf[n:]
		}
	}
	return keys, nil
}

var csiNames = map[string]string{
	"A": "up", "B": "down", "C": "right", "D": "left", "H": "home", "F": "end",
	"1~": "home", "4~": "end", "7~": "home", "8~": "end",
	"2~": "insert", "3~": "delete", "5~": "pgup", "6~": "pgdn",
	"[A": "f1", "[B": "f2", "[C": "f3", "[D": "f4", "[E": "f5", // Linux console F-keys
	"P": "f1", "Q": "f2", "R": "f3", "S": "f4",
}

// ---- styled text ----

// Style codes. Text is built from these and plain strings; Width ignores them.
const (
	Reset   = "\x1b[0m"
	Bold    = "\x1b[1m"
	Dim     = "\x1b[2m"
	Reverse = "\x1b[7m"
	Red     = "\x1b[31m"
	Green   = "\x1b[32m"
	Yellow  = "\x1b[33m"
	Cyan    = "\x1b[36m"
)

// Fg / Bg build true-colour escapes; the Linux console maps them to its palette.
func Fg(r, g, b uint8) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b) }
func Bg(r, g, b uint8) string { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b) }

// Width is the visible width of s (escape sequences excluded).
func Width(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
			}
			i = j + 1
			continue
		}
		_, n := utf8.DecodeRuneInString(s[i:])
		w++
		i += n
	}
	return w
}

// Fit pads or truncates s (keeping escape codes) to exactly w visible cells.
func Fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	vis := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
			}
			b.WriteString(s[i:min(j+1, len(s))])
			i = j + 1
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		if vis == w-1 && Width(s[i:]) > 1 {
			b.WriteRune('…')
			vis++
			break
		}
		if vis >= w {
			break
		}
		b.WriteRune(r)
		vis++
		i += n
	}
	if vis < w {
		b.WriteString(strings.Repeat(" ", w-vis))
	}
	return b.String()
}

// Center pads s on both sides to width w.
func Center(s string, w int) string {
	pad := w - Width(s)
	if pad <= 0 {
		return Fit(s, w)
	}
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}

// Wrap breaks plain text into lines of at most w cells.
func Wrap(text string, w int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			for Width(word) > w {
				if cur != "" {
					lines = append(lines, cur)
					cur = ""
				}
				lines = append(lines, string([]rune(word)[:w]))
				word = string([]rune(word)[w:])
			}
			switch {
			case cur == "":
				cur = word
			case Width(cur)+1+Width(word) <= w:
				cur += " " + word
			default:
				lines = append(lines, cur)
				cur = word
			}
		}
		lines = append(lines, cur)
	}
	return lines
}
