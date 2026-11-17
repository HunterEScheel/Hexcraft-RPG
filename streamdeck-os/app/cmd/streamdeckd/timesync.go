package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
)

// runTimesync sets the clock from NTP (SNTP, RFC 4330). Mini PCs that shipped
// with Windows keep the hardware clock in local time, and TLS to Govee and
// Spotify needs a correct clock. Retries until it succeeds or `wait` elapses.
func runTimesync(server string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		t, err := sntp(server)
		if err == nil {
			tv := syscall.NsecToTimeval(t.UnixNano())
			if err := syscall.Settimeofday(&tv); err != nil {
				return fmt.Errorf("set clock: %w", err)
			}
			log.Printf("clock set from %s: %s", server, t.UTC().Format(time.RFC3339))
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(5 * time.Second)
	}
}

func sntp(server string) (time.Time, error) {
	c, err := net.DialTimeout("udp", net.JoinHostPort(server, "123"), 5*time.Second)
	if err != nil {
		return time.Time{}, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	req := make([]byte, 48)
	req[0] = 0x23 // LI=0, VN=4, Mode=3 (client)
	sent := time.Now()
	if _, err := c.Write(req); err != nil {
		return time.Time{}, err
	}
	resp := make([]byte, 48)
	n, err := c.Read(resp)
	if err != nil {
		return time.Time{}, err
	}
	if n < 48 || resp[0]&0x07 != 4 || resp[1] == 0 {
		return time.Time{}, fmt.Errorf("bad NTP reply")
	}
	rtt := time.Since(sent)
	secs := binary.BigEndian.Uint32(resp[40:])
	frac := binary.BigEndian.Uint32(resp[44:])
	const ntpEpochOffset = 2208988800
	t := time.Unix(int64(secs)-ntpEpochOffset, int64(frac)*1e9>>32)
	return t.Add(rtt / 2), nil
}
