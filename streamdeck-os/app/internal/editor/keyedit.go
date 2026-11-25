package editor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"streamdeckd/internal/config"
	"streamdeckd/internal/spotify"
	"streamdeckd/internal/ui"
)

// editKey edits the selected key; changes show on the deck immediately and
// are saved when leaving.
func (e *Editor) editKey(ctx context.Context) {
	orig, existed := e.button(e.page, e.sel)
	b := orig
	b.Key = e.sel
	b.Actions = append([]config.Action(nil), orig.Actions...)
	sel := 0
	e.fresh = ""
	for {
		e.setButton(e.page, b)
		e.preview()

		var items []ui.Item
		openIdx := -1
		if f := folderOf(b); f != "" {
			openIdx = len(items)
			items = append(items, ui.Item{Label: "Open this folder", Detail: "add and edit the buttons inside " + f, Icon: "folder"})
		}
		base := len(items)
		items = append(items,
			ui.Item{Label: "Appearance", Header: true},
			ui.Item{Label: "Label", Value: orDash(b.Label), Icon: "key"},
			ui.Item{Label: "Key colour", Value: colorName(b.Color, "#202020"), Swatch: or(b.Color, "#202020"), Icon: "light"},
			ui.Item{Label: "Text colour", Value: colorName(b.TextColor, "#ffffff"), Swatch: or(b.TextColor, "#ffffff"), Icon: "light"},
			ui.Item{Label: "What it does, in order", Header: true},
		)
		labelIdx, colIdx, txtIdx := base+1, base+2, base+3
		firstAct := len(items)
		for i, a := range b.Actions {
			s, warn := e.describe(a)
			it := ui.Item{Label: fmt.Sprintf("%d. %s", i+1, s), Icon: actionIcon(a.Type)}
			if warn != "" {
				it.Detail = "! " + warn
				it.Danger = true
			}
			items = append(items, it)
		}
		addIdx := len(items)
		items = append(items, ui.Item{Label: "+ Add an action", Icon: "plus"}, ui.Item{Label: "", Header: true},
			ui.Item{Label: "Try it now", Icon: "play"}, ui.Item{Label: "Clear this key", Icon: "trash", Danger: true}, ui.Item{Label: "Done", Icon: "check"})
		tryIdx, clearIdx, doneIdx := addIdx+2, addIdx+3, addIdx+4
		if sel == 0 {
			sel = labelIdx
		}
		if sel == labelIdx && b.Label != "" && len(b.Actions) == 0 {
			sel = addIdx // named it: next, say what it does
		}
		e.trace("screen key %d", e.sel)
		title := fmt.Sprintf("Key %d", e.sel+1)
		if len(e.stack) > 1 {
			title += "  ·  " + e.page
		}
		r := e.U.Menu(ui.MenuOpts{ID: "key", Title: title, Items: items, Selected: sel})
		if !r.OK {
			r.Index = doneIdx
		}
		sel = r.Index
		switch {
		case r.Index == openIdx:
			e.setButton(e.page, b)
			if e.save() {
				e.open(folderOf(b))
				return
			}
			e.U.Message(ui.Note{ID: "save-error", Kind: ui.ErrorNote, Title: "Can't save this key", Lines: strings.Split(e.err, "\n")})
		case r.Index == labelIdx:
			if v, ok := e.U.Input(ui.InputOpts{ID: "label", Title: "Label", Intro: []string{"Text shown on the key. Keep it short: about 2 lines of 7 letters."}, Label: "Label", Initial: b.Label}); ok {
				b.Label = v
				e.trace("label %q", v)
			}
		case r.Index == colIdx:
			if c, ok := e.pickColor("Key colour", b.Color, "#202020"); ok {
				b.Color = c
			}
		case r.Index == txtIdx:
			if c, ok := e.pickColor("Text colour", b.TextColor, "#ffffff"); ok {
				b.TextColor = c
			}
		case r.Index >= firstAct && r.Index < addIdx:
			b.Actions = e.actionMenu(ctx, b.Actions, r.Index-firstAct)
		case r.Index == addIdx:
			if a, ok := e.newAction(ctx); ok {
				b.Actions = append(b.Actions, a)
				if a.Type == config.Page && b.Label == "" {
					b.Label = a.Page
				}
				sel = addIdx + 1 // "+ Add" again: the list grew by one
				if a.Type == config.Page && openIdx < 0 {
					sel++ // and "Open this folder" appeared at the top
				}
			}
		case r.Index == tryIdx:
			e.try(ctx, b)
		case r.Index == clearIdx:
			if e.U.Confirm(ui.ConfirmOpts{ID: "clear-key", Title: "Clear key", Question: fmt.Sprintf("Remove everything from key %d?", e.sel+1), Yes: "Clear", Danger: true}) {
				b = config.Button{Key: e.sel}
				e.setButton(e.page, b)
				e.save()
				return
			}
		case r.Index == doneIdx:
			e.setButton(e.page, b)
			if !e.save() {
				r := e.U.Menu(ui.MenuOpts{
					ID:    "save-error",
					Title: "Can't save this key",
					Intro: append([]string{"Problem:"}, strings.Split(e.err, "\n")...),
					Items: []ui.Item{{Label: "Go back and fix it", Icon: "back"}, {Label: "Undo my changes to this key", Icon: "trash", Danger: true}},
				})
				if r.OK && r.Index == 1 {
					// Back to what's already saved on disk: nothing to write.
					if existed {
						e.setButton(e.page, orig)
					} else {
						e.setButton(e.page, config.Button{Key: e.sel})
					}
					if e.fresh != "" {
						delete(e.L.Pages, e.fresh)
						e.fresh = ""
					}
					e.err = ""
					e.trace("undo %d", e.sel)
					return
				}
				continue
			}
			if f := e.fresh; f != "" && folderOf(b) == f {
				e.fresh = ""
				if e.U.Confirm(ui.ConfirmOpts{ID: "open-new-folder", Title: "Folder created",
					Question: fmt.Sprintf("Open %q now to add its buttons? It already has a Back key.", f), Yes: "Open it", No: "Later"}) {
					e.open(f)
				}
			}
			return
		}
	}
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
	r := e.U.Menu(ui.MenuOpts{
		ID:       "action",
		Title:    fmt.Sprintf("Action %d", i+1),
		Subtitle: s,
		Items: []ui.Item{{Label: "Change it", Icon: "gear"}, {Label: "Move up", Icon: "chevron"}, {Label: "Move down", Icon: "chevron"},
			{Label: "Delete it", Icon: "trash", Danger: true}},
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
	{"Lights (Govee)", "Set white temperature (brightest)", config.GoveeColorTemp},
	{"Music (Spotify)", "Play a playlist", config.SpotifyPlaylist},
	{"Music (Spotify)", "Pause", config.SpotifyPause},
	{"Music (Spotify)", "Next song", config.SpotifyNext},
	{"Music (Spotify)", "Set volume", config.SpotifyVolume},
	{"Folders", "Open a folder of buttons", config.Page},
	{"Other", "Wait a moment", config.Delay},
}

func (e *Editor) newAction(ctx context.Context) (config.Action, bool) {
	var items []ui.Item
	var types []string
	group := ""
	for _, t := range actionTypes {
		if t.group != group {
			items = append(items, ui.Item{Label: t.group, Header: true})
			types = append(types, "")
			group = t.group
		}
		items = append(items, ui.Item{Label: t.label, Icon: actionIcon(t.typ)})
		types = append(types, t.typ)
	}
	e.trace("screen add-action")
	r := e.U.Menu(ui.MenuOpts{ID: "add-action", Title: "Add an action", Items: items})
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
		lights, ok := e.pickLights(ctx, a.Lights(), true)
		if !ok {
			return a, false
		}
		scene, diy, ok := e.pickScene(ctx, lights, a.Scene)
		if !ok {
			return a, false
		}
		a.SetLights(lights)
		a.Scene, a.Type = scene, config.GoveeScene
		if diy {
			a.Type = config.GoveeDIYScene
		}
	case config.GoveePower:
		lights, ok := e.pickLights(ctx, a.Lights(), false)
		if !ok {
			return a, false
		}
		r := e.U.Menu(ui.MenuOpts{ID: "power", Title: e.lightsLabel(lights), Items: []ui.Item{{Label: "Turn on", Icon: "check"}, {Label: "Turn off", Icon: "pause"}}})
		if !r.OK {
			return a, false
		}
		on := r.Index == 0
		a.SetLights(lights)
		a.On = &on
	case config.GoveeBrightness:
		lights, ok := e.pickLights(ctx, a.Lights(), false)
		if !ok {
			return a, false
		}
		v, ok := e.U.Number(ui.NumberOpts{ID: "brightness", Title: e.lightsLabel(lights), Label: "Brightness", Value: or0(a.Brightness, 50), Min: 1, Max: 100, Step: 5, Unit: "%"})
		if !ok {
			return a, false
		}
		a.SetLights(lights)
		a.Brightness = v
	case config.GoveeColor:
		lights, ok := e.pickLights(ctx, a.Lights(), false)
		if !ok {
			return a, false
		}
		c, ok := e.pickColor("Colour for "+e.lightsLabel(lights), a.Color, "#ffffff")
		if !ok {
			return a, false
		}
		a.SetLights(lights)
		a.Color = c
	case config.GoveeColorTemp:
		lights, ok := e.pickLights(ctx, a.Lights(), false)
		if !ok {
			return a, false
		}
		k, ok := e.pickKelvin(e.lightsLabel(lights), a.Kelvin)
		if !ok {
			return a, false
		}
		a.SetLights(lights)
		a.Kelvin = k
	case config.SpotifyPlaylist:
		uri, ok := e.pickPlaylist(ctx, a.Playlist)
		if !ok {
			return a, false
		}
		spk, ok := e.pickSpeaker(ctx, a.SpotifyDevice)
		if !ok {
			return a, false
		}
		r := e.U.Menu(ui.MenuOpts{ID: "shuffle", Title: "Shuffle?", Items: []ui.Item{{Label: "Play in order", Icon: "play"}, {Label: "Shuffle", Icon: "next"}}, Selected: b2i(a.Shuffle)})
		if !r.OK {
			return a, false
		}
		shuffle := r.Index == 1
		vsel := 0
		if a.Volume != nil {
			vsel = 1
		}
		r = e.U.Menu(ui.MenuOpts{ID: "set-volume", Title: "Volume", Items: []ui.Item{{Label: "Leave the volume as it is", Icon: "volume"}, {Label: "Set the volume", Icon: "volume"}}, Selected: vsel})
		if !r.OK {
			return a, false
		}
		var vol *int
		if r.Index == 1 {
			start := 50
			if a.Volume != nil {
				start = *a.Volume
			}
			v, ok := e.U.Number(ui.NumberOpts{ID: "volume", Title: "Volume", Label: "Volume", Value: start, Min: 0, Max: 100, Step: 5, Unit: "%"})
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
		v, ok := e.U.Number(ui.NumberOpts{ID: "volume", Title: "Volume", Label: "Volume", Value: start, Min: 0, Max: 100, Step: 5, Unit: "%"})
		if !ok {
			return a, false
		}
		a.SpotifyDevice, a.Volume = spk, &v
	case config.Page:
		p, ok := e.pickFolder(a.Page)
		if !ok {
			return a, false
		}
		a.Page = p
	case config.Delay:
		v, ok := e.U.Number(ui.NumberOpts{ID: "wait", Title: "Wait", Label: "Seconds to wait", Value: max(1, a.Ms/1000), Min: 1, Max: 60, Step: 1, Unit: " s"})
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

// lightChoice is a light in the picker.
type lightChoice struct {
	ref, label, detail, icon string
}

// pickLights lists the Govee cloud lights and the lights on the network
// (LAN) as a checklist: one action can set several lights at once (e.g.
// four can lights). Scenes only exist in the cloud, so forScene leaves LAN
// lights out.
func (e *Editor) pickLights(ctx context.Context, cur []string, forScene bool) ([]string, bool) {
	var all []lightChoice
	var cloudErr error
	var devs []GoveeDevice
	if e.Src.GoveeReady() || len(e.L.LANLights) == 0 {
		e.U.Info(ui.InfoView{ID: "loading-lights", Title: "Lights", Spinner: true, Lines: []string{"Getting your lights from Govee…"}})
		devs, cloudErr = e.loadGovee(ctx)
		for _, d := range devs {
			if d.Group {
				continue
			}
			if _, lan := config.FindLANLight(e.L.LANLights, d.Name); lan {
				continue // listed below, as a LAN light
			}
			all = append(all, lightChoice{ref: lightRef(d, devs), label: lightLabel(d, devs), icon: "light",
				detail: fmt.Sprintf("Govee cloud · %d scenes", len(d.Scenes)+len(d.DIY))})
		}
	}
	lanLeftOut := 0
	for _, l := range e.L.LANLights {
		if forScene {
			lanLeftOut++
			continue
		}
		all = append(all, lightChoice{ref: l.Name, label: l.Name, icon: "wifi", detail: "on your network (LAN) · " + l.SKU})
	}
	if len(all) == 0 {
		switch {
		case lanLeftOut > 0:
			e.U.Message(ui.Note{ID: "no-scene-lights", Kind: ui.WarningNote, Title: "No lights with scenes",
				Lines: []string{"Your lights on the network (LAN) can be turned on and off, dimmed and coloured, but Govee's LAN control has no scenes.",
					"Scenes need a light from the Govee cloud (Home > Lights > API key)."}})
		case cloudErr != nil:
			e.U.Message(ui.Note{ID: "lights-error", Kind: ui.ErrorNote, Title: "Couldn't get your lights", Lines: []string{cloudErr.Error()}})
		default:
			e.U.Message(ui.Note{ID: "no-lights", Kind: ui.WarningNote, Title: "No lights",
				Lines: []string{"No lights found. Add your Govee API key, or add lights on your network, under Home > Lights."}})
		}
		return nil, false
	}
	chosen := map[string]bool{}
	for _, c := range cur {
		for _, l := range all {
			if strings.EqualFold(l.ref, c) || e.sameRef(devs, l.ref, c) {
				chosen[l.ref] = true
			}
		}
	}
	sel := -1
	for {
		var items []ui.Item
		var acts []string
		add := func(it ui.Item, act string) { items, acts = append(items, it), append(acts, act) }
		n := 0
		for _, l := range all {
			if chosen[l.ref] {
				n++
			}
		}
		for _, l := range all {
			it := ui.Item{Label: l.label, Detail: l.detail, Icon: l.icon}
			if chosen[l.ref] {
				it.Icon, it.Value = "check", "Selected"
			}
			add(it, "light:"+l.ref)
		}
		add(ui.Item{Label: "", Header: true}, "")
		done := ui.Item{Label: "Done", Detail: fmt.Sprintf("%d selected", n), Icon: "check"}
		if n == 0 {
			done.Detail = "select one or more lights first"
		}
		add(done, "done")
		if sel < 0 {
			sel = len(items) - 1
			if n == 0 {
				for i, a := range acts {
					if strings.HasPrefix(a, "light:") {
						sel = i
						break
					}
				}
			}
		}
		intro := []string{"Select one or more lights: they all get the same setting. Enter or click selects and unselects."}
		if forScene && lanLeftOut > 0 {
			intro = append(intro, "Lights on your network (LAN) aren't listed: Govee's LAN control has no scenes.")
		}
		e.trace("screen pick-light")
		r := e.U.Menu(ui.MenuOpts{ID: "pick-light", Title: "Which lights?", Intro: intro, Items: items, Selected: sel, Filter: len(all) > 8})
		if !r.OK {
			return nil, false
		}
		sel = r.Index
		switch act := acts[r.Index]; {
		case act == "done":
			if n == 0 {
				continue
			}
			var out []string
			for _, l := range all {
				if chosen[l.ref] {
					out = append(out, l.ref)
				}
			}
			e.trace("lights %s", strings.Join(out, "|"))
			return out, true
		case strings.HasPrefix(act, "light:"):
			ref := strings.TrimPrefix(act, "light:")
			chosen[ref] = !chosen[ref]
		}
	}
}

// pickScene offers the scenes every chosen light has.
func (e *Editor) pickScene(ctx context.Context, lights []string, cur string) (string, bool, bool) {
	devs, _ := e.loadGovee(ctx)
	common := func(get func(GoveeDevice) []string) []string {
		var out []string
		for i, ref := range lights {
			var have []string
			for _, x := range devs {
				if sameLight(x, ref) {
					have = get(x)
				}
			}
			if i == 0 {
				out = append(out, have...)
				continue
			}
			var keep []string
			for _, n := range out {
				for _, h := range have {
					if strings.EqualFold(n, h) {
						keep = append(keep, n)
						break
					}
				}
			}
			out = keep
		}
		return out
	}
	scenes := common(func(d GoveeDevice) []string { return d.Scenes })
	diys := common(func(d GoveeDevice) []string { return d.DIY })
	var items []ui.Item
	var names []string
	var diy []bool
	sel := 0
	add := func(header string, list []string, isDIY bool) {
		if len(list) == 0 {
			return
		}
		items = append(items, ui.Item{Label: header, Header: true})
		names = append(names, "")
		diy = append(diy, false)
		for _, n := range list {
			if strings.EqualFold(n, cur) {
				sel = len(items)
			}
			items = append(items, ui.Item{Label: n})
			names = append(names, n)
			diy = append(diy, isDIY)
		}
	}
	add("Scenes", scenes, false)
	add("Your DIY scenes", diys, true)
	title := e.lightsLabel(lights)
	if len(names) == 0 {
		msg := "This light has no scenes."
		if len(lights) > 1 {
			msg = "These lights have no scene in common. Pick lights of the same kind, or give each its own action."
		}
		e.U.Message(ui.Note{ID: "no-scenes", Kind: ui.WarningNote, Title: title, Lines: []string{msg}})
		return "", false, false
	}
	intro := []string(nil)
	if len(lights) > 1 {
		intro = []string{"Scenes all the selected lights have."}
	}
	e.trace("screen pick-scene")
	r := e.U.Menu(ui.MenuOpts{ID: "pick-scene", Title: "Scene for " + title, Intro: intro, Items: items, Selected: sel, Filter: true})
	if !r.OK {
		return "", false, false
	}
	return names[r.Index], diy[r.Index], true
}

func (e *Editor) pickPlaylist(ctx context.Context, cur string) (string, bool) {
	curURI, _ := spotify.NormalizeURI(cur)
	for {
		e.U.Info(ui.InfoView{ID: "loading-playlists", Title: "Playlists", Spinner: true, Lines: []string{"Getting your playlists from Spotify…"}})
		pls, err := e.loadPlaylists(ctx)
		var items []ui.Item
		type choice struct{ uri, kind string }
		var choices []choice
		if errors.Is(err, spotify.ErrNeedsReconnect) {
			items = append(items, ui.Item{Label: "Reconnect Spotify so your playlists can be listed", Detail: "one time, uses your phone"})
			choices = append(choices, choice{kind: "reconnect"})
		} else if err != nil {
			items = append(items, ui.Item{Label: "Couldn't get your playlists", Detail: err.Error(), Header: true})
			choices = append(choices, choice{})
		}
		sel := 0
		if len(pls) > 0 {
			items = append(items, ui.Item{Label: "Your playlists", Header: true})
			choices = append(choices, choice{})
			for _, p := range pls {
				if p.URI == curURI {
					sel = len(items)
				}
				items = append(items, ui.Item{Label: p.Name, Detail: fmt.Sprintf("by %s · %d songs", p.Owner, p.Tracks)})
				choices = append(choices, choice{uri: p.URI})
			}
		}
		items = append(items, ui.Item{Label: "", Header: true}, ui.Item{Label: "Type a playlist link instead…"})
		choices = append(choices, choice{}, choice{kind: "type"})
		e.trace("screen pick-playlist %d", len(pls))
		r := e.U.Menu(ui.MenuOpts{ID: "pick-playlist", Title: "Which playlist?", Items: items, Selected: sel, Filter: len(pls) > 8})
		if !r.OK {
			return "", false
		}
		switch c := choices[r.Index]; {
		case c.kind == "reconnect":
			if err := e.Src.ReconnectSpotify(ctx); err != nil {
				e.U.Message(ui.Note{ID: "reconnect-error", Kind: ui.ErrorNote, Title: "Reconnecting failed", Lines: []string{err.Error()}})
			}
			e.mu.Lock()
			e.playlists, e.plErr = nil, nil
			e.mu.Unlock()
		case c.kind == "type":
			for {
				v, ok := e.U.Input(ui.InputOpts{ID: "playlist-link", Title: "Playlist link", Intro: []string{
					"In Spotify: open the playlist > Share > Copy link, then type it here,",
					"or type its URI, e.g. spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
				}, Label: "Link", Initial: cur})
				if !ok {
					break
				}
				if uri, err := spotify.NormalizeURI(v); err == nil {
					return uri, true
				}
				e.U.Message(ui.Note{ID: "bad-link", Kind: ui.WarningNote, Title: "Playlist link", Lines: []string{"That doesn't look like a Spotify playlist, album or artist link."}})
			}
		case c.uri != "":
			return c.uri, true
		}
	}
}

func (e *Editor) pickSpeaker(ctx context.Context, cur string) (string, bool) {
	e.U.Info(ui.InfoView{ID: "loading-speakers", Title: "Speaker", Spinner: true, Lines: []string{"Asking Spotify which speakers are on…"}})
	cctx, cancel := context.WithTimeout(ctx, 20e9)
	devs, err := e.Src.SpotifyDevices(cctx)
	cancel()
	def := e.Src.DefaultSpeaker()
	items := []ui.Item{{Label: "The default speaker", Detail: or(def, "not set: use whichever is playing")}}
	vals := []string{""}
	sel := 0
	if err != nil {
		items = append(items, ui.Item{Label: "Couldn't list speakers", Detail: err.Error(), Header: true})
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
		items = append(items, ui.Item{Label: d})
		vals = append(vals, d)
	}
	items = append(items, ui.Item{Label: "Type a speaker name…", Detail: "for one that's off right now"})
	vals = append(vals, "\x00type")
	e.trace("screen pick-speaker")
	r := e.U.Menu(ui.MenuOpts{ID: "pick-speaker", Title: "Play on which speaker?", Intro: []string{"Speakers only show up while Spotify is open on them."}, Items: items, Selected: sel})
	if !r.OK {
		return "", false
	}
	if vals[r.Index] == "\x00type" {
		return e.U.Input(ui.InputOpts{ID: "speaker", Title: "Speaker", Intro: []string{"Exactly as it appears in Spotify's device list."}, Label: "Speaker name", Initial: cur})
	}
	return vals[r.Index], true
}

func (e *Editor) pickFolder(cur string) (string, bool) {
	var items []ui.Item
	names := e.pageNames()
	sel := 0
	for i, n := range names {
		if n == cur {
			sel = i
		}
		d := fmt.Sprintf("%d buttons", len(e.L.Pages[n]))
		if n == e.L.StartPage {
			d = "the main buttons"
		}
		items = append(items, ui.Item{Label: n, Detail: d, Icon: "folder"})
	}
	items = append(items, ui.Item{Label: "+ New folder…", Icon: "plus"})
	if cur == "" {
		sel = len(names)
	}
	e.trace("screen pick-folder")
	r := e.U.Menu(ui.MenuOpts{ID: "pick-folder", Title: "Open which folder?", Items: items, Selected: sel})
	if !r.OK {
		return "", false
	}
	if r.Index < len(names) {
		return names[r.Index], true
	}
	n, ok := e.newFolder(e.page)
	if ok {
		e.fresh = n
	}
	return n, ok
}

var palette = []struct{ name, hex string }{
	{"White", "#ffffff"}, {"Warm white", "#ffd6a0"}, {"Cool white", "#dbe8ff"}, {"Grey", "#6b6b6b"}, {"Charcoal", "#202020"},
	{"Red", "#c0262d"}, {"Coral", "#ff6f5e"}, {"Orange", "#e0701b"}, {"Amber", "#ffb000"}, {"Yellow", "#e8c020"},
	{"Lime", "#8bd400"}, {"Green", "#2f9e44"}, {"Spotify green", "#1db954"}, {"Teal", "#108a8a"}, {"Cyan", "#00c8d6"},
	{"Sky", "#4fa3ff"}, {"Blue", "#1d5fa0"}, {"Navy", "#1b2a6b"}, {"Lavender", "#b49cff"}, {"Purple", "#6a2fb0"},
	{"Magenta", "#d000a0"}, {"Hot pink", "#ff4fa0"}, {"Pink", "#c2185b"}, {"Brown", "#7a4a24"}, {"Black", "#000000"},
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
	presets := make([]ui.Swatch, len(palette))
	for i, p := range palette {
		presets[i] = ui.Swatch{Name: p.name, Hex: p.hex}
	}
	e.trace("screen color")
	return e.U.Color(ui.ColorOpts{ID: "color", Title: title, Value: or(cur, def), Presets: presets})
}

// ---- folders and deck settings ----

func (e *Editor) foldersMenu() {
	for {
		names := e.pageNames()
		var items []ui.Item
		for _, n := range names {
			d := fmt.Sprintf("%d buttons", len(e.L.Pages[n]))
			if n == e.L.StartPage {
				d += " · shown when the deck starts"
			}
			items = append(items, ui.Item{Label: n, Detail: d, Icon: "folder"})
		}
		items = append(items, ui.Item{Label: "", Header: true},
			ui.Item{Label: "Start over", Detail: "remove all buttons and folders", Icon: "trash", Danger: true})
		e.trace("screen folders")
		r := e.U.Menu(ui.MenuOpts{ID: "folders", Title: "Folders", Items: items,
			Intro: []string{"A folder is a set of 15 buttons, opened by a key on the deck. To make one, pick an empty key and choose New folder."}})
		if !r.OK {
			return
		}
		if r.Index == len(names)+1 {
			if e.U.Confirm(ui.ConfirmOpts{ID: "start-over", Title: "Start over", Question: "Remove every button in every folder?", Yes: "Remove all", Danger: true}) {
				e.L.Pages = map[string][]config.Button{"home": nil}
				e.L.StartPage = "home"
				e.jump("home")
				e.save()
				e.trace("start-over")
				return
			}
			continue
		}
		name := names[r.Index]
		r2 := e.U.Menu(ui.MenuOpts{ID: "folder", Title: name, Items: []ui.Item{
			{Label: "Open it", Icon: "folder"}, {Label: "Rename", Icon: "key"},
			{Label: "Show it when the deck starts", Icon: "deck"}, {Label: "Delete", Icon: "trash", Danger: true}}})
		if !r2.OK {
			continue
		}
		switch r2.Index {
		case 0:
			e.jump(name)
			return
		case 1:
			n, ok := e.U.Input(ui.InputOpts{ID: "folder-rename", Title: "Rename folder", Label: "New name", Initial: name})
			if !ok || n == "" || n == name {
				continue
			}
			if _, exists := e.L.Pages[n]; exists {
				e.U.Message(ui.Note{ID: "folder-exists", Kind: ui.WarningNote, Title: "Rename folder", Lines: []string{"There's already a folder called " + n + "."}})
				continue
			}
			e.L.Pages[n] = e.L.Pages[name]
			delete(e.L.Pages, name)
			e.forEachAction(func(a *config.Action) {
				if a.Type == config.Page && a.Page == name {
					a.Page = n
				}
			})
			// Keys still labelled with the old folder name follow the rename.
			for p, list := range e.L.Pages {
				for i := range list {
					if list[i].Label == name && folderOf(list[i]) == n {
						list[i].Label = n
					}
				}
				e.L.Pages[p] = list
			}
			if e.L.StartPage == name {
				e.L.StartPage = n
			}
			for i := range e.stack {
				if e.stack[i] == name {
					e.stack[i] = n
				}
			}
			if e.page == name {
				e.page = n
			}
			e.save()
		case 2:
			e.L.StartPage = name
			e.jump(name)
			e.save()
		case 3:
			if len(e.L.Pages) == 1 {
				e.U.Message(ui.Note{ID: "last-folder", Kind: ui.WarningNote, Title: "Delete folder", Lines: []string{"You need at least one folder."}})
				continue
			}
			if !e.U.Confirm(ui.ConfirmOpts{ID: "delete-folder", Title: "Delete folder", Question: fmt.Sprintf("Delete %q and its %d buttons?", name, len(e.L.Pages[name])), Yes: "Delete", Danger: true}) {
				continue
			}
			delete(e.L.Pages, name)
			// Keys that opened this folder now do nothing there.
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
			for _, p := range e.stack {
				if p == name {
					e.jump(e.L.StartPage)
					break
				}
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
	sel := 0
	for {
		dim := "never"
		if e.L.IdleDimSeconds > 0 {
			dim = fmt.Sprintf("after %d min", (e.L.IdleDimSeconds+59)/60)
		}
		e.trace("screen deck-settings")
		r := e.U.Menu(ui.MenuOpts{ID: "deck-settings", Title: "Stream Deck", Selected: sel, Items: []ui.Item{
			{Label: "Brightness", Value: fmt.Sprintf("%d%%", e.L.Brightness), Icon: "light"},
			{Label: "Dim when idle", Value: dim, Icon: "wait"},
			{Label: "Dimmed brightness", Value: fmt.Sprintf("%d%%", e.L.IdleBrightness), Icon: "light"},
		}})
		if !r.OK {
			return
		}
		sel = r.Index
		switch r.Index {
		case 0:
			if v, ok := e.U.Number(ui.NumberOpts{ID: "deck-brightness", Title: "Brightness", Label: "Key brightness", Value: e.L.Brightness, Min: 0, Max: 100, Step: 5, Unit: "%"}); ok {
				e.L.Brightness = v
			}
		case 1:
			if v, ok := e.U.Number(ui.NumberOpts{ID: "dim-after", Title: "Dim when idle", Label: "Minutes without a press before dimming (0 = never)", Value: (e.L.IdleDimSeconds + 59) / 60, Min: 0, Max: 240, Step: 5, Unit: " min"}); ok {
				e.L.IdleDimSeconds = v * 60
			}
		case 2:
			if v, ok := e.U.Number(ui.NumberOpts{ID: "dim-brightness", Title: "Dimmed brightness", Label: "Brightness while dimmed", Value: e.L.IdleBrightness, Min: 0, Max: 100, Step: 5, Unit: "%"}); ok {
				e.L.IdleBrightness = v
			}
		}
		e.save()
	}
}

// whites are common white colour temperatures.
var whites = []struct {
	name   string
	kelvin int
}{
	{"Candle", 2000}, {"Warm white", 2700}, {"Soft white", 3000}, {"Neutral white", 4000},
	{"Cool white", 5000}, {"Daylight", 6500}, {"Blue sky", 9000},
}

func whiteName(k int) string {
	for _, w := range whites {
		if w.kelvin == k {
			return w.name
		}
	}
	switch {
	case k < 3300:
		return "warm"
	case k < 5000:
		return "neutral"
	}
	return "cool"
}

// pickKelvin asks for a white colour temperature: presets or any value.
func (e *Editor) pickKelvin(title string, cur int) (int, bool) {
	items := make([]ui.Item, 0, len(whites)+1)
	sel := 1
	for i, w := range whites {
		items = append(items, ui.Item{Label: w.name, Value: fmt.Sprintf("%d K", w.kelvin), Swatch: kelvinHex(w.kelvin), Icon: "light"})
		if w.kelvin == cur {
			sel = i
		}
	}
	items = append(items, ui.Item{Label: "Other temperature…", Detail: "2000 K (warm, orange) to 9000 K (cool, blue)", Icon: "gear"})
	e.trace("screen kelvin")
	r := e.U.Menu(ui.MenuOpts{ID: "kelvin", Title: "White for " + title,
		Intro: []string{"White light is the brightest these lights get. Lower numbers are warmer (yellow), higher are cooler (blue)."},
		Items: items, Selected: sel})
	if !r.OK {
		return 0, false
	}
	if r.Index < len(whites) {
		return whites[r.Index].kelvin, true
	}
	return e.U.Number(ui.NumberOpts{ID: "kelvin-value", Title: "White for " + title, Label: "Colour temperature",
		Value: max(2000, min(9000, or0(cur, 4000))), Min: 2000, Max: 9000, Step: 100, Unit: " K"})
}

// kelvinHex approximates how a white temperature looks (for swatches).
func kelvinHex(k int) string {
	t := float64(k) / 100
	var r, g, b float64
	if t <= 66 {
		r = 255
		g = 99.47*math.Log(t) - 161.12
		if t <= 19 {
			b = 0
		} else {
			b = 138.52*math.Log(t-10) - 305.04
		}
	} else {
		r = 329.7 * math.Pow(t-60, -0.1332)
		g = 288.12 * math.Pow(t-60, -0.0755)
		b = 255
	}
	c := func(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }
	return ui.Hex(c(r), c(g), c(b))
}
