package setup

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"streamdeckd/internal/govee"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/ui"
)

// BackupScreen saves the settings to a setup file on a USB stick, or loads one.
func (a *App) BackupScreen(ctx context.Context) {
	for {
		r := a.U.Menu(ui.MenuOpts{ID: "backup", Title: "Setup file",
			Subtitle: "A setup file (" + SeedFileName + ") on a USB stick holds your Wi-Fi, Govee key and Spotify login.",
			Intro:    []string{"Use it after reinstalling so nothing needs typing. It's plain text: keep that stick safe."},
			Items: []ui.Item{
				{Label: "Load settings from a USB stick", Icon: "usb"},
				{Label: "Save settings to a USB stick", Icon: "disk"},
			}})
		if !r.OK {
			return
		}
		if r.Index == 0 {
			a.U.Info(ui.InfoView{ID: "seed-looking", Title: "Setup file", Spinner: true, Lines: []string{"Looking at USB sticks…"}})
			var sd *Seed
			var where string
			if a.SeedFinder != nil {
				sd, where, _ = a.SeedFinder()
			}
			if sd == nil {
				a.trace("seed-none")
				a.U.Message(ui.Note{ID: "seed-none", Kind: ui.WarningNote, Title: "No setup file found",
					Lines: []string{"Plug in a USB stick with " + SeedFileName + " on it and try again.",
						"There's an example on the StreamDeck OS download page."}})
				continue
			}
			a.ImportSeed(ctx, sd, where)
			continue
		}
		a.ExportSeed()
	}
}

// ExportSeed writes the setup file to a USB stick the user picks.
func (a *App) ExportSeed() {
	seed := SeedFromCurrent(a.SecretsPath)
	mediaDisk := ""
	if b, err := os.ReadFile("/run/streamdeck-media"); err == nil {
		dev, _, _ := strings.Cut(strings.TrimSpace(string(b)), ":")
		mediaDisk = mediaDiskOf(dev)
	}
	var parts []usbPart
	for _, p := range usbParts() {
		switch {
		case p.DevRO, p.Mounted != "" && p.ReadOnly:
			continue // can't write there
		case mediaDisk != "" && p.Disk == mediaDisk && p.Mounted == "":
			continue // the installer stick's own (read-only) partitions
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		a.trace("export-no-stick")
		a.U.Message(ui.Note{ID: "export-no-stick", Kind: ui.WarningNote, Title: "No USB stick found",
			Lines: []string{"Plug in a USB stick (FAT32 or exFAT) and try again."}})
		return
	}
	items := make([]ui.Item, len(parts))
	for i, p := range parts {
		items[i] = ui.Item{Label: p.Dev, Detail: p.Desc, Icon: "usb"}
	}
	r := a.U.Menu(ui.MenuOpts{ID: "export-stick", Title: "Save to which USB stick?", Items: items})
	if !r.OK {
		return
	}
	p := parts[r.Index]
	err := withMount(p, true, func(dir string) error {
		target := filepath.Join(dir, SeedFileName)
		if old := seedFileIn(dir); old != "" {
			target = old
		}
		return os.WriteFile(target, []byte(seed.Format()), 0o600)
	})
	if err != nil {
		a.trace("export-error %v", err)
		a.U.Message(ui.Note{ID: "export-error", Kind: ui.ErrorNote, Title: "Couldn't save", Lines: []string{"Couldn't write to " + p.Dev + ": " + err.Error()}})
		return
	}
	a.trace("exported %s", p.Dev)
	a.U.Message(ui.Note{ID: "exported", Kind: ui.SuccessNote, Title: "Saved", Lines: []string{"Saved " + SeedFileName + " to " + p.Dev + "."}})
}

// ImportSeed applies a setup file: Wi-Fi first (the rest needs the
// internet), then the Govee key and Spotify, each checked before saving.
func (a *App) ImportSeed(ctx context.Context, sd *Seed, where string) {
	if !a.U.Confirm(ui.ConfirmOpts{ID: "seed-import", Title: "Use the setup file?",
		Question: "Found on " + where + ": " + strings.Join(sd.Summary(), ", ") + ". Set these up now?", Yes: "Set up", No: "Not now"}) {
		return
	}
	a.trace("seed-used")
	var report []string
	if sd.HasWifi() {
		if name, ok := wifiSummary(); ok && name == sd.WifiName {
			report = append(report, "Wi-Fi: already connected to "+sd.WifiName)
		} else {
			report = append(report, a.seedWifi(sd))
		}
	}
	online := LocalIPv4() != ""
	if online {
		for i := 0; i < 60 && time.Now().Year() < 2024; i++ {
			if i == 0 {
				a.U.Info(ui.InfoView{ID: "wait-clock", Title: "Setup file", Spinner: true, Lines: []string{"Setting the clock over the internet…"}})
			}
			time.Sleep(time.Second)
		}
	}
	s := a.secrets()
	if sd.GoveeKey != "" {
		a.U.Info(ui.InfoView{ID: "seed-govee", Title: "Setup file", Spinner: true, Lines: []string{"Checking the Govee key…"}})
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		devs, err := govee.New(sd.GoveeKey).Devices(cctx)
		cancel()
		devs = govee.Lights(devs)
		if err == nil {
			s.Govee.APIKey = sd.GoveeKey
			a.trace("govee-ok %d", len(devs))
			report = append(report, "Govee: key works, "+plural(len(devs), "light"))
		} else {
			a.trace("govee-error %v", err)
			report = append(report, "Govee: the key didn't work ("+err.Error()+")")
		}
	}
	if sd.HasSpotify() {
		s.Spotify.ClientID, s.Spotify.ClientSecret = sd.SpotifyID, sd.SpotifySecret
		if sd.SpotifySpeaker != "" {
			s.Spotify.DefaultDevice = sd.SpotifySpeaker
		}
		if sd.SpotifyRefresh != "" {
			a.U.Info(ui.InfoView{ID: "seed-spotify", Title: "Setup file", Spinner: true, Lines: []string{"Checking the Spotify login…"}})
			cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			_, err := spotify.New(sd.SpotifyID, sd.SpotifySecret, sd.SpotifyRefresh).Devices(cctx)
			cancel()
			if err == nil {
				s.Spotify.RefreshToken = sd.SpotifyRefresh
				a.trace("spotify-ok seeded")
				report = append(report, "Spotify: logged in")
			} else {
				a.trace("spotify-error %v", err)
				s.Spotify.RefreshToken = ""
				report = append(report, "Spotify: the saved login didn't work; log in again under Music")
			}
		} else {
			s.Spotify.RefreshToken = ""
			report = append(report, "Spotify: app keys saved; log in once under Music")
		}
		if sd.SpotifySpeaker != "" {
			report = append(report, "Default speaker: "+sd.SpotifySpeaker)
		}
	}
	if !a.saveSecrets(s) {
		return
	}
	a.trace("seed-applied")
	a.seedMu.Lock()
	a.seed = nil // don't keep offering it
	a.seedMu.Unlock()
	report = append(report, "", "Reminder: the setup file holds your keys in plain text. Keep that USB stick safe.")
	a.U.Message(ui.Note{ID: "seed-done", Kind: ui.SuccessNote, Title: "Setup file used", Lines: report})
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
