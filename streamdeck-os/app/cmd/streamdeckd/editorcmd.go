package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"streamdeckd/internal/config"
	"streamdeckd/internal/deckipc"
	"streamdeckd/internal/editor"
	"streamdeckd/internal/govee"
	"streamdeckd/internal/runner"
	"streamdeckd/internal/setup"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/tui"
)

// swapScreen lets the editor survive the terminal being handed back to
// line-based prompts (Spotify reconnect) and taken again.
type swapScreen struct{ t *tui.Term }

func (s *swapScreen) Size() (int, int)     { return s.t.Size() }
func (s *swapScreen) Draw(l []string)      { s.t.Draw(l) }
func (s *swapScreen) Keys() <-chan tui.Key { return s.t.Keys() }

type deckAdapter struct{ c *deckipc.Client }

func (d deckAdapter) Preview(l config.Layout, page string, sel int) error {
	return d.c.Preview(l, page, sel)
}
func (d deckAdapter) Presses() <-chan int { return d.c.Presses }

// realSources reads the saved credentials each time, so changes made in
// Settings apply without restarting the editor.
type realSources struct {
	secretsPath string
	w           *setup.Wizard
	suspend     func()
	resume      func() error
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
		gd := editor.GoveeDevice{Name: d.Name}
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
	return runner.New(cfg).Run(ctx, actions).Err
}

func (r *realSources) ReconnectSpotify(ctx context.Context) error {
	r.suspend()
	err := r.w.ReconnectSpotify(ctx)
	if rerr := r.resume(); rerr != nil && err == nil {
		err = rerr
	}
	return err
}

// runEditor runs the full-screen button editor on the console.
func runEditor(ctx context.Context, w *setup.Wizard, cfgPath, secretsPath string, trace *os.File) {
	scr := &swapScreen{}
	var release func()
	open := func() error {
		in, rel := w.RawInput()
		t, err := tui.Open(int(os.Stdin.Fd()), os.Stdout, in)
		if err != nil {
			rel()
			return err
		}
		scr.t, release = t, rel
		return nil
	}
	closeTerm := func() {
		scr.t.Close()
		release()
	}
	if err := open(); err != nil {
		fmt.Printf("Can't open the editor: %v\n", err)
		return
	}
	defer closeTerm()

	src := &realSources{secretsPath: secretsPath, w: w, suspend: closeTerm, resume: open}
	e := &editor.Editor{S: scr, Src: src, LayoutPath: cfgPath, Secrets: src.secrets}
	if trace != nil {
		e.Trace = trace
	}
	if c, err := deckipc.Dial(); err == nil {
		defer c.Close()
		e.Deck = deckAdapter{c}
	}
	_ = e.Run(ctx)
}

// buttonSummary is the status screen's one-liner about the layout.
func buttonSummary(cfgPath string) string {
	l, err := config.ReadLayout(cfgPath)
	if err != nil {
		return "layout can't be read: " + err.Error()
	}
	n := 0
	for _, b := range l.Pages {
		n += len(b)
	}
	pages := "page"
	if len(l.Pages) != 1 {
		pages = "pages"
	}
	if n == 0 {
		return "none yet: choose 1 to set them up"
	}
	return fmt.Sprintf("%d on %d %s", n, len(l.Pages), pages)
}
