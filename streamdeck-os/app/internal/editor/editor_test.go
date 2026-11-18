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
	e := &Editor{S: scr, Src: src, Deck: deck, LayoutPath: p, Trace: tr, Secrets: func() *config.Secrets { return secrets }}
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

func TestConsoleColorByHue(t *testing.T) {
	for hex, want := range map[string]int{
		"#3a1d6e": 5, "#b0186e": 5, "#105a6e": 6, "#1db954": 2, "#c0262d": 1, "#e0701b": 3,
		"#1d5fa0": 4, "#202020": 0, "#ffffff": 7, "#6b6b6b": 0, "#e8c020": 3,
	} {
		c, _ := config.ParseColor(hex)
		if got := consoleColor(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255); got != want {
			t.Errorf("%s: got %d want %d", hex, got, want)
		}
	}
}
