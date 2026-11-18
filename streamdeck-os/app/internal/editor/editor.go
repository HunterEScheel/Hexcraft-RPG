// Package editor is the on-screen button editor: a full-screen app on the
// mini PC's monitor, driven by its keyboard, with the Stream Deck itself as a
// live preview (and pressing a deck key selects it).
package editor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/tui"
)

const (
	cols = 5
	rows = 3
	keys = cols * rows
)

// GoveeDevice is a light with its scene names.
type GoveeDevice struct {
	Name   string
	Scenes []string
	DIY    []string
}

// Sources is where the editor gets lists and runs "Try it".
type Sources interface {
	GoveeReady() bool
	SpotifyReady() bool
	DefaultSpeaker() string
	GoveeDevices(ctx context.Context) ([]GoveeDevice, error)
	SpotifyDevices(ctx context.Context) ([]string, error)
	Playlists(ctx context.Context) ([]spotify.Playlist, error)
	Run(ctx context.Context, actions []config.Action) error
	// ReconnectSpotify runs the Spotify login again (needs the phone page).
	ReconnectSpotify(ctx context.Context) error
}

// Deck is the live preview on the Stream Deck (nil if the deck isn't connected).
type Deck interface {
	Preview(l config.Layout, page string, sel int) error
	Presses() <-chan int
}

// Editor holds the editing session.
type Editor struct {
	S          tui.Screen
	Src        Sources
	Deck       Deck
	LayoutPath string
	Secrets    func() *config.Secrets
	Trace      io.Writer

	L    config.Layout
	page string
	sel  int
	clip *config.Button
	err  string // last save problem (shown on the grid)

	mu        sync.Mutex
	govee     []GoveeDevice
	goveeErr  error
	playlists []spotify.Playlist
	plErr     error
}

func (e *Editor) trace(format string, a ...any) {
	if e.Trace != nil {
		fmt.Fprintf(e.Trace, "EDITOR: "+format+"\n", a...)
	}
}

// Run edits the layout until the user leaves.
func (e *Editor) Run(ctx context.Context) error {
	l, err := config.ReadLayout(e.LayoutPath)
	if err != nil {
		l = config.Layout{Brightness: 70, IdleDimSeconds: 600, IdleBrightness: 10, StartPage: "home"}
	}
	if l.Pages == nil {
		l.Pages = map[string][]config.Button{}
	}
	if l.StartPage == "" {
		l.StartPage = "home"
	}
	if _, ok := l.Pages[l.StartPage]; !ok {
		l.Pages[l.StartPage] = nil
	}
	e.L, e.page, e.sel = l, l.StartPage, 0

	// Fetch account data in the background so lists open instantly later.
	go e.loadGovee(ctx)
	go e.loadPlaylists(ctx)

	e.preview()
	e.trace("open")
	defer e.trace("closed")
	return e.grid(ctx)
}

func (e *Editor) loadGovee(ctx context.Context) ([]GoveeDevice, error) {
	e.mu.Lock()
	if e.govee != nil {
		d := e.govee
		e.mu.Unlock()
		return d, nil
	}
	e.mu.Unlock()
	if !e.Src.GoveeReady() {
		return nil, errors.New("Govee isn't set up yet: add your Govee API key in Settings")
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	d, err := e.Src.GoveeDevices(cctx)
	e.mu.Lock()
	if err == nil {
		e.govee = d
	}
	e.goveeErr = err
	e.mu.Unlock()
	return d, err
}

func (e *Editor) loadPlaylists(ctx context.Context) ([]spotify.Playlist, error) {
	e.mu.Lock()
	if e.playlists != nil {
		p := e.playlists
		e.mu.Unlock()
		return p, nil
	}
	e.mu.Unlock()
	if !e.Src.SpotifyReady() {
		return nil, errors.New("Spotify isn't set up yet: log in to Spotify in Settings")
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p, err := e.Src.Playlists(cctx)
	e.mu.Lock()
	if err == nil {
		e.playlists = p
	}
	e.plErr = err
	e.mu.Unlock()
	return p, err
}

func (e *Editor) preview() {
	if e.Deck != nil {
		_ = e.Deck.Preview(e.L, e.page, e.sel)
	}
}

// ---- layout helpers ----

func (e *Editor) button(page string, key int) (config.Button, bool) {
	for _, b := range e.L.Pages[page] {
		if b.Key == key {
			return b, true
		}
	}
	return config.Button{Key: key}, false
}

func (e *Editor) setButton(page string, b config.Button) {
	list := e.L.Pages[page][:0:0]
	for _, x := range e.L.Pages[page] {
		if x.Key != b.Key {
			list = append(list, x)
		}
	}
	if b.Label != "" || len(b.Actions) > 0 || b.Icon != "" {
		list = append(list, b)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	e.L.Pages[page] = list
}

func (e *Editor) pageNames() []string {
	names := make([]string, 0, len(e.L.Pages))
	for n := range e.L.Pages {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == e.L.StartPage {
			return true
		}
		if names[j] == e.L.StartPage {
			return false
		}
		return names[i] < names[j]
	})
	return names
}

// save validates and writes the layout; the daemon picks it up.
func (e *Editor) save() bool {
	var sec *config.Secrets
	if e.Secrets != nil {
		sec = e.Secrets()
	}
	if err := config.CheckLayout(e.L, sec); err != nil {
		e.err = err.Error()
		e.trace("save-error %s", strings.ReplaceAll(e.err, "\n", "; "))
		return false
	}
	if err := config.SaveLayout(e.LayoutPath, e.L); err != nil {
		e.err = err.Error()
		e.trace("save-error %v", err)
		return false
	}
	e.err = ""
	e.trace("saved")
	return true
}

// ---- grid ----

func (e *Editor) grid(ctx context.Context) error {
	var presses <-chan int
	if e.Deck != nil {
		presses = e.Deck.Presses()
	}
	for {
		e.drawGrid()
		var k tui.Key
		select {
		case key, ok := <-presses:
			if !ok {
				presses = nil
				continue
			}
			if key >= 0 && key < keys {
				e.sel = key
				e.preview()
				e.trace("deck-select %d", key)
			}
			continue
		case kk, ok := <-e.S.Keys():
			if !ok {
				return nil
			}
			k = kk
		}
		switch {
		case k.Name == "left":
			e.sel = (e.sel + keys - 1) % keys
		case k.Name == "right":
			e.sel = (e.sel + 1) % keys
		case k.Name == "up":
			e.sel = (e.sel + keys - cols) % keys
		case k.Name == "down":
			e.sel = (e.sel + cols) % keys
		case k.Name == "enter":
			e.editKey(ctx)
		case k.Name == "delete" || k.Name == "backspace":
			if _, ok := e.button(e.page, e.sel); ok && tui.Confirm(e.S, "Clear key", fmt.Sprintf("Clear key %d?", e.sel+1)) {
				e.setButton(e.page, config.Button{Key: e.sel})
				e.save()
			}
		case k.R == 'c' || k.R == 'C':
			if b, ok := e.button(e.page, e.sel); ok {
				c := b
				e.clip = &c
			}
		case k.R == 'v' || k.R == 'V':
			if e.clip != nil {
				b := *e.clip
				b.Key = e.sel
				b.Actions = append([]config.Action(nil), e.clip.Actions...)
				e.setButton(e.page, b)
				e.save()
			}
		case k.Name == "pgup" || k.R == '[':
			e.switchPage(-1)
		case k.Name == "pgdn" || k.R == ']':
			e.switchPage(1)
		case k.R == 'p' || k.R == 'P':
			e.pagesMenu()
		case k.R == 'd' || k.R == 'D':
			e.deckSettings()
		case k.R == 't' || k.R == 'T':
			if b, ok := e.button(e.page, e.sel); ok {
				e.try(ctx, b)
			}
		case k.Name == "esc" || k.R == 'q' || k.R == 'Q':
			if e.err != "" && !tui.Confirm(e.S, "Leave editor", "The last change couldn't be saved (see the red message). Leave anyway?") {
				continue
			}
			return nil
		}
		e.preview()
	}
}

func (e *Editor) switchPage(d int) {
	names := e.pageNames()
	for i, n := range names {
		if n == e.page {
			e.page = names[(i+d+len(names))%len(names)]
			break
		}
	}
}

func (e *Editor) drawGrid() {
	w, h := e.S.Size()
	cw := min(18, max(10, (w-1)/cols-1))
	var lines []string
	title := fmt.Sprintf(" Button editor   page: %s", e.page)
	if e.page == e.L.StartPage {
		title += " (start page)"
	}
	if n := len(e.L.Pages); n > 1 {
		title += fmt.Sprintf("   [%d pages]", n)
	}
	deck := "Stream Deck: live preview"
	if e.Deck == nil {
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
			b, ok := e.button(e.page, k)
			style := ""
			if ok {
				bg := "#202020"
				if b.Color != "" {
					bg = b.Color
				}
				fg := "#ffffff"
				if b.TextColor != "" {
					fg = b.TextColor
				}
				style = colorEsc(bg, true) + colorEsc(fg, false)
			}
			if k == e.sel {
				style = "\x1b[47;30m" // black on white: the selected key
			}
			label := b.Label
			if !ok {
				label = ""
			} else if label == "" {
				label = "(no label)"
			}
			wrapped := tui.Wrap(label, cw-2)
			num := strconv.Itoa(k + 1)
			for i := 0; i < 3; i++ {
				t := ""
				switch {
				case i == 0:
					if k == e.sel {
						num = "> " + num
					}
					t = tui.Fit(num, cw-2)
					if !ok && k != e.sel {
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
	lines = append(lines, hline("└", "┴", "┘"))

	// Details of the selected key.
	b, ok := e.button(e.page, e.sel)
	lines = append(lines, "")
	if !ok {
		lines = append(lines, fmt.Sprintf(" Key %d: empty. Press Enter to set it up.", e.sel+1))
	} else {
		lines = append(lines, fmt.Sprintf(" Key %d: %s%s%s", e.sel+1, tui.Bold, orDash(b.Label), tui.Reset))
		if len(b.Actions) == 0 {
			lines = append(lines, tui.Dim+"   (does nothing yet)"+tui.Reset)
		}
		for i, a := range b.Actions {
			s, warn := e.describe(a)
			if warn != "" {
				s += "  " + tui.Red + "! " + warn + tui.Reset
			}
			lines = append(lines, fmt.Sprintf("   %d. %s", i+1, s))
		}
	}
	if e.err != "" {
		lines = append(lines, "", tui.Red+tui.Bold+" Not saved: "+tui.Reset+tui.Red+strings.ReplaceAll(e.err, "\n", "; ")+tui.Reset)
	}
	help := []string{
		" Arrows: move  Enter: edit  or press a key on the Stream Deck to jump to it",
		" T: try  Del: clear  C/V: copy/paste  [ ]: page  P: pages  D: deck  Esc: done",
	}
	for len(lines) < h-len(help) {
		lines = append(lines, "")
	}
	lines = lines[:min(len(lines), h-len(help))]
	for _, hl := range help {
		lines = append(lines, tui.Dim+hl+tui.Reset)
	}
	e.S.Draw(lines)
}

func orDash(s string) string {
	if s == "" {
		return "(no label)"
	}
	return s
}

// colorEsc picks the closest of the console's 8 colours by hue (the Linux
// console can't show arbitrary colours, and its own nearest-match turns
// most dark colours black).
func colorEsc(hex string, bg bool) string {
	c, err := config.ParseColor(hex)
	if err != nil {
		return ""
	}
	n := consoleColor(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	if bg {
		return fmt.Sprintf("\x1b[%dm", 40+n)
	}
	if n == 0 {
		return "\x1b[30m"
	}
	return fmt.Sprintf("\x1b[1;%dm", 30+n) // bright text
}

// consoleColor returns an ANSI colour number 0-7.
func consoleColor(r, g, b float64) int {
	hi, lo := max(r, g, b), min(r, g, b)
	l := (hi + lo) / 2
	sat := 0.0
	if hi != lo {
		sat = (hi - lo) / (1 - abs(2*l-1))
	}
	if sat < 0.25 || hi < 0.12 {
		if l > 0.6 {
			return 7 // white/grey
		}
		return 0 // black/charcoal
	}
	var h float64
	switch hi {
	case r:
		h = 60 * mod((g-b)/(hi-lo), 6)
	case g:
		h = 60 * ((b-r)/(hi-lo) + 2)
	default:
		h = 60 * ((r-g)/(hi-lo) + 4)
	}
	switch {
	case h < 20 || h >= 330:
		return 1 // red
	case h < 70:
		return 3 // orange/yellow (brown on the console)
	case h < 160:
		return 2 // green
	case h < 200:
		return 6 // cyan/teal
	case h < 255:
		return 4 // blue
	default:
		return 5 // purple/pink
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func mod(a, b float64) float64 {
	for a < 0 {
		a += b
	}
	for a >= b {
		a -= b
	}
	return a
}

// describe summarises an action; warn flags things that won't work.
func (e *Editor) describe(a config.Action) (s, warn string) {
	e.mu.Lock()
	govee, playlists := e.govee, e.playlists
	e.mu.Unlock()
	lightOK := func(name string) string {
		if govee == nil {
			return ""
		}
		for _, d := range govee {
			if strings.EqualFold(d.Name, name) {
				return ""
			}
		}
		return "light not on your Govee account"
	}
	speaker := func(name string) string {
		if name == "" {
			if d := e.Src.DefaultSpeaker(); d != "" {
				return d
			}
			return "the active speaker"
		}
		return name
	}
	switch a.Type {
	case config.GoveeScene, config.GoveeDIYScene:
		warn = lightOK(a.Device)
		if warn == "" && govee != nil {
			warn = "scene not found on this light"
			for _, d := range govee {
				if strings.EqualFold(d.Name, a.Device) {
					for _, sc := range append(append([]string{}, d.Scenes...), d.DIY...) {
						if strings.EqualFold(sc, a.Scene) {
							warn = ""
						}
					}
				}
			}
		}
		return fmt.Sprintf("Lights: %s → scene %q", a.Device, a.Scene), warn
	case config.GoveePower:
		st := "off"
		if a.On != nil && *a.On {
			st = "on"
		}
		return fmt.Sprintf("Lights: turn %s %s", a.Device, st), lightOK(a.Device)
	case config.GoveeBrightness:
		return fmt.Sprintf("Lights: %s brightness %d%%", a.Device, a.Brightness), lightOK(a.Device)
	case config.GoveeColor:
		return fmt.Sprintf("Lights: %s colour %s  %s  %s", a.Device, colorEsc(a.Color, true), tui.Reset, a.Color), lightOK(a.Device)
	case config.SpotifyPlaylist:
		name := a.Playlist
		if uri, err := spotify.NormalizeURI(a.Playlist); err == nil {
			name = uri
			for _, p := range playlists {
				if p.URI == uri {
					name = "“" + p.Name + "”"
				}
			}
		}
		s = fmt.Sprintf("Music: play %s on %s", name, speaker(a.SpotifyDevice))
		if a.Shuffle {
			s += ", shuffled"
		}
		if a.Volume != nil {
			s += fmt.Sprintf(", volume %d%%", *a.Volume)
		}
		return s, ""
	case config.SpotifyPause:
		return "Music: pause on " + speaker(a.SpotifyDevice), ""
	case config.SpotifyNext:
		return "Music: next song on " + speaker(a.SpotifyDevice), ""
	case config.SpotifyVolume:
		v := 0
		if a.Volume != nil {
			v = *a.Volume
		}
		return fmt.Sprintf("Music: volume %d%% on %s", v, speaker(a.SpotifyDevice)), ""
	case config.Page:
		if _, ok := e.L.Pages[a.Page]; !ok {
			warn = "page doesn't exist"
		}
		return "Go to page " + a.Page, warn
	case config.Delay:
		return fmt.Sprintf("Wait %.1f s", float64(a.Ms)/1000), ""
	}
	return a.Type, "unknown action"
}

func (e *Editor) try(ctx context.Context, b config.Button) {
	var run []config.Action
	for _, a := range b.Actions {
		if a.Type != config.Page {
			run = append(run, a)
		}
	}
	if len(run) == 0 {
		tui.Message(e.S, "Try it", []string{"This key has nothing to run."})
		return
	}
	tui.Busy(e.S, "Try it", fmt.Sprintf("Running key %d (%s)…", b.Key+1, orDash(b.Label)))
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := e.Src.Run(cctx, run); err != nil {
		e.trace("try-error %v", err)
		tui.Message(e.S, "Try it", append([]string{tui.Red + "Something didn't work:" + tui.Reset, ""}, strings.Split(err.Error(), "\n")...))
		return
	}
	e.trace("try-ok %d", b.Key)
	tui.Message(e.S, "Try it", []string{tui.Green + "Done." + tui.Reset})
}
