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

func TestExtractCode(t *testing.T) {
	cases := []struct{ in, code, err string }{
		{"http://127.0.0.1:8888/callback?code=AQB-x_1&state=S1", "AQB-x_1", ""},
		{"AQB-x_1", "AQB-x_1", ""},
		{"http://127.0.0.1:8888/callback?code=c&state=OLD", "", "older"},
		{"http://127.0.0.1:8888/callback?error=access_denied&state=S1", "", "Agree"},
		{"", "", "nothing"},
	}
	for _, c := range cases {
		got, err := ExtractCode(c.in, "S1")
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%q: want error %q, got %v", c.in, c.err, err)
			}
		} else if got != c.code || err != nil {
			t.Errorf("%q: got %q %v", c.in, got, err)
		}
	}
}

func TestWizardFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/govee/user/devices":
			if r.Header.Get("Govee-API-Key") != "good-key" {
				w.WriteHeader(401)
				io.WriteString(w, `{"code":401,"message":"Invalid API Key"}`)
				return
			}
			io.WriteString(w, `{"code":200,"data":[{"sku":"H6199","device":"AA","deviceName":"TV Backlight"}]}`)
		case "/govee/device/scenes", "/govee/device/diy-scenes":
			io.WriteString(w, `{"code":200,"payload":{"capabilities":[]}}`)
		case "/token":
			r.ParseForm()
			if r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") == "RT" {
				io.WriteString(w, `{"access_token":"AT","expires_in":3600}`)
				return
			}
			if r.Form.Get("code") != "THECODE" || r.Form.Get("redirect_uri") != spotify.RedirectURI {
				w.WriteHeader(400)
				io.WriteString(w, `{"error":"invalid_grant"}`)
				return
			}
			io.WriteString(w, `{"access_token":"AT","refresh_token":"RT","expires_in":3600}`)
		case "/api/me/player/devices":
			io.WriteString(w, `{"devices":[{"id":"D1","name":"Living Room","type":"Speaker"}]}`)
		}
	}))
	defer srv.Close()
	t.Setenv("STREAMDECKD_GOVEE_URL", srv.URL+"/govee")
	spotify.DefaultTokenURL, spotify.DefaultAPIURL = srv.URL+"/token", srv.URL+"/api"

	dir := t.TempDir()
	// Answers: (Wi-Fi prompts if offline / optional), bad key, good key,
	// client id, secret, code, device #1.
	input := strings.Join([]string{"n", "bad-key", "good-key", "cid", "csecret", "THECODE", "1"}, "\n") + "\n"
	if LocalIPv4() == "" {
		t.Skip("needs a network interface")
	}
	var out, trace bytes.Buffer
	w := New(strings.NewReader(input), &out)
	w.Trace = &trace
	w.SecretsPath = dir + "/secrets.json"
	w.LayoutDir = dir
	w.WebAddr = "127.0.0.1:0"
	w.WifiRestart = nil
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	s, err := config.LoadSecrets(w.SecretsPath)
	if err != nil {
		t.Fatal(err)
	}
	if s.Govee.APIKey != "good-key" || s.Spotify.RefreshToken != "RT" || s.Spotify.ClientSecret != "csecret" || s.Spotify.DefaultDevice != "Living Room" {
		t.Fatalf("saved %+v", s)
	}
	if !strings.Contains(out.String(), "didn't work") || !strings.Contains(trace.String(), "govee-ok 1") {
		t.Fatalf("output:\n%s\ntrace:\n%s", out.String(), trace.String())
	}
}

func TestSettingsMenuChangesGoveeKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Govee-API-Key") != "new-key" {
			w.WriteHeader(401)
			io.WriteString(w, `{"code":401,"message":"Invalid API Key"}`)
			return
		}
		io.WriteString(w, `{"code":200,"data":[],"payload":{"capabilities":[]}}`)
	}))
	defer srv.Close()
	t.Setenv("STREAMDECKD_GOVEE_URL", srv.URL)

	dir := t.TempDir()
	sp := dir + "/secrets.json"
	orig := &config.Secrets{Govee: config.GoveeConfig{APIKey: "old-key-1234"},
		Spotify: config.SpotifyConfig{ClientID: "cid", ClientSecret: "sec", RefreshToken: "RT", DefaultDevice: "Den"}}
	if err := config.SaveSecrets(sp, orig); err != nil {
		t.Fatal(err)
	}
	// 1 = Govee, wrong key, right key, Enter to continue, 0 = back
	in := strings.Join([]string{"1", "bad", "new-key", "", "0"}, "\n") + "\n"
	var out bytes.Buffer
	w := New(strings.NewReader(in), &out)
	w.SecretsPath, w.LayoutDir = sp, dir
	if err := w.Settings(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, _ := config.LoadSecrets(sp)
	if s.Govee.APIKey != "new-key" || s.Spotify.RefreshToken != "RT" || s.Spotify.DefaultDevice != "Den" {
		t.Fatalf("saved %+v\n%s", s, out.String())
	}
	if !strings.Contains(out.String(), "old-****1234") || !strings.Contains(out.String(), "Saved.") {
		t.Fatalf("menu output:\n%s", out.String())
	}
}
