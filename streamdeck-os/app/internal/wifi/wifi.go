// Package wifi finds Wi-Fi adapters (including USB dongles), explains why
// one isn't working, and drives wpa_supplicant through wpa_cli.
//
// netwatch (os/rootfs/usr/sbin/netwatch) keeps wpa_supplicant running on the
// chosen adapter; this package only edits its config and asks it questions.
package wifi

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Paths, overridable in tests.
var (
	SysNet     = "/sys/class/net"
	SysUSB     = "/sys/bus/usb/devices"
	CtrlDir    = "/var/run/wpa_supplicant"
	LogFile    = "/var/log/wpa_supplicant.log"
	ConfigFile = "/var/lib/streamdeck/wpa_supplicant.conf"
)

// Adapter is a Wi-Fi network interface.
type Adapter struct {
	Name   string // wlan0
	Driver string // rtw88_8821au
	USB    bool
	USBID  string // 0bda:a811
}

func (a Adapter) String() string {
	s := a.Name
	if a.Driver != "" {
		s += ", driver " + a.Driver
	}
	if a.USB {
		s += ", USB " + a.USBID
	}
	return s
}

func readTrim(p string) string {
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

// Adapters lists Wi-Fi interfaces, USB ones first (the order netwatch uses).
func Adapters() []Adapter {
	ents, _ := os.ReadDir(SysNet)
	var out []Adapter
	for _, e := range ents {
		dir := filepath.Join(SysNet, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "phy80211")); err != nil {
			continue
		}
		a := Adapter{Name: e.Name()}
		if drv, err := filepath.EvalSymlinks(filepath.Join(dir, "device", "driver")); err == nil {
			a.Driver = filepath.Base(drv)
		}
		if dev, err := filepath.EvalSymlinks(filepath.Join(dir, "device")); err == nil && strings.Contains(dev, "/usb") {
			a.USB = true
			// device -> .../1-2/1-2:1.0 ; the USB device is the parent.
			usbDev := filepath.Dir(dev)
			a.USBID = readTrim(filepath.Join(usbDev, "idVendor")) + ":" + readTrim(filepath.Join(usbDev, "idProduct"))
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].USB && !out[j].USB })
	return out
}

// USBDevice is a USB device for which no driver is loaded.
type USBDevice struct {
	ID        string // vendor:product
	Name      string // manufacturer + product strings
	Wireless  bool   // looks like a Wi-Fi adapter
	Bluetooth bool
	Modalias  []string
	// Module is a driver in the image that matches this device ("" if none).
	Module string
}

// Known USB Wi-Fi chip vendors (helps tell a Wi-Fi dongle from, say, a webcam).
var wifiVendors = map[string]string{
	"0bda": "Realtek", "148f": "Ralink/MediaTek", "0e8d": "MediaTek", "0cf3": "Qualcomm Atheros",
	"2357": "TP-Link", "7392": "Edimax", "2001": "D-Link", "0b05": "ASUS", "13b1": "Linksys",
	"0846": "Netgear", "050d": "Belkin", "07b8": "AboCom", "2604": "Tenda", "20f4": "TRENDnet",
	"0586": "ZyXEL", "1737": "Linksys", "04bb": "I-O Data", "056e": "Elecom", "0411": "Buffalo",
	"8087": "Intel", "2c4e": "Mercusys", "3574": "MediaTek", "35bc": "Realtek",
}

// USBWithoutDriver lists USB devices none of whose interfaces has a driver.
func USBWithoutDriver() []USBDevice {
	ents, _ := os.ReadDir(SysUSB)
	var out []USBDevice
	for _, e := range ents {
		n := e.Name()
		if strings.Contains(n, ":") || strings.HasPrefix(n, "usb") {
			continue // interfaces and root hubs
		}
		dir := filepath.Join(SysUSB, n)
		if readTrim(filepath.Join(dir, "bDeviceClass")) == "09" {
			continue // hub
		}
		ifs, _ := filepath.Glob(filepath.Join(dir, n+":*"))
		if len(ifs) == 0 {
			continue
		}
		bound := false
		for _, i := range ifs {
			if _, err := os.Stat(filepath.Join(i, "driver")); err == nil {
				bound = true
			}
		}
		if bound {
			continue
		}
		ven := readTrim(filepath.Join(dir, "idVendor"))
		name := strings.TrimSpace(readTrim(filepath.Join(dir, "manufacturer")) + " " + readTrim(filepath.Join(dir, "product")))
		lname := strings.ToLower(name)
		d := USBDevice{ID: ven + ":" + readTrim(filepath.Join(dir, "idProduct")), Name: name}
		for _, i := range ifs {
			if readTrim(filepath.Join(i, "bInterfaceClass")) == "e0" { // wireless controller = Bluetooth
				d.Bluetooth = true
			}
			if ma := readTrim(filepath.Join(i, "modalias")); ma != "" {
				d.Modalias = append(d.Modalias, ma)
				if d.Module == "" {
					d.Module = ModuleFor(ma)
				}
			}
		}
		d.Bluetooth = d.Bluetooth || strings.Contains(lname, "bluetooth")
		d.Wireless = !d.Bluetooth && (wifiVendors[ven] != "" || strings.Contains(lname, "wlan") || strings.Contains(lname, "wireless") ||
			strings.Contains(lname, "802.11") || strings.Contains(lname, "wifi") || strings.Contains(lname, "wi-fi") || d.Module != "")
		out = append(out, d)
	}
	return out
}

// ModulesAlias is the kernel's alias table (overridable in tests).
var ModulesAlias = ""

func aliasFile() string {
	if ModulesAlias != "" {
		return ModulesAlias
	}
	var u syscall.Utsname
	_ = syscall.Uname(&u)
	rel := make([]byte, 0, len(u.Release))
	for _, c := range u.Release {
		if c == 0 {
			break
		}
		rel = append(rel, byte(c))
	}
	return filepath.Join("/lib/modules", string(rel), "modules.alias")
}

// ModuleFor returns the driver module in the image for a modalias, or "".
func ModuleFor(modalias string) string {
	b, err := os.ReadFile(aliasFile())
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != "alias" || !strings.HasPrefix(f[1], "usb:") {
			continue
		}
		if ok, _ := filepath.Match(f[1], modalias); ok {
			return f[2]
		}
	}
	return ""
}

// LoadDriver tries to load the module for a device and reports whether a
// driver is bound afterwards, plus kernel messages that mention the module.
func LoadDriver(d USBDevice) (bound bool, kernelMsgs []string) {
	args := append([]string{"-a"}, d.Modalias...)
	_ = exec.Command("modprobe", args...).Run()
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		bound = true
		for _, u := range USBWithoutDriver() {
			if u.ID == d.ID {
				bound = false
			}
		}
		if bound {
			return true, nil
		}
	}
	short := strings.TrimSuffix(strings.Split(d.Module, "_")[0], "u")
	for _, line := range strings.Split(Dmesg(), "\n") {
		if d.Module != "" && (strings.Contains(line, d.Module) || strings.Contains(line, short)) {
			kernelMsgs = append(kernelMsgs, strings.TrimSpace(line))
		}
	}
	if len(kernelMsgs) > 6 {
		kernelMsgs = kernelMsgs[len(kernelMsgs)-6:]
	}
	return false, kernelMsgs
}

var fwRx = regexp.MustCompile(`(?:Direct firmware load for (\S+) failed|firmware: failed to load (\S+)|[Ff]irmware file (\S+) not found|failed to (?:request|load) firmware[^\n]*?(\S+\.(?:bin|fw|ucode)))`)

// MissingFirmware returns firmware files the kernel asked for but couldn't find.
func MissingFirmware(dmesg string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range fwRx.FindAllStringSubmatch(dmesg, -1) {
		for _, g := range m[1:] {
			if g != "" && !seen[g] {
				seen[g] = true
				out = append(out, g)
			}
		}
	}
	return out
}

// Dmesg returns the kernel log.
func Dmesg() string {
	out, _ := exec.Command("dmesg").Output()
	return string(out)
}

// Network is one scan result.
type Network struct {
	SSID    string
	Signal  int // dBm
	Secure  bool
	FreqMHz int
}

// Bars renders signal strength.
func (n Network) Bars() string {
	switch {
	case n.Signal >= -55:
		return "****"
	case n.Signal >= -67:
		return "*** "
	case n.Signal >= -75:
		return "**  "
	default:
		return "*   "
	}
}

// ParseScan parses `wpa_cli scan_results`, strongest first, one entry per SSID.
func ParseScan(out string) []Network {
	best := map[string]Network{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 5 || strings.HasPrefix(line, "bssid") {
			continue
		}
		ssid := f[4]
		if ssid == "" || strings.HasPrefix(ssid, `\x00`) {
			continue // hidden
		}
		freq, _ := strconv.Atoi(f[1])
		sig, _ := strconv.Atoi(f[2])
		n := Network{SSID: ssid, Signal: sig, FreqMHz: freq, Secure: strings.Contains(f[3], "WPA") || strings.Contains(f[3], "WEP") || strings.Contains(f[3], "SAE")}
		if cur, ok := best[ssid]; !ok || n.Signal > cur.Signal {
			best[ssid] = n
		}
	}
	out2 := make([]Network, 0, len(best))
	for _, n := range best {
		out2 = append(out2, n)
	}
	sort.Slice(out2, func(i, j int) bool { return out2[i].Signal > out2[j].Signal })
	return out2
}

// ParseStatus parses `wpa_cli status` key=value lines.
func ParseStatus(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			m[k] = v
		}
	}
	return m
}

// Ctl talks to the wpa_supplicant running on Iface.
type Ctl struct{ Iface string }

func (c Ctl) cli(args ...string) (string, error) {
	a := append([]string{"-p", CtrlDir, "-i", c.Iface}, args...)
	out, err := exec.Command("wpa_cli", a...).CombinedOutput()
	s := string(out)
	if err != nil {
		return s, fmt.Errorf("wpa_cli %s: %v: %s", args[0], err, strings.TrimSpace(s))
	}
	if strings.Contains(s, "Failed to connect to non-global ctrl_ifname") {
		return s, fmt.Errorf("wpa_supplicant not running on %s", c.Iface)
	}
	return s, nil
}

// Ready waits until wpa_supplicant (started by netwatch) answers.
func (c Ctl) Ready(timeout time.Duration) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if out, err := c.cli("ping"); err == nil && strings.Contains(out, "PONG") {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func (c Ctl) Status() map[string]string {
	out, _ := c.cli("status")
	return ParseStatus(out)
}

// Scan triggers a scan (or joins the one already running: FAIL-BUSY) and
// waits for results. A freshly plugged-in adapter's first scan can take a
// while as it walks every channel. progress is called about once a second.
func (c Ctl) Scan(progress func()) ([]Network, error) {
	if _, err := c.cli("scan"); err != nil {
		return nil, err
	}
	var nets []Network
	for i := 0; i < 25; i++ {
		time.Sleep(time.Second)
		if progress != nil {
			progress()
		}
		out, err := c.cli("scan_results")
		if err != nil {
			return nil, err
		}
		nets = ParseScan(out)
		scanning := c.Status()["wpa_state"] == "SCANNING"
		if len(nets) > 0 && (i >= 3 || !scanning) {
			break
		}
		if i == 12 && len(nets) == 0 {
			c.cli("scan") // nudge once more
		}
	}
	return nets, nil
}

// Disconnect stops (re)connection attempts until Reconnect.
func (c Ctl) Disconnect() error {
	_, err := c.cli("disconnect")
	return err
}

func (c Ctl) Reconnect() error {
	_, err := c.cli("reconnect")
	return err
}

// Reconfigure makes wpa_supplicant re-read its config file.
func (c Ctl) Reconfigure() error {
	_, err := c.cli("reconfigure")
	return err
}

// Config returns the configured SSID and country (empty if none).
func Config() (ssid, country string) {
	b, _ := os.ReadFile(ConfigFile)
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "country="); ok {
			country = v
		}
		if v, ok := strings.CutPrefix(line, "ssid="); ok {
			if s, err := strconv.Unquote(v); err == nil {
				ssid = s
			}
		}
	}
	return
}

// WriteConfig writes wpa_supplicant.conf for one network.
func WriteConfig(ssid, pass, country string, hidden bool) error {
	if country == "" {
		country = "US"
	}
	q := strconv.Quote
	conf := "ctrl_interface=" + CtrlDir + "\nupdate_config=1\np2p_disabled=1\ncountry=" + strings.ToUpper(country) + "\n"
	if ssid != "" {
		conf += "\nnetwork={\n\tssid=" + q(ssid) + "\n"
		if hidden {
			conf += "\tscan_ssid=1\n"
		}
		if pass == "" {
			conf += "\tkey_mgmt=NONE\n"
		} else {
			// WPA2-PSK and WPA3-SAE (transition mode) with the same password.
			conf += "\tpsk=" + q(pass) + "\n\tkey_mgmt=WPA-PSK WPA-PSK-SHA256 SAE\n\tieee80211w=1\n"
		}
		conf += "}\n"
	}
	if err := os.MkdirAll(filepath.Dir(ConfigFile), 0o700); err != nil {
		return err
	}
	return os.WriteFile(ConfigFile, []byte(conf), 0o600)
}

// LogSize and LogSince let a caller look at wpa_supplicant messages written
// after a point (e.g. to spot a wrong password).
func LogSize() int64 {
	st, err := os.Stat(LogFile)
	if err != nil {
		return 0
	}
	return st.Size()
}

func LogSince(off int64) string {
	f, err := os.Open(LogFile)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, _ := f.Stat()
	if st.Size() < off {
		off = 0 // rotated
	}
	buf := make([]byte, st.Size()-off)
	n, _ := f.ReadAt(buf, off)
	return string(buf[:n])
}

// IfaceIPv4 returns the interface's IPv4 address, or "".
func IfaceIPv4(name string) string {
	i, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, _ := i.Addrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	return ""
}
