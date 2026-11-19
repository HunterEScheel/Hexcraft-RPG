package setup

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"streamdeckd/internal/config"
	"streamdeckd/internal/spotify"
)

func TestParseSeed(t *testing.T) {
	s, err := ParseSeed("\ufeff# my setup\nWiFi Name = \"WhIOT\"\nwifi-password: hunter2 pass\nGOVEE_API_KEY=abc-123\n" +
		"spotify_client_id = id1\nspotify_client_secret = sec1\nspotify_speaker = Hunter's S23+\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if s.WifiName != "WhIOT" || s.WifiPassword != "hunter2 pass" || s.GoveeKey != "abc-123" ||
		s.SpotifyID != "id1" || s.SpotifySecret != "sec1" || s.SpotifySpeaker != "Hunter's S23+" || s.SpotifyRefresh != "" {
		t.Fatalf("got %+v", s)
	}
	if _, err := ParseSeed("govee_api_key = x\nfavourite_colour = blue\n"); err == nil || !strings.Contains(err.Error(), "favourite_colour") {
		t.Fatalf("unknown key not reported: %v", err)
	}
	full := &Seed{WifiName: "Net", WifiPassword: "p w", WifiCountry: "GB", GoveeKey: "g", SpotifyID: "i", SpotifySecret: "s", SpotifyRefresh: "r", SpotifySpeaker: "Den"}
	back, err := ParseSeed(full.Format())
	if err != nil || *back != *full {
		t.Fatalf("round trip: %+v %v", back, err)
	}
	if strings.Contains(strings.Join(full.Summary(), " "), "p w") {
		t.Fatal("summary leaks the Wi-Fi password")
	}
}

func TestWizardUsesSeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/govee/user/devices":
			if r.Header.Get("Govee-API-Key") != "seed-key" {
				w.WriteHeader(401)
				return
			}
			io.WriteString(w, `{"code":200,"data":[{"sku":"H1","device":"AA","deviceName":"Lamp"}]}`)
		case "/govee/device/scenes", "/govee/device/diy-scenes":
			io.WriteString(w, `{"code":200,"payload":{"capabilities":[]}}`)
		case "/token":
			r.ParseForm()
			if r.Form.Get("refresh_token") == "seed-refresh" {
				io.WriteString(w, `{"access_token":"AT","expires_in":3600}`)
				return
			}
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"invalid_grant"}`)
		case "/api/me/player/devices":
			io.WriteString(w, `{"devices":[{"id":"D1","name":"Den","type":"Speaker"}]}`)
		}
	}))
	defer srv.Close()
	t.Setenv("STREAMDECKD_GOVEE_URL", srv.URL+"/govee")
	spotify.DefaultTokenURL, spotify.DefaultAPIURL = srv.URL+"/token", srv.URL+"/api"
	if LocalIPv4() == "" {
		t.Skip("needs a network interface")
	}

	dir := t.TempDir()
	var out, trace bytes.Buffer
	// Only answers: Enter (use the setup file) and "n" (no extra Wi-Fi).
	w := New(strings.NewReader("\nn\n"), &out)
	w.Trace = &trace
	w.SecretsPath, w.LayoutDir = dir+"/secrets.json", dir
	w.SeedFinder = func() (*Seed, string, error) {
		return &Seed{GoveeKey: "seed-key", SpotifyID: "cid", SpotifySecret: "csec", SpotifyRefresh: "seed-refresh", SpotifySpeaker: "Den"}, "/dev/sdb1: streamdeck-setup.txt", nil
	}
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	s, _ := config.LoadSecrets(w.SecretsPath)
	if s.Govee.APIKey != "seed-key" || s.Spotify.RefreshToken != "seed-refresh" || s.Spotify.ClientSecret != "csec" || s.Spotify.DefaultDevice != "Den" {
		t.Fatalf("saved %+v", s)
	}
	tr := trace.String()
	for _, never := range []string{"prompt govee-key", "prompt spotify-id", "prompt spotify-secret", "spotify-wait", "prompt spotify-device"} {
		if strings.Contains(tr, never) {
			t.Fatalf("asked %q despite the setup file:\n%s", never, tr)
		}
	}
	if !strings.Contains(tr, "seed-used") || !strings.Contains(tr, "spotify-ok seeded") {
		t.Fatalf("trace:\n%s", tr)
	}
}
