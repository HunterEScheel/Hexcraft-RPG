// Package setup is StreamDeck OS's on-screen app: the installer on the USB
// stick, and on the installed system the home dashboard with Wi-Fi, Govee,
// Spotify, setup-file and system settings, and the way into the button
// editor.
package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/ui"
	"streamdeckd/internal/wifi"
)

// App is the on-screen app.
type App struct {
	U     ui.UI
	Trace io.Writer // "APP: ..." markers for the VM tests

	SecretsPath string
	LayoutPath  string
	// WebAddr serves the phone page used for the Spotify login.
	WebAddr string

	// Live: running from the USB stick (installer only).
	Live       bool
	PayloadDir string // kernel/initramfs/boot loader on the live media
	LayoutDir  string // starter layout copied by the installer
	BootDisk   string // disk the live system booted from (never offered)

	Version  string
	Reboot   func() error
	PowerOff func() error
	// EditButtons opens the button editor.
	EditButtons func(ctx context.Context)
	// DeckName says which Stream Deck is plugged in ("" if none).
	DeckName func() string
	// SeedFinder looks for a setup file (streamdeck-setup.txt) on USB sticks.
	SeedFinder func() (*Seed, string, error)
	// FindInstalled returns an existing install's partition ("" if none).
	FindInstalled func() string

	seedMu    sync.Mutex
	seed      *Seed
	seedWhere string
	seedSeen  string // USB sticks present at the last look
}

func (a *App) trace(format string, a2 ...any) {
	if a.Trace != nil {
		fmt.Fprintf(a.Trace, "APP: "+format+"\n", a2...)
	}
}

func (a *App) secrets() *config.Secrets {
	s, err := config.LoadSecrets(a.SecretsPath)
	if err != nil || s == nil {
		return &config.Secrets{}
	}
	return s
}

func (a *App) saveSecrets(s *config.Secrets) bool {
	if err := config.SaveSecrets(a.SecretsPath, s); err != nil {
		a.U.Message(ui.Note{ID: "save-error", Kind: ui.ErrorNote, Title: "Couldn't save", Lines: []string{err.Error()}})
		return false
	}
	a.trace("settings-saved")
	return true
}

var errCancelled = errors.New("cancelled")

// mask shows just enough of a key to recognise it.
func mask(s string) string {
	switch {
	case s == "":
		return "not set"
	case len(s) <= 8:
		return strings.Repeat("•", len(s))
	default:
		return s[:4] + strings.Repeat("•", 4) + s[len(s)-4:]
	}
}

// LocalIPv4 returns the first non-loopback IPv4 address.
func LocalIPv4() string {
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				return n.IP.String()
			}
		}
	}
	return ""
}

// ---- home dashboard ----

type status struct {
	wifiOK         bool
	wifi           string
	deck           string
	govee, spotify bool
	lan            int // lights on the network
	speaker        string
	buttons        int
	layoutErr      string
}

func (a *App) status() status {
	var st status
	s := a.secrets()
	st.govee = s.Govee.APIKey != ""
	st.spotify = s.Spotify.RefreshToken != "" && s.Spotify.ClientID != ""
	st.speaker = s.Spotify.DefaultDevice
	st.wifi, st.wifiOK = wifiSummary()
	if a.DeckName != nil {
		st.deck = a.DeckName()
	}
	if l, err := config.ReadLayout(a.LayoutPath); err == nil {
		for _, b := range l.Pages {
			st.buttons += len(b)
		}
		st.lan = len(l.LANLights)
	} else if !os.IsNotExist(err) {
		st.layoutErr = err.Error()
	}
	return st
}

// wifiSummary is a short Wi-Fi state for the dashboard.
func wifiSummary() (string, bool) {
	ads := wifi.Adapters()
	if len(ads) == 0 {
		return "No adapter", false
	}
	st := wifi.Ctl{Iface: ads[0].Name}.Status()
	if st["wpa_state"] == "COMPLETED" && wifi.IfaceIPv4(ads[0].Name) != "" {
		return st["ssid"], true
	}
	if ssid, _ := wifi.Config(); ssid != "" {
		return "Connecting to " + ssid, false
	}
	return "Not connected", false
}

func (a *App) homeView(st status, sel int) ui.HomeView {
	card := func(icon, title, val string, ok bool, warn bool) ui.StatusCard {
		c := ui.StatusCard{Icon: icon, Title: title, Value: val}
		switch {
		case ok:
			c.State = ui.CardOK
		case warn:
			c.State = ui.CardWarn
		}
		return c
	}
	deck := st.deck
	if deck == "" {
		deck = "Not plugged in"
	}
	gv := "Not set up"
	switch {
	case st.govee && st.lan > 0:
		gv = fmt.Sprintf("Cloud + %d on network", st.lan)
	case st.govee:
		gv = "Ready"
	case st.lan > 0:
		gv = fmt.Sprintf("%d on network", st.lan)
	}
	sp := "Not connected"
	if st.spotify {
		sp = "Connected"
		if st.speaker != "" {
			sp = "Plays on " + st.speaker
		}
	}
	v := ui.HomeView{
		Title:    "StreamDeck OS",
		Subtitle: time.Now().Format("Monday, January 2"),
		Cards: []ui.StatusCard{
			card("wifi", "Wi-Fi", st.wifi, st.wifiOK, true),
			card("deck", "Stream Deck", deck, st.deck != "", true),
			card("light", "Lights", gv, st.govee || st.lan > 0, false),
			card("music", "Music", sp, st.spotify, false),
		},
		Tiles: []ui.Tile{
			{ID: "buttons", Label: "Buttons", Detail: buttonsDetail(st), Icon: "deck"},
			{ID: "wifi", Label: "Wi-Fi", Detail: "Connect to your network", Icon: "wifi"},
			{ID: "govee", Label: "Lights", Detail: "Govee lights on your network, and the API key", Icon: "light"},
			{ID: "spotify", Label: "Music", Detail: "Spotify account and speaker", Icon: "music"},
			{ID: "backup", Label: "Setup file", Detail: "Save or load settings on a USB stick", Icon: "usb"},
			{ID: "system", Label: "System", Detail: "Restart, power off, version", Icon: "gear"},
		},
		Selected: sel,
	}
	steps := []ui.Step{
		{Label: "Connect Wi-Fi", Done: st.wifiOK},
		{Label: "Set up your lights", Done: st.govee || st.lan > 0},
		{Label: "Connect Spotify", Done: st.spotify},
		{Label: "Set up buttons", Done: st.buttons > 0},
	}
	for _, s := range steps {
		if !s.Done {
			v.Steps = steps
			break
		}
	}
	a.seedMu.Lock()
	if a.seed != nil {
		v.Banner = "Setup file found on a USB stick (" + strings.Join(a.seed.Summary(), ", ") + ")"
		v.BannerOK = "Use it"
	}
	a.seedMu.Unlock()
	return v
}

func buttonsDetail(st status) string {
	switch {
	case st.layoutErr != "":
		return "Layout problem: open to fix"
	case st.buttons == 0:
		return "Set up your Stream Deck keys"
	case st.buttons == 1:
		return "1 key set up · edit"
	}
	return fmt.Sprintf("%d keys set up · edit", st.buttons)
}

// lookForSeed checks USB sticks for a setup file when the set of sticks changes.
func (a *App) lookForSeed() {
	if a.SeedFinder == nil {
		return
	}
	var devs []string
	for _, p := range usbParts() {
		devs = append(devs, p.Dev)
	}
	seen := strings.Join(devs, ",")
	a.seedMu.Lock()
	if seen == a.seedSeen && a.seedSeen != "\x00" {
		a.seedMu.Unlock()
		return
	}
	a.seedSeen = seen
	a.seedMu.Unlock()
	sd, where, _ := a.SeedFinder()
	a.seedMu.Lock()
	a.seed, a.seedWhere = sd, where
	a.seedMu.Unlock()
	if sd != nil {
		a.trace("seed-found %s", where)
	}
}

// Home runs the dashboard of the installed system.
func (a *App) Home(ctx context.Context) error {
	sel := 0
	a.seedSeen = "\x00"
	go a.lookForSeed()
	st := a.status()
	last := time.Now()
	ids := []string{"buttons", "wifi", "govee", "spotify", "backup", "system"}
	a.trace("home")
	for {
		a.U.Home(a.homeView(st, sel))
		redraw := false
		for !redraw {
			ev, ok := <-a.U.Events()
			if !ok {
				return nil
			}
			open := ""
			switch ev.Kind {
			case ui.TickEvent:
				if time.Since(last) > 3*time.Second {
					st, last, redraw = a.status(), time.Now(), true
					go a.lookForSeed()
				}
			case ui.HoverEvent:
				for i, id := range ids {
					if ev.Target == "tile:"+id && sel != i {
						sel, redraw = i, true
					}
				}
			case ui.ClickEvent:
				if id, ok := strings.CutPrefix(ev.Target, "tile:"); ok {
					open = id
				} else if ev.Target == "banner" {
					open = "seed"
				}
			case ui.KeyEvent:
				k := ev.Key
				redraw = true
				switch {
				case k.Name == "left":
					sel = (sel + len(ids) - 1) % len(ids)
				case k.Name == "right" || k.Name == "tab":
					sel = (sel + 1) % len(ids)
				case k.Name == "up":
					sel = (sel + len(ids) - 3) % len(ids)
				case k.Name == "down":
					sel = (sel + 3) % len(ids)
				case k.Name == "enter" || k.R == ' ':
					open = ids[sel]
				case k.R == 'u' || k.R == 'U':
					open = "seed"
				default:
					redraw = false
				}
			}
			if open != "" {
				a.openTile(ctx, open)
				st, last, redraw = a.status(), time.Now(), true
			}
		}
	}
}

func (a *App) openTile(ctx context.Context, id string) {
	a.trace("open %s", id)
	switch id {
	case "buttons":
		if a.EditButtons != nil {
			a.EditButtons(ctx)
		}
	case "wifi":
		a.WifiScreen(ctx)
	case "govee":
		a.GoveeScreen(ctx)
	case "spotify":
		a.SpotifyScreen(ctx)
	case "backup":
		a.BackupScreen(ctx)
	case "system":
		a.SystemScreen(ctx)
	case "seed":
		a.seedMu.Lock()
		sd, where := a.seed, a.seedWhere
		a.seedMu.Unlock()
		if sd != nil {
			a.ImportSeed(ctx, sd, where)
		}
	}
}

// SystemScreen: version, restart, power off.
func (a *App) SystemScreen(ctx context.Context) {
	for {
		ip := LocalIPv4()
		if ip == "" {
			ip = "none"
		}
		r := a.U.Menu(ui.MenuOpts{ID: "system", Title: "System", Items: []ui.Item{
			{Label: "Version", Value: a.Version, Icon: "info"},
			{Label: "Network address", Value: ip, Icon: "wifi"},
			{Label: "", Header: true},
			{Label: "Restart", Icon: "wait"},
			{Label: "Power off", Icon: "pause", Danger: true},
		}})
		if !r.OK {
			return
		}
		switch r.Index {
		case 3:
			if a.U.Confirm(ui.ConfirmOpts{ID: "restart", Title: "Restart", Question: "Restart StreamDeck OS now?", Yes: "Restart"}) && a.Reboot != nil {
				a.U.Info(ui.InfoView{ID: "restarting", Title: "Restarting…", Spinner: true})
				_ = a.Reboot()
			}
		case 4:
			if a.U.Confirm(ui.ConfirmOpts{ID: "poweroff", Title: "Power off", Question: "Turn this computer off? Press its power button to start it again.", Yes: "Power off", Danger: true}) && a.PowerOff != nil {
				a.U.Info(ui.InfoView{ID: "powering-off", Title: "Powering off…", Spinner: true})
				_ = a.PowerOff()
			}
		}
	}
}

// waitOnline waits (with a spinner) until there's a network address and a
// sane clock (needed for HTTPS); false if there's no network.
func (a *App) waitOnline(title string) bool {
	for i := 0; i < 20 && LocalIPv4() == ""; i++ {
		if i == 0 {
			a.U.Info(ui.InfoView{ID: "wait-network", Title: title, Spinner: true, Lines: []string{"Waiting for the network…"}})
		}
		time.Sleep(time.Second)
	}
	if LocalIPv4() == "" {
		a.U.Message(ui.Note{ID: "offline", Kind: ui.WarningNote, Title: "Not connected",
			Lines: []string{"Connect to Wi-Fi first (Home > Wi-Fi), then try again."}})
		return false
	}
	for i := 0; i < 60 && time.Now().Year() < 2024; i++ {
		if i == 0 {
			a.U.Info(ui.InfoView{ID: "wait-clock", Title: title, Spinner: true, Lines: []string{"Setting the clock over the internet…"}})
		}
		time.Sleep(time.Second)
	}
	return true
}
