package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/deck"
	"streamdeckd/internal/render"
	"streamdeckd/internal/runner"
)

type daemon struct {
	cfgPath, secretsPath string

	mu          sync.Mutex
	cfg         *config.Config
	placeholder bool // true while waiting for the installer / a valid config
	lastErr     string
	run         *runner.Runner
	dev         *deck.Device
	page        string
	busy        map[int]bool
	dimmed      bool
	lastUsed    time.Time
}

// setupConfig is shown on the deck until the installer has run.
func setupConfig() *config.Config {
	return &config.Config{
		Brightness: 60, IdleBrightness: 10, StartPage: "setup",
		Pages: map[string][]config.Button{"setup": {
			{Key: 7, Label: "Finish setup on screen", Color: "#b05a00"},
		}},
	}
}

func runDaemon(cfgPath, secretsPath string) error {
	log.Printf("streamdeckd %s starting, layout %s, secrets %s", version, cfgPath, secretsPath)
	d := &daemon{cfgPath: cfgPath, secretsPath: secretsPath, busy: map[int]bool{}}
	d.reload()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			d.reload()
		}
	}()
	go d.watch(ctx)

	notFoundLogged := false
	for ctx.Err() == nil {
		dev, err := deck.Open()
		if err != nil {
			if !notFoundLogged {
				log.Printf("waiting for Stream Deck MK.2: %v", err)
				notFoundLogged = true
			}
			sleepCtx(ctx, 2*time.Second)
			continue
		}
		notFoundLogged = false
		log.Printf("found %s at %s", dev.Model.Name, dev.Path)
		err = d.serve(ctx, dev)
		dev.Close()
		if ctx.Err() == nil {
			log.Printf("deck disconnected: %v", err)
			sleepCtx(ctx, time.Second)
		}
	}
	log.Printf("shutting down")
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

// fileSig changes whenever either file is modified, created or removed.
func (d *daemon) fileSig() string {
	sig := ""
	for _, p := range []string{d.cfgPath, d.secretsPath} {
		if st, err := os.Stat(p); err == nil {
			sig += fmt.Sprintf("%d/%d;", st.ModTime().UnixNano(), st.Size())
		} else {
			sig += "-;"
		}
	}
	return sig
}

// watch reloads when the layout or secrets change (e.g. the installer finished).
func (d *daemon) watch(ctx context.Context) {
	last := d.fileSig()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if sig := d.fileSig(); sig != last {
				last = sig
				d.reload()
			}
		}
	}
}

func (d *daemon) reload() {
	cfg, err := config.LoadAll(d.cfgPath, d.secretsPath)
	d.mu.Lock()
	if err != nil {
		if msg := err.Error(); msg != d.lastErr {
			log.Printf("config not usable yet:\n%v", err)
			d.lastErr = msg
		}
		if d.cfg != nil && !d.placeholder {
			log.Printf("keeping the previous working config")
			d.mu.Unlock()
			return
		}
		cfg = setupConfig()
		d.placeholder = true
	} else {
		if d.placeholder || d.cfg == nil {
			log.Printf("config loaded: %d page(s)", len(cfg.Pages))
		} else {
			log.Printf("config reloaded")
		}
		d.placeholder = false
		d.lastErr = ""
	}
	d.cfg = cfg
	d.run = runner.New(cfg)
	if _, ok := cfg.Pages[d.page]; !ok {
		d.page = cfg.StartPage
	}
	dev := d.dev
	d.mu.Unlock()
	if dev != nil {
		_ = dev.SetBrightness(cfg.Brightness)
		d.renderPage()
	}
}

func (d *daemon) serve(ctx context.Context, dev *deck.Device) error {
	d.mu.Lock()
	d.dev = dev
	d.dimmed = false
	d.lastUsed = time.Now()
	cfg := d.cfg
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.dev = nil
		d.mu.Unlock()
	}()

	if err := dev.Reset(); err != nil {
		log.Printf("reset: %v", err)
	}
	if err := dev.SetBrightness(cfg.Brightness); err != nil {
		return fmt.Errorf("set brightness: %w", err)
	}
	for _, name := range cfg.PageNames() {
		for _, b := range cfg.Pages[name] {
			if b.Key >= dev.Model.Keys {
				log.Printf("warning: page %q key %d does not exist on a %s (keys 0-%d)", name, b.Key, dev.Model.Name, dev.Model.Keys-1)
			}
		}
	}
	d.renderPage()

	// Closing the device unblocks ReadKeys on shutdown.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = dev.Reset()
			dev.Close()
		case <-done:
		}
	}()
	go d.idleLoop(done, dev)

	prev := make([]bool, dev.Model.Keys)
	for {
		states, err := dev.ReadKeys()
		if err != nil {
			return err
		}
		for k, down := range states {
			if down && !prev[k] {
				d.press(ctx, k)
			}
		}
		prev = states
	}
}

func (d *daemon) idleLoop(done <-chan struct{}, dev *deck.Device) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			d.mu.Lock()
			idle := d.cfg.IdleDimSeconds
			if idle > 0 && !d.dimmed && time.Since(d.lastUsed) > time.Duration(idle)*time.Second {
				d.dimmed = true
				_ = dev.SetBrightness(d.cfg.IdleBrightness)
			}
			d.mu.Unlock()
		}
	}
}

func (d *daemon) press(ctx context.Context, key int) {
	d.mu.Lock()
	d.lastUsed = time.Now()
	if d.dimmed {
		// First press only wakes the deck.
		d.dimmed = false
		if d.dev != nil {
			_ = d.dev.SetBrightness(d.cfg.Brightness)
		}
		d.mu.Unlock()
		return
	}
	page := d.page
	btn, ok := d.cfg.Button(page, key)
	if !ok || d.busy[key] || len(btn.Actions) == 0 {
		d.mu.Unlock()
		return
	}
	run := d.run
	d.busy[key] = true
	d.mu.Unlock()

	log.Printf("key %d on %q: %q", key, page, btn.Label)
	go func() {
		defer func() {
			d.mu.Lock()
			delete(d.busy, key)
			d.mu.Unlock()
		}()
		onlyPage := true
		for _, a := range btn.Actions {
			if a.Type != config.Page {
				onlyPage = false
			}
		}
		if !onlyPage {
			if time.Now().Year() < 2024 {
				log.Printf("clock not set yet (waiting for network time); TLS would fail")
				d.flash(page, key, btn, render.Failed, 2*time.Second)
				return
			}
			d.drawKey(page, key, btn, render.Busy)
		}
		res := run.Run(ctx, btn.Actions)
		if res.Err != nil {
			log.Printf("key %d %q failed:\n%v", key, btn.Label, res.Err)
		} else if !onlyPage {
			log.Printf("key %d %q done", key, btn.Label)
		}
		if res.SwitchTo != "" {
			d.mu.Lock()
			d.page = res.SwitchTo
			d.mu.Unlock()
			d.renderPage()
			return
		}
		if res.Err != nil {
			d.flash(page, key, btn, render.Failed, 2*time.Second)
		} else {
			d.flash(page, key, btn, render.OK, 500*time.Millisecond)
		}
	}()
}

func (d *daemon) flash(page string, key int, btn config.Button, st render.State, dur time.Duration) {
	d.drawKey(page, key, btn, st)
	time.Sleep(dur)
	d.drawKey(page, key, btn, render.Normal)
}

// drawKey draws a key if page is still showing.
func (d *daemon) drawKey(page string, key int, btn config.Button, st render.State) {
	d.mu.Lock()
	dev, cur, dir := d.dev, d.page, d.cfg.Dir
	d.mu.Unlock()
	if dev == nil || cur != page || key >= dev.Model.Keys {
		return
	}
	m := dev.Model
	img, err := render.Key(m.Pixels, btn, dir, st, m.FlipH, m.FlipV)
	if err != nil {
		log.Printf("render key %d: %v", key, err)
		return
	}
	if err := dev.SetKeyImage(key, img); err != nil && !errors.Is(err, os.ErrClosed) {
		log.Printf("draw key %d: %v", key, err)
	}
}

func (d *daemon) renderPage() {
	d.mu.Lock()
	dev, page, cfg := d.dev, d.page, d.cfg
	d.mu.Unlock()
	if dev == nil {
		return
	}
	blank := render.Blank(dev.Model.Pixels)
	for k := 0; k < dev.Model.Keys; k++ {
		if b, ok := cfg.Button(page, k); ok {
			d.drawKey(page, k, b, render.Normal)
		} else {
			_ = dev.SetKeyImage(k, blank)
		}
	}
}
