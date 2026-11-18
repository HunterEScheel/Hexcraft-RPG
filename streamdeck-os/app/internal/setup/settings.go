package setup

import (
	"context"
	"fmt"
	"strings"

	"streamdeckd/internal/config"
	"streamdeckd/internal/wifi"
)

// mask shows just enough of a key to recognise it.
func mask(s string) string {
	switch {
	case s == "":
		return "not set"
	case len(s) <= 8:
		return strings.Repeat("*", len(s))
	default:
		return s[:4] + strings.Repeat("*", 4) + s[len(s)-4:]
	}
}

// Settings is a menu for changing individual keys and settings without
// running the whole installer again. Every change is saved immediately;
// the daemon picks it up within a few seconds.
func (w *Wizard) Settings(ctx context.Context) error {
	for {
		s, _ := config.LoadSecrets(w.SecretsPath)
		if s == nil {
			s = &config.Secrets{}
		}
		w.say("\033[H\033[2J")
		w.say("StreamDeck OS - Settings\n")
		w.say("========================\n\n")
		w.say("  1) Govee API key             %s\n", mask(s.Govee.APIKey))
		w.say("  2) Spotify Client ID/Secret  %s / %s\n", mask(s.Spotify.ClientID), mask(s.Spotify.ClientSecret))
		login := "not logged in"
		if s.Spotify.RefreshToken != "" {
			login = "logged in"
		}
		w.say("  3) Spotify login             %s\n", login)
		dev := s.Spotify.DefaultDevice
		if dev == "" {
			dev = "not set"
		}
		w.say("  4) Spotify speaker           %s\n", dev)
		w.say("  5) Wi-Fi                     %s\n", WifiStatus())
		if w.Live {
			w.say("  6) Install to this computer's disk\n")
		}
		w.say("  0) Back\n\n")

		ans, err := w.ask("settings", "Choose: ")
		if err != nil {
			return err
		}
		changed := false
		switch ans {
		case "", "0", "q", "b":
			return nil
		case "1":
			before := s.Govee.APIKey
			if err := w.goveeStep(ctx, s); err != nil {
				return err
			}
			changed = s.Govee.APIKey != before
		case "2":
			changed, err = w.changeSpotifyKeys(ctx, s)
		case "3":
			if s.Spotify.ClientID == "" || s.Spotify.ClientSecret == "" {
				w.say("Set the Client ID and Secret first (option 2).\n")
				w.pause()
				continue
			}
			err = w.spotifyLogin(ctx, s, s.Spotify.ClientID, s.Spotify.ClientSecret, LocalIPv4())
			changed = err == nil
		case "4":
			if s.Spotify.RefreshToken == "" {
				w.say("Log in to Spotify first (option 3).\n")
				w.pause()
				continue
			}
			before := s.Spotify.DefaultDevice
			err = w.pickDevice(ctx, s)
			changed = s.Spotify.DefaultDevice != before
		case "5":
			for again := true; again && err == nil; {
				again, err = w.wifiSetup()
				if again && err == nil && wifi.IfaceIPv4(firstAdapter()) != "" {
					again = false
				}
			}
			w.pause()
		case "6":
			if w.Live {
				err = w.InstallStep()
			}
		default:
			continue
		}
		if err != nil {
			return err
		}
		if changed {
			if err := config.SaveSecrets(w.SecretsPath, s); err != nil {
				w.say("Could not save: %v\n", err)
			} else {
				w.trace("settings-saved")
				w.say("Saved.\n")
			}
			w.pause()
		}
	}
}

func firstAdapter() string {
	if ads := wifi.Adapters(); len(ads) > 0 {
		return ads[0].Name
	}
	return ""
}

func (w *Wizard) pause() {
	_, _ = w.ask("continue", "Press Enter to continue.")
}

// changeSpotifyKeys replaces the app keys and, because a login is tied to
// the app, logs in again.
func (w *Wizard) changeSpotifyKeys(ctx context.Context, s *config.Secrets) (bool, error) {
	w.say("\nSpotify app keys are on developer.spotify.com/dashboard > your app > Settings.\n")
	id, err := w.ask("spotify-id", fmt.Sprintf("Client ID (Enter to keep %s): ", mask(s.Spotify.ClientID)))
	if err != nil {
		return false, err
	}
	if id == "" {
		id = s.Spotify.ClientID
	}
	secret, err := w.ask("spotify-secret", fmt.Sprintf("Client Secret (Enter to keep %s): ", mask(s.Spotify.ClientSecret)))
	if err != nil {
		return false, err
	}
	if secret == "" {
		secret = s.Spotify.ClientSecret
	}
	if id == "" || secret == "" {
		w.say("Both are needed.\n")
		return false, nil
	}
	if id == s.Spotify.ClientID && secret == s.Spotify.ClientSecret && s.Spotify.RefreshToken != "" {
		w.say("No change.\n")
		return false, nil
	}
	if err := w.spotifyLogin(ctx, s, id, secret, LocalIPv4()); err != nil {
		return false, err
	}
	return true, nil
}

// ReconnectSpotify logs in to Spotify again with the saved app keys (e.g. to
// grant the playlist permission) and saves the new login.
func (w *Wizard) ReconnectSpotify(ctx context.Context) error {
	s, err := config.LoadSecrets(w.SecretsPath)
	if err != nil || s.Spotify.ClientID == "" || s.Spotify.ClientSecret == "" {
		return fmt.Errorf("set up Spotify first (Settings > Spotify Client ID/Secret)")
	}
	w.say("\033[H\033[2J")
	w.say("Reconnecting Spotify so the editor can list your playlists.\n")
	if err := w.spotifyLogin(ctx, s, s.Spotify.ClientID, s.Spotify.ClientSecret, LocalIPv4()); err != nil {
		return err
	}
	return config.SaveSecrets(w.SecretsPath, s)
}
