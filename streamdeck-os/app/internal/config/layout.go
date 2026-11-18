package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Layout is the part of the config the button editor reads and writes:
// everything except credentials (those stay in the secrets file).
type Layout struct {
	Brightness     int                 `json:"brightness"`
	IdleDimSeconds int                 `json:"idle_dim_seconds"`
	IdleBrightness int                 `json:"idle_brightness"`
	StartPage      string              `json:"start_page"`
	Pages          map[string][]Button `json:"pages"`
}

// LayoutOf extracts the layout from a config.
func LayoutOf(c *Config) Layout {
	return Layout{Brightness: c.Brightness, IdleDimSeconds: c.IdleDimSeconds, IdleBrightness: c.IdleBrightness,
		StartPage: c.StartPage, Pages: c.Pages}
}

// ReadLayout reads the layout file without validating it (so the editor
// can open, and fix, a layout that currently has problems).
func ReadLayout(path string) (Layout, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Layout{}, err
	}
	c, err := parse(raw)
	if err != nil {
		return Layout{}, err
	}
	l := LayoutOf(c)
	if l.Pages == nil {
		l.Pages = map[string][]Button{}
	}
	return l, nil
}

// CheckLayout validates a layout together with the saved credentials, the
// same way the daemon will when it loads it.
func CheckLayout(l Layout, secrets *Secrets) error {
	c := &Config{Brightness: l.Brightness, IdleDimSeconds: l.IdleDimSeconds, IdleBrightness: l.IdleBrightness,
		StartPage: l.StartPage, Pages: l.Pages}
	if c.StartPage == "" {
		c.StartPage = "home"
	}
	if secrets != nil {
		c.ApplySecrets(secrets)
	}
	return c.Validate()
}

// SaveLayout writes the layout atomically (the daemon picks it up within seconds).
func SaveLayout(path string, l Layout) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(l); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
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
		return fmt.Errorf("saving layout: %w", err)
	}
	syscall.Sync() // FAT: get the rename and allocation tables onto the disk now
	return nil
}
