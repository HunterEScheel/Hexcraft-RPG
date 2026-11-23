// Package editor is the button editor: a full-screen app on the mini PC's
// monitor, driven by keyboard and touchpad, with the Stream Deck itself as a
// live preview (pressing a deck key selects it; pressing a folder key twice
// opens the folder).
package editor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/ui"
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
	U          ui.UI
	Src        Sources
	Deck       Deck
	LayoutPath string
	Secrets    func() *config.Secrets
	Trace      io.Writer

	L     config.Layout
	stack []string // folders opened, from the start page; the last is shown
	page  string
	sel   int
	clip  *config.Button
	err   string // last save problem (shown on the grid)
	fresh string // folder created during the current key edit

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
	e.L, e.sel = l, 0
	e.stack = []string{l.StartPage}
	e.page = l.StartPage

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
		return nil, errors.New("Spotify isn't set up yet: connect Spotify in Settings")
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

// folderOf returns the folder a key opens ("" if it isn't a folder key).
func folderOf(b config.Button) string {
	for _, a := range b.Actions {
		if a.Type == config.Page {
			return a.Page
		}
	}
	return ""
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

// ---- folder navigation ----

// open shows folder name. Opening a folder that's already on the path (e.g.
// with a "Back" key) goes back up to it.
func (e *Editor) open(name string) {
	if _, ok := e.L.Pages[name]; !ok {
		e.U.Message(ui.Note{ID: "no-folder", Kind: ui.WarningNote, Title: "Folder missing",
			Lines: []string{fmt.Sprintf("There's no folder called %q any more. Edit this key to point it somewhere else.", name)}})
		return
	}
	for i, p := range e.stack {
		if p == name {
			e.stack = e.stack[:i+1]
			e.page, e.sel = name, 0
			e.trace("folder-open %s", name)
			return
		}
	}
	e.stack = append(e.stack, name)
	e.page, e.sel = name, 0
	e.trace("folder-open %s", name)
}

// up goes to the parent folder; false at the top.
func (e *Editor) up() bool {
	if len(e.stack) <= 1 {
		return false
	}
	from := e.page
	e.stack = e.stack[:len(e.stack)-1]
	e.page = e.stack[len(e.stack)-1]
	e.sel = 0
	// Land on the key that opened the folder.
	for _, b := range e.L.Pages[e.page] {
		if folderOf(b) == from {
			e.sel = b.Key
			break
		}
	}
	e.trace("folder-up %s", e.page)
	return true
}

// jump shows any folder (from the folders list).
func (e *Editor) jump(name string) {
	e.stack = []string{e.L.StartPage}
	e.page, e.sel = e.L.StartPage, 0
	if name != e.L.StartPage {
		e.open(name)
	}
}

// newFolder asks for a name and creates the folder with a Back key that
// returns to parent.
func (e *Editor) newFolder(parent string) (string, bool) {
	for {
		n, ok := e.U.Input(ui.InputOpts{ID: "folder-name", Title: "New folder",
			Intro: []string{"A folder is another set of 15 buttons, opened by a key. Give it a short name, e.g. Lights or Music."},
			Label: "Folder name", Placeholder: "e.g. Lights"})
		if !ok || n == "" {
			return "", false
		}
		if _, exists := e.L.Pages[n]; exists {
			e.U.Message(ui.Note{ID: "folder-exists", Kind: ui.WarningNote, Title: "New folder", Lines: []string{"There's already a folder called " + n + "."}})
			continue
		}
		e.L.Pages[n] = []config.Button{{Key: 0, Label: "‹ Back", Color: "#3a3f4b",
			Actions: []config.Action{{Type: config.Page, Page: parent}}}}
		e.trace("folder-new %s", n)
		return n, true
	}
}

// ---- grid ----

func (e *Editor) leave() bool {
	if e.err != "" && !e.U.Confirm(ui.ConfirmOpts{ID: "leave-unsaved", Title: "Leave the editor?",
		Question: "The last change couldn't be saved (see the red message). Leave anyway?", Yes: "Leave", No: "Stay"}) {
		return false
	}
	return true
}

func (e *Editor) grid(ctx context.Context) error {
	var presses <-chan int
	if e.Deck != nil {
		presses = e.Deck.Presses()
	}
	redraw := true
	lastPress, lastAt := -1, time.Time{}
	for {
		if redraw {
			e.drawGrid()
		}
		redraw = true
		var ev ui.Event
		select {
		case key, ok := <-presses:
			if !ok {
				presses = nil
				continue
			}
			if key < 0 || key >= keys {
				continue
			}
			b, _ := e.button(e.page, key)
			if key == lastPress && time.Since(lastAt) < 2*time.Second && folderOf(b) != "" {
				// Folder key pressed twice: go in.
				e.open(folderOf(b))
				lastPress = -1
			} else {
				e.sel = key
				lastPress, lastAt = key, time.Now()
				e.trace("deck-select %d", key)
			}
			e.preview()
			continue
		case ev2, ok := <-e.U.Events():
			if !ok {
				return nil
			}
			ev = ev2
		}
		switch ev.Kind {
		case ui.TickEvent, ui.WheelEvent:
			redraw = false
			continue
		case ui.HoverEvent:
			redraw = strings.HasPrefix(ev.Target, "cell:") || strings.HasPrefix(ev.Target, "btn:") || ev.Target == "" || ev.Target == "back"
			continue
		case ui.ClickEvent:
			var n int
			switch {
			case ev.Target == "back":
				if !e.up() && e.leave() {
					return nil
				}
			case scan(ev.Target, "cell:%d", &n):
				if n == e.sel {
					e.activate(ctx)
				} else {
					e.sel = n
				}
			case strings.HasPrefix(ev.Target, "btn:"):
				e.command(ctx, strings.TrimPrefix(ev.Target, "btn:"))
			}
		case ui.KeyEvent:
			k := ev.Key
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
			case k.R == ' ' || k.R == 'o' || k.R == 'O':
				e.activate(ctx)
			case k.Name == "delete" || k.Name == "backspace":
				e.command(ctx, "clear key")
			case k.R == 'c' || k.R == 'C':
				if b, ok := e.button(e.page, e.sel); ok {
					c := b
					c.Actions = append([]config.Action(nil), b.Actions...)
					e.clip = &c
					e.trace("copy %d", e.sel)
				}
			case k.R == 'v' || k.R == 'V':
				e.command(ctx, "paste")
			case k.R == 'n' || k.R == 'N':
				e.command(ctx, "new folder here")
			case k.R == 'f' || k.R == 'F' || k.R == 'p' || k.R == 'P':
				e.foldersMenu()
			case k.R == 'd' || k.R == 'D':
				e.deckSettings()
			case k.R == 't' || k.R == 'T':
				e.command(ctx, "try it")
			case k.Name == "esc" || k.R == 'q' || k.R == 'Q':
				if !e.up() && e.leave() {
					return nil
				}
			}
		}
		e.preview()
	}
}

func scan(s, format string, n *int) bool {
	_, err := fmt.Sscanf(s, format, n)
	return err == nil
}

// activate opens a folder key, or edits any other key.
func (e *Editor) activate(ctx context.Context) {
	b, _ := e.button(e.page, e.sel)
	if f := folderOf(b); f != "" {
		e.open(f)
		return
	}
	e.editKey(ctx)
}

// command runs a side-panel button (also used by keyboard shortcuts).
func (e *Editor) command(ctx context.Context, name string) {
	b, exists := e.button(e.page, e.sel)
	switch name {
	case "edit key", "set up this key":
		e.editKey(ctx)
	case "open folder":
		e.activate(ctx)
	case "try it":
		if exists {
			e.try(ctx, b)
		}
	case "clear key":
		if exists && e.U.Confirm(ui.ConfirmOpts{ID: "clear-key", Title: "Clear key", Question: fmt.Sprintf("Remove everything from key %d?", e.sel+1), Yes: "Clear", Danger: true}) {
			e.setButton(e.page, config.Button{Key: e.sel})
			e.save()
		}
	case "paste":
		if e.clip != nil {
			nb := *e.clip
			nb.Key = e.sel
			nb.Actions = append([]config.Action(nil), e.clip.Actions...)
			e.setButton(e.page, nb)
			e.save()
		}
	case "new folder here":
		if exists {
			return
		}
		n, ok := e.newFolder(e.page)
		if !ok {
			return
		}
		e.setButton(e.page, config.Button{Key: e.sel, Label: n, Color: "#1d5fa0",
			Actions: []config.Action{{Type: config.Page, Page: n}}})
		if e.save() {
			e.open(n)
		}
	case "folders":
		e.foldersMenu()
	case "deck settings":
		e.deckSettings()
	}
}

func (e *Editor) drawGrid() {
	v := ui.GridView{
		Page:      e.page,
		StartPage: e.L.StartPage,
		Pages:     e.pageNames(),
		Dir:       filepath.Dir(e.LayoutPath),
		Selected:  e.sel,
		DeckLive:  e.Deck != nil,
		Err:       e.err,
	}
	if len(e.stack) > 1 {
		v.Path = append([]string(nil), e.stack[1:]...)
	}
	for k := 0; k < keys; k++ {
		b, ok := e.button(e.page, k)
		c := ui.Cell{Button: b, Empty: !ok}
		for _, a := range b.Actions {
			if _, w := e.describe(a); w != "" {
				c.Warn = true
			}
		}
		v.Cells = append(v.Cells, c)
	}
	b, ok := e.button(e.page, e.sel)
	if !ok {
		v.KeyTitle = fmt.Sprintf("Key %d: empty", e.sel+1)
		v.Details = []string{"Click it again or press Enter to set it up."}
		v.Buttons = []string{"Set up this key", "New folder here"}
		if e.clip != nil {
			v.Buttons = append(v.Buttons, "Paste")
		}
	} else {
		v.KeyTitle = fmt.Sprintf("Key %d: %s", e.sel+1, orDash(b.Label))
		if len(b.Actions) == 0 {
			v.Details = append(v.Details, "Does nothing yet")
		}
		for _, a := range b.Actions {
			s, warn := e.describe(a)
			v.Details = append(v.Details, s)
			if warn != "" {
				v.Warnings = append(v.Warnings, warn)
			}
		}
		v.Buttons = []string{"Edit key"}
		if folderOf(b) != "" {
			v.Buttons = []string{"Open folder", "Edit key"}
		}
		if e.runnable(b) {
			v.Buttons = append(v.Buttons, "Try it")
		}
		v.Buttons = append(v.Buttons, "Clear key")
	}
	v.Buttons = append(v.Buttons, "Folders", "Deck settings")
	e.U.Grid(v)
}

func orDash(s string) string {
	if s == "" {
		return "(no label)"
	}
	return s
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
		return name + " isn't on your Govee account"
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
			warn = "scene " + a.Scene + " not found on " + a.Device
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
		return fmt.Sprintf("%s: scene “%s”", a.Device, a.Scene), warn
	case config.GoveePower:
		st := "off"
		if a.On != nil && *a.On {
			st = "on"
		}
		return fmt.Sprintf("%s: turn %s", a.Device, st), lightOK(a.Device)
	case config.GoveeBrightness:
		return fmt.Sprintf("%s: brightness %d%%", a.Device, a.Brightness), lightOK(a.Device)
	case config.GoveeColor:
		return fmt.Sprintf("%s: colour %s", a.Device, colorName(a.Color, "#ffffff")), lightOK(a.Device)
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
		s = fmt.Sprintf("Play %s on %s", name, speaker(a.SpotifyDevice))
		if a.Shuffle {
			s += ", shuffled"
		}
		if a.Volume != nil {
			s += fmt.Sprintf(", volume %d%%", *a.Volume)
		}
		return s, ""
	case config.SpotifyPause:
		return "Pause music on " + speaker(a.SpotifyDevice), ""
	case config.SpotifyNext:
		return "Next song on " + speaker(a.SpotifyDevice), ""
	case config.SpotifyVolume:
		v := 0
		if a.Volume != nil {
			v = *a.Volume
		}
		return fmt.Sprintf("Volume %d%% on %s", v, speaker(a.SpotifyDevice)), ""
	case config.Page:
		if _, ok := e.L.Pages[a.Page]; !ok {
			warn = "folder " + a.Page + " doesn't exist"
		}
		if len(e.stack) > 1 && a.Page == e.stack[len(e.stack)-2] {
			return "Back to " + a.Page, warn
		}
		return "Opens folder " + a.Page, warn
	case config.Delay:
		return fmt.Sprintf("Wait %.1f s", float64(a.Ms)/1000), ""
	}
	return a.Type, "unknown action " + a.Type
}

func actionIcon(t string) string {
	switch t {
	case config.GoveeScene, config.GoveeDIYScene, config.GoveePower, config.GoveeBrightness, config.GoveeColor:
		return "light"
	case config.SpotifyPlaylist, config.SpotifyPause, config.SpotifyNext, config.SpotifyVolume:
		return "music"
	case config.Page:
		return "folder"
	case config.Delay:
		return "wait"
	}
	return "info"
}

func (e *Editor) runnable(b config.Button) bool {
	for _, a := range b.Actions {
		if a.Type != config.Page {
			return true
		}
	}
	return false
}

func (e *Editor) try(ctx context.Context, b config.Button) {
	var run []config.Action
	for _, a := range b.Actions {
		if a.Type != config.Page {
			run = append(run, a)
		}
	}
	if len(run) == 0 {
		e.U.Message(ui.Note{ID: "try-nothing", Title: "Try it", Lines: []string{"This key has nothing to run."}})
		return
	}
	e.U.Info(ui.InfoView{ID: "try", Title: "Try it", Spinner: true, Lines: []string{fmt.Sprintf("Running key %d (%s)…", b.Key+1, orDash(b.Label))}})
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := e.Src.Run(cctx, run); err != nil {
		e.trace("try-error %v", err)
		e.U.Message(ui.Note{ID: "try-error", Kind: ui.ErrorNote, Title: "Something didn't work", Lines: strings.Split(err.Error(), "\n")})
		return
	}
	e.trace("try-ok %d", b.Key)
	e.U.Message(ui.Note{ID: "try-ok", Kind: ui.SuccessNote, Title: "Done", Lines: []string{"The key ran without problems."}})
}
