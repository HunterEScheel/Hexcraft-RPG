package govee

// Govee's LAN API: lights on the local network, controlled without the
// cloud. It is UDP: a "scan" sent to a multicast address finds the lights
// (they answer on port 4002), and commands go to each light's port 4003.
// LAN control must be switched on per light in the Govee Home app.
//
// The LAN API turns lights on and off and sets brightness and colour;
// it has no scenes.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Addresses of the LAN protocol (variables so tests can use loopback).
var (
	LANScanAddrs  = []string{"239.255.255.250:4001", "255.255.255.255:4001"}
	LANListenPort = 4002
	LANCmdPort    = 4003
)

func init() {
	// Test images add a loopback scan address (their fake lights).
	if v := os.Getenv("STREAMDECKD_GOVEE_LAN_SCAN"); v != "" {
		LANScanAddrs = append(LANScanAddrs, v)
	}
}

// LANDevice is a light that answered a scan.
type LANDevice struct {
	IP     string `json:"ip"`
	Device string `json:"device"` // MAC-like id, same as the cloud API's
	SKU    string `json:"sku"`
}

type lanMsg struct {
	Msg struct {
		Cmd  string          `json:"cmd"`
		Data json.RawMessage `json:"data"`
	} `json:"msg"`
}

func lanPacket(cmd string, data any) []byte {
	b, _ := json.Marshal(map[string]any{"msg": map[string]any{"cmd": cmd, "data": data}})
	return b
}

// listenReplies binds the reply port. Only one scan can listen at a time
// (replies would otherwise be split between the daemon and the app), so
// wait for the port while another scan finishes.
func listenReplies(ctx context.Context) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		})
		if err != nil {
			return err
		}
		return serr
	}}
	for i := 0; ; i++ {
		pc, err := lc.ListenPacket(ctx, "udp4", fmt.Sprintf(":%d", LANListenPort))
		if err == nil || !errors.Is(err, syscall.EADDRINUSE) || i >= 40 {
			return pc, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// LANDiscover scans the network for lights with LAN control switched on,
// listening for answers for `wait`.
func LANDiscover(ctx context.Context, wait time.Duration) ([]LANDevice, error) {
	pc, err := listenReplies(ctx)
	if err != nil {
		return nil, fmt.Errorf("govee LAN: %w", err)
	}
	defer pc.Close()
	scan := lanPacket("scan", map[string]string{"account_topic": "reserve"})
	sent := 0
	var sendErr error
	send := func() {
		for _, a := range LANScanAddrs {
			ua, err := net.ResolveUDPAddr("udp4", a)
			if err != nil {
				continue
			}
			if _, err := pc.WriteTo(scan, ua); err != nil {
				sendErr = err
				continue
			}
			sent++
		}
	}
	send()
	if sent == 0 {
		return nil, fmt.Errorf("govee LAN: can't send a scan (is the network up?): %v", sendErr)
	}
	found := map[string]LANDevice{}
	end := time.Now().Add(wait)
	resent := false
	buf := make([]byte, 2048)
	for time.Now().Before(end) {
		if !resent && time.Until(end) < wait/2 {
			send() // UDP: ask twice
			resent = true
		}
		dl := time.Now().Add(150 * time.Millisecond)
		if dl.After(end) {
			dl = end
		}
		_ = pc.SetReadDeadline(dl)
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		var m lanMsg
		if json.Unmarshal(buf[:n], &m) != nil || m.Msg.Cmd != "scan" {
			continue
		}
		var d LANDevice
		if json.Unmarshal(m.Msg.Data, &d) != nil || d.Device == "" {
			continue
		}
		if d.IP == "" {
			if ua, ok := from.(*net.UDPAddr); ok {
				d.IP = ua.IP.String()
			}
		}
		found[d.Device] = d
	}
	out := make([]LANDevice, 0, len(found))
	for _, d := range found {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Device < out[j].Device })
	return out, nil
}

func lanSend(ip string, pkt []byte) error {
	if ip == "" {
		return errors.New("govee LAN: light's address unknown (not found on the network)")
	}
	c, err := net.Dial("udp4", net.JoinHostPort(ip, fmt.Sprint(LANCmdPort)))
	if err != nil {
		return fmt.Errorf("govee LAN: %w", err)
	}
	defer c.Close()
	// UDP has no delivery guarantee: send twice; the commands are idempotent.
	for i := 0; i < 2; i++ {
		if _, err := c.Write(pkt); err != nil {
			return fmt.Errorf("govee LAN: %w", err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}

// LANPower turns a light on or off.
func LANPower(ip string, on bool) error {
	v := 0
	if on {
		v = 1
	}
	return lanSend(ip, lanPacket("turn", map[string]int{"value": v}))
}

// LANBrightness sets brightness 1-100.
func LANBrightness(ip string, pct int) error {
	return lanSend(ip, lanPacket("brightness", map[string]int{"value": pct}))
}

// LANColor sets an RGB colour.
func LANColor(ip string, r, g, b uint8) error {
	return lanSend(ip, lanPacket("colorwc", map[string]any{
		"color":            map[string]int{"r": int(r), "g": int(g), "b": int(b)},
		"colorTemInKelvin": 0,
	}))
}

// LANIdentify blinks a light so it can be told apart from the others.
func LANIdentify(ip string) error {
	for i := 0; i < 2; i++ {
		if err := LANPower(ip, false); err != nil {
			return err
		}
		time.Sleep(600 * time.Millisecond)
		if err := LANPower(ip, true); err != nil {
			return err
		}
		time.Sleep(600 * time.Millisecond)
	}
	return nil
}

// SameDevice compares device ids loosely (case, separators).
func SameDevice(a, b string) bool {
	norm := func(s string) string { return strings.ToUpper(strings.NewReplacer(":", "", "-", "").Replace(s)) }
	return a != "" && norm(a) == norm(b)
}
