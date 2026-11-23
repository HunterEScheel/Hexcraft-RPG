package setup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"streamdeckd/internal/install"
	"streamdeckd/internal/ui"
)

// Installer is the whole app on the USB stick: install (or update) and
// restart. Wi-Fi, keys and buttons are set up on the installed system.
func (a *App) Installer(ctx context.Context) error {
	a.trace("installer")
	for {
		if install.CheckPayload(a.PayloadDir) != nil {
			// Not found at boot (slow stick, unusual way of writing it): look again.
			a.U.Info(ui.InfoView{ID: "find-media", Title: "Install StreamDeck OS", Spinner: true,
				Lines: []string{"Looking for the installer files on the USB stick…"}})
			_ = exec.Command("/usr/sbin/find-live-media", "15").Run()
		}
		if err := install.CheckPayload(a.PayloadDir); err != nil {
			a.trace("install-no-media")
			r := a.U.Menu(ui.MenuOpts{ID: "no-media", Title: "Install StreamDeck OS", Root: true,
				Subtitle: "Can't find StreamDeck OS's files on the USB stick.",
				Intro: []string{"Nothing on this computer's disks was changed.",
					"Re-write the stick with balenaEtcher (or Rufus in \"DD Image\" mode) and start from it again."},
				Items: []ui.Item{{Label: "Look again", Icon: "wait"}, {Label: "Restart", Icon: "back"}}})
			if r.OK && r.Index == 1 {
				a.restart()
			}
			continue
		}
		if b, err := os.ReadFile("/run/streamdeck-media"); err == nil {
			a.BootDisk = install.MediaDisk("/sys/block", string(b))
		}
		installed := ""
		if a.FindInstalled != nil {
			installed = a.FindInstalled()
		}
		var items []ui.Item
		var acts []string
		add := func(it ui.Item, act string) { items, acts = append(items, it), append(acts, act) }
		if installed != "" {
			add(ui.Item{Label: "Update StreamDeck OS", Detail: "on " + installed + " · keeps your Wi-Fi, keys and buttons", Icon: "disk"}, "update")
			add(ui.Item{Label: "Erase and install fresh", Detail: "start over with nothing set up", Icon: "trash"}, "install")
		} else {
			add(ui.Item{Label: "Install StreamDeck OS", Detail: "onto this computer's disk", Icon: "disk"}, "install")
		}
		add(ui.Item{Label: "", Header: true}, "")
		add(ui.Item{Label: "Restart", Icon: "back"}, "restart")
		r := a.U.Menu(ui.MenuOpts{ID: "installer", Title: "Install StreamDeck OS", Root: true,
			Subtitle: "Welcome! This USB stick installs StreamDeck OS onto this computer.",
			Intro: []string{"Wi-Fi, your Govee key, Spotify and the buttons are set up after the install, " +
				"on the installed system, with this keyboard and touchpad."},
			Items: items})
		if !r.OK {
			continue
		}
		switch acts[r.Index] {
		case "update":
			a.update(installed)
		case "install":
			a.install()
		case "restart":
			a.restart()
		}
	}
}

func (a *App) restart() {
	a.U.Info(ui.InfoView{ID: "restarting", Title: "Restarting…", Spinner: true})
	if a.Reboot != nil {
		_ = a.Reboot()
	}
}

// progress shows a spinner with the last few log lines while f runs.
func (a *App) progress(id, title string, f func(say install.Progress) error) error {
	var mu sync.Mutex
	var log []string
	say := func(format string, args ...any) {
		mu.Lock()
		line := fmt.Sprintf(format, args...)
		a.trace("progress %s", line)
		log = append(log, line)
		if len(log) > 6 {
			log = log[len(log)-6:]
		}
		lines := append([]string(nil), log...)
		mu.Unlock()
		a.U.Info(ui.InfoView{ID: id, Title: title, Spinner: true, Lines: lines, Hint: "Don't turn the computer off"})
	}
	say("Starting…")
	return f(say)
}

func (a *App) install() {
	for {
		disks := install.ListDisks("/sys/block", a.BootDisk)
		if len(disks) == 0 {
			a.trace("install-no-disk")
			a.U.Message(ui.Note{ID: "no-disk", Kind: ui.WarningNote, Title: "No disk found",
				Lines: []string{"This computer has no disk StreamDeck OS can be installed to."}})
			return
		}
		items := make([]ui.Item, len(disks))
		for i, d := range disks {
			items[i] = ui.Item{Label: d.Model, Detail: d.String(), Icon: "disk"}
			if strings.TrimSpace(d.Model) == "" {
				items[i].Label = d.Name
			}
		}
		r := a.U.Menu(ui.MenuOpts{ID: "install-disk", Title: "Install to which disk?", Items: items})
		if !r.OK {
			return
		}
		d := disks[r.Index]
		if !a.U.Confirm(ui.ConfirmOpts{ID: "install-confirm", Title: "Erase " + d.Name + "?", Danger: true,
			Question: "EVERYTHING on " + d.String() + " will be erased, Windows included. This can't be undone.",
			Yes:      "Erase and install", No: "Cancel"}) {
			continue
		}
		err := a.progress("installing", "Installing StreamDeck OS", func(say install.Progress) error {
			return install.Install(d, a.PayloadDir, a.LayoutDir, "", say)
		})
		if err != nil {
			a.trace("install-error %v", err)
			a.U.Message(ui.Note{ID: "install-error", Kind: ui.ErrorNote, Title: "Install failed", Lines: []string{err.Error()}})
			continue
		}
		a.trace("installed %s", d.Name)
		a.done("Installed!")
		return
	}
}

func (a *App) update(part string) {
	err := a.progress("updating", "Updating StreamDeck OS", func(say install.Progress) error {
		return install.Update(part, a.PayloadDir, say)
	})
	if err != nil {
		a.trace("update-error %v", err)
		a.U.Message(ui.Note{ID: "update-error", Kind: ui.ErrorNote, Title: "Update failed", Lines: []string{err.Error()}})
		return
	}
	a.trace("updated")
	a.done("Updated!")
}

func (a *App) done(title string) {
	a.U.Message(ui.Note{ID: "install-done", Kind: ui.SuccessNote, Title: title, Lines: []string{
		"Remove the USB stick, then press Enter to restart.",
		"After the restart, set up Wi-Fi, your lights, Spotify and the buttons from the home screen.",
		"If it starts from the USB stick again, pick the internal disk in the boot menu (often F7, F11 or F12).",
	}})
	a.restart()
}
