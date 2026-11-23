package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"streamdeckd/internal/deck"
	"streamdeckd/internal/fb"
	"streamdeckd/internal/install"
	"streamdeckd/internal/setup"
	"streamdeckd/internal/tui"
	"streamdeckd/internal/ui"
	"streamdeckd/internal/ui/gfx"
	"streamdeckd/internal/ui/text"
)

// runConsole owns the monitor, keyboard and touchpad (tty1). From the USB
// stick it is the installer; on the installed system it is the home screen
// with the settings and the button editor.
func runConsole(cfgPath, secretsPath string) error {
	var trace io.Writer
	if p := os.Getenv("STREAMDECKD_TRACE"); p != "" {
		if f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644); err == nil {
			trace = f
		}
	}
	// One reader owns the console input; the terminal parses it into keys
	// (the console's keymap, Dvorak, applies).
	in := make(chan []byte, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				in <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				close(in)
				return
			}
		}
	}()
	term, err := tui.Open(int(os.Stdin.Fd()), os.Stdout, in)
	if err != nil {
		return err
	}
	defer term.Close()

	tracer := ui.Tracer{W: trace}
	var u ui.UI
	if dev, err := fb.Open("/dev/fb0"); err == nil && os.Getenv("STREAMDECKD_TEXT_UI") == "" {
		restore, _ := fb.GraphicsMode(os.Stdin.Fd())
		defer restore()
		mice, err := fb.Mice("/dev/input/mice")
		if err != nil {
			mice = nil
		}
		u = gfx.New(dev, term.Keys(), mice, tracer)
		tracer.Printf("renderer gfx %dx%d", dev.W, dev.H)
	} else {
		u = text.New(term, tracer)
		tracer.Printf("renderer text")
	}

	app := &setup.App{
		U: u, Trace: trace,
		SecretsPath: secretsPath, LayoutPath: cfgPath, LayoutDir: filepath.Dir(cfgPath),
		WebAddr: ":8080", Version: version,
		Reboot: func() error {
			syscall.Sync()
			return exec.Command("reboot").Run()
		},
		PowerOff: func() error {
			syscall.Sync()
			return exec.Command("poweroff").Run()
		},
		DeckName:      deck.Present,
		SeedFinder:    setup.FindSeedOnUSB,
		FindInstalled: install.FindInstalled,
	}
	ctx := context.Background()
	if isLive() {
		app.Live = true
		app.PayloadDir = "/media/live/streamdeck/boot"
		return app.Installer(ctx)
	}
	app.EditButtons = func(ctx context.Context) { runEditor(ctx, u, app, cfgPath, secretsPath, trace) }
	return app.Home(ctx)
}

func isLive() bool {
	b, _ := os.ReadFile("/proc/cmdline")
	return strings.Contains(string(b), "streamdeck.live")
}
