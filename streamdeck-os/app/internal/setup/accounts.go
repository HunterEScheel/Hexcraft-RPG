package setup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"streamdeckd/internal/config"
	"streamdeckd/internal/govee"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/ui"
)

// ---- Govee ----

// GoveeScreen: the Govee cloud API key (needed for scenes, and for lights
// without LAN control) and the lights controlled on the local network.
func (a *App) GoveeScreen(ctx context.Context) {
	sel := 0
	for {
		s := a.secrets()
		lans := a.lanLights()
		var items []ui.Item
		var acts []string
		add := func(it ui.Item, act string) { items, acts = append(items, it), append(acts, act) }
		add(ui.Item{Label: "Lights on your network (LAN)", Header: true}, "")
		for _, l := range lans {
			add(ui.Item{Label: l.Name, Detail: l.SKU + " · " + l.Device, Icon: "wifi"}, "lan:"+l.Name)
		}
		add(ui.Item{Label: "Find lights on my network…", Detail: "for lights with LAN Control on in the Govee Home app", Icon: "plus"}, "scan")
		add(ui.Item{Label: "Govee cloud", Header: true}, "")
		add(ui.Item{Label: "Govee API key", Value: mask(s.Govee.APIKey), Detail: "needed for scenes, and for lights without LAN control", Icon: "key"}, "key")
		if s.Govee.APIKey != "" {
			add(ui.Item{Label: "Show my cloud lights", Detail: "checks that the key works", Icon: "light"}, "check")
			add(ui.Item{Label: "Remove the API key", Icon: "trash", Danger: true}, "remove")
		}
		r := a.U.Menu(ui.MenuOpts{ID: "govee", Title: "Lights (Govee)", Items: items, Selected: sel})
		if !r.OK {
			return
		}
		sel = r.Index
		switch act := acts[r.Index]; {
		case act == "key":
			a.enterGoveeKey(ctx)
		case act == "check":
			a.checkGovee(ctx, s.Govee.APIKey)
		case act == "remove":
			if a.U.Confirm(ui.ConfirmOpts{ID: "govee-remove", Title: "Remove the Govee key?", Question: "Scenes, and lights without LAN control, stop working until a key is added again.", Yes: "Remove", Danger: true}) {
				s.Govee.APIKey = ""
				a.saveSecrets(s)
			}
		case act == "scan":
			a.findLANLights(ctx)
		case strings.HasPrefix(act, "lan:"):
			a.lanLightMenu(ctx, strings.TrimPrefix(act, "lan:"))
		}
	}
}

func (a *App) lanLights() []config.LANLight {
	l, err := config.ReadLayout(a.LayoutPath)
	if err != nil {
		return nil
	}
	return l.LANLights
}

// saveLANLights rewrites the layout file's LAN lights (the daemon reloads it).
func (a *App) saveLANLights(f func(ls []config.LANLight) []config.LANLight) bool {
	l, err := config.ReadLayout(a.LayoutPath)
	if err != nil {
		l = config.Layout{Brightness: 70, IdleDimSeconds: 600, IdleBrightness: 10, StartPage: "home",
			Pages: map[string][]config.Button{"home": nil}}
	}
	l.LANLights = f(l.LANLights)
	if err := config.SaveLayout(a.LayoutPath, l); err != nil {
		a.U.Message(ui.Note{ID: "save-error", Kind: ui.ErrorNote, Title: "Couldn't save", Lines: []string{err.Error()}})
		return false
	}
	a.trace("lan-saved %d", len(l.LANLights))
	return true
}

// findLANLights scans the network and lets the user name new lights.
func (a *App) findLANLights(ctx context.Context) {
	for {
		a.U.Info(ui.InfoView{ID: "lan-scanning", Title: "Lights on your network", Spinner: true,
			Lines: []string{"Looking for Govee lights with LAN Control switched on…"}})
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		devs, err := govee.LANDiscover(cctx, 3*time.Second)
		cancel()
		a.trace("lan-found %d", len(devs))
		have := a.lanLights()
		var items []ui.Item
		var fresh []govee.LANDevice
		for _, d := range devs {
			name := ""
			for _, l := range have {
				if govee.SameDevice(l.Device, d.Device) {
					name = l.Name
				}
			}
			if name != "" {
				continue
			}
			fresh = append(fresh, d)
			items = append(items, ui.Item{Label: d.SKU + " light", Detail: d.Device + " · " + d.IP, Value: "new", Icon: "light"})
		}
		intro := []string{fmt.Sprintf("Found %d light(s); %d already added.", len(devs), len(devs)-len(fresh))}
		if err != nil {
			intro = []string{"Couldn't look: " + err.Error()}
		}
		if len(devs) == 0 && err == nil {
			intro = []string{"No lights answered.",
				"In the Govee Home app, open each light, tap the gear (settings) and switch on LAN Control. The light must be on the same Wi-Fi as this mini PC."}
		}
		items = append(items, ui.Item{Label: "", Header: true}, ui.Item{Label: "Look again", Icon: "wait"})
		r := a.U.Menu(ui.MenuOpts{ID: "lan-found", Title: "Lights on your network", Intro: intro, Items: items})
		if !r.OK {
			return
		}
		if r.Index >= len(fresh) {
			continue // Look again
		}
		d := fresh[r.Index]
		a.U.Info(ui.InfoView{ID: "lan-blink", Title: "Which light is it?", Spinner: true, Lines: []string{"The light is blinking so you can see which one it is…"}})
		_ = govee.LANIdentify(d.IP)
		name := a.askLANName(d.SKU+" light", "")
		if name == "" {
			continue
		}
		if a.saveLANLights(func(ls []config.LANLight) []config.LANLight {
			return append(ls, config.LANLight{Name: name, Device: d.Device, SKU: d.SKU, IP: d.IP})
		}) {
			a.trace("lan-added %s", name)
		}
	}
}

// askLANName asks for a unique light name ("" = cancelled).
func (a *App) askLANName(title, cur string) string {
	for {
		n, ok := a.U.Input(ui.InputOpts{ID: "lan-name", Title: title, Label: "Name for this light", Initial: cur,
			Intro: []string{"The name buttons use for it, e.g. Spotlight Left or Flood Lights."}})
		if !ok || n == "" {
			return ""
		}
		if n == cur {
			return n
		}
		if _, dup := config.FindLANLight(a.lanLights(), n); dup {
			a.U.Message(ui.Note{ID: "lan-name-taken", Kind: ui.WarningNote, Title: "Name taken", Lines: []string{"There's already a light called " + n + "."}})
			continue
		}
		return n
	}
}

func (a *App) lanLightMenu(ctx context.Context, name string) {
	l, ok := config.FindLANLight(a.lanLights(), name)
	if !ok {
		return
	}
	r := a.U.Menu(ui.MenuOpts{ID: "lan-light", Title: l.Name, Subtitle: l.SKU + " · " + l.Device, Items: []ui.Item{
		{Label: "Blink it", Detail: "to see which light this is", Icon: "light"},
		{Label: "Rename", Icon: "key"},
		{Label: "Remove", Detail: "buttons using it stop working", Icon: "trash", Danger: true},
	}})
	if !r.OK {
		return
	}
	switch r.Index {
	case 0:
		a.U.Info(ui.InfoView{ID: "lan-blink", Title: l.Name, Spinner: true, Lines: []string{"Blinking…"}})
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		ip := l.IP
		if devs, err := govee.LANDiscover(cctx, 1500*time.Millisecond); err == nil {
			for _, d := range devs {
				if govee.SameDevice(d.Device, l.Device) {
					ip = d.IP
				}
			}
		}
		cancel()
		if err := govee.LANIdentify(ip); err != nil {
			a.U.Message(ui.Note{ID: "lan-error", Kind: ui.ErrorNote, Title: l.Name, Lines: []string{err.Error()}})
		}
	case 1:
		n := a.askLANName("Rename "+l.Name, l.Name)
		if n == "" || n == l.Name {
			return
		}
		a.saveLANLights(func(ls []config.LANLight) []config.LANLight {
			for i := range ls {
				if ls[i].Name == l.Name {
					ls[i].Name = n
				}
			}
			return ls
		})
		a.renameLightInButtons(l.Name, n)
	case 2:
		if a.U.Confirm(ui.ConfirmOpts{ID: "lan-remove", Title: "Remove " + l.Name + "?", Question: "Buttons that use it will show a warning until you change them.", Yes: "Remove", Danger: true}) {
			a.saveLANLights(func(ls []config.LANLight) []config.LANLight {
				var out []config.LANLight
				for _, x := range ls {
					if x.Name != l.Name {
						out = append(out, x)
					}
				}
				return out
			})
		}
	}
}

// renameLightInButtons keeps buttons pointing at a renamed light.
func (a *App) renameLightInButtons(old, new string) {
	l, err := config.ReadLayout(a.LayoutPath)
	if err != nil {
		return
	}
	for p, list := range l.Pages {
		for i := range list {
			for j := range list[i].Actions {
				if strings.EqualFold(list[i].Actions[j].Device, old) {
					list[i].Actions[j].Device = new
				}
			}
		}
		l.Pages[p] = list
	}
	_ = config.SaveLayout(a.LayoutPath, l)
}

func (a *App) checkGovee(ctx context.Context, key string) ([]govee.Device, error) {
	if !a.waitOnline("Lights (Govee)") {
		return nil, errCancelled
	}
	a.U.Info(ui.InfoView{ID: "govee-checking", Title: "Lights (Govee)", Spinner: true, Lines: []string{"Asking Govee for your lights…"}})
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	devs, err := govee.New(key).Devices(cctx)
	cancel()
	devs = govee.Lights(devs)
	if err != nil {
		a.trace("govee-error %v", err)
		a.U.Message(ui.Note{ID: "govee-error", Kind: ui.ErrorNote, Title: "That key didn't work", Lines: []string{err.Error(), "Check it and try again."}})
		return nil, err
	}
	a.trace("govee-ok %d", len(devs))
	lines := []string{fmt.Sprintf("Found %d light(s):", len(devs))}
	for _, d := range devs {
		label := d.Name + " (" + d.SKU + ")"
		for _, x := range devs {
			if x.ID != d.ID && strings.EqualFold(x.Name, d.Name) {
				label += " · id " + d.ID
				break
			}
		}
		lines = append(lines, "• "+label)
	}
	lines = append(lines, "", "Lights added or renamed in the Govee Home app after you got this key may be missing or show old names: Govee's API keeps the list from when the key was made. Applying for a new key in the Govee Home app updates it.")
	if len(devs) == 0 {
		lines = []string{"The key works, but no lights on your account work with the Govee API."}
	}
	a.U.Message(ui.Note{ID: "govee-ok", Kind: ui.SuccessNote, Title: "Govee works", Lines: lines})
	return devs, nil
}

func (a *App) enterGoveeKey(ctx context.Context) {
	for {
		key, ok := a.U.Input(ui.InputOpts{ID: "govee-key", Title: "Lights (Govee)", Secret: true, Label: "Govee API key",
			Intro:       []string{"Get a key in the Govee Home app: Profile > Settings (gear) > Apply for API Key. Govee emails it to you."},
			Placeholder: "e.g. 1a2b3c4d-…"})
		if !ok || key == "" {
			return
		}
		if _, err := a.checkGovee(ctx, key); err != nil {
			if errors.Is(err, errCancelled) {
				return
			}
			continue
		}
		s := a.secrets()
		s.Govee.APIKey = key
		a.saveSecrets(s)
		return
	}
}

// ---- Spotify ----

// SpotifyScreen sets up the Spotify app keys, login and default speaker.
func (a *App) SpotifyScreen(ctx context.Context) {
	for {
		s := a.secrets()
		if s.Spotify.ClientID == "" || s.Spotify.ClientSecret == "" {
			if !a.spotifyKeys(ctx) {
				return
			}
			continue
		}
		login := "not logged in"
		if s.Spotify.RefreshToken != "" {
			login = "logged in"
		}
		r := a.U.Menu(ui.MenuOpts{ID: "spotify", Title: "Music (Spotify)", Subtitle: "Spotify Premium is needed to start music from the Stream Deck.", Items: []ui.Item{
			{Label: "Default speaker", Value: or(s.Spotify.DefaultDevice, "not set"), Icon: "volume"},
			{Label: "Log in again", Value: login, Icon: "key"},
			{Label: "App keys", Value: mask(s.Spotify.ClientID), Icon: "gear"},
			{Label: "Disconnect Spotify", Icon: "trash", Danger: true},
		}})
		if !r.OK {
			return
		}
		switch r.Index {
		case 0:
			if s.Spotify.RefreshToken == "" {
				a.U.Message(ui.Note{ID: "spotify-login-first", Kind: ui.WarningNote, Title: "Log in first", Lines: []string{"Choose Log in again first."}})
				continue
			}
			if a.pickSpeaker(ctx, s) {
				a.saveSecrets(s)
			}
		case 1:
			a.loginAndSave(ctx, s.Spotify.ClientID, s.Spotify.ClientSecret)
		case 2:
			a.spotifyKeys(ctx)
		case 3:
			if a.U.Confirm(ui.ConfirmOpts{ID: "spotify-remove", Title: "Disconnect Spotify?", Question: "Music buttons stop working until Spotify is set up again.", Yes: "Disconnect", Danger: true}) {
				s.Spotify = config.SpotifyConfig{}
				a.saveSecrets(s)
				return
			}
		}
	}
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// spotifyKeys asks for the app keys, then logs in; true when saved.
func (a *App) spotifyKeys(ctx context.Context) bool {
	s := a.secrets()
	intro := []string{
		"Spotify needs a free \"app\" of your own. On a computer or phone go to developer.spotify.com/dashboard, " +
			"create an app, tick Web API, and add this Redirect URI:",
		spotify.RedirectURI,
		"Then open the app's Settings to find its Client ID and Client Secret.",
	}
	id, ok := a.U.Input(ui.InputOpts{ID: "spotify-id", Title: "Spotify app keys", Intro: intro, Label: "Client ID", Initial: s.Spotify.ClientID})
	if !ok || id == "" {
		return false
	}
	secret, ok := a.U.Input(ui.InputOpts{ID: "spotify-secret", Title: "Spotify app keys", Label: "Client Secret", Secret: true, Initial: map[bool]string{true: s.Spotify.ClientSecret}[id == s.Spotify.ClientID]})
	if !ok || secret == "" {
		return false
	}
	return a.loginAndSave(ctx, id, secret)
}

// loginAndSave runs the phone login, saves it and asks for the speaker.
func (a *App) loginAndSave(ctx context.Context, id, secret string) bool {
	rt, err := a.spotifyLogin(ctx, id, secret)
	if err != nil {
		if !errors.Is(err, errCancelled) {
			a.U.Message(ui.Note{ID: "spotify-error", Kind: ui.ErrorNote, Title: "Spotify login failed", Lines: []string{err.Error()}})
		}
		return false
	}
	s := a.secrets()
	s.Spotify.ClientID, s.Spotify.ClientSecret, s.Spotify.RefreshToken = id, secret, rt
	if !a.saveSecrets(s) {
		return false
	}
	if a.pickSpeaker(ctx, s) {
		a.saveSecrets(s)
	}
	return true
}

// ReconnectSpotify logs in again with the saved app keys (e.g. to grant
// the playlist permission) and saves the login.
func (a *App) ReconnectSpotify(ctx context.Context) error {
	s := a.secrets()
	if s.Spotify.ClientID == "" || s.Spotify.ClientSecret == "" {
		return errors.New("set up Spotify first (Home > Music)")
	}
	rt, err := a.spotifyLogin(ctx, s.Spotify.ClientID, s.Spotify.ClientSecret)
	if err != nil {
		return err
	}
	s = a.secrets()
	s.Spotify.RefreshToken = rt
	return config.SaveSecrets(a.SecretsPath, s)
}

// spotifyLogin serves a page the user opens on their phone and waits for the
// pasted redirect address (or a code typed here); returns the refresh token.
func (a *App) spotifyLogin(ctx context.Context, id, secret string) (string, error) {
	if !a.waitOnline("Connect Spotify") {
		return "", errCancelled
	}
	for {
		code, err := a.spotifyCode(ctx, id)
		if err != nil {
			return "", err
		}
		a.U.Info(ui.InfoView{ID: "spotify-token", Title: "Connect Spotify", Spinner: true, Lines: []string{"Finishing the login…"}})
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		tr, err := spotify.ExchangeToken(cctx, http.DefaultClient, spotify.DefaultTokenURL, id, secret, url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {code},
			"redirect_uri": {spotify.RedirectURI},
		})
		cancel()
		if err != nil {
			a.trace("spotify-error %v", err)
			a.U.Message(ui.Note{ID: "spotify-error", Kind: ui.ErrorNote, Title: "Spotify login failed", Lines: []string{err.Error(), "Let's try again. Check the Client Secret if this keeps happening."}})
			continue
		}
		a.trace("spotify-ok")
		return tr.RefreshToken, nil
	}
}

func (a *App) spotifyCode(ctx context.Context, clientID string) (string, error) {
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
	addr := a.WebAddr
	if addr == "" {
		addr = ":8080"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("starting the login page: %w", err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer func() {
		// Graceful: let the phone receive its "Got it" page before closing.
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	page := fmt.Sprintf("http://%s:%d", LocalIPv4(), port)
	a.trace("spotify-wait state=%s port=%d", state, port)
	view := ui.InfoView{ID: "spotify-phone", Title: "Connect Spotify", Spinner: true,
		Lines: []string{"On your phone or a computer on the same Wi-Fi, open this address and follow the two steps there:"},
		Big:   page, Buttons: []string{"Type the code", "Cancel"}, Hint: "Waiting for your phone · Esc to cancel"}
	for {
		a.U.Info(view)
		select {
		case code := <-codes:
			return code, nil
		case <-ctx.Done():
			return "", ctx.Err()
		case ev, ok := <-a.U.Events():
			if !ok {
				return "", errCancelled
			}
			typed := false
			switch {
			case ev.Kind == ui.ClickEvent && (ev.Target == "btn:cancel" || ev.Target == "back"),
				ev.Kind == ui.KeyEvent && ev.Key.Name == "esc":
				return "", errCancelled
			case ev.Kind == ui.ClickEvent && ev.Target == "btn:type the code",
				ev.Kind == ui.KeyEvent && (ev.Key.Name == "enter" || ev.Key.R == 't' || ev.Key.R == 'T'):
				typed = true
			case ev.Kind == ui.TickEvent || ev.Kind == ui.HoverEvent:
				continue
			}
			if !typed {
				continue
			}
			v, ok := a.U.Input(ui.InputOpts{ID: "spotify-code", Title: "Connect Spotify", Label: "Code or the full address you ended up on"})
			if !ok || v == "" {
				continue
			}
			code, err := ExtractCode(v, state)
			if err != nil {
				a.U.Message(ui.Note{ID: "spotify-code-error", Kind: ui.WarningNote, Title: "Connect Spotify", Lines: []string{err.Error()}})
				continue
			}
			return code, nil
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

// pickSpeaker sets the default speaker in s; true if it changed.
func (a *App) pickSpeaker(ctx context.Context, s *config.Secrets) bool {
	c := spotify.New(s.Spotify.ClientID, s.Spotify.ClientSecret, s.Spotify.RefreshToken)
	for {
		a.U.Info(ui.InfoView{ID: "spotify-devices", Title: "Default speaker", Spinner: true, Lines: []string{"Asking Spotify which speakers are on…"}})
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		devs, err := c.Devices(cctx)
		cancel()
		var items []ui.Item
		var names []string
		if err != nil {
			items = append(items, ui.Item{Label: "Couldn't list speakers: " + err.Error(), Header: true})
			names = append(names, "")
		}
		for _, d := range devs {
			items = append(items, ui.Item{Label: d.Name, Detail: d.Type, Icon: "volume"})
			names = append(names, d.Name)
		}
		base := len(items)
		items = append(items, ui.Item{Label: "", Header: true},
			ui.Item{Label: "Look again", Detail: "open Spotify on the speaker first", Icon: "wait"},
			ui.Item{Label: "Type a speaker name…", Detail: "for one that's off right now", Icon: "key"})
		a.trace("spotify-speakers %d", len(devs))
		r := a.U.Menu(ui.MenuOpts{ID: "spotify-speaker", Title: "Default speaker",
			Subtitle: "Music buttons play here unless a button names another speaker.",
			Intro:    []string{"Speakers only show up while Spotify is open on them. Current: " + or(s.Spotify.DefaultDevice, "none")},
			Items:    items})
		if !r.OK {
			return false
		}
		switch {
		case r.Index == base+1:
			continue
		case r.Index == base+2:
			v, ok := a.U.Input(ui.InputOpts{ID: "spotify-speaker-name", Title: "Default speaker", Label: "Speaker name, exactly as Spotify shows it", Initial: s.Spotify.DefaultDevice})
			if !ok || v == "" {
				continue
			}
			s.Spotify.DefaultDevice = v
		case r.Index < base && names[r.Index] != "":
			s.Spotify.DefaultDevice = names[r.Index]
		default:
			continue
		}
		a.trace("spotify-device %q", s.Spotify.DefaultDevice)
		return true
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
