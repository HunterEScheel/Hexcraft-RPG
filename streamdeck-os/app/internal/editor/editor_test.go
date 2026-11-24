package editor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/tui"
	"streamdeckd/internal/ui"
	"streamdeckd/internal/ui/text"
)

type fakeSrc struct {
	govee, spotify bool
	ran            [][]config.Action
	needReconnect  bool
}

func (f *fakeSrc) GoveeReady() bool       { return f.govee }
func (f *fakeSrc) SpotifyReady() bool     { return f.spotify }
func (f *fakeSrc) DefaultSpeaker() string { return "Living Room" }
func (f *fakeSrc) GoveeDevices(context.Context) ([]GoveeDevice, error) {
	return []GoveeDevice{
		{Name: "TV Backlight", Scenes: []string{"Movie", "Party", "Sunset"}},
		{Name: "Floor Lamp", Scenes: []string{"Rainbow", "Party Time", "Reading"}, DIY: []string{"My Glow"}},
		{Name: "Living Room Lights", Group: true},
	}, nil
}
func (f *fakeSrc) SpotifyDevices(context.Context) ([]string, error) {
	return []string{"Living Room", "Kitchen Echo"}, nil
}
func (f *fakeSrc) Playlists(context.Context) ([]spotify.Playlist, error) {
	if f.needReconnect {
		return nil, spotify.ErrNeedsReconnect
	}
	return []spotify.Playlist{
		{URI: "spotify:playlist:AAA", Name: "Dinner Jazz", Owner: "me", Tracks: 40},
		{URI: "spotify:playlist:BBB", Name: "Workout", Owner: "me", Tracks: 25},
	}, nil
}
func (f *fakeSrc) Run(_ context.Context, a []config.Action) error {
	f.ran = append(f.ran, a)
	return nil
}
func (f *fakeSrc) ReconnectSpotify(context.Context) error { f.needReconnect = false; return nil }

type fakeDeck struct {
	mu       sync.Mutex
	previews int
	lastSel  int
	p        chan int
}

func (d *fakeDeck) Preview(_ config.Layout, _ string, sel int) error {
	d.mu.Lock()
	d.previews++
	d.lastSel = sel
	d.mu.Unlock()
	return nil
}
func (d *fakeDeck) Presses() <-chan int { return d.p }

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func setup(t *testing.T, layout string, src *fakeSrc) (*Editor, *tui.Fake, *fakeDeck, *syncBuf, chan error) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if layout != "" {
		os.WriteFile(p, []byte(layout), 0o644)
	}
	scr := tui.NewFake(100, 34)
	deck := &fakeDeck{p: make(chan int, 4)}
	tr := &syncBuf{}
	secrets := &config.Secrets{}
	if src.govee {
		secrets.Govee.APIKey = "k"
	}
	if src.spotify {
		secrets.Spotify = config.SpotifyConfig{ClientID: "i", ClientSecret: "s", RefreshToken: "r", DefaultDevice: "Living Room"}
	}
	e := &Editor{U: text.New(scr, ui.Tracer{W: tr}), Src: src, Deck: deck, LayoutPath: p, Trace: tr, Secrets: func() *config.Secrets { return secrets }}
	done := make(chan error, 1)
	go func() { done <- e.Run(context.Background()) }()
	return e, scr, deck, tr, done
}

func waitFor(t *testing.T, tr *syncBuf, s string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if strings.Contains(tr.String(), s) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never saw %q in trace:\n%s", s, tr.String())
}

func finish(t *testing.T, done chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("editor did not exit")
	}
}

func TestBuildKeyWithSceneAndPlaylist(t *testing.T) {
	src := &fakeSrc{govee: true, spotify: true}
	e, scr, deck, tr, done := setup(t, "", src)
	scr.Press("enter")                                 // edit key 1
	scr.Press("enter", "Party", "enter")               // label
	scr.Press("enter")                                 // "+ Add an action" (auto-selected)
	scr.Press("enter")                                 // "Set a scene"
	scr.Press("down", "enter")                         // Floor Lamp
	scr.Press("party", "enter")                        // filter scenes -> "Party Time"
	scr.Press("enter")                                 // "+ Add an action" again
	scr.Press("down", "down", "down", "down", "enter") // "Play a playlist"
	scr.Press("down", "enter")                         // Workout
	scr.Press("down", "enter")                         // speaker: Living Room
	scr.Press("down", "enter")                         // shuffle
	scr.Press("down", "enter")                         // set volume
	scr.Press("right", "right", "enter")               // 60%
	scr.Press("esc")                                   // done -> saves
	waitFor(t, tr, "saved")
	scr.Press("esc") // leave editor
	finish(t, done)

	l, err := config.ReadLayout(e.LayoutPath)
	if err != nil {
		t.Fatal(err)
	}
	b := l.Pages["home"]
	if len(b) != 1 || b[0].Key != 0 || b[0].Label != "Party" || len(b[0].Actions) != 2 {
		t.Fatalf("layout: %+v", l.Pages)
	}
	a0, a1 := b[0].Actions[0], b[0].Actions[1]
	if a0.Type != config.GoveeScene || a0.Device != "Floor Lamp" || a0.Scene != "Party Time" {
		t.Fatalf("scene action: %+v", a0)
	}
	if a1.Type != config.SpotifyPlaylist || a1.Playlist != "spotify:playlist:BBB" || a1.SpotifyDevice != "Living Room" ||
		!a1.Shuffle || a1.Volume == nil || *a1.Volume != 60 {
		t.Fatalf("playlist action: %+v (vol %v)", a1, a1.Volume)
	}
	if strings.Contains(tr.String(), "Living Room Lights") {
		t.Fatalf("Govee group offered in the light picker:\n%s", tr.String())
	}
	if deck.previews < 3 {
		t.Fatalf("deck preview not updated live (%d)", deck.previews)
	}
}

func TestDeckPressSelectsKey(t *testing.T) {
	src := &fakeSrc{govee: true, spotify: true}
	e, scr, deck, tr, done := setup(t, "", src)
	waitFor(t, tr, "open")
	deck.p <- 7
	waitFor(t, tr, "deck-select 7")
	if !strings.Contains(scr.Text(), "Key 8: empty") {
		t.Fatalf("grid didn't move to key 8:\n%s", scr.Text())
	}
	scr.Press("enter", "enter", "Lamp", "enter", "esc")
	waitFor(t, tr, "saved")
	scr.Press("esc")
	finish(t, done)
	l, _ := config.ReadLayout(e.LayoutPath)
	if b := l.Pages["home"]; len(b) != 1 || b[0].Key != 7 || b[0].Label != "Lamp" {
		t.Fatalf("got %+v", l.Pages)
	}
	if deck.lastSel != 7 {
		t.Fatalf("deck highlight on %d", deck.lastSel)
	}
}

func TestGoveeNotSetUpAndInvalidSaveUndo(t *testing.T) {
	// Existing layout uses Govee, but no Govee key is saved: saving must fail
	// with a clear message, and "Undo" must restore the key.
	layout := `{"pages":{"home":[{"key":0,"label":"Old","actions":[{"type":"govee_power","device":"X","on":true}]}]}}`
	src := &fakeSrc{govee: false, spotify: false}
	e, scr, _, tr, done := setup(t, layout, src)
	scr.Press("enter", "enter", "backspace", "backspace", "backspace", "New", "enter", "esc")
	waitFor(t, tr, "save-error")
	if !strings.Contains(scr.Text(), "Govee API key missing") {
		t.Fatalf("no explanation shown:\n%s", scr.Text())
	}
	scr.Press("down", "enter") // Undo my changes
	time.Sleep(100 * time.Millisecond)
	scr.Press("esc")
	finish(t, done)
	raw, _ := os.ReadFile(e.LayoutPath)
	if !strings.Contains(string(raw), `"Old"`) || strings.Contains(string(raw), `"New"`) {
		t.Fatalf("layout changed:\n%s", raw)
	}
}

func TestPlaylistReconnect(t *testing.T) {
	src := &fakeSrc{govee: true, spotify: true, needReconnect: true}
	e, scr, _, tr, done := setup(t, "", src)
	scr.Press("enter", "enter", "X", "enter", "enter") // key 1, label X, add action
	scr.Press("down", "down", "down", "down", "enter") // Play a playlist
	waitFor(t, tr, "pick-playlist 0")
	scr.Press("enter") // Reconnect Spotify…
	waitFor(t, tr, "pick-playlist 2")
	scr.Press("enter", "enter", "enter", "enter", "esc") // Dinner Jazz, default speaker, in order, keep volume, done
	waitFor(t, tr, "saved")
	scr.Press("esc")
	finish(t, done)
	l, _ := config.ReadLayout(e.LayoutPath)
	a := l.Pages["home"][0].Actions[0]
	if a.Playlist != "spotify:playlist:AAA" || a.SpotifyDevice != "" || a.Shuffle || a.Volume != nil {
		t.Fatalf("got %+v", a)
	}
}

var _ = errors.New

func TestNewFolderHereAndNavigate(t *testing.T) {
	src := &fakeSrc{govee: true, spotify: true}
	e, scr, deck, tr, done := setup(t, "", src)
	waitFor(t, tr, "open")
	scr.Press("n", "Lights", "enter") // new folder on empty key 1
	waitFor(t, tr, "folder-open Lights")
	waitFor(t, tr, "grid page=Lights sel=0")
	if txt := scr.Text(); !strings.Contains(txt, "Buttons > Lights") || !strings.Contains(txt, "Back") {
		t.Fatalf("not inside the folder:\n%s", txt)
	}
	scr.Press("right", "enter", "enter", "Lamp", "enter", "esc") // key 2 in the folder
	waitFor(t, tr, "saved")
	scr.Press("esc") // up to the main buttons
	waitFor(t, tr, "folder-up home")
	// Pressing the folder key on the deck twice opens it again.
	deck.p <- 0
	deck.p <- 0
	for i := 0; strings.Count(tr.String(), "folder-open Lights") < 2; i++ {
		if i > 200 {
			t.Fatalf("deck double press didn't open the folder:\n%s", tr.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Opening a folder key that points back up returns to the parent.
	scr.Press("o")
	waitFor(t, tr, "grid page=home")
	scr.Press("esc")
	finish(t, done)

	l, _ := config.ReadLayout(e.LayoutPath)
	h := l.Pages["home"]
	if len(h) != 1 || h[0].Label != "Lights" || folderOf(h[0]) != "Lights" {
		t.Fatalf("home: %+v", h)
	}
	f := l.Pages["Lights"]
	if len(f) != 2 || folderOf(f[0]) != "home" || f[1].Key != 1 || f[1].Label != "Lamp" {
		t.Fatalf("Lights: %+v", f)
	}
}

func TestFolderActionCreatesAndOpens(t *testing.T) {
	src := &fakeSrc{govee: true, spotify: true}
	e, scr, _, tr, done := setup(t, "", src)
	scr.Press("right", "enter", "down", "down", "down", "enter") // key 2: add an action (no label yet)
	waitFor(t, tr, "screen add-action")
	scr.Press("down", "down", "down", "down", "down", "down", "down", "down", "enter") // Open a folder
	waitFor(t, tr, "screen pick-folder")
	scr.Press("enter", "Music", "enter") // + New folder…
	waitFor(t, tr, "folder-new Music")
	scr.Press("esc") // done: saves, asks to open
	waitFor(t, tr, "menu open-new-folder")
	scr.Press("down", "enter")
	waitFor(t, tr, "folder-open Music")
	scr.Press("esc", "esc")
	finish(t, done)
	l, _ := config.ReadLayout(e.LayoutPath)
	if b := l.Pages["home"]; len(b) != 1 || b[0].Key != 1 || b[0].Label != "Music" || folderOf(b[0]) != "Music" {
		t.Fatalf("home: %+v", l.Pages)
	}
	if b := l.Pages["Music"]; len(b) != 1 || folderOf(b[0]) != "home" {
		t.Fatalf("Music: %+v", b)
	}
}

func TestLANLightColourFromWheel(t *testing.T) {
	// No Govee API key at all: a light on the network still works.
	layout := `{"pages":{"home":[]},"lan_lights":[{"name":"Flood Lights","device":"AA:BB:CC:DD:EE:FF:00:61","sku":"H7061","ip":"192.168.1.50"}]}`
	src := &fakeSrc{govee: false, spotify: false}
	e, scr, _, tr, done := setup(t, layout, src)
	scr.Press("enter", "enter", "Flood", "enter", "enter") // key 1: label, + Add an action
	waitFor(t, tr, "screen add-action")
	scr.Press("down", "down", "down", "enter") // Set a colour
	waitFor(t, tr, "menu pick-light")
	if !strings.Contains(tr.String(), "items=Flood Lights") {
		t.Fatalf("LAN light not offered:\n%s", tr.String())
	}
	scr.Press("enter")
	waitFor(t, tr, "color color value=#ffffff")
	scr.Press("up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up") // full strength
	scr.Press("right", "right", "right", "right", "right", "right")                                                                   // hue 30° (orange)
	waitFor(t, tr, "value=#ff8000")
	scr.Press("enter", "esc")
	waitFor(t, tr, "saved")
	scr.Press("esc")
	finish(t, done)
	l, _ := config.ReadLayout(e.LayoutPath)
	a := l.Pages["home"][0].Actions[0]
	if a.Type != config.GoveeColor || a.Device != "Flood Lights" || a.Color != "#ff8000" {
		t.Fatalf("got %+v", a)
	}
	// Scenes leave LAN lights out (Govee's LAN control has none).
	e2, scr2, _, tr2, done2 := setup(t, layout, src)
	_ = e2
	scr2.Press("enter", "enter", "X", "enter", "enter", "enter") // Set a scene
	waitFor(t, tr2, "message no-scene-lights")
	scr2.Press("enter", "esc", "esc")
	finish(t, done2)
}
