// Package ui is the screen toolkit of StreamDeck OS: menus, text fields,
// sliders, dialogs, the home dashboard and the button-editor grid.
//
// Two renderers implement it: a graphical one on the framebuffer with mouse
// and touchpad support (ui/gfx), and a text one on the console (ui/text,
// used by tests and when there is no framebuffer).
package ui

import (
	"fmt"
	"io"
	"strings"

	"streamdeckd/internal/config"
	"streamdeckd/internal/tui"
)

type Key = tui.Key

// EventKind says what happened.
type EventKind int

const (
	KeyEvent   EventKind = iota
	ClickEvent           // Target clicked (mouse/touchpad)
	HoverEvent           // pointer moved over Target
	WheelEvent           // Delta > 0: scroll up
	TickEvent            // periodic, for animations and refreshes
)

// Event is input from the keyboard or the pointer.
type Event struct {
	Kind   EventKind
	Key    Key
	Target string
	Delta  int
}

// Item is a row in a menu.
type Item struct {
	Label  string
	Detail string // second line / dimmed text
	Value  string // right-aligned value
	Swatch string // #rrggbb colour sample
	Icon   string // icon name (see gfx icons): wifi, light, music, key, gear, disk, usb, page, wait, check, plus, trash, play, back, deck, info, warn
	Header bool   // section heading, not selectable
	Danger bool
}

type MenuOpts struct {
	ID       string
	Title    string
	Subtitle string
	Intro    []string
	Items    []Item
	Selected int
	Filter   bool // typing narrows the list
	Root     bool // top-level screen: no Back button
	// Keys handles extra keys; returning true ends the menu with that key.
	Keys func(k Key, index int) bool
}

type MenuResult struct {
	Index int
	Key   Key
	OK    bool // false: cancelled (Esc / Back)
}

type InputOpts struct {
	ID          string
	Title       string
	Intro       []string
	Label       string
	Initial     string
	Placeholder string
	Secret      bool // show dots (with a "show" toggle)
}

type NumberOpts struct {
	ID       string
	Title    string
	Label    string
	Value    int
	Min, Max int
	Step     int
	Unit     string
}

type NoteKind int

const (
	InfoNote NoteKind = iota
	SuccessNote
	ErrorNote
	WarningNote
)

type Note struct {
	ID    string
	Kind  NoteKind
	Title string
	Lines []string
}

type ConfirmOpts struct {
	ID       string
	Title    string
	Question string
	Yes, No  string
	Danger   bool
}

// InfoView is a non-blocking screen (progress, waiting for the phone...).
type InfoView struct {
	ID      string
	Title   string
	Lines   []string
	Big     string // shown large (e.g. an address to type on a phone)
	Spinner bool
	Hint    string
	Buttons []string // clickable; targets "btn:<label>"
}

type CardState int

const (
	CardOff CardState = iota
	CardOK
	CardWarn
)

type StatusCard struct {
	Icon  string
	Title string
	Value string
	State CardState
}

type Tile struct {
	ID     string
	Label  string
	Detail string
	Icon   string
}

type Step struct {
	Label string
	Done  bool
}

// HomeView is the dashboard. Clicking a tile gives target "tile:<ID>",
// the banner "banner".
type HomeView struct {
	Title    string
	Subtitle string
	Cards    []StatusCard
	Steps    []Step // "getting started" checklist (empty when done)
	Banner   string // e.g. "Setup file found on a USB stick"
	BannerOK string // its button label
	Tiles    []Tile
	Selected int
}

// Cell is one deck key in the editor grid.
type Cell struct {
	Button config.Button
	Empty  bool
	Warn   bool
}

// GridView is the button editor. Targets: "cell:<n>", "tab:<page>", "btn:<label>".
type GridView struct {
	Path      []string // breadcrumb of folders below the start page
	Page      string
	Pages     []string
	StartPage string
	Dir       string // for icons
	Cells     []Cell
	Selected  int
	KeyTitle  string
	Details   []string
	Warnings  []string
	Err       string
	DeckLive  bool
	Buttons   []string
}

// UI is implemented by the graphical and text renderers.
type UI interface {
	Events() <-chan Event
	Menu(MenuOpts) MenuResult
	Input(InputOpts) (string, bool)
	Number(NumberOpts) (int, bool)
	Message(Note)
	Confirm(ConfirmOpts) bool
	Info(InfoView)
	Home(HomeView)
	Grid(GridView)
	Close()
}

// Tracer writes machine-readable markers for the VM tests.
type Tracer struct{ W io.Writer }

func (t Tracer) Printf(format string, a ...any) {
	if t.W != nil {
		fmt.Fprintf(t.W, "UI: "+format+"\n", a...)
	}
}

// TraceMenu records a menu with its rows so tests can navigate by label.
func (t Tracer) TraceMenu(o MenuOpts, sel int) {
	if t.W == nil {
		return
	}
	labels := make([]string, len(o.Items))
	for i, it := range o.Items {
		l := strings.ReplaceAll(it.Label, "|", "/")
		if it.Header {
			l = "#" + l
		}
		labels[i] = l
	}
	t.Printf("menu %s sel=%d items=%s", o.ID, sel, strings.Join(labels, "|"))
}
