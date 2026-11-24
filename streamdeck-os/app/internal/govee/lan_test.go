package govee

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

// A fake light: answers scans like a Govee LAN device and records commands.
func TestLANDiscoverAndControl(t *testing.T) {
	scanPort, cmdPort, replyPort := freePort(t), freePort(t), freePort(t)
	oldScan, oldListen, oldCmd := LANScanAddrs, LANListenPort, LANCmdPort
	defer func() { LANScanAddrs, LANListenPort, LANCmdPort = oldScan, oldListen, oldCmd }()
	LANScanAddrs = []string{fmt.Sprintf("127.0.0.1:%d", scanPort)}
	LANListenPort, LANCmdPort = replyPort, cmdPort

	scan, err := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", scanPort))
	if err != nil {
		t.Fatal(err)
	}
	defer scan.Close()
	go func() {
		buf := make([]byte, 1024)
		for {
			n, from, err := scan.ReadFrom(buf)
			if err != nil {
				return
			}
			if !strings.Contains(string(buf[:n]), `"scan"`) {
				continue
			}
			to := &net.UDPAddr{IP: from.(*net.UDPAddr).IP, Port: replyPort}
			for _, d := range []string{"AA:BB:CC:DD:EE:FF:00:01", "AA:BB:CC:DD:EE:FF:00:02"} {
				r := fmt.Sprintf(`{"msg":{"cmd":"scan","data":{"ip":"127.0.0.1","device":%q,"sku":"H7065"}}}`, d)
				scan.WriteTo([]byte(r), to)
			}
		}
	}()
	devs, err := LANDiscover(context.Background(), 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 || devs[0].SKU != "H7065" || devs[0].IP != "127.0.0.1" || devs[1].Device != "AA:BB:CC:DD:EE:FF:00:02" {
		t.Fatalf("got %+v", devs)
	}

	cmd, err := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", cmdPort))
	if err != nil {
		t.Fatal(err)
	}
	defer cmd.Close()
	if err := LANColor("127.0.0.1", 255, 64, 0); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1024)
	cmd.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := cmd.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Msg struct {
			Cmd  string `json:"cmd"`
			Data struct {
				Color struct{ R, G, B int } `json:"color"`
			} `json:"data"`
		} `json:"msg"`
	}
	if json.Unmarshal(buf[:n], &m) != nil || m.Msg.Cmd != "colorwc" || m.Msg.Data.Color.R != 255 || m.Msg.Data.Color.G != 64 {
		t.Fatalf("command: %s", buf[:n])
	}
	if !SameDevice("aa:bb:cc:dd:ee:ff:00:01", "AABBCCDDEEFF0001") || SameDevice("", "") {
		t.Fatal("SameDevice")
	}
}
