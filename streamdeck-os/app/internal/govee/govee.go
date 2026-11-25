// Package govee talks to the Govee Developer Platform API (cloud, v1 "router" API).
// Docs: https://developer.govee.com/reference
package govee

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const DefaultBaseURL = "https://openapi.api.govee.com/router/api/v1"

type Client struct {
	BaseURL string
	apiKey  string
	http    *http.Client

	mu         sync.Mutex
	devices    []Device
	devicesAt  time.Time
	sceneCache map[string][]Option // key: instance + "|" + device id
}

func New(apiKey string) *Client {
	base := DefaultBaseURL
	if v := os.Getenv("STREAMDECKD_GOVEE_URL"); v != "" { // for tests
		base = v
	}
	return &Client{
		BaseURL:    base,
		apiKey:     apiKey,
		http:       &http.Client{Timeout: 15 * time.Second},
		sceneCache: map[string][]Option{},
	}
}

type Device struct {
	SKU          string       `json:"sku"`
	ID           string       `json:"device"`
	Name         string       `json:"deviceName"`
	Type         string       `json:"type"`
	Capabilities []Capability `json:"capabilities"`
}

// IsGroup reports whether this is a group of lights made in the Govee app
// (e.g. SKU "SameModeGroup") rather than a single light.
func (d Device) IsGroup() bool {
	return d.Type == "devices.types.group" || strings.HasSuffix(strings.ToLower(d.SKU), "group")
}

// Lights returns only the individual lights (no groups).
func Lights(devs []Device) []Device {
	out := make([]Device, 0, len(devs))
	for _, d := range devs {
		if !d.IsGroup() {
			out = append(out, d)
		}
	}
	return out
}

type Capability struct {
	Type       string     `json:"type"`
	Instance   string     `json:"instance"`
	Parameters Parameters `json:"parameters"`
}

type Parameters struct {
	DataType string   `json:"dataType"`
	Options  []Option `json:"options"`
	Range    *struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"range,omitempty"`
}

// Option is a named scene; Value is sent back to Govee verbatim.
type Option struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type envelope struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Payload json.RawMessage `json:"payload"`
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*envelope, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Govee-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("govee %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var env envelope
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode != http.StatusOK || (env.Code != 0 && env.Code != 200) {
		msg := env.Msg
		if msg == "" {
			msg = env.Message
		}
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return nil, fmt.Errorf("govee %s: HTTP %d: %s", path, resp.StatusCode, msg)
	}
	return &env, nil
}

func requestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Devices lists the account's devices (cached for 10 minutes).
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	c.mu.Lock()
	if c.devices != nil && time.Since(c.devicesAt) < 10*time.Minute {
		d := c.devices
		c.mu.Unlock()
		return d, nil
	}
	c.mu.Unlock()
	env, err := c.do(ctx, http.MethodGet, "/user/devices", nil)
	if err != nil {
		return nil, err
	}
	var devs []Device
	if err := json.Unmarshal(env.Data, &devs); err != nil {
		return nil, fmt.Errorf("govee devices: %w", err)
	}
	c.mu.Lock()
	c.devices, c.devicesAt = devs, time.Now()
	c.mu.Unlock()
	return devs, nil
}

// FindDevice matches by device name (case-insensitive) or device id.
func (c *Client) FindDevice(ctx context.Context, nameOrID string) (Device, error) {
	devs, err := c.Devices(ctx)
	if err != nil {
		return Device{}, err
	}
	for _, d := range devs {
		if strings.EqualFold(d.Name, nameOrID) || strings.EqualFold(d.ID, nameOrID) {
			return d, nil
		}
	}
	names := make([]string, 0, len(devs))
	for _, d := range devs {
		names = append(names, fmt.Sprintf("%q", d.Name))
	}
	return Device{}, fmt.Errorf("govee device %q not found (have: %s)", nameOrID, strings.Join(names, ", "))
}

// Scenes returns the dynamic ("lightScene") or DIY ("diyScene") scenes of a device.
func (c *Client) Scenes(ctx context.Context, d Device, diy bool) ([]Option, error) {
	path, instance := "/device/scenes", "lightScene"
	if diy {
		path, instance = "/device/diy-scenes", "diyScene"
	}
	key := instance + "|" + d.ID
	c.mu.Lock()
	if s, ok := c.sceneCache[key]; ok {
		c.mu.Unlock()
		return s, nil
	}
	c.mu.Unlock()

	env, err := c.do(ctx, http.MethodPost, path, map[string]any{
		"requestId": requestID(),
		"payload":   map[string]string{"sku": d.SKU, "device": d.ID},
	})
	if err != nil {
		return nil, err
	}
	var p struct {
		Capabilities []Capability `json:"capabilities"`
	}
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return nil, fmt.Errorf("govee scenes: %w", err)
	}
	var opts []Option
	for _, cp := range p.Capabilities {
		if cp.Instance == instance {
			opts = append(opts, cp.Parameters.Options...)
		}
	}
	c.mu.Lock()
	c.sceneCache[key] = opts
	c.mu.Unlock()
	return opts, nil
}

func (c *Client) control(ctx context.Context, d Device, typ, instance string, value any) error {
	_, err := c.do(ctx, http.MethodPost, "/device/control", map[string]any{
		"requestId": requestID(),
		"payload": map[string]any{
			"sku":    d.SKU,
			"device": d.ID,
			"capability": map[string]any{
				"type":     typ,
				"instance": instance,
				"value":    value,
			},
		},
	})
	return err
}

// ApplyScene applies a scene by name (case-insensitive) to a device.
func (c *Client) ApplyScene(ctx context.Context, device, scene string, diy bool) error {
	d, err := c.FindDevice(ctx, device)
	if err != nil {
		return err
	}
	opts, err := c.Scenes(ctx, d, diy)
	if err != nil {
		return err
	}
	for _, o := range opts {
		if strings.EqualFold(o.Name, scene) {
			instance := "lightScene"
			if diy {
				instance = "diyScene"
			}
			return c.control(ctx, d, "devices.capabilities.dynamic_scene", instance, o.Value)
		}
	}
	return fmt.Errorf("govee: scene %q not found on %q (run `streamdeckd govee-list` to see names)", scene, d.Name)
}

func (c *Client) Power(ctx context.Context, device string, on bool) error {
	d, err := c.FindDevice(ctx, device)
	if err != nil {
		return err
	}
	v := 0
	if on {
		v = 1
	}
	return c.control(ctx, d, "devices.capabilities.on_off", "powerSwitch", v)
}

func (c *Client) Brightness(ctx context.Context, device string, pct int) error {
	d, err := c.FindDevice(ctx, device)
	if err != nil {
		return err
	}
	return c.control(ctx, d, "devices.capabilities.range", "brightness", pct)
}

// ColorTemp sets a white colour temperature in kelvin (clamped to the
// light's range). Lights are brightest in this mode.
func (c *Client) ColorTemp(ctx context.Context, device string, kelvin int) error {
	d, err := c.FindDevice(ctx, device)
	if err != nil {
		return err
	}
	for _, cp := range d.Capabilities {
		if cp.Instance == "colorTemperatureK" && cp.Parameters.Range != nil && cp.Parameters.Range.Max > 0 {
			kelvin = max(cp.Parameters.Range.Min, min(cp.Parameters.Range.Max, kelvin))
		}
	}
	return c.control(ctx, d, "devices.capabilities.color_setting", "colorTemperatureK", kelvin)
}

func (c *Client) Color(ctx context.Context, device string, r, g, b uint8) error {
	d, err := c.FindDevice(ctx, device)
	if err != nil {
		return err
	}
	return c.control(ctx, d, "devices.capabilities.color_setting", "colorRgb", int(r)<<16|int(g)<<8|int(b))
}
