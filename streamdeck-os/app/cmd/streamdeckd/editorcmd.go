package main

import (
	"context"
	"errors"
	"io"

	"streamdeckd/internal/config"
	"streamdeckd/internal/deckipc"
	"streamdeckd/internal/editor"
	"streamdeckd/internal/govee"
	"streamdeckd/internal/runner"
	"streamdeckd/internal/setup"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/ui"
)

type deckAdapter struct{ c *deckipc.Client }

func (d deckAdapter) Preview(l config.Layout, page string, sel int) error {
	return d.c.Preview(l, page, sel)
}
func (d deckAdapter) Presses() <-chan int { return d.c.Presses }

// realSources reads the saved credentials each time, so changes made in
// Settings apply without restarting the editor.
type realSources struct {
	secretsPath string
	cfgPath     string
	app         *setup.App
}

func (r *realSources) secrets() *config.Secrets {
	s, err := config.LoadSecrets(r.secretsPath)
	if err != nil {
		return &config.Secrets{}
	}
	return s
}

func (r *realSources) GoveeReady() bool { return r.secrets().Govee.APIKey != "" }
func (r *realSources) SpotifyReady() bool {
	s := r.secrets()
	return s.Spotify.RefreshToken != "" && s.Spotify.ClientID != ""
}
func (r *realSources) DefaultSpeaker() string { return r.secrets().Spotify.DefaultDevice }

func (r *realSources) spotify() *spotify.Client {
	s := r.secrets()
	return spotify.New(s.Spotify.ClientID, s.Spotify.ClientSecret, s.Spotify.RefreshToken)
}

func (r *realSources) GoveeDevices(ctx context.Context) ([]editor.GoveeDevice, error) {
	g := govee.New(r.secrets().Govee.APIKey)
	devs, err := g.Devices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]editor.GoveeDevice, 0, len(devs))
	for _, d := range devs {
		gd := editor.GoveeDevice{Name: d.Name, Group: d.IsGroup()}
		if gd.Group {
			out = append(out, gd) // known, but not offered; no scene lookups
			continue
		}
		if opts, err := g.Scenes(ctx, d, false); err == nil {
			for _, o := range opts {
				gd.Scenes = append(gd.Scenes, o.Name)
			}
		}
		if opts, err := g.Scenes(ctx, d, true); err == nil {
			for _, o := range opts {
				gd.DIY = append(gd.DIY, o.Name)
			}
		}
		out = append(out, gd)
	}
	return out, nil
}

func (r *realSources) SpotifyDevices(ctx context.Context) ([]string, error) {
	if !r.SpotifyReady() {
		return nil, errors.New("Spotify isn't set up")
	}
	devs, err := r.spotify().Devices(ctx)
	names := make([]string, 0, len(devs))
	for _, d := range devs {
		names = append(names, d.Name)
	}
	return names, err
}

func (r *realSources) Playlists(ctx context.Context) ([]spotify.Playlist, error) {
	return r.spotify().Playlists(ctx, 1000)
}

func (r *realSources) Run(ctx context.Context, actions []config.Action) error {
	cfg := &config.Config{}
	cfg.ApplySecrets(r.secrets())
	if l, err := config.ReadLayout(r.cfgPath); err == nil {
		cfg.LANLights = l.LANLights
	}
	return runner.New(cfg).Run(ctx, actions).Err
}

func (r *realSources) ReconnectSpotify(ctx context.Context) error {
	return r.app.ReconnectSpotify(ctx)
}

// runEditor runs the button editor; the Stream Deck shows the layout live.
func runEditor(ctx context.Context, u ui.UI, app *setup.App, cfgPath, secretsPath string, trace io.Writer) {
	src := &realSources{secretsPath: secretsPath, cfgPath: cfgPath, app: app}
	e := &editor.Editor{U: u, Src: src, LayoutPath: cfgPath, Secrets: src.secrets, Trace: trace}
	if c, err := deckipc.Dial(); err == nil {
		defer c.Close()
		e.Deck = deckAdapter{c}
	}
	_ = e.Run(ctx)
}
