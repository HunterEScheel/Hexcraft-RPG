package setup

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"streamdeckd/internal/modeswitch"
	"streamdeckd/internal/wifi"
)

// findAdapter waits briefly for a Wi-Fi adapter: a USB dongle needs a few
// seconds after being plugged in (driver + firmware).
func (w *Wizard) findAdapter() *wifi.Adapter {
	for i := 0; i < 12; i++ {
		if ads := wifi.Adapters(); len(ads) > 0 {
			if i > 0 {
				w.say("\n")
			}
			return &ads[0]
		}
		if i == 0 {
			w.say("Looking for a Wi-Fi adapter")
		}
		w.say(".")
		time.Sleep(time.Second)
	}
	w.say("\n")
	return nil
}

// explainNoAdapter tells the user why no Wi-Fi adapter shows up.
func (w *Wizard) explainNoAdapter() {
	w.say("\nNo Wi-Fi adapter found.\n")
	if cd := modeswitch.Candidates(nil, ""); len(cd) > 0 {
		// netwatch ejects these automatically; if we still see one, it failed.
		w.say("Your adapter (USB ID %s) is stuck in its \"driver CD\" mode and didn't switch\n", cd[0].ID)
		w.say("to Wi-Fi mode. Unplug it, plug it back in, and press Enter.\n")
		w.trace("wifi-cd-mode %s", cd[0].ID)
		return
	}
	fw := wifi.MissingFirmware(wifi.Dmesg())
	usb := wifi.USBWithoutDriver()
	switch {
	case len(fw) > 0:
		w.say("The adapter's driver loaded but its firmware is missing:\n")
		for _, f := range fw {
			w.say("  - %s\n", f)
		}
		w.trace("wifi-missing-firmware %s", strings.Join(fw, " "))
	case len(usb) > 0:
		w.say("These USB devices have no driver in StreamDeck OS:\n")
		for _, d := range usb {
			mark := ""
			if d.Wireless {
				mark = "   <- probably your Wi-Fi adapter"
			}
			w.say("  - USB ID %s  %s%s\n", d.ID, d.Name, mark)
		}
		w.trace("wifi-no-driver %s", usb[0].ID)
	default:
		w.say("Nothing plugged in looks like a Wi-Fi adapter. Try another USB port\n")
		w.say("(a USB 2 port, or one directly on the PC rather than a hub).\n")
		w.trace("wifi-no-adapter")
	}
	w.say("Please send the line(s) above to whoever maintains this OS so support can be added.\n")
}

// wifiSetup connects to a Wi-Fi network. ok is false if the user skipped.
func (w *Wizard) wifiSetup() (ok bool, err error) {
	w.say("\n--- Wi-Fi ---\n")
	ad := w.findAdapter()
	if ad == nil {
		w.explainNoAdapter()
		ans, err := w.ask("wifi-retry", "Plug the adapter in and press Enter to look again, or type s to skip: ")
		return !strings.EqualFold(ans, "s"), err
	}
	w.trace("wifi-adapter %s %s usb=%v", ad.Name, ad.Driver, ad.USB)
	w.say("Wi-Fi adapter: %s\n", ad)

	ctl := wifi.Ctl{Iface: ad.Name}
	if !ctl.Ready(15 * time.Second) {
		w.say("The Wi-Fi service didn't start on %s.\n", ad.Name)
		if fw := wifi.MissingFirmware(wifi.Dmesg()); len(fw) > 0 {
			w.say("Missing firmware: %s\n", strings.Join(fw, ", "))
		}
		w.trace("wifi-not-ready %s", ad.Name)
		_, err := w.ask("wifi-retry", "Press Enter to try again, or type s to skip: ")
		return true, err
	}

	w.say("Scanning for networks")
	nets, err := ctl.Scan(func() { w.say(".") })
	w.say("\n")
	if err != nil {
		w.say("Scan failed: %v\n", err)
	}
	if len(nets) > 15 {
		nets = nets[:15]
	}
	for i, n := range nets {
		lock := " "
		if n.Secure {
			lock = "L"
		}
		band := "2.4G"
		if n.FreqMHz > 4900 {
			band = "5G"
		}
		if n.FreqMHz > 5925 {
			band = "6G"
		}
		w.say("  %2d) %s %s %-4s %s\n", i+1, n.Bars(), lock, band, n.SSID)
	}
	if len(nets) == 0 {
		w.say("  (no networks found)\n")
	}
	w.trace("wifi-scan %d", len(nets))

	ans, err := w.ask("wifi-ssid", "Network number, or type a hidden network's name (Enter = rescan, s = skip): ")
	if err != nil {
		return false, err
	}
	switch {
	case ans == "":
		return true, nil
	case strings.EqualFold(ans, "s"):
		return false, nil
	}
	ssid, hidden, secure := ans, true, true
	if n, err := strconv.Atoi(ans); err == nil {
		if n < 1 || n > len(nets) {
			w.say("There's no network %d in the list.\n", n)
			return true, nil
		}
		ssid, hidden, secure = nets[n-1].SSID, false, nets[n-1].Secure
	}
	pass := ""
	if secure {
		for {
			pass, err = w.ask("wifi-pass", fmt.Sprintf("Password for %q: ", ssid))
			if err != nil {
				return false, err
			}
			if hidden && pass == "" {
				break // open hidden network
			}
			if len(pass) >= 8 && len(pass) <= 63 {
				break
			}
			w.say("Wi-Fi passwords are 8-63 characters.\n")
		}
	}
	_, country := wifi.Config()
	if country == "" {
		country = "US"
	}
	c, err := w.ask("wifi-country", fmt.Sprintf("Country code [%s]: ", country))
	if err != nil {
		return false, err
	}
	if c != "" {
		country = c
	}
	if err := wifi.WriteConfig(ssid, pass, country, hidden); err != nil {
		w.say("Could not save Wi-Fi settings: %v\n", err)
		return true, nil
	}
	w.connectWifi(ctl, ssid)
	return true, nil
}

// connectWifi applies the new config and reports what happened in plain words.
func (w *Wizard) connectWifi(ctl wifi.Ctl, ssid string) {
	// Stop any attempt with the old settings first, so its messages (e.g. an
	// old wrong password) can't be mistaken for this attempt's.
	_ = ctl.Disconnect()
	if err := ctl.Reconfigure(); err != nil {
		w.say("%v\n", err)
	}
	time.Sleep(500 * time.Millisecond)
	off := wifi.LogSize()
	_ = ctl.Reconnect()
	w.say("Connecting to %q", ssid)
	steps := map[string]string{
		"SCANNING": "looking for it", "ASSOCIATING": "joining", "ASSOCIATED": "joining",
		"4WAY_HANDSHAKE": "checking the password", "GROUP_HANDSHAKE": "checking the password",
		"COMPLETED": "getting an address",
	}
	seen := map[string]bool{}
	last := ""
	result, detail := "timeout", ""
	for i := 0; i < 45; i++ {
		time.Sleep(time.Second)
		st := ctl.Status()
		state := st["wpa_state"]
		if msg := steps[state]; msg != "" && msg != last {
			w.say("\n  %s", msg)
			last = msg
		} else {
			w.say(".")
		}
		seen[state] = true
		log := wifi.LogSince(off)
		if strings.Contains(log, "reason=WRONG_KEY") || strings.Contains(log, "pre-shared key may be incorrect") {
			result = "wrong-password"
			break
		}
		if ip := wifi.IfaceIPv4(ctl.Iface); state == "COMPLETED" && ip != "" {
			result, detail = "connected", ip
			break
		}
		if state == "COMPLETED" && i > 25 {
			result = "no-ip"
		}
	}
	w.say("\n")
	if result == "timeout" && !seen["ASSOCIATING"] && !seen["4WAY_HANDSHAKE"] {
		result = "not-found"
	}
	if result == "wrong-password" {
		_ = ctl.Disconnect() // don't keep hammering the router with a bad password
	}
	w.trace("wifi-result %s %s", result, detail)
	switch result {
	case "connected":
		w.say("Connected to %q (%s).\n", ssid, detail)
	case "wrong-password":
		w.say("Wrong password for %q. (Dvorak layout: check the keys you typed.)\n", ssid)
	case "not-found":
		w.say("Couldn't find %q. Is it in range? Some USB adapters only do 2.4 GHz;\n", ssid)
		w.say("if your router has separate 2.4/5 GHz names, try the 2.4 GHz one.\n")
	case "no-ip":
		w.say("Joined %q but the router didn't give out an address (DHCP).\n", ssid)
	default:
		w.say("Couldn't connect to %q.\n", ssid)
	}
}

// WifiStatus is a one-line summary for the status screen and Settings.
func WifiStatus() string {
	ads := wifi.Adapters()
	if len(ads) == 0 {
		if len(wifi.USBWithoutDriver()) > 0 || len(wifi.MissingFirmware(wifi.Dmesg())) > 0 {
			return "no working adapter (open Settings > Wi-Fi for details)"
		}
		return "no adapter"
	}
	st := wifi.Ctl{Iface: ads[0].Name}.Status()
	ssid, _ := wifi.Config()
	ip := wifi.IfaceIPv4(ads[0].Name)
	switch {
	case st["wpa_state"] == "COMPLETED" && ip != "":
		return fmt.Sprintf("%s on %s (%s)", st["ssid"], ads[0].Name, ip)
	case ssid == "":
		return "not set up (" + ads[0].Name + ")"
	default:
		state := strings.ToLower(strings.ReplaceAll(st["wpa_state"], "_", " "))
		if state == "" {
			state = "starting"
		}
		return fmt.Sprintf("%s: %s", ssid, state)
	}
}
