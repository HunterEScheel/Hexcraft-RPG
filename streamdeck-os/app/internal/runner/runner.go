// Package runner executes a button's actions against Govee and Spotify.
package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/govee"
	"streamdeckd/internal/spotify"
)

// Govee and Spotify are the subsets of the API clients the runner needs
// (interfaces so tests can fake them).
type Govee interface {
	ApplyScene(ctx context.Context, device, scene string, diy bool) error
	Power(ctx context.Context, device string, on bool) error
	Brightness(ctx context.Context, device string, pct int) error
	Color(ctx context.Context, device string, r, g, b uint8) error
	ColorTemp(ctx context.Context, device string, kelvin int) error
}

type Spotify interface {
	PlayContext(ctx context.Context, device, uri string, opt spotify.PlayOptions) error
	Pause(ctx context.Context, device string) error
	Next(ctx context.Context, device string) error
	Volume(ctx context.Context, device string, pct int) error
}

type Runner struct {
	Cfg     *config.Config
	Govee   Govee
	Spotify Spotify
	LAN     LAN
}

// New builds a runner with real API clients for whatever is configured.
func New(cfg *config.Config) *Runner {
	r := &Runner{Cfg: cfg, LAN: lanClient{}}
	if cfg.Govee.APIKey != "" {
		r.Govee = govee.New(cfg.Govee.APIKey)
	}
	if cfg.Spotify.RefreshToken != "" {
		r.Spotify = spotify.New(cfg.Spotify.ClientID, cfg.Spotify.ClientSecret, cfg.Spotify.RefreshToken)
	}
	return r
}

// Result of running a button.
type Result struct {
	// SwitchTo is non-empty if a page action ran.
	SwitchTo string
	Err      error
}

// Run executes actions in order. Govee/Spotify failures don't stop later
// actions (lights still change if Spotify is down); all errors are returned.
func (r *Runner) Run(ctx context.Context, actions []config.Action) Result {
	var res Result
	var errs []error
	for i, a := range actions {
		if a.Type == config.Page {
			res.SwitchTo = a.Page
			continue
		}
		if err := r.one(ctx, a); err != nil {
			errs = append(errs, fmt.Errorf("action %d (%s): %w", i, a.Type, err))
		}
	}
	res.Err = errors.Join(errs...)
	return res
}

func (r *Runner) spotifyDevice(a config.Action) string {
	if a.SpotifyDevice != "" {
		return a.SpotifyDevice
	}
	return r.Cfg.Spotify.DefaultDevice
}

func (r *Runner) one(ctx context.Context, a config.Action) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	switch a.Type {
	case config.Delay:
		select {
		case <-time.After(time.Duration(a.Ms) * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	case config.GoveeScene, config.GoveeDIYScene, config.GoveePower, config.GoveeBrightness, config.GoveeColor, config.GoveeColorTemp:
		return r.lights(ctx, a)
	case config.SpotifyPlaylist, config.SpotifyPause, config.SpotifyNext, config.SpotifyVolume:
		if r.Spotify == nil {
			return errors.New("spotify not configured")
		}
	}

	switch a.Type {
	case config.SpotifyPlaylist:
		return r.Spotify.PlayContext(ctx, r.spotifyDevice(a), a.Playlist,
			spotify.PlayOptions{Shuffle: a.Shuffle, Volume: a.Volume})
	case config.SpotifyPause:
		return r.Spotify.Pause(ctx, r.spotifyDevice(a))
	case config.SpotifyNext:
		return r.Spotify.Next(ctx, r.spotifyDevice(a))
	case config.SpotifyVolume:
		return r.Spotify.Volume(ctx, r.spotifyDevice(a), *a.Volume)
	}
	return fmt.Errorf("unknown action %q", a.Type)
}

func (r *Runner) lan(ctx context.Context, l config.LANLight, a config.Action) error {
	switch a.Type {
	case config.GoveePower:
		return r.LAN.Power(ctx, l, *a.On)
	case config.GoveeBrightness:
		return r.LAN.Brightness(ctx, l, a.Brightness)
	case config.GoveeColorTemp:
		return r.LAN.ColorTemp(ctx, l, a.Kelvin)
	default:
		c, err := config.ParseColor(a.Color)
		if err != nil {
			return err
		}
		return r.LAN.Color(ctx, l, c.R, c.G, c.B)
	}
}

// lights applies a Govee action to each of its lights at the same time
// (LAN lights directly, the others through the cloud).
func (r *Runner) lights(ctx context.Context, a config.Action) error {
	ls := a.Lights()
	if len(ls) == 0 {
		return errors.New("no light chosen")
	}
	errs := make([]error, len(ls))
	var wg sync.WaitGroup
	for i, dev := range ls {
		wg.Add(1)
		go func(i int, dev string) {
			defer wg.Done()
			if err := r.light(ctx, a, dev); err != nil {
				if len(ls) > 1 {
					err = fmt.Errorf("%s: %w", dev, err)
				}
				errs[i] = err
			}
		}(i, dev)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (r *Runner) light(ctx context.Context, a config.Action, dev string) error {
	if l, ok := r.Cfg.LANLight(dev); ok && r.LAN != nil && a.Type != config.GoveeScene && a.Type != config.GoveeDIYScene {
		return r.lan(ctx, l, a)
	}
	if r.Govee == nil {
		return errors.New("govee not configured")
	}
	switch a.Type {
	case config.GoveeScene:
		return r.Govee.ApplyScene(ctx, dev, a.Scene, false)
	case config.GoveeDIYScene:
		return r.Govee.ApplyScene(ctx, dev, a.Scene, true)
	case config.GoveePower:
		return r.Govee.Power(ctx, dev, *a.On)
	case config.GoveeBrightness:
		return r.Govee.Brightness(ctx, dev, a.Brightness)
	case config.GoveeColorTemp:
		return r.Govee.ColorTemp(ctx, dev, a.Kelvin)
	default:
		c, err := config.ParseColor(a.Color)
		if err != nil {
			return err
		}
		return r.Govee.Color(ctx, dev, c.R, c.G, c.B)
	}
}
