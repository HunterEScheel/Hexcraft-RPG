package setup

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/tui"
	"streamdeckd/internal/ui"
	"streamdeckd/internal/ui/text"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func testApp(t *testing.T) (*App, *tui.Fake, *syncBuf) {
	t.Helper()
	if LocalIPv4() == "" {
		t.Skip("needs a network interface")
	}
	scr := tui.NewFake(100, 34)
	tr := &syncBuf{}
	dir := t.TempDir()
	a := &App{U: text.New(scr, ui.Tracer{W: tr}), Trace: tr, SecretsPath: dir + "/secrets.json",
		LayoutPath: dir + "/config.json", WebAddr: "127.0.0.1:0"}
	return a, scr, tr
}

func waitTrace(t *testing.T, tr *syncBuf, s string) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if strings.Contains(tr.String(), s) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never saw %q in trace:\n%s", s, tr.String())
}

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

func TestGoveeKeyRetriesThenSaves(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Govee-API-Key") != "good-key" {
			w.WriteHeader(401)
			io.WriteString(w, `{"code":401,"message":"Invalid API Key"}`)
			return
		}
		io.WriteString(w, `{"code":200,"data":[{"sku":"H6199","device":"AA","deviceName":"TV Backlight"}]}`)
	}))
	defer srv.Close()
	t.Setenv("STREAMDECKD_GOVEE_URL", srv.URL)
	a, scr, tr := testApp(t)
	done := make(chan struct{})
	go func() { a.GoveeScreen(context.Background()); close(done) }()
	waitTrace(t, tr, "input govee-key")
	scr.Press("bad-key", "enter")
	waitTrace(t, tr, "message govee-error")
	scr.Press("enter", "good-key", "enter")
	waitTrace(t, tr, "message govee-ok")
	if !strings.Contains(scr.Text(), "TV Backlight") {
		t.Fatalf("lights not listed:\n%s", scr.Text())
	}
	scr.Press("enter")
	<-done
	s, _ := config.LoadSecrets(a.SecretsPath)
	if s.Govee.APIKey != "good-key" {
		t.Fatalf("saved %+v", s)
	}
}

func TestSpotifyPhoneLoginAndSpeaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
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
	spotify.DefaultTokenURL, spotify.DefaultAPIURL = srv.URL+"/token", srv.URL+"/api"
	a, scr, tr := testApp(t)
	done := make(chan struct{})
	go func() { a.SpotifyScreen(context.Background()); close(done) }()
	waitTrace(t, tr, "input spotify-id")
	scr.Press("cid", "enter")
	waitTrace(t, tr, "input spotify-secret")
	scr.Press("csecret", "enter")
	waitTrace(t, tr, "spotify-wait")
	m := regexp.MustCompile(`state=(\w+) port=(\d+)`).FindStringSubmatch(tr.String())
	// The phone posts the address it ended up on.
	resp, err := http.PostForm("http://127.0.0.1:"+m[2]+"/", url.Values{"url": {spotify.RedirectURI + "?code=THECODE&state=" + m[1]}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	waitTrace(t, tr, "menu spotify-speaker")
	scr.Press("enter") // Living Room
	waitTrace(t, tr, "menu spotify sel=")
	scr.Press("esc")
	<-done
	s, _ := config.LoadSecrets(a.SecretsPath)
	if s.Spotify.ClientID != "cid" || s.Spotify.ClientSecret != "csecret" || s.Spotify.RefreshToken != "RT" || s.Spotify.DefaultDevice != "Living Room" {
		t.Fatalf("saved %+v", s)
	}
}
