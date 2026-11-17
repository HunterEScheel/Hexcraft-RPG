package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"streamdeckd/internal/modeswitch"
)

const modeswitchDone = "/run/streamdeck-modeswitch.done"

// runModeswitch ejects the "driver CD" of any USB Wi-Fi dongle still in that
// mode so it re-appears as a Wi-Fi adapter. Called by netwatch whenever USB
// devices change. Each plug-in is tried once.
func runModeswitch() error {
	var extra []string
	if v := os.Getenv("STREAMDECKD_MODESWITCH_EXTRA"); v != "" { // VM test only
		extra = strings.Split(v, ",")
	}
	skip := ""
	if b, err := os.ReadFile("/run/streamdeck-media"); err == nil {
		skip = filepath.Base(strings.TrimSpace(string(b)))
	}
	doneRaw, _ := os.ReadFile(modeswitchDone)
	done := string(doneRaw)
	for _, d := range modeswitch.Candidates(extra, skip) {
		if strings.Contains(done, d.Key()+"\n") {
			continue
		}
		desc := modeswitch.KnownIDs[d.ID]
		if desc == "" {
			desc = "test device"
		}
		if err := modeswitch.Eject(d); err != nil {
			fmt.Printf("modeswitch: %s (%s, %s): %v\n", d.ID, d.Name, desc, err)
		} else {
			fmt.Printf("modeswitch: switched %s (%s, %s) to Wi-Fi mode\n", d.ID, d.Name, desc)
		}
		f, err := os.OpenFile(modeswitchDone, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, d.Key())
			f.Close()
		}
	}
	return nil
}
