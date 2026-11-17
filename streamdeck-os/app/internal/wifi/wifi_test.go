package wifi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mk(t *testing.T, p, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeSys builds a sysfs-like tree: onboard PCI Wi-Fi (wlp1s0), a working
// USB dongle (wlan0, rtw88_8821au), and a USB dongle with no driver.
func fakeSys(t *testing.T) {
	root := t.TempDir()
	dev := filepath.Join(root, "devices")
	// PCI adapter
	pci := filepath.Join(dev, "pci0000:00/0000:00:14.3")
	mk(t, filepath.Join(pci, "x"), "")
	os.MkdirAll(filepath.Join(root, "drivers/iwlwifi"), 0o755)
	os.Symlink(filepath.Join(root, "drivers/iwlwifi"), filepath.Join(pci, "driver"))
	// USB adapter with driver
	usbDev := filepath.Join(dev, "pci0000:00/0000:00:14.0/usb1/1-2")
	usbIf := filepath.Join(usbDev, "1-2:1.0")
	mk(t, filepath.Join(usbDev, "idVendor"), "0bda\n")
	mk(t, filepath.Join(usbDev, "idProduct"), "a811\n")
	os.MkdirAll(usbIf, 0o755)
	os.MkdirAll(filepath.Join(root, "drivers/rtw88_8821au"), 0o755)
	os.Symlink(filepath.Join(root, "drivers/rtw88_8821au"), filepath.Join(usbIf, "driver"))
	// USB dongle without driver
	nd := filepath.Join(dev, "pci0000:00/0000:00:14.0/usb1/1-3")
	mk(t, filepath.Join(nd, "idVendor"), "0bda\n")
	mk(t, filepath.Join(nd, "idProduct"), "8813\n")
	mk(t, filepath.Join(nd, "product"), "802.11ac NIC\n")
	mk(t, filepath.Join(nd, "manufacturer"), "Realtek\n")
	mk(t, filepath.Join(nd, "bDeviceClass"), "00\n")
	// The user's AC600 dongle after its driver CD was ejected.
	mk(t, filepath.Join(nd, "1-3:1.0", "modalias"), "usb:v0BDApC811d0200dcEFdsc02dp01icFFiscFFipFFin00\n")
	mk(t, filepath.Join(nd, "1-3:1.0", "bInterfaceClass"), "ff\n")
	// TP-Link UB500-style Bluetooth adapter, no driver in the image.
	bt := filepath.Join(dev, "pci0000:00/0000:00:14.0/usb1/1-4")
	mk(t, filepath.Join(bt, "idVendor"), "2357\n")
	mk(t, filepath.Join(bt, "idProduct"), "0604\n")
	mk(t, filepath.Join(bt, "product"), "TP-Link Bluetooth USB Adapter\n")
	mk(t, filepath.Join(bt, "bDeviceClass"), "e0\n")
	mk(t, filepath.Join(bt, "1-4:1.0", "bInterfaceClass"), "e0\n")
	mk(t, filepath.Join(bt, "1-4:1.0", "modalias"), "usb:v2357p0604d0200dcE0dsc01dp01icE0isc01ip01in00\n")
	ModulesAlias = filepath.Join(root, "modules.alias")
	mk(t, ModulesAlias, "alias usb:v0BDApC811d*dc*dsc*dp*icFFiscFFipFFin* rtw88_8821cu\nalias usb:v0BDApA811d*dc*dsc*dp*icFFiscFFipFFin* rtw88_8821au\n")
	// A hub (ignored)
	hub := filepath.Join(dev, "pci0000:00/0000:00:14.0/usb1/1-1")
	mk(t, filepath.Join(hub, "bDeviceClass"), "09\n")
	os.MkdirAll(filepath.Join(hub, "1-1:1.0"), 0o755)

	SysNet = filepath.Join(root, "class/net")
	SysUSB = filepath.Join(root, "bus/usb/devices")
	for name, d := range map[string]string{"wlp1s0": pci, "wlan0": usbIf} {
		n := filepath.Join(SysNet, name)
		os.MkdirAll(filepath.Join(n, "phy80211"), 0o755)
		os.Symlink(d, filepath.Join(n, "device"))
	}
	os.MkdirAll(filepath.Join(SysNet, "eth0"), 0o755)
	os.MkdirAll(SysUSB, 0o755)
	for _, d := range []string{usbDev, usbIf, nd, filepath.Join(nd, "1-3:1.0"), hub, filepath.Join(hub, "1-1:1.0"), bt, filepath.Join(bt, "1-4:1.0")} {
		os.Symlink(d, filepath.Join(SysUSB, filepath.Base(d)))
	}
	os.Symlink(filepath.Join(dev, "pci0000:00/0000:00:14.0/usb1"), filepath.Join(SysUSB, "usb1"))
}

func TestAdaptersPreferUSB(t *testing.T) {
	fakeSys(t)
	ads := Adapters()
	if len(ads) != 2 {
		t.Fatalf("got %+v", ads)
	}
	if ads[0].Name != "wlan0" || !ads[0].USB || ads[0].Driver != "rtw88_8821au" || ads[0].USBID != "0bda:a811" {
		t.Fatalf("USB dongle should come first: %+v", ads[0])
	}
	if ads[1].Name != "wlp1s0" || ads[1].USB || ads[1].Driver != "iwlwifi" {
		t.Fatalf("onboard: %+v", ads[1])
	}
}

func TestUSBWithoutDriver(t *testing.T) {
	fakeSys(t)
	d := USBWithoutDriver()
	if len(d) != 2 {
		t.Fatalf("got %+v", d)
	}
	if d[0].ID != "0bda:8813" || !d[0].Wireless || d[0].Bluetooth || d[0].Name != "Realtek 802.11ac NIC" || d[0].Module != "rtw88_8821cu" {
		t.Fatalf("dongle: %+v", d[0])
	}
	if d[1].ID != "2357:0604" || !d[1].Bluetooth || d[1].Wireless || d[1].Module != "" {
		t.Fatalf("bluetooth adapter: %+v", d[1])
	}
}

func TestMissingFirmware(t *testing.T) {
	dmesg := `[    5.1] usbcore: registered new interface driver rtw88_8821au
[    5.2] rtw88_8821au 1-2:1.0: Direct firmware load for rtw88/rtw8821a_fw.bin failed with error -2
[    5.3] rtw88_8821au 1-2:1.0: failed to request firmware
[    6.0] usb 1-3: ath9k_htc: Firmware htc_9271.fw requested
[    6.1] ath9k_htc 1-3:1.0: ath9k_htc: Failed to get firmware htc_9271.fw
[    6.2] mt7601u 1-4:1.0: Direct firmware load for mt7601u.bin failed with error -2
[    7.0] rtw88_8821au 1-2:1.0: Direct firmware load for rtw88/rtw8821a_fw.bin failed with error -2`
	got := strings.Join(MissingFirmware(dmesg), ",")
	if got != "rtw88/rtw8821a_fw.bin,mt7601u.bin" {
		t.Fatalf("got %q", got)
	}
}

func TestParseScan(t *testing.T) {
	out := "bssid / frequency / signal level / flags / ssid\n" +
		"aa:bb:cc:00:00:01\t2437\t-71\t[WPA2-PSK-CCMP][ESS]\tHome\n" +
		"aa:bb:cc:00:00:02\t5180\t-52\t[WPA2-PSK-CCMP][ESS]\tHome\n" +
		"aa:bb:cc:00:00:03\t2412\t-80\t[ESS]\tCafe Free\n" +
		"aa:bb:cc:00:00:04\t2462\t-60\t[WPA2-PSK-CCMP][ESS]\t\n" +
		"aa:bb:cc:00:00:05\t5745\t-66\t[SAE-CCMP][ESS]\tNeighbour WPA3\n"
	n := ParseScan(out)
	if len(n) != 3 || n[0].SSID != "Home" || n[0].Signal != -52 || n[0].FreqMHz != 5180 || !n[0].Secure {
		t.Fatalf("got %+v", n)
	}
	if n[1].SSID != "Neighbour WPA3" || !n[1].Secure || n[2].SSID != "Cafe Free" || n[2].Secure {
		t.Fatalf("got %+v", n)
	}
}

func TestParseStatus(t *testing.T) {
	s := ParseStatus("bssid=aa:bb\nssid=Home\nwpa_state=COMPLETED\nip_address=192.168.1.20\n")
	if s["wpa_state"] != "COMPLETED" || s["ssid"] != "Home" {
		t.Fatalf("%v", s)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	ConfigFile = filepath.Join(t.TempDir(), "wpa.conf")
	if err := WriteConfig(`My "Home" Wi-Fi`, "correct horse", "gb", false); err != nil {
		t.Fatal(err)
	}
	ssid, country := Config()
	if ssid != `My "Home" Wi-Fi` || country != "GB" {
		t.Fatalf("%q %q", ssid, country)
	}
	b, _ := os.ReadFile(ConfigFile)
	if !strings.Contains(string(b), `psk="correct horse"`) || strings.Contains(string(b), "scan_ssid") {
		t.Fatalf("%s", b)
	}
	WriteConfig("Hidden", "", "US", true)
	b, _ = os.ReadFile(ConfigFile)
	if !strings.Contains(string(b), "scan_ssid=1") || !strings.Contains(string(b), "key_mgmt=NONE") {
		t.Fatalf("%s", b)
	}
}
