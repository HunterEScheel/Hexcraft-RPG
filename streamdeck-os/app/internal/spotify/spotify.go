// Package spotify controls playback on Spotify Connect devices through the
// Spotify Web API. Requires a Spotify Premium account.
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Scopes = "user-read-playback-state user-modify-playback-state"

// RedirectURI must be added to the Spotify app's settings. Spotify only allows
// plain http for loopback addresses, so the browser lands on an error page and
// the user pastes that address into the installer page.
const RedirectURI = "http://127.0.0.1:8888/callback"

// Endpoints; overridable with environment variables for testing.
var (
	DefaultAPIURL   = env("STREAMDECKD_SPOTIFY_API_URL", "https://api.spotify.com/v1")
	DefaultTokenURL = env("STREAMDECKD_SPOTIFY_TOKEN_URL", "https://accounts.spotify.com/api/token")
	AuthorizeURL    = env("STREAMDECKD_SPOTIFY_AUTH_URL", "https://accounts.spotify.com/authorize")
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type Client struct {
	APIURL   string
	TokenURL string

	clientID, clientSecret string
	http                   *http.Client

	mu           sync.Mutex
	refreshToken string
	accessToken  string
	expires      time.Time
}

func New(clientID, clientSecret, refreshToken string) *Client {
	return &Client{
		APIURL:       DefaultAPIURL,
		TokenURL:     DefaultTokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

// APIError is an error returned by the Web API.
type APIError struct {
	Status  int
	Message string
	Reason  string
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("spotify: HTTP %d: %s", e.Status, e.Message)
	if e.Reason != "" {
		s += " (" + e.Reason + ")"
	}
	return s
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// ExchangeToken posts a form to the token endpoint with client credentials.
func ExchangeToken(ctx context.Context, hc *http.Client, tokenURL, clientID, clientSecret string, form url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify token: %w", err)
	}
	defer resp.Body.Close()
	var tr TokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tr); err != nil {
		return nil, fmt.Errorf("spotify token: HTTP %d: %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || tr.AccessToken == "" {
		return nil, fmt.Errorf("spotify token: HTTP %d: %s %s", resp.StatusCode, tr.Error, tr.ErrorDesc)
	}
	return &tr, nil
}

func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Until(c.expires) > time.Minute {
		return c.accessToken, nil
	}
	tr, err := ExchangeToken(ctx, c.http, c.TokenURL, c.clientID, c.clientSecret, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.refreshToken},
	})
	if err != nil {
		return "", err
	}
	c.accessToken = tr.AccessToken
	c.expires = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if tr.RefreshToken != "" {
		c.refreshToken = tr.RefreshToken
	}
	return c.accessToken, nil
}

func (c *Client) call(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	for attempt := 0; ; attempt++ {
		tok, err := c.token(ctx)
		if err != nil {
			return err
		}
		var rd io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			rd = bytes.NewReader(b)
		}
		u := c.APIURL + path
		if len(query) > 0 {
			u += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rd)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("spotify %s: %w", path, err)
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			wait, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			if wait >= 0 && wait <= 5 {
				select {
				case <-time.After(time.Duration(wait+1) * time.Second):
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			c.mu.Lock()
			c.accessToken = ""
			c.mu.Unlock()
			continue
		}
		if resp.StatusCode >= 300 {
			var e struct {
				Error struct {
					Status  int    `json:"status"`
					Message string `json:"message"`
					Reason  string `json:"reason"`
				} `json:"error"`
			}
			_ = json.Unmarshal(raw, &e)
			msg := e.Error.Message
			if msg == "" {
				msg = strings.TrimSpace(string(raw))
			}
			return &APIError{Status: resp.StatusCode, Message: msg, Reason: e.Error.Reason}
		}
		if out != nil && len(raw) > 0 {
			return json.Unmarshal(raw, out)
		}
		return nil
	}
}

type Device struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	IsActive      bool   `json:"is_active"`
	VolumePercent *int   `json:"volume_percent"`
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var r struct {
		Devices []Device `json:"devices"`
	}
	err := c.call(ctx, http.MethodGet, "/me/player/devices", nil, nil, &r)
	return r.Devices, err
}

// ResolveDevice finds a device id by name. An empty name means the active device.
// Spotify Connect devices sometimes take a moment to show up, so it retries briefly.
func (c *Client) ResolveDevice(ctx context.Context, name string) (string, error) {
	var last []Device
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		devs, err := c.Devices(ctx)
		if err != nil {
			return "", err
		}
		last = devs
		for _, d := range devs {
			if (name == "" && d.IsActive) || (name != "" && strings.EqualFold(d.Name, name)) {
				return d.ID, nil
			}
		}
	}
	names := make([]string, 0, len(last))
	for _, d := range last {
		names = append(names, fmt.Sprintf("%q", d.Name))
	}
	if name == "" {
		return "", fmt.Errorf("spotify: no active device and no device named (visible: %s)", strings.Join(names, ", "))
	}
	return "", fmt.Errorf("spotify: device %q not visible — is it powered on and logged in? (visible: %s)", name, strings.Join(names, ", "))
}

// PlayOptions configure PlayContext.
type PlayOptions struct {
	Shuffle bool
	Volume  *int
}

// PlayContext starts a playlist/album/artist on the named device.
func (c *Client) PlayContext(ctx context.Context, deviceName, uri string, opt PlayOptions) error {
	ctxURI, err := NormalizeURI(uri)
	if err != nil {
		return err
	}
	id, err := c.ResolveDevice(ctx, deviceName)
	if err != nil {
		return err
	}
	q := url.Values{"device_id": {id}}
	if opt.Volume != nil {
		// Set volume first so the first note isn't blasted at the old level.
		_ = c.call(ctx, http.MethodPut, "/me/player/volume",
			url.Values{"device_id": {id}, "volume_percent": {strconv.Itoa(*opt.Volume)}}, nil, nil)
	}
	play := func() error {
		return c.call(ctx, http.MethodPut, "/me/player/play", q, map[string]any{"context_uri": ctxURI}, nil)
	}
	if err := play(); err != nil {
		var ae *APIError
		if !errors.As(err, &ae) || ae.Status != http.StatusNotFound {
			return err
		}
		// Device not active yet: transfer playback to it, then try again.
		if terr := c.call(ctx, http.MethodPut, "/me/player", nil,
			map[string]any{"device_ids": []string{id}, "play": false}, nil); terr != nil {
			return fmt.Errorf("%w (transfer also failed: %v)", err, terr)
		}
		time.Sleep(700 * time.Millisecond)
		if err := play(); err != nil {
			return err
		}
	}
	shuffle := url.Values{"device_id": {id}, "state": {strconv.FormatBool(opt.Shuffle)}}
	if err := c.call(ctx, http.MethodPut, "/me/player/shuffle", shuffle, nil, nil); err != nil {
		return err
	}
	if opt.Shuffle {
		// Playback always starts at track 1; skip once so shuffle picks a random start.
		return c.call(ctx, http.MethodPost, "/me/player/next", url.Values{"device_id": {id}}, nil, nil)
	}
	return nil
}

func (c *Client) deviceQuery(ctx context.Context, deviceName string) (url.Values, error) {
	if deviceName == "" {
		return url.Values{}, nil
	}
	id, err := c.ResolveDevice(ctx, deviceName)
	if err != nil {
		return nil, err
	}
	return url.Values{"device_id": {id}}, nil
}

func (c *Client) Pause(ctx context.Context, deviceName string) error {
	q, err := c.deviceQuery(ctx, deviceName)
	if err != nil {
		return err
	}
	err = c.call(ctx, http.MethodPut, "/me/player/pause", q, nil, nil)
	var ae *APIError
	if errors.As(err, &ae) && ae.Reason == "UNKNOWN" && ae.Status == http.StatusForbidden {
		return nil // already paused
	}
	return err
}

func (c *Client) Next(ctx context.Context, deviceName string) error {
	q, err := c.deviceQuery(ctx, deviceName)
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, "/me/player/next", q, nil, nil)
}

func (c *Client) Volume(ctx context.Context, deviceName string, pct int) error {
	q, err := c.deviceQuery(ctx, deviceName)
	if err != nil {
		return err
	}
	q.Set("volume_percent", strconv.Itoa(pct))
	return c.call(ctx, http.MethodPut, "/me/player/volume", q, nil, nil)
}

var contextTypes = map[string]bool{"playlist": true, "album": true, "artist": true, "show": true}

// NormalizeURI turns spotify:playlist:ID or https://open.spotify.com/playlist/ID?si=…
// (also album/artist/show, and /intl-xx/ links) into a spotify: context URI.
func NormalizeURI(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "spotify:") {
		parts := strings.Split(s, ":")
		if len(parts) >= 3 && contextTypes[parts[len(parts)-2]] && parts[len(parts)-1] != "" {
			return "spotify:" + parts[len(parts)-2] + ":" + parts[len(parts)-1], nil
		}
		return "", fmt.Errorf("spotify: unsupported URI %q", s)
	}
	u, err := url.Parse(s)
	if err != nil || !strings.HasSuffix(u.Host, "spotify.com") {
		return "", fmt.Errorf("spotify: %q is not a spotify: URI or open.spotify.com link", s)
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(segs); i++ {
		if contextTypes[segs[i]] && segs[i+1] != "" {
			return "spotify:" + segs[i] + ":" + segs[i+1], nil
		}
	}
	return "", fmt.Errorf("spotify: can't find a playlist/album/artist in %q", s)
}
