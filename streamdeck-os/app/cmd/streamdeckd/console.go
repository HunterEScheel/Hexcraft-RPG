package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/deck"
	"streamdeckd/internal/install"
	"streamdeckd/internal/setup"
)

func configured(secretsPath string) bool {
	s, err := config.LoadSecrets(secretsPath)
	return err == nil && (s.Govee.APIKey != "" || s.Spotify.RefreshToken != "")
}

// runConsole owns the HDMI console (tty1): installer on first boot (or when
// forced), then a status screen from which the installer can be re-run.
func runConsole(cfgPath, secretsPath string, forceSetup bool) error {
	w := setup.New(os.Stdin, os.Stdout)
	w.SecretsPath = secretsPath
	w.LayoutDir = filepath.Dir(cfgPath)
	if p := os.Getenv("STREAMDECKD_TRACE"); p != "" {
		if f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644); err == nil {
			w.Trace = f
		}
	}
	if v := os.Getenv("STREAMDECKD_WIFI_RESTART"); v != "" {
		w.WifiRestart = strings.Fields(v)
	}
	live := isLive()
	if live {
		w.Live = true
		w.PayloadDir = "/media/live/streamdeck/boot"
		if dev, err := os.ReadFile("/run/streamdeck-media"); err == nil {
			w.BootDisk = install.DiskOf("/sys/block", strings.TrimSpace(string(dev)))
		}
		w.Reboot = func() error {
			syscall.Sync()
			return exec.Command("reboot").Run()
		}
	}
	ctx := context.Background()

	runSetup := func() {
		for {
			err := w.Run(ctx)
			if err == nil {
				return
			}
			fmt.Printf("\nInstaller stopped: %v\nPress Enter to start over.\n", err)
			if _, ok := <-w.Lines(); !ok {
				return
			}
		}
	}
	if forceSetup {
		runSetup()
		return nil
	}
	if !configured(secretsPath) {
		runSetup()
	}

	lines := w.Lines()
	for {
		printStatus(cfgPath, secretsPath, live)
		select {
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			switch strings.ToLower(line) {
			case "1", "settings":
				if err := w.Settings(ctx); err != nil {
					fmt.Printf("settings: %v\n", err)
				}
			case "2", "setup":
				runSetup()
			case "3", "install":
				if live {
					if err := w.InstallStep(); err != nil {
						fmt.Printf("install: %v\n", err)
					}
				}
			}
		case <-time.After(15 * time.Second):
		}
	}
}

func isLive() bool {
	b, _ := os.ReadFile("/proc/cmdline")
	return strings.Contains(string(b), "streamdeck.live")
}

func printStatus(cfgPath, secretsPath string, live bool) {
	fmt.Print("\033[H\033[2J")
	fmt.Println("StreamDeck OS")
	fmt.Println("=============")
	ip := setup.LocalIPv4()
	if ip == "" {
		ip = "not connected"
	}
	dk := deck.Present()
	if dk == "" {
		dk = "not connected (plug in a Stream Deck MK.2)"
	}
	if live {
		fmt.Println("Running from the USB stick (not installed).")
	}
	fmt.Printf("Network:     %s\n", ip)
	fmt.Printf("Stream Deck: %s\n", dk)

	s, _ := config.LoadSecrets(secretsPath)
	if s == nil {
		s = &config.Secrets{}
	}
	yes := func(b bool) string {
		if b {
			return "set up"
		}
		return "not set up"
	}
	fmt.Printf("Govee:       %s\n", yes(s.Govee.APIKey != ""))
	sp := yes(s.Spotify.RefreshToken != "")
	if s.Spotify.DefaultDevice != "" {
		sp += ", plays on " + s.Spotify.DefaultDevice
	}
	fmt.Printf("Spotify:     %s\n", sp)
	if _, err := config.LoadAll(cfgPath, secretsPath); err != nil {
		fmt.Printf("\nLayout problem in %s:\n%v\n", cfgPath, err)
	} else {
		fmt.Printf("Layout:      %s OK\n", cfgPath)
	}
	fmt.Printf("\n%s\n", time.Now().Format("Mon Jan 2 15:04"))
	fmt.Print("\n  1) Settings (change Govee/Spotify keys, speaker, Wi-Fi)\n")
	fmt.Print("  2) Run the full installer again\n")
	if live {
		fmt.Print("  3) Install to this computer's disk\n")
	}
	fmt.Print("\nChoose and press Enter: ")
}
