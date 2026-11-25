package runner

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
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
func (f *fake) ColorTemp(_ context.Context, d string, k int) error {
	f.calls = append(f.calls, fmt.Sprintf("temp:%s:%d", d, k))
	return nil
}
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

type fakeLAN struct{ calls []string }

func (f *fakeLAN) Power(_ context.Context, l config.LANLight, on bool) error {
	f.calls = append(f.calls, "lan-power:"+l.Device)
	return nil
}
func (f *fakeLAN) Brightness(_ context.Context, l config.LANLight, pct int) error {
	f.calls = append(f.calls, "lan-bright:"+l.Device)
	return nil
}
func (f *fakeLAN) ColorTemp(_ context.Context, l config.LANLight, k int) error {
	f.calls = append(f.calls, fmt.Sprintf("lan-temp:%s:%d", l.Device, k))
	return nil
}
func (f *fakeLAN) Color(_ context.Context, l config.LANLight, r, g, b uint8) error {
	f.calls = append(f.calls, "lan-color:"+l.Device)
	return nil
}

type cloudRec struct{ fake }

func (c *cloudRec) Color(_ context.Context, d string, r, g, b uint8) error {
	c.calls = append(c.calls, "cloud-color:"+d)
	return nil
}

func TestLANLightsSkipTheCloud(t *testing.T) {
	cloud, lan := &cloudRec{}, &fakeLAN{}
	cfg := &config.Config{LANLights: []config.LANLight{{Name: "Spotlights", Device: "AA:01", SKU: "H7065"}}}
	r := &Runner{Cfg: cfg, Govee: cloud, LAN: lan}
	on := true
	res := r.Run(context.Background(), []config.Action{
		{Type: config.GoveeColor, Device: "spotlights", Color: "#ff0000"},
		{Type: config.GoveePower, Device: "Spotlights", On: &on},
		{Type: config.GoveeColor, Device: "Downlights", Color: "#00ff00"},
	})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if strings.Join(lan.calls, ",") != "lan-color:AA:01,lan-power:AA:01" || strings.Join(cloud.calls, ",") != "cloud-color:Downlights" {
		t.Fatalf("lan %v cloud %v", lan.calls, cloud.calls)
	}
	// A LAN-only layout needs no Govee API key; a scene on a LAN light is refused.
	l := config.Layout{StartPage: "home", Pages: map[string][]config.Button{"home": {{Key: 0, Label: "x",
		Actions: []config.Action{{Type: config.GoveePower, Device: "Spotlights", On: &on}}}}}, LANLights: cfg.LANLights}
	if err := config.CheckLayout(l, &config.Secrets{}); err != nil {
		t.Fatalf("LAN-only layout rejected: %v", err)
	}
	l.Pages["home"][0].Actions = []config.Action{{Type: config.GoveeScene, Device: "Spotlights", Scene: "Party"}}
	if err := config.CheckLayout(l, &config.Secrets{Govee: config.GoveeConfig{APIKey: "k"}}); err == nil || !strings.Contains(err.Error(), "no scenes") {
		t.Fatalf("scene on a LAN light: %v", err)
	}
}

type lockedCloud struct {
	fake
	mu sync.Mutex
}

func (c *lockedCloud) Brightness(_ context.Context, d string, pct int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, "bright:"+d)
	return nil
}

func TestOneActionManyLights(t *testing.T) {
	cloud, lan := &lockedCloud{}, &fakeLAN{}
	cfg := &config.Config{LANLights: []config.LANLight{{Name: "Flood Lights", Device: "AA:61"}}}
	r := &Runner{Cfg: cfg, Govee: cloud, LAN: lan}
	a := config.Action{Type: config.GoveeBrightness, Brightness: 40}
	a.SetLights([]string{"dnd1", "dnd2", "Smart LED Downlight 3", "Smart LED Downlight 4", "Flood Lights"})
	if res := r.Run(context.Background(), []config.Action{a}); res.Err != nil {
		t.Fatal(res.Err)
	}
	sort.Strings(cloud.calls)
	if strings.Join(cloud.calls, ",") != "bright:Smart LED Downlight 3,bright:Smart LED Downlight 4,bright:dnd1,bright:dnd2" ||
		strings.Join(lan.calls, ",") != "lan-bright:AA:61" {
		t.Fatalf("cloud %v lan %v", cloud.calls, lan.calls)
	}
	if a.Device != "dnd1" || len(a.Devices) != 4 || len(a.Lights()) != 5 {
		t.Fatalf("stored as %+v", a)
	}
}

func TestColorTemperature(t *testing.T) {
	cloud, lan := &fake{}, &fakeLAN{}
	cfg := &config.Config{LANLights: []config.LANLight{{Name: "Flood Lights", Device: "AA:61"}}}
	r := &Runner{Cfg: cfg, Govee: cloud, LAN: lan}
	a := config.Action{Type: config.GoveeColorTemp, Kelvin: 4000}
	a.SetLights([]string{"dnd1", "Flood Lights"})
	if res := r.Run(context.Background(), []config.Action{a}); res.Err != nil {
		t.Fatal(res.Err)
	}
	if strings.Join(cloud.calls, ",") != "temp:dnd1:4000" || strings.Join(lan.calls, ",") != "lan-temp:AA:61:4000" {
		t.Fatalf("cloud %v lan %v", cloud.calls, lan.calls)
	}
	l := config.Layout{StartPage: "home", Pages: map[string][]config.Button{"home": {{Key: 0, Label: "x", Actions: []config.Action{{Type: config.GoveeColorTemp, Device: "Flood Lights", Kelvin: 100}}}}}, LANLights: cfg.LANLights}
	if err := config.CheckLayout(l, &config.Secrets{}); err == nil || !strings.Contains(err.Error(), "kelvin") {
		t.Fatalf("bad kelvin accepted: %v", err)
	}
}
