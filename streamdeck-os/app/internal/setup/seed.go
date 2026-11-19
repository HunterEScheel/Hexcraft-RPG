package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/wifi"
)

// SeedFileName is the setup file the installer looks for on USB sticks.
const SeedFileName = "streamdeck-setup.txt"

// Seed is the content of a setup file: answers the installer would otherwise ask for.
type Seed struct {
	WifiName, WifiPassword, WifiCountry string
	GoveeKey                            string
	SpotifyID, SpotifySecret            string
	SpotifyRefresh                      string // the Spotify login; skips the phone step
	SpotifySpeaker                      string
}

func (s *Seed) HasWifi() bool    { return s != nil && s.WifiName != "" }
func (s *Seed) HasSpotify() bool { return s != nil && s.SpotifyID != "" && s.SpotifySecret != "" }

var seedKeys = map[string]string{
	"wifi_name": "wifi_name", "wifi_ssid": "wifi_name", "wifi_network": "wifi_name", "ssid": "wifi_name",
	"wifi_password": "wifi_password", "wifi_pass": "wifi_password", "wifi_psk": "wifi_password",
	"wifi_country": "wifi_country", "country": "wifi_country",
	"govee_api_key": "govee", "govee_key": "govee", "govee": "govee",
	"spotify_client_id": "sp_id", "spotify_id": "sp_id",
	"spotify_client_secret": "sp_secret", "spotify_secret": "sp_secret",
	"spotify_refresh_token": "sp_refresh", "spotify_login": "sp_refresh",
	"spotify_speaker": "sp_speaker", "spotify_default_device": "sp_speaker", "spotify_device": "sp_speaker",
}

// ParseSeed reads "key = value" lines (# comments). Unknown keys are reported.
func ParseSeed(text string) (*Seed, error) {
	s := &Seed{}
	var unknown []string
	sc := bufio.NewScanner(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		i := strings.IndexAny(line, "=:")
		if i < 0 {
			unknown = append(unknown, line)
			continue
		}
		k := strings.ToLower(strings.TrimSpace(line[:i]))
		k = strings.NewReplacer(" ", "_", "-", "_").Replace(k)
		v := strings.TrimSpace(line[i+1:])
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		switch seedKeys[k] {
		case "wifi_name":
			s.WifiName = v
		case "wifi_password":
			s.WifiPassword = v
		case "wifi_country":
			s.WifiCountry = v
		case "govee":
			s.GoveeKey = v
		case "sp_id":
			s.SpotifyID = v
		case "sp_secret":
			s.SpotifySecret = v
		case "sp_refresh":
			s.SpotifyRefresh = v
		case "sp_speaker":
			s.SpotifySpeaker = v
		default:
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		return s, fmt.Errorf("didn't understand: %s", strings.Join(unknown, ", "))
	}
	return s, nil
}

// Format writes a seed as a setup file.
func (s *Seed) Format() string {
	var b strings.Builder
	b.WriteString("# StreamDeck OS setup file\n")
	b.WriteString("# Put this file on a USB stick as " + SeedFileName + " and plug it in while\n")
	b.WriteString("# installing: the installer fills these in instead of asking.\n")
	b.WriteString("# It contains your keys and Spotify login in plain text: keep the stick safe.\n")
	b.WriteString("# Leave a value empty (or delete the line) to be asked during install.\n\n")
	line := func(k, v string) { fmt.Fprintf(&b, "%s = %s\n", k, v) }
	line("wifi_name", s.WifiName)
	line("wifi_password", s.WifiPassword)
	line("wifi_country", s.WifiCountry)
	b.WriteString("\n")
	line("govee_api_key", s.GoveeKey)
	b.WriteString("\n")
	line("spotify_client_id", s.SpotifyID)
	line("spotify_client_secret", s.SpotifySecret)
	b.WriteString("# The Spotify login. Written by \"Save settings to a USB stick\"; with it\n# the installer skips the phone login.\n")
	line("spotify_refresh_token", s.SpotifyRefresh)
	line("spotify_speaker", s.SpotifySpeaker)
	return b.String()
}

// Summary lists what a seed provides, without revealing secrets.
func (s *Seed) Summary() []string {
	var out []string
	if s.WifiName != "" {
		out = append(out, fmt.Sprintf("Wi-Fi network %q", s.WifiName))
	}
	if s.GoveeKey != "" {
		out = append(out, "Govee API key")
	}
	if s.HasSpotify() {
		if s.SpotifyRefresh != "" {
			out = append(out, "Spotify app keys and login")
		} else {
			out = append(out, "Spotify app keys (you'll still log in once on your phone)")
		}
	}
	if s.SpotifySpeaker != "" {
		out = append(out, fmt.Sprintf("default speaker %q", s.SpotifySpeaker))
	}
	return out
}

// ---- finding USB sticks ----

// usbPart is a filesystem on a USB/removable drive.
type usbPart struct {
	Dev      string // /dev/sdb1
	Mounted  string // existing mount point, if any
	ReadOnly bool
}

var sysClassBlock = "/sys/class/block"

func usbParts() []usbPart {
	mounts := map[string][2]string{} // dev -> mountpoint, options
	if b, err := os.ReadFile("/proc/mounts"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			f := strings.Fields(l)
			if len(f) >= 4 && strings.HasPrefix(f[0], "/dev/") {
				mounts[f[0]] = [2]string{f[1], f[3]}
			}
		}
	}
	ents, _ := os.ReadDir(sysClassBlock)
	var out []usbPart
	for _, e := range ents {
		n := e.Name()
		if !(strings.HasPrefix(n, "sd") || strings.HasPrefix(n, "mmcblk")) {
			continue
		}
		dir := filepath.Join(sysClassBlock, n)
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			continue
		}
		disk := n
		if _, err := os.Stat(filepath.Join(dir, "partition")); err == nil {
			disk = filepath.Base(filepath.Dir(real))
		} else {
			// whole disk: only if it has no partitions (a "superfloppy" stick)
			subs, _ := filepath.Glob(filepath.Join(real, n+"*"))
			if len(subs) > 0 {
				continue
			}
		}
		rem := strings.TrimSpace(readFile(filepath.Join("/sys/block", disk, "removable"))) == "1"
		if !rem && !strings.Contains(real, "/usb") {
			continue
		}
		p := usbPart{Dev: "/dev/" + n}
		if m, ok := mounts[p.Dev]; ok {
			p.Mounted = m[0]
			p.ReadOnly = strings.HasPrefix(m[1], "ro")
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dev < out[j].Dev })
	return out
}

func readFile(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// withMount runs f with the partition's files available at a directory.
func withMount(p usbPart, writable bool, f func(dir string) error) error {
	if p.Mounted != "" {
		if writable && p.ReadOnly {
			return fmt.Errorf("%s is mounted read-only", p.Dev)
		}
		return f(p.Mounted)
	}
	dir, err := os.MkdirTemp("", "usb")
	if err != nil {
		return err
	}
	defer os.Remove(dir)
	var flags uintptr = syscall.MS_RDONLY
	if writable {
		flags = 0
	}
	var lastErr error = fmt.Errorf("no readable filesystem on %s", p.Dev)
	for _, fs := range []string{"vfat", "exfat", "ntfs3", "iso9660"} {
		if writable && fs == "iso9660" {
			continue
		}
		if err := syscall.Mount(p.Dev, dir, fs, flags, ""); err != nil {
			continue
		}
		lastErr = f(dir)
		if writable {
			syscall.Sync()
		}
		_ = syscall.Unmount(dir, 0)
		return lastErr
	}
	return lastErr
}

// seedFileIn finds the setup file in dir (Windows may hide a doubled ".txt").
func seedFileIn(dir string) string {
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		n := strings.ToLower(e.Name())
		if !e.IsDir() && (n == SeedFileName || n == SeedFileName+".txt" || n == strings.TrimSuffix(SeedFileName, ".txt")) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// FindSeedOnUSB looks for the setup file on every USB stick.
func FindSeedOnUSB() (*Seed, string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		for _, p := range usbParts() {
			var seed *Seed
			var perr error
			where := p.Dev
			_ = withMount(p, false, func(dir string) error {
				if f := seedFileIn(dir); f != "" {
					raw, err := os.ReadFile(f)
					if err != nil {
						return err
					}
					seed, perr = ParseSeed(string(raw))
					where = p.Dev + ": " + filepath.Base(f)
				}
				return nil
			})
			if seed != nil {
				return seed, where, perr
			}
		}
		time.Sleep(time.Second) // sticks can still be appearing just after boot
	}
	return nil, "", nil
}

// SeedFromCurrent builds a setup file from the saved settings.
func SeedFromCurrent(secretsPath string) *Seed {
	s, _ := config.LoadSecrets(secretsPath)
	if s == nil {
		s = &config.Secrets{}
	}
	ssid, country := wifi.Config()
	return &Seed{
		WifiName: ssid, WifiPassword: wifiPassword(), WifiCountry: country,
		GoveeKey:  s.Govee.APIKey,
		SpotifyID: s.Spotify.ClientID, SpotifySecret: s.Spotify.ClientSecret,
		SpotifyRefresh: s.Spotify.RefreshToken, SpotifySpeaker: s.Spotify.DefaultDevice,
	}
}

func wifiPassword() string {
	b, _ := os.ReadFile(wifi.ConfigFile)
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "psk="); ok {
			if p, err := strconv.Unquote(v); err == nil {
				return p
			}
		}
	}
	return ""
}

// ExportSeedStep asks which USB stick to write the setup file to.
func (w *Wizard) ExportSeedStep() error {
	seed := SeedFromCurrent(w.SecretsPath)
	media := ""
	if b, err := os.ReadFile("/run/streamdeck-media"); err == nil {
		media, _, _ = strings.Cut(strings.TrimSpace(string(b)), ":")
	}
	var parts []usbPart
	for _, p := range usbParts() {
		if p.Dev != media && !(p.Mounted != "" && p.ReadOnly) {
			parts = append(parts, p)
		}
	}
	w.say("\n--- Save settings to a USB stick ---\n")
	w.say("Writes %s with your Wi-Fi, Govee key and Spotify app keys and login,\n", SeedFileName)
	w.say("so a reinstall needs no typing. Keep that stick safe: the keys are in plain text.\n\n")
	if len(parts) == 0 {
		w.say("No USB stick found. Plug one in (FAT32 or exFAT) and try again.\n")
		w.trace("export-no-stick")
		return nil
	}
	for i, p := range parts {
		w.say("  %d) %s\n", i+1, p.Dev)
	}
	ans, err := w.ask("export-stick", "Which one? (Enter = cancel): ")
	if err != nil || ans == "" {
		return err
	}
	n, err := strconv.Atoi(ans)
	if err != nil || n < 1 || n > len(parts) {
		w.say("Cancelled.\n")
		return nil
	}
	err = withMount(parts[n-1], true, func(dir string) error {
		target := filepath.Join(dir, SeedFileName)
		if old := seedFileIn(dir); old != "" {
			target = old
		}
		return os.WriteFile(target, []byte(seed.Format()), 0o600)
	})
	if err != nil {
		w.trace("export-error %v", err)
		w.say("Couldn't write to %s: %v\n", parts[n-1].Dev, err)
		return nil
	}
	w.trace("exported %s", parts[n-1].Dev)
	w.say("Saved %s to %s.\n", SeedFileName, parts[n-1].Dev)
	return nil
}
