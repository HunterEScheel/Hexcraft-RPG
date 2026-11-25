package main

import (
	"fmt"
	"log"
	"net"
	"strings"
	"syscall"
	"time"
)

// Fake Govee LAN lights (H7065 spotlights, H7061 flood lights): they answer
// the multicast scan and log the commands sent to them.
var lanLights = []struct{ ip, device, sku string }{
	{"127.0.0.2", "D7:65:AA:BB:CC:DD:00:01", "H7065"},
	{"127.0.0.3", "D7:61:AA:BB:CC:DD:00:02", "H7061"},
}

func startLAN() {
	pc, err := net.ListenPacket("udp4", ":4001")
	if err != nil {
		log.Printf("LAN scan listener: %v", err)
		return
	}
	scan := pc.(*net.UDPConn)
	// Also join the multicast group real lights listen on (once the network
	// is up); unicast and broadcast scans arrive on this socket anyway.
	go func() {
		raw, _ := scan.SyscallConn()
		for i := 0; i < 120; i++ {
			var jerr error
			raw.Control(func(fd uintptr) {
				mreq := &syscall.IPMreq{Multiaddr: [4]byte{239, 255, 255, 250}}
				jerr = syscall.SetsockoptIPMreq(int(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
			})
			if jerr == nil {
				log.Printf("LAN: joined 239.255.255.250")
				return
			}
			time.Sleep(time.Second)
		}
	}()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := scan.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if !strings.Contains(string(buf[:n]), `"scan"`) {
				continue
			}
			log.Printf("LAN scan from %s", from.IP)
			out, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: from.IP, Port: 4002})
			if err != nil {
				continue
			}
			for _, l := range lanLights {
				fmt.Fprintf(out, `{"msg":{"cmd":"scan","data":{"ip":%q,"device":%q,"sku":%q,"bleVersionHard":"3.01.01","wifiVersionSoft":"1.02.03"}}}`, l.ip, l.device, l.sku)
			}
			out.Close()
		}
	}()
	for _, l := range lanLights {
		go func(ip, dev, sku string) {
			// Early in boot the loopback interface may not be up yet.
			var c net.PacketConn
			var err error
			for i := 0; i < 120; i++ {
				if c, err = net.ListenPacket("udp4", ip+":4003"); err == nil {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			if err != nil {
				log.Printf("LAN light %s: %v", sku, err)
				return
			}
			lanServe(dev, c)
		}(l.ip, l.device, l.sku)
	}
}

func lanServe(dev string, c net.PacketConn) {
	{
		func(dev string, c net.PacketConn) {
			buf := make([]byte, 2048)
			for {
				n, _, err := c.ReadFrom(buf)
				if err != nil {
					return
				}
				log.Printf("LAN %s %s", dev, strings.TrimSpace(string(buf[:n])))
			}
		}(dev, c)
	}
}
