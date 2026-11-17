// Package setup is the on-device installer that runs on the HDMI console
// (tty1) at first boot: network, Govee API key, Spotify login.
package setup

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/govee"
	"streamdeckd/internal/spotify"
)

type Wizard struct {
	In  io.Reader
	Out io.Writer
	// Trace receives machine-readable progress markers (used by the VM test).
	Trace io.Writer

	SecretsPath string
	// LayoutDir gets a govee-devices.txt listing names to use in config.json.
	LayoutDir string
	// WebAddr serves the phone page used for the Spotify login.
	WebAddr string
	// WifiConf is written when the user enters Wi-Fi details; WifiRestart applies it.
	WifiConf    string
	WifiRestart []string

	lines chan string
}

// Defaults for the real appliance.
func New(in io.Reader, out io.Writer) *Wizard {
	return &Wizard{
		In: in, Out: out,
		SecretsPath: config.DefaultSecretsPath,
		LayoutDir:   "/boot/streamdeck",
		WebAddr:     ":8080",
		WifiConf:    "/etc/wpa_supplicant.conf",
		WifiRestart: []string{"/etc/init.d/S35wifi", "restart"},
	}
}

func (w *Wizard) trace(format string, a ...any) {
	if w.Trace != nil {
		fmt.Fprintf(w.Trace, "SETUP: "+format+"\n", a...)
	}
}

func (w *Wizard) say(format string, a ...any) { fmt.Fprintf(w.Out, format, a...) }

func (w *Wizard) startInput() {
	if w.lines != nil {
		return
	}
	w.lines = make(chan string)
	go func() {
		sc := bufio.NewScanner(w.In)
		for sc.Scan() {
			w.lines <- strings.TrimSpace(sc.Text())
		}
		close(w.lines)
	}()
}

// Lines exposes console input so the status screen can share it.
func (w *Wizard) Lines() <-chan string {
	w.startInput()
	return w.lines
}

var errEOF = errors.New("input closed")

func (w *Wizard) ask(step, prompt string) (string, error) {
	w.startInput()
	w.trace("prompt %s", step)
	w.say("%s", prompt)
	line, ok := <-w.lines
	if !ok {
		return "", errEOF
	}
	return line, nil
}

// Run walks through the whole installer and saves the secrets.
func (w *Wizard) Run(ctx context.Context) error {
	old, _ := config.LoadSecrets(w.SecretsPath)
	if old == nil {
		old = &config.Secrets{}
	}
	s := *old

	w.say("\033[H\033[2J")
	w.say("==============================================\n")
	w.say("          StreamDeck OS  -  Installer\n")
	w.say("==============================================\n")
	w.say("Keyboard layout: Dvorak.\n\n")

	ip, err := w.network(ctx)
	if err != nil {
		return err
	}
	w.waitClock()

	if err := w.goveeStep(ctx, &s); err != nil {
		return err
	}
	if err := w.spotifyStep(ctx, &s, ip); err != nil {
		return err
	}
	if err := config.SaveSecrets(w.SecretsPath, &s); err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}
	w.trace("saved")
	w.say("\nAll set! Settings saved. Your Stream Deck is ready.\n")
	w.say("Edit the button layout in streamdeck/config.json on the SD card's boot partition.\n\n")
	return nil
}

// ---- network ----

// LocalIPv4 returns the first non-loopback IPv4 address.
func LocalIPv4() string {
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				return n.IP.String()
			}
		}
	}
	return ""
}

func (w *Wizard) network(ctx context.Context) (string, error) {
	for {
		ip := LocalIPv4()
		if ip != "" {
			w.say("Network: connected (%s)\n", ip)
			ans, err := w.ask("wifi-optional", "Set up Wi-Fi as well? [y/N]: ")
			if err != nil {
				return "", err
			}
			if !strings.HasPrefix(strings.ToLower(ans), "y") {
				return ip, nil
			}
		} else {
			w.say("Network: not connected. Plug in Ethernet, or set up Wi-Fi.\n")
		}
		ssid, err := w.ask("wifi-ssid", "Wi-Fi name (SSID), or Enter to re-check Ethernet: ")
		if err != nil {
			return "", err
		}
		if ssid == "" {
			time.Sleep(2 * time.Second)
			continue
		}
		pass, err := w.ask("wifi-pass", "Wi-Fi password: ")
		if err != nil {
			return "", err
		}
		country, err := w.ask("wifi-country", "Country code for Wi-Fi [US]: ")
		if err != nil {
			return "", err
		}
		if err := w.writeWifi(ssid, pass, country); err != nil {
			w.say("Could not save Wi-Fi settings: %v\n", err)
			continue
		}
		w.say("Connecting to %q", ssid)
		for i := 0; i < 30 && LocalIPv4() == ""; i++ {
			w.say(".")
			time.Sleep(time.Second)
		}
		w.say("\n")
		if LocalIPv4() == "" {
			w.say("Still no connection — check the name and password.\n")
		}
	}
}

func (w *Wizard) writeWifi(ssid, pass, country string) error {
	if country == "" {
		country = "US"
	}
	quote := func(s string) string { return strconv.Quote(s) }
	conf := "ctrl_interface=/var/run/wpa_supplicant\nupdate_config=1\ncountry=" + strings.ToUpper(country) + "\n\nnetwork={\n\tssid=" + quote(ssid) + "\n"
	if pass == "" {
		conf += "\tkey_mgmt=NONE\n"
	} else {
		conf += "\tpsk=" + quote(pass) + "\n"
	}
	conf += "}\n"
	if err := os.WriteFile(w.WifiConf, []byte(conf), 0o600); err != nil {
		return err
	}
	if len(w.WifiRestart) > 0 {
		out, err := exec.Command(w.WifiRestart[0], w.WifiRestart[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
	}
	return nil
}

func (w *Wizard) waitClock() {
	if time.Now().Year() >= 2024 {
		return
	}
	w.say("Waiting for the clock to sync over the internet")
	for i := 0; i < 60 && time.Now().Year() < 2024; i++ {
		w.say(".")
		time.Sleep(time.Second)
	}
	w.say("\n")
}

// ---- Govee ----

func (w *Wizard) goveeStep(ctx context.Context, s *config.Secrets) error {
	w.say("\n--- Govee ---\n")
	w.say("Get an API key in the Govee Home app: Profile > Settings > Apply for API Key.\n")
	for {
		prompt := "Govee API key (Enter to skip): "
		if s.Govee.APIKey != "" {
			prompt = "Govee API key (Enter to keep the current one): "
		}
		key, err := w.ask("govee-key", prompt)
		if err != nil {
			return err
		}
		if key == "" {
			if s.Govee.APIKey == "" {
				w.say("Skipping Govee.\n")
				return nil
			}
			key = s.Govee.APIKey
		}
		g := govee.New(key)
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		devs, err := g.Devices(cctx)
		cancel()
		if err != nil {
			w.trace("govee-error %v", err)
			w.say("That key didn't work: %v\nCheck it and try again.\n", err)
			continue
		}
		s.Govee.APIKey = key
		w.trace("govee-ok %d", len(devs))
		w.say("Key OK. Found %d Govee device(s):\n", len(devs))
		for _, d := range devs {
			w.say("  - %s (%s)\n", d.Name, d.SKU)
		}
		w.writeGoveeList(ctx, g, devs)
		return nil
	}
}

// writeGoveeList saves device + scene names next to config.json so they can
// be copied into the layout from any computer.
func (w *Wizard) writeGoveeList(ctx context.Context, g *govee.Client, devs []govee.Device) {
	if w.LayoutDir == "" {
		return
	}
	var b strings.Builder
	b.WriteString("Govee devices and scene names for config.json\n\n")
	for _, d := range devs {
		fmt.Fprintf(&b, "device: %q (%s)\n", d.Name, d.SKU)
		for _, diy := range []bool{false, true} {
			cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			opts, err := g.Scenes(cctx, d, diy)
			cancel()
			if err != nil || len(opts) == 0 {
				continue
			}
			kind := "govee_scene"
			if diy {
				kind = "govee_diy_scene"
			}
			names := make([]string, len(opts))
			for i, o := range opts {
				names[i] = o.Name
			}
			sort.Strings(names)
			fmt.Fprintf(&b, "  %s: %s\n", kind, strings.Join(names, ", "))
		}
		b.WriteString("\n")
	}
	p := filepath.Join(w.LayoutDir, "govee-devices.txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err == nil {
		w.say("Scene names saved to %s\n", p)
	}
}

// ---- Spotify ----

func (w *Wizard) spotifyStep(ctx context.Context, s *config.Secrets, ip string) error {
	w.say("\n--- Spotify (Premium required) ---\n")
	w.say("At developer.spotify.com/dashboard create an app, select \"Web API\", and add\n")
	w.say("this Redirect URI:  %s\n", spotify.RedirectURI)
	keep := s.Spotify.ClientID != "" && s.Spotify.RefreshToken != ""
	prompt := "Spotify Client ID (Enter to skip): "
	if keep {
		prompt = "Spotify Client ID (Enter to keep the current login): "
	}
	id, err := w.ask("spotify-id", prompt)
	if err != nil {
		return err
	}
	if id == "" {
		if !keep {
			w.say("Skipping Spotify.\n")
		}
		return nil
	}
	secret, err := w.ask("spotify-secret", "Spotify Client Secret: ")
	if err != nil {
		return err
	}

	for {
		code, err := w.spotifyCode(ctx, id, ip)
		if err != nil {
			return err
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		tr, err := spotify.ExchangeToken(cctx, http.DefaultClient, spotify.DefaultTokenURL, id, secret, url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {code},
			"redirect_uri": {spotify.RedirectURI},
		})
		cancel()
		if err != nil {
			w.trace("spotify-error %v", err)
			w.say("Spotify login failed: %v\nLet's try again.\n", err)
			continue
		}
		s.Spotify.ClientID, s.Spotify.ClientSecret, s.Spotify.RefreshToken = id, secret, tr.RefreshToken
		w.trace("spotify-ok")
		w.say("Spotify login OK.\n")
		break
	}
	return w.pickDevice(ctx, s)
}

// spotifyCode serves a page the user opens on their phone, and waits for the
// pasted redirect address (or a code typed on the console).
func (w *Wizard) spotifyCode(ctx context.Context, clientID, ip string) (string, error) {
	var sb [12]byte
	_, _ = rand.Read(sb[:])
	state := hex.EncodeToString(sb[:])
	authURL := spotify.AuthorizeURL + "?" + url.Values{
		"client_id":     {clientID},
		"response_type": {"code"},
		"redirect_uri":  {spotify.RedirectURI},
		"scope":         {spotify.Scopes},
		"state":         {state},
	}.Encode()

	codes := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		msg := ""
		if r.Method == http.MethodPost {
			code, err := ExtractCode(r.FormValue("url"), state)
			if err == nil {
				select {
				case codes <- code:
				default:
				}
				msg = "Got it! Look at the StreamDeck OS screen."
			} else {
				msg = "Hmm: " + err.Error()
			}
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pageTmpl.Execute(rw, map[string]string{"Auth": authURL, "Redirect": spotify.RedirectURI, "Msg": msg})
	})
	ln, err := net.Listen("tcp", w.WebAddr)
	if err != nil {
		return "", fmt.Errorf("starting setup page: %w", err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()

	port := w.WebAddr[strings.LastIndex(w.WebAddr, ":")+1:]
	w.say("\nOn your phone or computer (same network), open:\n\n")
	w.say("        http://%s:%s\n\n", ip, port)
	w.say("and follow the two steps there. Waiting...\n")
	w.say("(Or type the code / pasted address here and press Enter.)\n")
	w.trace("spotify-wait state=%s", state)

	w.startInput()
	for {
		select {
		case code := <-codes:
			return code, nil
		case line, ok := <-w.lines:
			if !ok {
				return "", errEOF
			}
			if line == "" {
				continue
			}
			code, err := ExtractCode(line, state)
			if err != nil {
				w.say("%v\n", err)
				continue
			}
			return code, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// ExtractCode accepts the full redirect address or just the code.
func ExtractCode(s, state string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("nothing pasted")
	}
	if !strings.Contains(s, "code=") && !strings.Contains(s, "error=") && !strings.ContainsAny(s, "/?&=") {
		return s, nil // bare code
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("couldn't read that address")
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return "", fmt.Errorf("Spotify said %q — did you press Agree?", e)
	}
	if st := q.Get("state"); st != "" && st != state {
		return "", errors.New("that address is from an older login attempt — use the newest link")
	}
	if c := q.Get("code"); c != "" {
		return c, nil
	}
	return "", errors.New("no code= in that address")
}

func (w *Wizard) pickDevice(ctx context.Context, s *config.Secrets) error {
	c := spotify.New(s.Spotify.ClientID, s.Spotify.ClientSecret, s.Spotify.RefreshToken)
	for {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		devs, err := c.Devices(cctx)
		cancel()
		if err != nil {
			w.say("Couldn't list Spotify devices: %v\n", err)
			devs = nil
		}
		w.say("\nWhich speaker should playlists start on?\n")
		for i, d := range devs {
			w.say("  %d) %s (%s)\n", i+1, d.Name, d.Type)
		}
		w.say("  r) refresh list (open Spotify on the speaker/computer first)\n")
		cur := ""
		if s.Spotify.DefaultDevice != "" {
			cur = " [" + s.Spotify.DefaultDevice + "]"
		}
		ans, err := w.ask("spotify-device", "Number, or type a device name"+cur+": ")
		if err != nil {
			return err
		}
		switch {
		case strings.EqualFold(ans, "r"):
			continue
		case ans == "":
			if s.Spotify.DefaultDevice == "" {
				w.say("No default set; buttons must name a spotify_device.\n")
			}
			return nil
		}
		if n, err := strconv.Atoi(ans); err == nil {
			if n < 1 || n > len(devs) {
				w.say("Pick 1-%d.\n", len(devs))
				continue
			}
			ans = devs[n-1].Name
		}
		s.Spotify.DefaultDevice = ans
		w.trace("spotify-device %q", ans)
		w.say("Default speaker: %s\n", ans)
		return nil
	}
}

var pageTmpl = template.Must(template.New("p").Parse(`<!doctype html>
<html><head><meta name="viewport" content="width=device-width,initial-scale=1">
<title>StreamDeck OS setup</title>
<style>
body{font-family:system-ui,sans-serif;max-width:34rem;margin:2rem auto;padding:0 1rem;line-height:1.5;background:#111;color:#eee}
a.btn,button{display:inline-block;background:#1db954;color:#000;padding:.7rem 1.2rem;border-radius:2rem;font-weight:700;text-decoration:none;border:0;font-size:1rem}
textarea{width:100%;height:6rem;font-size:.9rem}
.msg{padding:.6rem;border-radius:.4rem;background:#333}
code{word-break:break-all}
</style></head><body>
<h1>Connect Spotify</h1>
{{if .Msg}}<p class="msg">{{.Msg}}</p>{{end}}
<p><b>1.</b> Log in and press <i>Agree</i>:</p>
<p><a class="btn" href="{{.Auth}}" target="_blank" rel="noopener">Log in with Spotify</a></p>
<p><b>2.</b> You'll end up on a page that doesn't load (<code>{{.Redirect}}…</code>). That's expected.
Copy that page's <b>full address</b> from the address bar and paste it here:</p>
<form method="post"><textarea name="url" placeholder="http://127.0.0.1:8888/callback?code=..."></textarea>
<p><button type="submit">Send to StreamDeck</button></p></form>
</body></html>`))
