// Package config loads and validates the appliance configuration file
// (by default /boot/streamdeck/config.json on the SD card's FAT partition).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// Config is the whole appliance configuration.
type Config struct {
	// Brightness of the deck, 0-100.
	Brightness int `json:"brightness"`
	// Dim the deck after this many idle seconds (0 = never).
	IdleDimSeconds int `json:"idle_dim_seconds"`
	// Brightness while dimmed, 0-100.
	IdleBrightness int `json:"idle_brightness"`
	// Page shown at startup. Defaults to "home".
	StartPage string `json:"start_page"`

	Govee   GoveeConfig         `json:"govee,omitzero"`
	Spotify SpotifyConfig       `json:"spotify,omitzero"`
	Pages   map[string][]Button `json:"pages"`
	// LANLights are Govee lights controlled on the local network (no cloud).
	LANLights []LANLight `json:"lan_lights,omitempty"`

	// Dir is the directory the config was loaded from; icon paths are relative to it.
	Dir string `json:"-"`
}

// LANLight is a Govee light controlled over the LAN API, named by the user
// (the LAN API doesn't know names). Actions refer to it by Name.
type LANLight struct {
	Name   string `json:"name"`
	Device string `json:"device"` // id from the LAN scan (MAC-like)
	SKU    string `json:"sku"`
	IP     string `json:"ip"` // last known address; refreshed by scans
}

// LANLight finds a LAN light by name (case-insensitive).
func (c *Config) LANLight(name string) (LANLight, bool) {
	return FindLANLight(c.LANLights, name)
}

// FindLANLight finds a LAN light by name (case-insensitive).
func FindLANLight(ls []LANLight, name string) (LANLight, bool) {
	for _, l := range ls {
		if strings.EqualFold(l.Name, name) {
			return l, true
		}
	}
	return LANLight{}, false
}

type GoveeConfig struct {
	APIKey string `json:"api_key"`
}

type SpotifyConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	// Spotify Connect device name used when an action names none.
	DefaultDevice string `json:"default_device"`
}

// Button is one key on one page.
type Button struct {
	Key       int      `json:"key"`
	Label     string   `json:"label"`
	Color     string   `json:"color"`
	TextColor string   `json:"text_color"`
	Icon      string   `json:"icon"`
	Actions   []Action `json:"actions"`
}

// Action types.
const (
	GoveeScene      = "govee_scene"
	GoveeDIYScene   = "govee_diy_scene"
	GoveePower      = "govee_power"
	GoveeBrightness = "govee_brightness"
	GoveeColor      = "govee_color"
	SpotifyPlaylist = "spotify_playlist"
	SpotifyPause    = "spotify_pause"
	SpotifyNext     = "spotify_next"
	SpotifyVolume   = "spotify_volume"
	Page            = "page"
	Delay           = "delay"
)

// Action is a single step a button performs. Fields used depend on Type.
type Action struct {
	Type string `json:"type"`

	// Govee: device name (as shown in the Govee app) or device id (MAC).
	Device     string `json:"device,omitempty"`
	Scene      string `json:"scene,omitempty"`
	On         *bool  `json:"on,omitempty"`
	Brightness int    `json:"brightness,omitempty"`
	Color      string `json:"color,omitempty"`

	// Spotify: playlist/album/artist URI or open.spotify.com link.
	Playlist string `json:"playlist,omitempty"`
	Shuffle  bool   `json:"shuffle,omitempty"`
	Volume   *int   `json:"volume,omitempty"`
	// Spotify uses SpotifyDevice; falls back to spotify.default_device.
	SpotifyDevice string `json:"spotify_device,omitempty"`

	// page: page to switch to.
	Page string `json:"page,omitempty"`

	// delay: milliseconds to wait.
	Ms int `json:"ms,omitempty"`
}

// DefaultSecretsPath is where the on-device installer stores API credentials.
const DefaultSecretsPath = "/var/lib/streamdeck/secrets.json"

// Secrets holds credentials entered in the installer. They are kept apart
// from the button layout so the layout can be shared/edited freely.
type Secrets struct {
	Govee   GoveeConfig   `json:"govee"`
	Spotify SpotifyConfig `json:"spotify"`
}

// Load reads, defaults and validates a config file (no secrets merged).
func Load(path string) (*Config, error) {
	return LoadAll(path, "")
}

// LoadAll reads the layout config, overlays secrets (if secretsPath is set
// and exists), then validates the result.
func LoadAll(path, secretsPath string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if abs, err := filepath.Abs(path); err == nil {
		c.Dir = filepath.Dir(abs)
	}
	if secretsPath != "" {
		s, err := LoadSecrets(secretsPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if s != nil {
			c.ApplySecrets(s)
		}
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// ApplySecrets overrides credentials with any non-empty secret values.
func (c *Config) ApplySecrets(s *Secrets) {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&c.Govee.APIKey, s.Govee.APIKey)
	set(&c.Spotify.ClientID, s.Spotify.ClientID)
	set(&c.Spotify.ClientSecret, s.Spotify.ClientSecret)
	set(&c.Spotify.RefreshToken, s.Spotify.RefreshToken)
	set(&c.Spotify.DefaultDevice, s.Spotify.DefaultDevice)
}

func LoadSecrets(path string) (*Secrets, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Secrets
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

// SaveSecrets writes secrets atomically with mode 0600.
func SaveSecrets(path string, s *Secrets) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	syscall.Sync() // FAT: get the rename and allocation tables onto the disk now
	return nil
}

// Parse decodes and validates config JSON.
func Parse(raw []byte) (*Config, error) {
	c, err := parse(raw)
	if err != nil {
		return nil, err
	}
	return c, c.Validate()
}

func parse(raw []byte) (*Config, error) {
	c := &Config{Brightness: 60, IdleBrightness: 10}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if c.StartPage == "" {
		c.StartPage = "home"
	}
	return c, nil
}

// Validate checks the config for mistakes and returns all of them at once.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if c.Brightness < 0 || c.Brightness > 100 {
		add("brightness must be 0-100")
	}
	if c.IdleBrightness < 0 || c.IdleBrightness > 100 {
		add("idle_brightness must be 0-100")
	}
	if len(c.Pages) == 0 {
		add("no pages defined")
	} else if _, ok := c.Pages[c.StartPage]; !ok {
		add("start_page %q is not defined in pages", c.StartPage)
	}

	usesGovee, usesSpotify := false, false
	for _, name := range c.PageNames() {
		seen := map[int]bool{}
		for i, b := range c.Pages[name] {
			where := fmt.Sprintf("pages.%s[%d]", name, i)
			if b.Key < 0 {
				add("%s: key must be >= 0", where)
			}
			if seen[b.Key] {
				add("%s: key %d used twice on page %q", where, b.Key, name)
			}
			seen[b.Key] = true
			for _, col := range []string{b.Color, b.TextColor} {
				if col != "" {
					if _, err := ParseColor(col); err != nil {
						add("%s: %v", where, err)
					}
				}
			}
			for j, a := range b.Actions {
				aw := fmt.Sprintf("%s.actions[%d] (%s)", where, j, a.Type)
				_, lan := c.LANLight(a.Device)
				switch a.Type {
				case GoveeScene, GoveeDIYScene:
					usesGovee = true
					if a.Device == "" || a.Scene == "" {
						add("%s: needs device and scene", aw)
					}
					if lan {
						add("%s: %s is controlled on your network, and Govee's LAN control has no scenes", aw, a.Device)
					}
				case GoveePower:
					usesGovee = usesGovee || !lan
					if a.Device == "" || a.On == nil {
						add("%s: needs device and on (true/false)", aw)
					}
				case GoveeBrightness:
					usesGovee = usesGovee || !lan
					if a.Device == "" || a.Brightness < 1 || a.Brightness > 100 {
						add("%s: needs device and brightness 1-100", aw)
					}
				case GoveeColor:
					usesGovee = usesGovee || !lan
					if a.Device == "" {
						add("%s: needs device", aw)
					}
					if _, err := ParseColor(a.Color); err != nil {
						add("%s: %v", aw, err)
					}
				case SpotifyPlaylist:
					usesSpotify = true
					if a.Playlist == "" {
						add("%s: needs playlist", aw)
					}
					if a.Volume != nil && (*a.Volume < 0 || *a.Volume > 100) {
						add("%s: volume must be 0-100", aw)
					}
				case SpotifyPause, SpotifyNext:
					usesSpotify = true
				case SpotifyVolume:
					usesSpotify = true
					if a.Volume == nil || *a.Volume < 0 || *a.Volume > 100 {
						add("%s: needs volume 0-100", aw)
					}
				case Page:
					if _, ok := c.Pages[a.Page]; !ok {
						add("%s: page %q is not defined", aw, a.Page)
					}
				case Delay:
					if a.Ms <= 0 || a.Ms > 60000 {
						add("%s: ms must be 1-60000", aw)
					}
				default:
					add("%s: unknown action type", aw)
				}
			}
		}
	}
	seenLAN := map[string]bool{}
	for i, l := range c.LANLights {
		if l.Name == "" || l.Device == "" {
			add("lan_lights[%d]: needs name and device", i)
		}
		if seenLAN[strings.ToLower(l.Name)] {
			add("lan_lights[%d]: two lights are called %q", i, l.Name)
		}
		seenLAN[strings.ToLower(l.Name)] = true
	}
	if usesGovee && c.Govee.APIKey == "" {
		add("Govee API key missing (run the installer: streamdeckd setup)")
	}
	if usesSpotify && (c.Spotify.ClientID == "" || c.Spotify.ClientSecret == "" || c.Spotify.RefreshToken == "") {
		add("Spotify login missing (run the installer: streamdeckd setup)")
	}
	return errors.Join(errs...)
}

// PageNames returns page names sorted, for stable output.
func (c *Config) PageNames() []string {
	names := make([]string, 0, len(c.Pages))
	for n := range c.Pages {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Button returns the button bound to key on page, if any.
func (c *Config) Button(page string, key int) (Button, bool) {
	for _, b := range c.Pages[page] {
		if b.Key == key {
			return b, true
		}
	}
	return Button{}, false
}

// ParseColor parses "#rrggbb" or "rrggbb".
func ParseColor(s string) (color.RGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) != 6 {
		return color.RGBA{}, fmt.Errorf("color %q must look like #rrggbb", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("color %q must look like #rrggbb", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}
