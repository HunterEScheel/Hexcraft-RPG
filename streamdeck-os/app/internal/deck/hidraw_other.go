//go:build !linux

package deck

import "errors"

// Device is only implemented on Linux; other platforms can still run the
// helper subcommands (spotify-auth, check, govee-list, press).
type Device struct {
	Model Model
	Path  string
}

func Open() (*Device, error)                    { return nil, errors.New("Stream Deck access needs Linux hidraw") }
func (d *Device) Close() error                  { return nil }
func (d *Device) Reset() error                  { return nil }
func (d *Device) SetBrightness(int) error       { return nil }
func (d *Device) SetKeyImage(int, []byte) error { return nil }
func (d *Device) ReadKeys() ([]bool, error)     { return nil, errors.New("unsupported") }

func Present() string { return "" }
