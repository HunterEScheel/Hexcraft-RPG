package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/govee"
	"streamdeckd/internal/runner"
	"streamdeckd/internal/spotify"
)

// runCheck validates the config and, where credentials exist, checks that
// every referenced Govee device/scene and Spotify device can be found.
func runCheck(ctx context.Context, path, secrets string) error {
	cfg, err := config.LoadAll(path, secrets)
	if err != nil {
		return err
	}
	fmt.Printf("config OK: %d page(s), start page %q\n", len(cfg.Pages), cfg.StartPage)
	problems := 0

	if cfg.Govee.APIKey != "" {
		g := govee.New(cfg.Govee.APIKey)
		devs, err := g.Devices(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("govee OK: %d device(s)\n", len(devs))
		for _, page := range cfg.PageNames() {
			for _, b := range cfg.Pages[page] {
				for _, a := range b.Actions {
					if !strings.HasPrefix(a.Type, "govee_") {
						continue
					}
					where := fmt.Sprintf("  %s key %d %q:", page, b.Key, b.Label)
					d, err := g.FindDevice(ctx, a.Device)
					if err != nil {
						fmt.Println(where, err)
						problems++
						continue
					}
					if a.Type == config.GoveeScene || a.Type == config.GoveeDIYScene {
						opts, err := g.Scenes(ctx, d, a.Type == config.GoveeDIYScene)
						if err != nil {
							fmt.Println(where, err)
							problems++
							continue
						}
						found := false
						for _, o := range opts {
							found = found || strings.EqualFold(o.Name, a.Scene)
						}
						if !found {
							fmt.Printf("%s scene %q not found on %q\n", where, a.Scene, d.Name)
							problems++
						}
					}
				}
			}
		}
	}

	if cfg.Spotify.RefreshToken != "" {
		s := spotify.New(cfg.Spotify.ClientID, cfg.Spotify.ClientSecret, cfg.Spotify.RefreshToken)
		devs, err := s.Devices(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("spotify OK: %d device(s) visible right now\n", len(devs))
		visible := map[string]bool{}
		for _, d := range devs {
			visible[strings.ToLower(d.Name)] = true
		}
		want := map[string]bool{}
		if cfg.Spotify.DefaultDevice != "" {
			want[cfg.Spotify.DefaultDevice] = true
		}
		for _, page := range cfg.PageNames() {
			for _, b := range cfg.Pages[page] {
				for _, a := range b.Actions {
					if a.SpotifyDevice != "" {
						want[a.SpotifyDevice] = true
					}
					if a.Type == config.SpotifyPlaylist {
						if _, err := spotify.NormalizeURI(a.Playlist); err != nil {
							fmt.Printf("  %s key %d: %v\n", page, b.Key, err)
							problems++
						}
					}
				}
			}
		}
		for name := range want {
			if !visible[strings.ToLower(name)] {
				fmt.Printf("  note: Spotify device %q is not visible now (it must be on and logged in when pressed)\n", name)
			}
		}
	}
	if problems > 0 {
		return fmt.Errorf("%d problem(s) found", problems)
	}
	fmt.Println("all good")
	return nil
}

func runGoveeList(ctx context.Context, secrets, key string) error {
	if key == "" {
		s, err := config.LoadSecrets(secrets)
		if err != nil {
			return fmt.Errorf("%w (run setup, or pass -api-key)", err)
		}
		key = s.Govee.APIKey
	}
	if key == "" {
		return errors.New("no Govee API key")
	}
	g := govee.New(key)
	devs, err := g.Devices(ctx)
	if err != nil {
		return err
	}
	for _, d := range govee.Lights(devs) {
		fmt.Printf("%q  sku=%s  id=%s\n", d.Name, d.SKU, d.ID)
		for _, diy := range []bool{false, true} {
			opts, err := g.Scenes(ctx, d, diy)
			kind := "scenes"
			if diy {
				kind = "DIY scenes"
			}
			if err != nil {
				fmt.Printf("    %s: error: %v\n", kind, err)
				continue
			}
			if len(opts) == 0 {
				continue
			}
			names := make([]string, len(opts))
			for i, o := range opts {
				names[i] = o.Name
			}
			fmt.Printf("    %s: %s\n", kind, strings.Join(names, ", "))
		}
	}
	return nil
}

func runSpotifyDevices(ctx context.Context, secrets string) error {
	sec, err := config.LoadSecrets(secrets)
	if err != nil {
		return fmt.Errorf("%w (run setup first)", err)
	}
	s := spotify.New(sec.Spotify.ClientID, sec.Spotify.ClientSecret, sec.Spotify.RefreshToken)
	devs, err := s.Devices(ctx)
	if err != nil {
		return err
	}
	for _, d := range devs {
		active := ""
		if d.IsActive {
			active = "  (active)"
		}
		fmt.Printf("%q  type=%s%s\n", d.Name, d.Type, active)
	}
	if len(devs) == 0 {
		fmt.Println("no devices visible — open Spotify on the target device first")
	}
	return nil
}

func runPress(ctx context.Context, path, secrets, page string, key int) error {
	cfg, err := config.LoadAll(path, secrets)
	if err != nil {
		return err
	}
	if page == "" {
		page = cfg.StartPage
	}
	b, ok := cfg.Button(page, key)
	if !ok {
		return fmt.Errorf("no button with key %d on page %q", key, page)
	}
	fmt.Printf("running %q (%d action(s))\n", b.Label, len(b.Actions))
	res := runner.New(cfg).Run(ctx, b.Actions)
	if res.SwitchTo != "" {
		fmt.Printf("would switch to page %q\n", res.SwitchTo)
	}
	if res.Err != nil {
		return res.Err
	}
	fmt.Println("done")
	return nil
}

// runSpotifyAuth performs the OAuth authorization-code flow once, on a
// computer with a browser, and prints the refresh token for config.json.
// It's an alternative to the on-device login, e.g. for testing from a laptop.
func runSpotifyAuth(clientID, clientSecret string) error {
	if clientID == "" || clientSecret == "" {
		return errors.New("-client-id and -client-secret are required (from developer.spotify.com/dashboard)")
	}
	redirect := spotify.RedirectURI
	port := 8888
	var sb [16]byte
	_, _ = rand.Read(sb[:])
	state := hex.EncodeToString(sb[:])

	authURL := spotify.AuthorizeURL + "?" + url.Values{
		"client_id":     {clientID},
		"response_type": {"code"},
		"redirect_uri":  {redirect},
		"scope":         {spotify.Scopes},
		"state":         {state},
	}.Encode()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	result := make(chan error, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			fmt.Fprintf(w, "Spotify said: %s", e)
			result <- fmt.Errorf("authorization failed: %s", e)
			return
		}
		tr, err := spotify.ExchangeToken(r.Context(), http.DefaultClient, spotify.DefaultTokenURL,
			clientID, clientSecret, url.Values{
				"grant_type":   {"authorization_code"},
				"code":         {q.Get("code")},
				"redirect_uri": {redirect},
			})
		if err != nil {
			fmt.Fprintf(w, "token exchange failed: %v", err)
			result <- err
			return
		}
		fmt.Fprintln(w, "Done! Return to the terminal; you can close this tab.")
		fmt.Printf("\nrefresh token: %s\n", tr.RefreshToken)
		result <- nil
	})
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	fmt.Fprintf(os.Stderr, "1. In your Spotify app settings, add this Redirect URI exactly:\n   %s\n", redirect)
	fmt.Fprintf(os.Stderr, "2. Open this URL in a browser on THIS computer and approve:\n   %s\n\nWaiting...\n", authURL)
	return <-result
}
