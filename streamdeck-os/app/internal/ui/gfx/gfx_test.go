package gfx

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/fb"
	"streamdeckd/internal/tui"
	"streamdeckd/internal/ui"
)

// TestScreens draws every kind of screen; with SCREENSHOT_DIR set it saves
// them as PNGs (used for the README).
func TestScreens(t *testing.T) {
	dev := fb.Memory(1280, 800)
	keys := make(chan tui.Key, 8)
	mice := make(chan fb.MouseEvent, 8)
	g := New(dev, keys, mice, ui.Tracer{})
	out := os.Getenv("SCREENSHOT_DIR")
	shot := func(name string) {
		img := dev.Snapshot()
		// Something besides the background must have been drawn.
		if img.At(640, 400) == img.At(5, 400) && img.At(400, 300) == img.At(5, 300) {
			t.Errorf("%s: screen looks empty", name)
		}
		if out == "" {
			return
		}
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		png.Encode(f, img)
	}
	blocking := func(name string, f func()) {
		done := make(chan struct{})
		go func() { f(); close(done) }()
		time.Sleep(300 * time.Millisecond)
		shot(name)
		keys <- tui.Key{Name: "esc"}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("%s didn't close on Esc", name)
		}
	}

	g.Home(ui.HomeView{Title: "StreamDeck OS", Subtitle: "Monday, November 23",
		Cards: []ui.StatusCard{{Icon: "wifi", Title: "Wi-Fi", Value: "WhIOT", State: ui.CardOK}, {Icon: "deck", Title: "Stream Deck", Value: "Stream Deck MK.2", State: ui.CardOK},
			{Icon: "light", Title: "Lights", Value: "Ready", State: ui.CardOK}, {Icon: "music", Title: "Music", Value: "Not connected"}},
		Steps:  []ui.Step{{Label: "Connect Wi-Fi", Done: true}, {Label: "Add your Govee key", Done: true}, {Label: "Connect Spotify"}, {Label: "Set up buttons"}},
		Banner: "Setup file found on a USB stick (Spotify app keys)", BannerOK: "Use it",
		Tiles: []ui.Tile{{ID: "buttons", Label: "Buttons", Detail: "Set up your Stream Deck keys", Icon: "deck"}, {ID: "wifi", Label: "Wi-Fi", Detail: "Connect to your network", Icon: "wifi"},
			{ID: "govee", Label: "Lights", Detail: "Your Govee API key", Icon: "light"}, {ID: "spotify", Label: "Music", Detail: "Spotify account and speaker", Icon: "music"},
			{ID: "backup", Label: "Setup file", Detail: "Save or load settings on a USB stick", Icon: "usb"}, {ID: "system", Label: "System", Detail: "Restart, power off, version", Icon: "gear"}}})
	time.Sleep(100 * time.Millisecond)
	shot("home")

	on := true
	cells := make([]ui.Cell, 15)
	for i := range cells {
		cells[i].Empty = true
	}
	cells[0] = ui.Cell{Button: config.Button{Label: "Movie night", Color: "#3a1d6e"}}
	cells[1] = ui.Cell{Button: config.Button{Label: "Party", Color: "#c2185b"}}
	cells[2] = ui.Cell{Button: config.Button{Label: "Lights", Color: "#1d5fa0", Actions: []config.Action{{Type: config.Page, Page: "Lights"}}}}
	cells[5] = ui.Cell{Button: config.Button{Label: "Dinner Jazz", Color: "#1db954", TextColor: "#000000"}}
	cells[6] = ui.Cell{Button: config.Button{Label: "Pause", Color: "#202020", Actions: []config.Action{{Type: config.GoveePower, On: &on}}}, Warn: true}
	g.Grid(ui.GridView{Page: "home", Cells: cells, Selected: 2, KeyTitle: "Key 3: Lights", Details: []string{"Opens folder Lights"},
		DeckLive: true, Buttons: []string{"Open folder", "Edit key", "Clear key", "Folders", "Deck settings"}})
	time.Sleep(100 * time.Millisecond)
	shot("editor")

	blocking("menu", func() {
		g.Menu(ui.MenuOpts{ID: "key", Title: "Key 1", Items: []ui.Item{
			{Label: "Appearance", Header: true}, {Label: "Label", Value: "Movie night", Icon: "key"},
			{Label: "Key colour", Value: "Purple", Swatch: "#6a2fb0", Icon: "light"}, {Label: "Text colour", Value: "White", Swatch: "#ffffff", Icon: "light"},
			{Label: "What it does, in order", Header: true}, {Label: "1. TV Backlight: scene “Movie”", Icon: "light"},
			{Label: "2. Play “Dinner Jazz” on Living Room", Icon: "music"}, {Label: "+ Add an action", Icon: "plus"}}, Selected: 1})
	})
	blocking("input", func() {
		g.Input(ui.InputOpts{ID: "wifi-pass", Title: "WhIOT", Label: "Wi-Fi password", Initial: "hunter22", Secret: true,
			Intro: []string{"Keyboard layout: Dvorak. Click Show to check what you typed."}})
	})
	blocking("number", func() {
		g.Number(ui.NumberOpts{ID: "volume", Title: "Volume", Label: "Volume", Value: 60, Min: 0, Max: 100, Step: 5, Unit: "%"})
	})
	blocking("confirm", func() {
		g.Confirm(ui.ConfirmOpts{ID: "install-confirm", Title: "Erase nvme0n1?", Question: "EVERYTHING on nvme0n1 (256 GB, internal) will be erased, Windows included.", Yes: "Erase and install", No: "Cancel", Danger: true})
	})
	g.Info(ui.InfoView{ID: "spotify-phone", Title: "Connect Spotify", Spinner: true, Lines: []string{"On your phone or a computer on the same Wi-Fi, open this address and follow the two steps there:"},
		Big: "http://192.168.1.42:8080", Buttons: []string{"Type the code", "Cancel"}})
	time.Sleep(100 * time.Millisecond)
	shot("spotify")
	blocking("message", func() {
		g.Message(ui.Note{ID: "govee-ok", Kind: ui.SuccessNote, Title: "Govee works", Lines: []string{"Found 2 light(s):", "• TV Backlight (H6199)", "• Floor Lamp (H6072)"}})
	})

	// The pointer: moving it over a key and clicking selects that key.
	g.Grid(ui.GridView{Page: "home", Cells: cells, Selected: 0, KeyTitle: "Key 1"})
	var r image.Rectangle
	for _, h := range g.hits {
		if h.target == "cell:6" {
			r = h.r
		}
	}
	for len(g.events) > 0 {
		<-g.events
	}
	g.mu.Lock()
	g.cx, g.cy = 0, 0
	g.mu.Unlock()
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	for x, y := 0, 0; x < cx || y < cy; {
		dx, dy := min(4, cx-x), min(4, cy-y)
		mice <- fb.MouseEvent{DX: dx, DY: dy}
		x, y = x+dx, y+dy
	}
	mice <- fb.MouseEvent{Buttons: 1}
	mice <- fb.MouseEvent{}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-g.events:
			if ev.Kind == ui.ClickEvent {
				if ev.Target != "cell:6" {
					t.Fatalf("clicked %q, want cell:6", ev.Target)
				}
				return
			}
		case <-deadline:
			t.Fatal("no click event")
		}
	}
}
