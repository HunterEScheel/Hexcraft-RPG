package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"streamdeckd/internal/config"
	"streamdeckd/internal/spotify"
)

type fake struct{ calls []string }

func (f *fake) ApplyScene(_ context.Context, d, s string, diy bool) error {
	f.calls = append(f.calls, "scene:"+d+":"+s)
	return errors.New("govee down")
}
func (f *fake) Power(context.Context, string, bool) error                { return nil }
func (f *fake) Brightness(context.Context, string, int) error            { return nil }
func (f *fake) Color(context.Context, string, uint8, uint8, uint8) error { return nil }
func (f *fake) PlayContext(_ context.Context, d, uri string, o spotify.PlayOptions) error {
	f.calls = append(f.calls, "play:"+d+":"+uri)
	return nil
}
func (f *fake) Pause(context.Context, string) error       { return nil }
func (f *fake) Next(context.Context, string) error        { return nil }
func (f *fake) Volume(context.Context, string, int) error { return nil }

func TestRunContinuesAfterError(t *testing.T) {
	f := &fake{}
	r := &Runner{Cfg: &config.Config{Spotify: config.SpotifyConfig{DefaultDevice: "Den"}}, Govee: f, Spotify: f}
	res := r.Run(context.Background(), []config.Action{
		{Type: config.GoveeScene, Device: "Strip", Scene: "Party"},
		{Type: config.SpotifyPlaylist, Playlist: "spotify:playlist:abc"},
		{Type: config.Page, Page: "music"},
	})
	if strings.Join(f.calls, ",") != "scene:Strip:Party,play:Den:spotify:playlist:abc" {
		t.Fatalf("calls: %v", f.calls)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "govee down") || res.SwitchTo != "music" {
		t.Fatalf("res: %+v", res)
	}
}
