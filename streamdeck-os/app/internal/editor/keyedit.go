package editor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"streamdeckd/internal/config"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/tui"
)

// editKey edits the selected key; changes show on the deck immediately and
// are saved when leaving.
func (e *Editor) editKey(ctx context.Context) {
	orig, existed := e.button(e.page, e.sel)
	b := orig
	b.Key = e.sel
	b.Actions = append([]config.Action(nil), orig.Actions...)
	sel := 0
	for {
		e.setButton(e.page, b)
		e.preview()

		items := []tui.Item{
			{Label: "Label", Detail: orDash(b.Label)},
			{Label: "Colour", Detail: colorName(b.Color, "#202020"), Swatch: colorEsc(or(b.Color, "#202020"), true)},
			{Label: "Text colour", Detail: colorName(b.TextColor, "#ffffff"), Swatch: colorEsc(or(b.TextColor, "#ffffff"), true)},
			{Label: "What it does (in order)", Header: true},
		}
		for i, a := range b.Actions {
			s, warn := e.describe(a)
			if warn != "" {
				s += "  ! " + warn
			}
			items = append(items, tui.Item{Label: fmt.Sprintf("%d. %s", i+1, s)})
		}
		addIdx := len(items)
		items = append(items, tui.Item{Label: "+ Add an action"}, tui.Item{Label: "", Header: true},
			tui.Item{Label: "Try it now"}, tui.Item{Label: "Clear this key"}, tui.Item{Label: "Done"})
		tryIdx, clearIdx, doneIdx := addIdx+2, addIdx+3, addIdx+4
		if sel == 0 && len(b.Actions) == 0 && b.Label != "" {
			sel = addIdx
		}
		e.trace("screen key %d", e.sel)
		r := tui.Menu(e.S, tui.MenuOpts{
			Title:    fmt.Sprintf("Key %d  (page %s)", e.sel+1, e.page),
			Items:    items,
			Selected: sel,
			Help:     "Up/Down: choose   Enter: change   Esc: done (saves)",
		})
		if !r.OK {
			r.Index = doneIdx
		}
		sel = r.Index
		switch {
		case r.Index == 0:
			if v, ok := tui.Input(e.S, "Label", []string{"Text shown on the key. Keep it short: about 2 lines of 7 letters."}, "Label:", b.Label); ok {
				b.Label = v
				e.trace("label %q", v)
			}
		case r.Index == 1:
			if c, ok := e.pickColor("Key colour", b.Color, "#202020"); ok {
				b.Color = c
			}
		case r.Index == 2:
			if c, ok := e.pickColor("Text colour", b.TextColor, "#ffffff"); ok {
				b.TextColor = c
			}
		case r.Index >= 4 && r.Index < addIdx:
			i := r.Index - 4
			b.Actions = e.actionMenu(ctx, b.Actions, i)
		case r.Index == addIdx:
			if a, ok := e.newAction(ctx); ok {
				b.Actions = append(b.Actions, a)
				sel = addIdx + 1 // land on "+ Add" again (list grew by one)
			}
		case r.Index == tryIdx:
			e.try(ctx, b)
		case r.Index == clearIdx:
			if tui.Confirm(e.S, "Clear key", fmt.Sprintf("Remove everything from key %d?", e.sel+1)) {
				b = config.Button{Key: e.sel}
				e.setButton(e.page, b)
				e.save()
				return
			}
		case r.Index == doneIdx:
			e.setButton(e.page, b)
			if !e.save() {
				r := tui.Menu(e.S, tui.MenuOpts{
					Title: "Can't save this key",
					Intro: append([]string{"  " + tui.Red + "Problem:" + tui.Reset}, indent(strings.Split(e.err, "\n"))...),
					Items: []tui.Item{{Label: "Go back and fix it"}, {Label: "Undo my changes to this key"}},
				})
				if r.OK && r.Index == 1 {
					// Back to what's already saved on disk: nothing to write.
					if existed {
						e.setButton(e.page, orig)
					} else {
						e.setButton(e.page, config.Button{Key: e.sel})
					}
					e.err = ""
					e.trace("undo %d", e.sel)
					return
				}
				continue
			}
			return
		}
	}
}

func indent(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = "  " + l
	}
	return out
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// actionMenu changes, moves or deletes action i.
func (e *Editor) actionMenu(ctx context.Context, acts []config.Action, i int) []config.Action {
	s, _ := e.describe(acts[i])
	r := tui.Menu(e.S, tui.MenuOpts{
		Title: fmt.Sprintf("Action %d", i+1),
		Intro: []string{"  " + s},
		Items: []tui.Item{{Label: "Change it"}, {Label: "Move up"}, {Label: "Move down"}, {Label: "Delete it"}, {Label: "Back"}},
	})
	if !r.OK {
		return acts
	}
	switch r.Index {
	case 0:
		if a, ok := e.buildAction(ctx, acts[i].Type, &acts[i]); ok {
			acts[i] = a
		}
	case 1:
		if i > 0 {
			acts[i-1], acts[i] = acts[i], acts[i-1]
		}
	case 2:
		if i < len(acts)-1 {
			acts[i+1], acts[i] = acts[i], acts[i+1]
		}
	case 3:
		acts = append(acts[:i], acts[i+1:]...)
	}
	return acts
}

var actionTypes = []struct {
	group, label, typ string
}{
	{"Lights (Govee)", "Set a scene", config.GoveeScene},
	{"Lights (Govee)", "Turn on or off", config.GoveePower},
	{"Lights (Govee)", "Set brightness", config.GoveeBrightness},
	{"Lights (Govee)", "Set a colour", config.GoveeColor},
	{"Music (Spotify)", "Play a playlist", config.SpotifyPlaylist},
	{"Music (Spotify)", "Pause", config.SpotifyPause},
	{"Music (Spotify)", "Next song", config.SpotifyNext},
	{"Music (Spotify)", "Set volume", config.SpotifyVolume},
	{"Other", "Go to another page of buttons", config.Page},
	{"Other", "Wait a moment", config.Delay},
}

func (e *Editor) newAction(ctx context.Context) (config.Action, bool) {
	var items []tui.Item
	var types []string
	group := ""
	for _, t := range actionTypes {
		if t.group != group {
			items = append(items, tui.Item{Label: t.group, Header: true})
			types = append(types, "")
			group = t.group
		}
		items = append(items, tui.Item{Label: t.label})
		types = append(types, t.typ)
	}
	e.trace("screen add-action")
	r := tui.Menu(e.S, tui.MenuOpts{Title: "Add an action", Items: items})
	if !r.OK {
		return config.Action{}, false
	}
	return e.buildAction(ctx, types[r.Index], nil)
}

// buildAction asks for an action's details (cur pre-fills when changing one).
func (e *Editor) buildAction(ctx context.Context, typ string, cur *config.Action) (config.Action, bool) {
	a := config.Action{Type: typ}
	if cur != nil {
		a = *cur
	}
	switch typ {
	case config.GoveeScene, config.GoveeDIYScene:
		dev, ok := e.pickLight(ctx, a.Device)
		if !ok {
			return a, false
		}
		scene, diy, ok := e.pickScene(ctx, dev, a.Scene)
		if !ok {
			return a, false
		}
		a.Device, a.Scene, a.Type = dev, scene, config.GoveeScene
		if diy {
			a.Type = config.GoveeDIYScene
		}
	case config.GoveePower:
		dev, ok := e.pickLight(ctx, a.Device)
		if !ok {
			return a, false
		}
		r := tui.Menu(e.S, tui.MenuOpts{Title: dev, Items: []tui.Item{{Label: "Turn on"}, {Label: "Turn off"}}})
		if !r.OK {
			return a, false
		}
		on := r.Index == 0
		a.Device, a.On = dev, &on
	case config.GoveeBrightness:
		dev, ok := e.pickLight(ctx, a.Device)
		if !ok {
			return a, false
		}
		v, ok := tui.Number(e.S, dev, "Brightness:", or0(a.Brightness, 50), 1, 100, 5, "%")
		if !ok {
			return a, false
		}
		a.Device, a.Brightness = dev, v
	case config.GoveeColor:
		dev, ok := e.pickLight(ctx, a.Device)
		if !ok {
			return a, false
		}
		c, ok := e.pickColor("Light colour", a.Color, "#ffffff")
		if !ok {
			return a, false
		}
		a.Device, a.Color = dev, c
	case config.SpotifyPlaylist:
		uri, ok := e.pickPlaylist(ctx, a.Playlist)
		if !ok {
			return a, false
		}
		spk, ok := e.pickSpeaker(ctx, a.SpotifyDevice)
		if !ok {
			return a, false
		}
		r := tui.Menu(e.S, tui.MenuOpts{Title: "Shuffle?", Items: []tui.Item{{Label: "Play in order"}, {Label: "Shuffle"}}, Selected: b2i(a.Shuffle)})
		if !r.OK {
			return a, false
		}
		shuffle := r.Index == 1
		vsel := 0
		if a.Volume != nil {
			vsel = 1
		}
		r = tui.Menu(e.S, tui.MenuOpts{Title: "Volume", Items: []tui.Item{{Label: "Leave the volume as it is"}, {Label: "Set the volume"}}, Selected: vsel})
		if !r.OK {
			return a, false
		}
		var vol *int
		if r.Index == 1 {
			start := 50
			if a.Volume != nil {
				start = *a.Volume
			}
			v, ok := tui.Number(e.S, "Volume", "Volume:", start, 0, 100, 5, "%")
			if !ok {
				return a, false
			}
			vol = &v
		}
		a.Playlist, a.SpotifyDevice, a.Shuffle, a.Volume = uri, spk, shuffle, vol
	case config.SpotifyPause, config.SpotifyNext:
		spk, ok := e.pickSpeaker(ctx, a.SpotifyDevice)
		if !ok {
			return a, false
		}
		a.SpotifyDevice = spk
	case config.SpotifyVolume:
		spk, ok := e.pickSpeaker(ctx, a.SpotifyDevice)
		if !ok {
			return a, false
		}
		start := 50
		if a.Volume != nil {
			start = *a.Volume
		}
		v, ok := tui.Number(e.S, "Volume", "Volume:", start, 0, 100, 5, "%")
		if !ok {
			return a, false
		}
		a.SpotifyDevice, a.Volume = spk, &v
	case config.Page:
		p, ok := e.pickPage(a.Page)
		if !ok {
			return a, false
		}
		a.Page = p
	case config.Delay:
		v, ok := tui.Number(e.S, "Wait", "Seconds to wait:", max(1, a.Ms/1000), 1, 60, 1, " s")
		if !ok {
			return a, false
		}
		a.Ms = v * 1000
	}
	e.trace("action %s", a.Type)
	return a, true
}

func or0(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- pickers ----

func (e *Editor) pickLight(ctx context.Context, cur string) (string, bool) {
	tui.Busy(e.S, "Lights", "Getting your lights from Govee…")
	devs, err := e.loadGovee(ctx)
	if err != nil {
		tui.Message(e.S, "Lights", []string{tui.Red + "Couldn't get your lights:" + tui.Reset, err.Error()})
		return "", false
	}
	if len(devs) == 0 {
		tui.Message(e.S, "Lights", []string{"Your Govee account has no lights that work with the Govee API."})
		return "", false
	}
	items := make([]tui.Item, len(devs))
	sel := 0
	for i, d := range devs {
		items[i] = tui.Item{Label: d.Name, Detail: fmt.Sprintf("%d scenes", len(d.Scenes)+len(d.DIY))}
		if strings.EqualFold(d.Name, cur) {
			sel = i
		}
	}
	e.trace("screen pick-light")
	r := tui.Menu(e.S, tui.MenuOpts{Title: "Which light?", Items: items, Selected: sel, Filter: len(items) > 8})
	if !r.OK {
		return "", false
	}
	return devs[r.Index].Name, true
}

func (e *Editor) pickScene(ctx context.Context, light, cur string) (string, bool, bool) {
	devs, _ := e.loadGovee(ctx)
	var d GoveeDevice
	for _, x := range devs {
		if x.Name == light {
			d = x
		}
	}
	var items []tui.Item
	var names []string
	var diy []bool
	sel := 0
	add := func(header string, list []string, isDIY bool) {
		if len(list) == 0 {
			return
		}
		items = append(items, tui.Item{Label: header, Header: true})
		names = append(names, "")
		diy = append(diy, false)
		for _, n := range list {
			if strings.EqualFold(n, cur) {
				sel = len(items)
			}
			items = append(items, tui.Item{Label: n})
			names = append(names, n)
			diy = append(diy, isDIY)
		}
	}
	add("Scenes", d.Scenes, false)
	add("Your DIY scenes", d.DIY, true)
	if len(names) == 0 {
		tui.Message(e.S, light, []string{"This light has no scenes."})
		return "", false, false
	}
	e.trace("screen pick-scene")
	r := tui.Menu(e.S, tui.MenuOpts{Title: "Scene for " + light, Items: items, Selected: sel, Filter: true})
	if !r.OK {
		return "", false, false
	}
	return names[r.Index], diy[r.Index], true
}

func (e *Editor) pickPlaylist(ctx context.Context, cur string) (string, bool) {
	curURI, _ := spotify.NormalizeURI(cur)
	for {
		tui.Busy(e.S, "Playlists", "Getting your playlists from Spotify…")
		pls, err := e.loadPlaylists(ctx)
		var items []tui.Item
		type choice struct{ uri, kind string }
		var choices []choice
		if errors.Is(err, spotify.ErrNeedsReconnect) {
			items = append(items, tui.Item{Label: "Reconnect Spotify so your playlists can be listed", Detail: "one time, uses your phone"})
			choices = append(choices, choice{kind: "reconnect"})
		} else if err != nil {
			items = append(items, tui.Item{Label: "Couldn't get your playlists", Detail: err.Error(), Header: true})
			choices = append(choices, choice{})
		}
		sel := 0
		if len(pls) > 0 {
			items = append(items, tui.Item{Label: "Your playlists", Header: true})
			choices = append(choices, choice{})
			for _, p := range pls {
				if p.URI == curURI {
					sel = len(items)
				}
				items = append(items, tui.Item{Label: p.Name, Detail: fmt.Sprintf("by %s · %d songs", p.Owner, p.Tracks)})
				choices = append(choices, choice{uri: p.URI})
			}
		}
		items = append(items, tui.Item{Label: "", Header: true}, tui.Item{Label: "Type a playlist link instead…"})
		choices = append(choices, choice{}, choice{kind: "type"})
		e.trace("screen pick-playlist %d", len(pls))
		r := tui.Menu(e.S, tui.MenuOpts{Title: "Which playlist?", Items: items, Selected: sel, Filter: len(pls) > 8})
		if !r.OK {
			return "", false
		}
		switch c := choices[r.Index]; {
		case c.kind == "reconnect":
			if err := e.Src.ReconnectSpotify(ctx); err != nil {
				tui.Message(e.S, "Spotify", []string{tui.Red + "Reconnecting failed:" + tui.Reset, err.Error()})
			}
			e.mu.Lock()
			e.playlists, e.plErr = nil, nil
			e.mu.Unlock()
		case c.kind == "type":
			for {
				v, ok := tui.Input(e.S, "Playlist link", []string{
					"In Spotify: open the playlist > Share > Copy link, then type it here,",
					"or type its URI, e.g. spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
				}, "Link:", cur)
				if !ok {
					break
				}
				if uri, err := spotify.NormalizeURI(v); err == nil {
					return uri, true
				}
				tui.Message(e.S, "Playlist link", []string{"That doesn't look like a Spotify playlist, album or artist link."})
			}
		case c.uri != "":
			return c.uri, true
		}
	}
}

func (e *Editor) pickSpeaker(ctx context.Context, cur string) (string, bool) {
	tui.Busy(e.S, "Speaker", "Asking Spotify which speakers are on…")
	cctx, cancel := context.WithTimeout(ctx, 20e9)
	devs, err := e.Src.SpotifyDevices(cctx)
	cancel()
	def := e.Src.DefaultSpeaker()
	items := []tui.Item{{Label: "The default speaker", Detail: or(def, "not set: use whichever is playing")}}
	vals := []string{""}
	sel := 0
	if err != nil {
		items = append(items, tui.Item{Label: "Couldn't list speakers", Detail: err.Error(), Header: true})
		vals = append(vals, "")
	}
	seen := map[string]bool{}
	for _, d := range append(devs, cur) {
		if d == "" || seen[strings.ToLower(d)] {
			continue
		}
		seen[strings.ToLower(d)] = true
		if d == cur {
			sel = len(items)
		}
		items = append(items, tui.Item{Label: d})
		vals = append(vals, d)
	}
	items = append(items, tui.Item{Label: "Type a speaker name…", Detail: "for one that's off right now"})
	vals = append(vals, "\x00type")
	e.trace("screen pick-speaker")
	r := tui.Menu(e.S, tui.MenuOpts{Title: "Play on which speaker?", Intro: []string{"  Speakers only show up while Spotify is open on them."}, Items: items, Selected: sel})
	if !r.OK {
		return "", false
	}
	if vals[r.Index] == "\x00type" {
		return tui.Input(e.S, "Speaker", []string{"Exactly as it appears in Spotify's device list."}, "Speaker name:", cur)
	}
	return vals[r.Index], true
}

func (e *Editor) pickPage(cur string) (string, bool) {
	var items []tui.Item
	names := e.pageNames()
	sel := 0
	for i, n := range names {
		if n == cur {
			sel = i
		}
		items = append(items, tui.Item{Label: n})
	}
	items = append(items, tui.Item{Label: "+ A new page…"})
	r := tui.Menu(e.S, tui.MenuOpts{Title: "Go to which page?", Items: items, Selected: sel})
	if !r.OK {
		return "", false
	}
	if r.Index < len(names) {
		return names[r.Index], true
	}
	return e.newPage()
}

func (e *Editor) newPage() (string, bool) {
	for {
		n, ok := tui.Input(e.S, "New page", []string{"A page is another set of 15 buttons. Give it a short name."}, "Page name:", "")
		if !ok || n == "" {
			return "", false
		}
		if _, exists := e.L.Pages[n]; exists {
			tui.Message(e.S, "New page", []string{"There's already a page called " + n + "."})
			continue
		}
		e.L.Pages[n] = nil
		return n, true
	}
}

var palette = []struct{ name, hex string }{
	{"Charcoal", "#202020"}, {"Black", "#000000"}, {"White", "#ffffff"}, {"Grey", "#6b6b6b"},
	{"Red", "#c0262d"}, {"Orange", "#e0701b"}, {"Yellow", "#e8c020"}, {"Green", "#2f9e44"},
	{"Spotify green", "#1db954"}, {"Teal", "#108a8a"}, {"Blue", "#1d5fa0"}, {"Navy", "#1b2a6b"},
	{"Purple", "#6a2fb0"}, {"Pink", "#c2185b"}, {"Brown", "#7a4a24"},
}

func colorName(hex, def string) string {
	if hex == "" {
		hex = def
	}
	for _, p := range palette {
		if strings.EqualFold(p.hex, hex) {
			return p.name
		}
	}
	return hex
}

func (e *Editor) pickColor(title, cur, def string) (string, bool) {
	cur = or(cur, def)
	items := make([]tui.Item, 0, len(palette)+1)
	sel := 0
	for i, p := range palette {
		if strings.EqualFold(p.hex, cur) {
			sel = i
		}
		items = append(items, tui.Item{Label: p.name, Swatch: colorEsc(p.hex, true)})
	}
	items = append(items, tui.Item{Label: "Other (type a #rrggbb code)…"})
	r := tui.Menu(e.S, tui.MenuOpts{Title: title, Items: items, Selected: sel})
	if !r.OK {
		return "", false
	}
	if r.Index < len(palette) {
		return palette[r.Index].hex, true
	}
	for {
		v, ok := tui.Input(e.S, title, []string{"A colour code like #ff8800."}, "Colour:", cur)
		if !ok {
			return "", false
		}
		if !strings.HasPrefix(v, "#") {
			v = "#" + v
		}
		if _, err := config.ParseColor(v); err == nil {
			return strings.ToLower(v), true
		}
		tui.Message(e.S, title, []string{"That isn't a colour code. Use 6 hex digits, like #ff8800."})
	}
}

// ---- pages and deck settings ----

func (e *Editor) pagesMenu() {
	for {
		names := e.pageNames()
		var items []tui.Item
		for _, n := range names {
			d := fmt.Sprintf("%d buttons", len(e.L.Pages[n]))
			if n == e.L.StartPage {
				d += " · start page"
			}
			items = append(items, tui.Item{Label: n, Detail: d})
		}
		items = append(items, tui.Item{Label: "+ Add a page"}, tui.Item{Label: "Start over (remove all buttons and pages)"})
		e.trace("screen pages")
		r := tui.Menu(e.S, tui.MenuOpts{Title: "Pages", Items: items, Intro: []string{"  A page is a set of 15 buttons. A \"Go to page\" action switches between them."}})
		if !r.OK {
			return
		}
		if r.Index == len(names)+1 {
			if tui.Confirm(e.S, "Start over", "Remove every button on every page?") {
				e.L.Pages = map[string][]config.Button{"home": nil}
				e.L.StartPage, e.page, e.sel = "home", "home", 0
				e.save()
				e.trace("start-over")
				return
			}
			continue
		}
		if r.Index == len(names) {
			if n, ok := e.newPage(); ok {
				e.page = n
				e.save()
				return
			}
			continue
		}
		name := names[r.Index]
		r2 := tui.Menu(e.S, tui.MenuOpts{Title: "Page " + name,
			Items: []tui.Item{{Label: "Edit this page"}, {Label: "Rename"}, {Label: "Make it the start page"}, {Label: "Delete"}, {Label: "Back"}}})
		if !r2.OK {
			continue
		}
		switch r2.Index {
		case 0:
			e.page = name
			return
		case 1:
			n, ok := tui.Input(e.S, "Rename page", nil, "New name:", name)
			if !ok || n == "" || n == name {
				continue
			}
			if _, exists := e.L.Pages[n]; exists {
				tui.Message(e.S, "Rename page", []string{"There's already a page called " + n + "."})
				continue
			}
			e.L.Pages[n] = e.L.Pages[name]
			delete(e.L.Pages, name)
			e.forEachAction(func(a *config.Action) {
				if a.Type == config.Page && a.Page == name {
					a.Page = n
				}
			})
			if e.L.StartPage == name {
				e.L.StartPage = n
			}
			if e.page == name {
				e.page = n
			}
			e.save()
		case 2:
			e.L.StartPage = name
			e.save()
		case 3:
			if len(e.L.Pages) == 1 {
				tui.Message(e.S, "Delete page", []string{"You need at least one page."})
				continue
			}
			if !tui.Confirm(e.S, "Delete page", fmt.Sprintf("Delete page %q and its %d buttons?", name, len(e.L.Pages[name]))) {
				continue
			}
			delete(e.L.Pages, name)
			// Buttons that went to this page now do nothing there.
			for p, list := range e.L.Pages {
				for i := range list {
					var keep []config.Action
					for _, a := range list[i].Actions {
						if !(a.Type == config.Page && a.Page == name) {
							keep = append(keep, a)
						}
					}
					list[i].Actions = keep
				}
				e.L.Pages[p] = list
			}
			if e.L.StartPage == name {
				e.L.StartPage = e.pageNames()[0]
			}
			if e.page == name {
				e.page = e.L.StartPage
			}
			e.save()
		}
	}
}

func (e *Editor) forEachAction(f func(*config.Action)) {
	for p, list := range e.L.Pages {
		for i := range list {
			for j := range list[i].Actions {
				f(&list[i].Actions[j])
			}
		}
		e.L.Pages[p] = list
	}
}

func (e *Editor) deckSettings() {
	for {
		dim := "never"
		if e.L.IdleDimSeconds > 0 {
			dim = fmt.Sprintf("after %d min", (e.L.IdleDimSeconds+59)/60)
		}
		e.trace("screen deck-settings")
		r := tui.Menu(e.S, tui.MenuOpts{Title: "Stream Deck settings", Items: []tui.Item{
			{Label: "Brightness", Detail: fmt.Sprintf("%d%%", e.L.Brightness)},
			{Label: "Dim when idle", Detail: dim},
			{Label: "Dimmed brightness", Detail: fmt.Sprintf("%d%%", e.L.IdleBrightness)},
			{Label: "Back"},
		}})
		if !r.OK || r.Index == 3 {
			return
		}
		switch r.Index {
		case 0:
			if v, ok := tui.Number(e.S, "Brightness", "Brightness:", e.L.Brightness, 0, 100, 5, "%"); ok {
				e.L.Brightness = v
			}
		case 1:
			if v, ok := tui.Number(e.S, "Dim when idle", "Minutes without a press before dimming (0 = never):", (e.L.IdleDimSeconds+59)/60, 0, 240, 5, " min"); ok {
				e.L.IdleDimSeconds = v * 60
			}
		case 2:
			if v, ok := tui.Number(e.S, "Dimmed brightness", "Brightness while dimmed:", e.L.IdleBrightness, 0, 100, 5, "%"); ok {
				e.L.IdleBrightness = v
			}
		}
		e.save()
	}
}
