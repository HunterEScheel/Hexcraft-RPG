package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/govee"
)

// LAN controls Govee lights on the local network.
type LAN interface {
	Power(ctx context.Context, l config.LANLight, on bool) error
	Brightness(ctx context.Context, l config.LANLight, pct int) error
	Color(ctx context.Context, l config.LANLight, r, g, b uint8) error
	ColorTemp(ctx context.Context, l config.LANLight, kelvin int) error
}

// lanClient finds each light's current address with a LAN scan (DHCP can
// change it), cached for a few minutes, falling back to the saved address.
type lanClient struct{}

var lanCache = struct {
	sync.Mutex
	ips map[string]string
	at  time.Time
}{ips: map[string]string{}}

// RefreshLAN rescans the network now (the daemon calls it at start-up so
// the first press is quick).
func RefreshLAN(ctx context.Context) error {
	devs, err := govee.LANDiscover(ctx, 1500*time.Millisecond)
	if err != nil {
		return err
	}
	lanCache.Lock()
	for _, d := range devs {
		lanCache.ips[d.Device] = d.IP
	}
	lanCache.at = time.Now()
	lanCache.Unlock()
	return nil
}

func (lanClient) ip(ctx context.Context, l config.LANLight) (string, error) {
	look := func() string {
		lanCache.Lock()
		defer lanCache.Unlock()
		for dev, ip := range lanCache.ips {
			if govee.SameDevice(dev, l.Device) {
				return ip
			}
		}
		return ""
	}
	lanCache.Lock()
	stale := time.Since(lanCache.at) > 5*time.Minute
	lanCache.Unlock()
	if ip := look(); ip != "" {
		if stale { // addresses can change (DHCP): check again, without waiting
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = RefreshLAN(ctx)
			}()
		}
		return ip, nil
	}
	_ = RefreshLAN(ctx)
	if ip := look(); ip != "" {
		return ip, nil
	}
	if l.IP != "" {
		return l.IP, nil
	}
	return "", fmt.Errorf("%s wasn't found on the network: is LAN Control on for it in the Govee Home app?", l.Name)
}

func (c lanClient) Power(ctx context.Context, l config.LANLight, on bool) error {
	ip, err := c.ip(ctx, l)
	if err != nil {
		return err
	}
	return govee.LANPower(ip, on)
}

func (c lanClient) Brightness(ctx context.Context, l config.LANLight, pct int) error {
	ip, err := c.ip(ctx, l)
	if err != nil {
		return err
	}
	return govee.LANBrightness(ip, pct)
}

func (c lanClient) Color(ctx context.Context, l config.LANLight, r, g, b uint8) error {
	ip, err := c.ip(ctx, l)
	if err != nil {
		return err
	}
	return govee.LANColor(ip, r, g, b)
}

func (c lanClient) ColorTemp(ctx context.Context, l config.LANLight, kelvin int) error {
	ip, err := c.ip(ctx, l)
	if err != nil {
		return err
	}
	return govee.LANColorTemp(ip, kelvin)
}
