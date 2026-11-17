package spotify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNormalizeURI(t *testing.T) {
	cases := map[string]string{
		"spotify:playlist:37i9dQZF1DX":                                  "spotify:playlist:37i9dQZF1DX",
		"https://open.spotify.com/playlist/37i9dQZF1DX?si=abc":          "spotify:playlist:37i9dQZF1DX",
		"https://open.spotify.com/intl-de/album/4aawyAB9vmqN3uQ7FjRGTy": "spotify:album:4aawyAB9vmqN3uQ7FjRGTy",
		"spotify:user:bob:playlist:XYZ":                                 "spotify:playlist:XYZ",
	}
	for in, want := range cases {
		got, err := NormalizeURI(in)
		if err != nil || got != want {
			t.Errorf("%s: got %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"spotify:track:x", "https://example.com/playlist/x", "hello"} {
		if _, err := NormalizeURI(bad); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
}

func TestPlayContextTransfersInactiveDevice(t *testing.T) {
	var mu sync.Mutex
	var log []string
	played := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/token" {
			if user, pass, _ := r.BasicAuth(); user != "id" || pass != "secret" {
				w.WriteHeader(400)
				return
			}
			io.WriteString(w, `{"access_token":"AT","expires_in":3600}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer AT" {
			w.WriteHeader(401)
			return
		}
		log = append(log, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(body))
		switch r.URL.Path {
		case "/me/player/devices":
			io.WriteString(w, `{"devices":[{"id":"D1","name":"Living Room","type":"Speaker","is_active":false}]}`)
		case "/me/player/play":
			played++
			if played == 1 {
				w.WriteHeader(404)
				io.WriteString(w, `{"error":{"status":404,"message":"Device not found"}}`)
				return
			}
			w.WriteHeader(204)
		default:
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()

	c := New("id", "secret", "RT")
	c.APIURL, c.TokenURL = srv.URL, srv.URL+"/token"
	vol := 40
	err := c.PlayContext(context.Background(), "living room", "https://open.spotify.com/playlist/PL1", PlayOptions{Shuffle: true, Volume: &vol})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(log, "\n")
	want := []string{
		"GET /me/player/devices",
		"PUT /me/player/volume?device_id=D1&volume_percent=40",
		`PUT /me/player/play?device_id=D1 {"context_uri":"spotify:playlist:PL1"}`,
		`PUT /me/player? {"device_ids":["D1"],"play":false}`,
		`PUT /me/player/play?device_id=D1 {"context_uri":"spotify:playlist:PL1"}`,
		"PUT /me/player/shuffle?device_id=D1&state=true",
		"POST /me/player/next?device_id=D1",
	}
	pos := 0
	for _, w := range want {
		i := strings.Index(got[pos:], w)
		if i < 0 {
			t.Fatalf("missing/out of order %q in:\n%s", w, got)
		}
		pos += i + len(w)
	}
}

func TestMissingDevice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			io.WriteString(w, `{"access_token":"AT","expires_in":3600}`)
			return
		}
		io.WriteString(w, `{"devices":[{"id":"D1","name":"Phone","is_active":true}]}`)
	}))
	defer srv.Close()
	c := New("id", "secret", "RT")
	c.APIURL, c.TokenURL = srv.URL, srv.URL+"/token"
	_, err := c.ResolveDevice(context.Background(), "Kitchen")
	if err == nil || !strings.Contains(err.Error(), `"Phone"`) {
		t.Fatalf("got %v", err)
	}
}
