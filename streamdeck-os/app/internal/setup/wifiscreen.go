package setup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"streamdeckd/internal/modeswitch"
	"streamdeckd/internal/ui"
	"streamdeckd/internal/wifi"
)

// findAdapter waits briefly for a Wi-Fi adapter: a USB dongle needs a few
// seconds after being plugged in (driver + firmware).
func (a *App) findAdapter() *wifi.Adapter {
	for i := 0; i < 12; i++ {
		if ads := wifi.Adapters(); len(ads) > 0 {
			return &ads[0]
		}
		if i == 0 {
			a.U.Info(ui.InfoView{ID: "find-adapter", Title: "Wi-Fi", Spinner: true, Lines: []string{"Looking for a Wi-Fi adapter…"}})
		}
		time.Sleep(time.Second)
	}
	return nil
}

// noAdapterLines explains why no Wi-Fi adapter shows up.
func (a *App) noAdapterLines() []string {
	var out []string
	if cd := modeswitch.Candidates(nil, ""); len(cd) > 0 {
		a.trace("wifi-cd-mode %s", cd[0].ID)
		return []string{fmt.Sprintf("Your adapter (USB ID %s) is stuck in its \"driver CD\" mode and didn't switch to Wi-Fi mode.", cd[0].ID),
			"Unplug it, plug it back in, then choose Look again."}
	}
	fw := wifi.MissingFirmware(wifi.Dmesg())
	usb := wifi.USBWithoutDriver()
	switch {
	case len(fw) > 0:
		out = append(out, "The adapter's driver started but its firmware is missing: "+strings.Join(fw, ", "))
		a.trace("wifi-missing-firmware %s", strings.Join(fw, " "))
	case len(usb) > 0:
		handled := false
		for _, d := range usb {
			if d.Module == "" || d.Bluetooth {
				continue
			}
			handled = true
			bound, msgs := wifi.LoadDriver(d)
			if bound {
				a.trace("wifi-driver-loaded %s %s", d.ID, d.Module)
				out = append(out, fmt.Sprintf("Started the driver %s for %s. Choose Look again.", d.Module, nameOr(d.Name, d.ID)))
				continue
			}
			a.trace("wifi-driver-failed %s %s", d.ID, d.Module)
			out = append(out, fmt.Sprintf("The driver %s didn't take %s (USB ID %s).", d.Module, nameOr(d.Name, "the adapter"), d.ID))
			out = append(out, msgs...)
		}
		for _, d := range usb {
			if d.Module == "" && !d.Bluetooth {
				mark := ""
				if d.Wireless {
					mark = " (probably your Wi-Fi adapter)"
				}
				out = append(out, fmt.Sprintf("No driver for USB ID %s %s%s.", d.ID, d.Name, mark))
				a.trace("wifi-no-driver %s", d.ID)
			}
		}
		if len(out) == 0 && !handled {
			out = append(out, "Nothing plugged in looks like a Wi-Fi adapter (Bluetooth adapters don't count).")
			a.trace("wifi-no-adapter")
		}
	default:
		out = append(out, "Nothing plugged in looks like a Wi-Fi adapter. Try another USB port: a USB 2 port, or one directly on the PC rather than a hub.")
		a.trace("wifi-no-adapter")
	}
	return out
}

func nameOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// WifiScreen connects to a Wi-Fi network.
func (a *App) WifiScreen(ctx context.Context) {
	for {
		ad := a.findAdapter()
		if ad == nil {
			r := a.U.Menu(ui.MenuOpts{ID: "wifi-no-adapter", Title: "Wi-Fi", Subtitle: "No Wi-Fi adapter found",
				Intro: a.noAdapterLines(),
				Items: []ui.Item{{Label: "Look again", Icon: "wait"}}})
			if !r.OK {
				return
			}
			continue
		}
		a.trace("wifi-adapter %s %s usb=%v", ad.Name, ad.Driver, ad.USB)
		ctl := wifi.Ctl{Iface: ad.Name}
		if !ctl.Ready(15 * time.Second) {
			a.trace("wifi-not-ready %s", ad.Name)
			lines := []string{"The Wi-Fi service didn't start on " + ad.Name + "."}
			if fw := wifi.MissingFirmware(wifi.Dmesg()); len(fw) > 0 {
				lines = append(lines, "Missing firmware: "+strings.Join(fw, ", "))
			}
			r := a.U.Menu(ui.MenuOpts{ID: "wifi-not-ready", Title: "Wi-Fi", Intro: lines, Items: []ui.Item{{Label: "Try again", Icon: "wait"}}})
			if !r.OK {
				return
			}
			continue
		}
		if a.pickNetwork(ctl, ad) {
			return
		}
		return
	}
}

// pickNetwork scans and joins; true when connected.
func (a *App) pickNetwork(ctl wifi.Ctl, ad *wifi.Adapter) bool {
	for {
		a.U.Info(ui.InfoView{ID: "wifi-scanning", Title: "Wi-Fi", Spinner: true, Lines: []string{"Looking for networks with " + ad.String() + "…"}})
		nets, err := ctl.Scan(nil)
		if len(nets) > 30 {
			nets = nets[:30]
		}
		a.trace("wifi-scan %d", len(nets))
		cur, _ := wifiSummary()
		var items []ui.Item
		if len(nets) > 0 {
			items = append(items, ui.Item{Label: "Networks near you", Header: true})
		}
		for _, n := range nets {
			band := "2.4 GHz"
			if n.FreqMHz > 4900 {
				band = "5 GHz"
			}
			if n.FreqMHz > 5925 {
				band = "6 GHz"
			}
			det := "open"
			if n.Secure {
				det = "password"
			}
			items = append(items, ui.Item{Label: n.SSID, Detail: det + " · " + band, Value: n.Bars(), Icon: "wifi"})
		}
		base := len(items)
		_, country := wifi.Config()
		if country == "" {
			country = "US"
		}
		items = append(items, ui.Item{Label: "", Header: true},
			ui.Item{Label: "Look again", Icon: "wait"},
			ui.Item{Label: "Join a hidden network…", Icon: "plus"},
			ui.Item{Label: "Country", Value: country, Icon: "gear"})
		intro := []string(nil)
		if err != nil {
			intro = append(intro, "Scan failed: "+err.Error())
		} else if len(nets) == 0 {
			intro = append(intro, "No networks found. Some USB adapters only see 2.4 GHz networks.")
		}
		r := a.U.Menu(ui.MenuOpts{ID: "wifi-networks", Title: "Wi-Fi", Subtitle: "Now: " + cur, Intro: intro, Items: items, Filter: len(nets) > 8})
		if !r.OK {
			return false
		}
		var ssid string
		hidden, secure := false, true
		switch r.Index {
		case base + 1:
			continue
		case base + 2:
			v, ok := a.U.Input(ui.InputOpts{ID: "wifi-hidden", Title: "Hidden network", Label: "Network name (exactly, capitals matter)"})
			if !ok || v == "" {
				continue
			}
			ssid, hidden = v, true
		case base + 3:
			if v, ok := a.U.Input(ui.InputOpts{ID: "wifi-country", Title: "Country", Intro: []string{"Two letters, e.g. US, GB, DE. It sets which Wi-Fi channels may be used."}, Label: "Country code", Initial: country}); ok && len(v) == 2 {
				cur, _ := wifi.Config()
				_ = wifi.WriteConfig(cur, wifiPassword(), strings.ToUpper(v), true)
				a.trace("wifi-country %s", strings.ToUpper(v))
			}
			continue
		default:
			n := nets[r.Index-1]
			ssid, secure = n.SSID, n.Secure
		}
		pass := ""
		if secure {
			for {
				v, ok := a.U.Input(ui.InputOpts{ID: "wifi-pass", Title: ssid, Label: "Wi-Fi password", Secret: true,
					Intro: []string{"Keyboard layout: Dvorak. Click Show to check what you typed."}})
				if !ok {
					pass = "\x00"
					break
				}
				if (hidden && v == "") || (len(v) >= 8 && len(v) <= 63) {
					pass = v
					break
				}
				a.U.Message(ui.Note{ID: "wifi-pass-length", Kind: ui.WarningNote, Title: "Check the password", Lines: []string{"Wi-Fi passwords are 8 to 63 characters."}})
			}
			if pass == "\x00" {
				continue
			}
		}
		if err := wifi.WriteConfig(ssid, pass, country, hidden); err != nil {
			a.U.Message(ui.Note{ID: "wifi-save-error", Kind: ui.ErrorNote, Title: "Couldn't save Wi-Fi settings", Lines: []string{err.Error()}})
			continue
		}
		if a.connectWifi(ctl, ssid, false) {
			return true
		}
	}
}

// connectWifi applies the saved config and reports what happened in plain
// words (unless quiet); true when connected.
func (a *App) connectWifi(ctl wifi.Ctl, ssid string, quiet bool) bool {
	show := func(step string) {
		a.U.Info(ui.InfoView{ID: "wifi-connecting", Title: "Connecting to " + ssid, Spinner: true, Lines: []string{step}})
	}
	show("Starting…")
	// Stop any attempt with the old settings first, so its messages (e.g. an
	// old wrong password) can't be mistaken for this attempt's.
	_ = ctl.Disconnect()
	_ = ctl.Reconfigure()
	time.Sleep(500 * time.Millisecond)
	off := wifi.LogSize()
	_ = ctl.Reconnect()
	steps := map[string]string{
		"SCANNING": "Looking for the network…", "ASSOCIATING": "Joining…", "ASSOCIATED": "Joining…",
		"4WAY_HANDSHAKE": "Checking the password…", "GROUP_HANDSHAKE": "Checking the password…",
		"COMPLETED": "Getting an address…",
	}
	seen := map[string]bool{}
	last := ""
	result, detail := "timeout", ""
	for i := 0; i < 45; i++ {
		time.Sleep(time.Second)
		st := ctl.Status()
		state := st["wpa_state"]
		if msg := steps[state]; msg != "" && msg != last {
			show(msg)
			last = msg
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
	if result == "timeout" && !seen["ASSOCIATING"] && !seen["4WAY_HANDSHAKE"] {
		result = "not-found"
	}
	if result == "wrong-password" {
		_ = ctl.Disconnect() // don't keep hammering the router with a bad password
	}
	a.trace("wifi-result %s %s", result, detail)
	if quiet {
		return result == "connected"
	}
	switch result {
	case "connected":
		a.U.Message(ui.Note{ID: "wifi-connected", Kind: ui.SuccessNote, Title: "Connected", Lines: []string{fmt.Sprintf("Connected to %s (%s).", ssid, detail)}})
		return true
	case "wrong-password":
		a.U.Message(ui.Note{ID: "wifi-wrong-password", Kind: ui.ErrorNote, Title: "Wrong password", Lines: []string{"The password for " + ssid + " isn't right. The keyboard layout is Dvorak: click Show to check what you typed."}})
	case "not-found":
		a.U.Message(ui.Note{ID: "wifi-not-found", Kind: ui.ErrorNote, Title: "Network not found", Lines: []string{"Couldn't find " + ssid + ". Is it in range? Some USB adapters only do 2.4 GHz; if your router has separate 2.4 and 5 GHz names, try the 2.4 GHz one."}})
	case "no-ip":
		a.U.Message(ui.Note{ID: "wifi-no-ip", Kind: ui.ErrorNote, Title: "No address", Lines: []string{"Joined " + ssid + " but the router didn't give out an address (DHCP)."}})
	default:
		a.U.Message(ui.Note{ID: "wifi-failed", Kind: ui.ErrorNote, Title: "Couldn't connect", Lines: []string{"Couldn't connect to " + ssid + "."}})
	}
	return false
}

// WifiStatus is a one-line summary (the "check" command and the dashboard).
func WifiStatus() string {
	s, _ := wifiSummary()
	return s
}

// seedWifi connects with the setup file's Wi-Fi details.
func (a *App) seedWifi(sd *Seed) string {
	ad := a.findAdapter()
	if ad == nil {
		return "Wi-Fi: no adapter found"
	}
	ctl := wifi.Ctl{Iface: ad.Name}
	if !ctl.Ready(15 * time.Second) {
		return "Wi-Fi: the adapter didn't start"
	}
	if err := wifi.WriteConfig(sd.WifiName, sd.WifiPassword, sd.WifiCountry, true); err != nil {
		return "Wi-Fi: " + err.Error()
	}
	if a.connectWifi(ctl, sd.WifiName, true) {
		return "Wi-Fi: connected to " + sd.WifiName
	}
	return "Wi-Fi: couldn't connect to " + sd.WifiName
}
