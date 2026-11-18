#!/usr/bin/env python3
"""On-screen button editor test (the HDMI console is the editor).

Live boot from the USB stick, run the installer, then from the status screen
open "Edit buttons" and build a key with only the keyboard (Dvorak) and the
Stream Deck itself:
  - press key 8 on the (virtual) Stream Deck: the editor jumps to it
  - label it, add a Govee scene picked from the account's lights/scenes,
    add a Spotify playlist picked from the account's playlists
  - the deck shows the edit live; leaving the key saves it; "T" tries it
  - after closing the editor, pressing key 8 on the deck runs it for real

Usage: test/editor-test.py <workdir containing streamdeck-os-test.iso>
"""
import os
import re
import sys
import time

sys.path.insert(0, os.path.dirname(__file__))
import vmlib
from vmlib import CLIENT_ID, CLIENT_SECRET, GOVEE_KEY, Monitor, Shell, check, deck, deck_images, finish, save_deck, setup, wait_log


def main():
    setup(sys.argv[1], results="results-editor")
    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("editor", usb_stick=True, shell=sock)
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        wait_log(r"TEST: boot complete", 300)

        # Installer (Govee + Spotify; the Spotify code is typed on the console).
        wait_log(r"SETUP: prompt wifi-optional", 120)
        mon.type("n")
        wait_log(r"SETUP: prompt govee-key")
        mon.type(GOVEE_KEY)
        wait_log(r"SETUP: govee-ok")
        wait_log(r"SETUP: prompt spotify-id")
        mon.type(CLIENT_ID)
        wait_log(r"SETUP: prompt spotify-secret")
        mon.type(CLIENT_SECRET)
        wait_log(r"SETUP: spotify-wait")
        mon.type("MOCKCODE")
        wait_log(r"SETUP: prompt spotify-device")
        mon.type("1")
        wait_log(r"SETUP: prompt install-disk")
        mon.type("")  # keep running from USB
        wait_log(r"config loaded: 2 page", 60)
        time.sleep(2)

        # Status screen -> 1) Edit buttons
        mark = len(vmlib.log_text())
        mon.type("1")
        wait_log(r"EDITOR: open", 30, mark)
        check(True, "status screen option 1 opens the on-screen editor")
        wait_log(r"button editor connected", 30, mark)
        check(True, "editor took over the Stream Deck (live preview mode)")
        time.sleep(2)
        mon.screenshot("e1-grid")

        # Press key 8 on the deck: selects it instead of running anything.
        deck("press 7")
        wait_log(r"EDITOR: deck-select 7", 30, mark)
        check(True, "pressing a key on the Stream Deck selects it in the editor")

        mon.key("ret")                      # edit key 8
        wait_log(r"EDITOR: screen key 7", 30, mark)
        mon.key("ret")                      # Label
        mon.type("Lamp On")
        wait_log(r'EDITOR: label "Lamp On"', 30, mark)
        check(True, "label typed on the Dvorak keyboard")
        mon.key("ret")                      # + Add an action (auto-selected)
        wait_log(r"EDITOR: screen add-action", 30, mark)
        mon.key("ret")                      # Set a scene
        wait_log(r"EDITOR: screen pick-light", 60, mark)
        time.sleep(1)
        mon.screenshot("e2-pick-light")
        mon.key("down", "ret")              # Floor Lamp
        wait_log(r"EDITOR: screen pick-scene", 30, mark)
        mon.type("rain", enter=False)       # type-to-filter
        time.sleep(1)
        mon.screenshot("e3-pick-scene-filtered")
        mon.key("ret")                      # Rainbow
        wait_log(r"EDITOR: action govee_scene", 30, mark)
        check(True, "Govee scene picked from the account's lights and scenes")

        mon.key("ret")                      # + Add an action
        wait_log(r"EDITOR: screen add-action", 30, len(vmlib.log_text()) - 2000)
        mon.key("down", "down", "down", "down", "ret")  # Play a playlist
        m = wait_log(r"EDITOR: screen pick-playlist (\d+)", 60, mark)
        check(m.group(1) == "2", f"Spotify playlists listed from the account ({m.group(1)})")
        time.sleep(1)
        mon.screenshot("e4-pick-playlist")
        mon.key("ret")                      # Dinner Jazz
        wait_log(r"EDITOR: screen pick-speaker", 30, mark)
        mon.key("down", "down", "ret")      # Kitchen Echo
        mon.key("ret")                      # play in order
        mon.key("ret")                      # leave volume
        wait_log(r"EDITOR: action spotify_playlist", 30, mark)
        time.sleep(2)
        mon.screenshot("e5-key-menu")
        save_deck("e6-deck-live-preview")
        mon.key("esc")                      # done -> save
        wait_log(r"EDITOR: saved", 30, mark)
        check(True, "leaving the key saves the layout")
        time.sleep(1)
        mon.screenshot("e7-grid-after")

        # Try it from the grid.
        m2 = len(vmlib.log_text())
        mon.type("t", enter=False)
        wait_log(r"EDITOR: try-(ok|error)", 60, m2)
        calls = vmlib.log_text()[m2:]
        check("EDITOR: try-ok" in calls and '"value":{"paramId":104,"id":4}' in calls
              and 'PUT /spotify/api/me/player/play?device_id=dev-kitchen {"context_uri":"spotify:playlist:PLDINNER"}' in calls,
              "T (try it) runs the key: Floor Lamp Rainbow + Dinner Jazz on Kitchen Echo")
        mon.key("ret")                      # dismiss "Done."

        out = sh.run("cat /etc/streamdeck/config.json")
        check('"label": "Lamp On"' in out and '"scene": "Rainbow"' in out and "PLDINNER" in out
              and "api_key" not in out and "refresh_token" not in out,
              "layout file has the new key and no credentials")

        mon.key("esc")                      # leave editor
        wait_log(r"EDITOR: closed", 30, mark)
        wait_log(r"button editor closed", 30, mark)
        time.sleep(3)
        mon.screenshot("e8-status")
        save_deck("e9-deck-after")

        # Now the key works from the deck.
        m3 = len(vmlib.log_text())
        deck("press 7")
        wait_log(r'key 7 "Lamp On" done', 60, m3)
        calls = vmlib.log_text()[m3:]
        check('"value":{"paramId":104,"id":4}' in calls and "AA:AA:AA:AA:AA:AA:AA:02" in calls
              and "play?device_id=dev-kitchen" in calls,
              "after closing the editor, pressing key 8 on the deck runs it")
    finally:
        vm.kill()
        vm.wait()
    finish()


if __name__ == "__main__":
    main()
