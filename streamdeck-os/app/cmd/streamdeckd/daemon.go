package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/deck"
	"streamdeckd/internal/deckipc"
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
	edit        *editSession // set while the on-screen editor is open
}

// editSession is the on-screen button editor connected over deckipc.
type editSession struct {
	conn   net.Conn
	wmu    sync.Mutex
	layout config.Layout
	page   string
	sel    int
}

func (e *editSession) send(m deckipc.Msg) {
	e.wmu.Lock()
	defer e.wmu.Unlock()
	_ = e.conn.SetWriteDeadline(time.Now().Add(time.Second))
	_ = json.NewEncoder(e.conn).Encode(m)
}

// serveEditor accepts editor connections (one at a time; a new one wins).
func (d *daemon) serveEditor(ctx context.Context) {
	_ = os.Remove(deckipc.Socket)
	ln, err := net.Listen("unix", deckipc.Socket)
	if err != nil {
		log.Printf("editor socket: %v", err)
		return
	}
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go d.editorConn(conn)
	}
}

func (d *daemon) editorConn(conn net.Conn) {
	sess := &editSession{conn: conn, sel: -1}
	d.mu.Lock()
	if old := d.edit; old != nil {
		old.conn.Close()
	}
	d.edit = sess
	d.dimmed = false
	if d.dev != nil {
		_ = d.dev.SetBrightness(max(d.cfg.Brightness, 30))
	}
	d.mu.Unlock()
	log.Printf("button editor connected")
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var m deckipc.Msg
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.Op != "preview" || m.Layout == nil {
			continue
		}
		d.mu.Lock()
		if d.edit != sess {
			d.mu.Unlock()
			break
		}
		sess.layout, sess.page, sess.sel = *m.Layout, m.Page, m.Select
		d.mu.Unlock()
		d.renderPage()
	}
	conn.Close()
	d.mu.Lock()
	if d.edit == sess {
		d.edit = nil
	}
	d.mu.Unlock()
	log.Printf("button editor closed")
	d.renderPage()
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
	go d.serveEditor(ctx)

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
			if idle > 0 && !d.dimmed && d.edit == nil && time.Since(d.lastUsed) > time.Duration(idle)*time.Second {
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
	if e := d.edit; e != nil {
		// Editing: a press selects the key in the editor instead of running it.
		d.mu.Unlock()
		e.send(deckipc.Msg{Op: "press", Key: key})
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
	dev, page, cfg, edit, placeholder := d.dev, d.page, d.cfg, d.edit, d.placeholder
	var sess editSession
	if edit != nil {
		sess = editSession{layout: edit.layout, page: edit.page, sel: edit.sel}
	}
	d.mu.Unlock()
	if dev == nil {
		return
	}
	blank := render.Blank(dev.Model.Pixels)

	if edit != nil {
		// Live preview of the editor's (possibly unsaved) layout.
		for k := 0; k < dev.Model.Keys; k++ {
			b, ok := layoutButton(sess.layout, sess.page, k)
			switch {
			case k == sess.sel && !ok:
				d.drawRaw(dev, k, config.Button{Color: "#000000"}, cfg.Dir, render.Busy)
			case k == sess.sel:
				d.drawRaw(dev, k, b, cfg.Dir, render.Busy)
			case ok:
				d.drawRaw(dev, k, b, cfg.Dir, render.Normal)
			default:
				_ = dev.SetKeyImage(k, blank)
			}
		}
		return
	}

	empty := len(cfg.Pages[page]) == 0
	for k := 0; k < dev.Model.Keys; k++ {
		if b, ok := cfg.Button(page, k); ok {
			d.drawKey(page, k, b, render.Normal)
		} else if empty && !placeholder && k == 7 {
			d.drawRaw(dev, k, config.Button{Label: "Edit buttons on the PC screen", Color: "#1d5fa0"}, cfg.Dir, render.Normal)
		} else {
			_ = dev.SetKeyImage(k, blank)
		}
	}
}

func layoutButton(l config.Layout, page string, key int) (config.Button, bool) {
	for _, b := range l.Pages[page] {
		if b.Key == key {
			return b, true
		}
	}
	return config.Button{}, false
}

// drawRaw draws a key unconditionally (used for previews and hints).
func (d *daemon) drawRaw(dev *deck.Device, key int, btn config.Button, dir string, st render.State) {
	m := dev.Model
	img, err := render.Key(m.Pixels, btn, dir, st, m.FlipH, m.FlipV)
	if err != nil {
		return
	}
	if err := dev.SetKeyImage(key, img); err != nil && !errors.Is(err, os.ErrClosed) {
		log.Printf("draw key %d: %v", key, err)
	}
}
