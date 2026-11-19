#!/usr/bin/env python3
"""Setup-file test: install without typing any keys.

A second USB stick holds the setup file, saved the way Windows often does it
(CRLF line endings, "streamdeck-setup.txt.txt" because extensions are hidden).
The PC has no Ethernet; the Wi-Fi "dongle" and router are virtual radios.

Checks: the installer finds the file, asks only "Use it?", connects to Wi-Fi,
uses the Govee key and the Spotify login from the file, and never asks for a
key, a Wi-Fi password or the phone login. Then Settings > "Save these
settings to a USB stick" rewrites the file from the running system.

Usage: test/seed-test.py <workdir containing streamdeck-os-test.iso>
"""
import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(__file__))
import vmlib
from vmlib import CLIENT_ID, CLIENT_SECRET, GOVEE_KEY, Monitor, Shell, check, finish, setup, wait_log

SEED = "\r\n".join([
    "wifi_name = StreamDeckTest",
    "wifi_password = correct horse",
    f"govee_api_key = {GOVEE_KEY}",
    f"spotify_client_id = {CLIENT_ID}",
    f"spotify_client_secret = {CLIENT_SECRET}",
    "spotify_refresh_token = MOCKREFRESH",
    "spotify_speaker = Kitchen Echo",
]) + "\r\n"


def main():
    setup(sys.argv[1], results="results-seed")
    stick = os.path.join(vmlib.WORK, "stick-seed.img")
    subprocess.run(["truncate", "-s", "0", stick], check=True)
    subprocess.run(["truncate", "-s", "64M", stick], check=True)
    subprocess.run(["sfdisk", "-q", stick], input="label: dos\nstart=2048, type=c\n", text=True, check=True)
    img = f"{stick}@@1M"
    subprocess.run(["mformat", "-i", img, "-F", "-v", "KEYS", "::"], check=True)
    seedfile = os.path.join(vmlib.WORK, "streamdeck-setup.txt.txt")
    open(seedfile, "w", newline="").write(SEED)
    subprocess.run(["mcopy", "-i", img, seedfile, "::/"], check=True)

    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("seed", usb_stick=True, nic=False, shell=sock, extra=[
        "-drive", f"if=none,id=seedstick,format=raw,file={stick}",
        "-device", "usb-storage,bus=xhci.0,drive=seedstick,removable=on",
    ])
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        wait_log(r"TEST: boot complete", 300)
        wait_log(r"SETUP: prompt use-seed", 90)
        time.sleep(1)
        mon.screenshot("s1-found-setup-file")
        check(True, "installer found streamdeck-setup.txt(.txt) on the second USB stick")
        mon.type("")  # Use it? -> yes
        # Not online yet: then "plug in" the dongle (and the virtual router;
        # in this VM the router lives on the same machine, so it must appear
        # after the installer's "already online?" check).
        m = wait_log(r"SETUP: network-ip (\S*)", 60)
        check(m.group(1) == "", "no network before Wi-Fi")
        sh.run("/usr/sbin/test-wifi-ap")
        m = wait_log(r"SETUP: wifi-result (\S+)", 120)
        check(m.group(1) == "connected", f"Wi-Fi joined with the file's name and password ({m.group(1)})")
        wait_log(r"SETUP: govee-ok", 60)
        check(True, "Govee key from the file accepted")
        wait_log(r"SETUP: spotify-ok seeded", 60)
        check(True, "Spotify login from the file accepted (no phone step)")
        m = wait_log(r'SETUP: spotify-device "([^"]+)"', 30)
        check(m.group(1) == "Kitchen Echo", "default speaker from the file")
        wait_log(r"SETUP: prompt install-disk", 60)
        log = vmlib.log_text()
        asked = [p for p in ("prompt govee-key", "prompt spotify-id", "prompt spotify-secret", "spotify-wait",
                             "prompt wifi-ssid", "prompt wifi-pass", "prompt spotify-device") if p in log]
        check(not asked, f"nothing had to be typed except 'Use it?' (asked: {asked or 'nothing'})")
        mon.screenshot("s2-install-step")

        mon.type("")       # don't install; keep running from USB
        time.sleep(3)
        mon.type("2")      # Settings
        wait_log(r"SETUP: prompt settings", 30)
        mon.type("6")      # Save these settings to a USB stick
        wait_log(r"SETUP: prompt export-stick", 30)
        mon.screenshot("s3-export")
        mon.type("1")
        m = wait_log(r"SETUP: (exported \S+|export-error .*)", 30)
        check(m.group(1).startswith("exported /dev/sd"), f"settings saved to the USB stick ({m.group(1)})")
        sh.run("sync")
    finally:
        vm.kill()
        vm.wait()

    out = subprocess.run(["mtype", "-i", img, "::/streamdeck-setup.txt.txt"], capture_output=True, text=True).stdout
    check("# StreamDeck OS setup file" in out and "spotify_refresh_token = MOCKREFRESH" in out
          and "wifi_password = correct horse" in out and f"govee_api_key = {GOVEE_KEY}" in out
          and "spotify_speaker = Kitchen Echo" in out,
          "exported file has Wi-Fi, Govee, Spotify app keys, login and speaker")
    finish()


if __name__ == "__main__":
    main()
