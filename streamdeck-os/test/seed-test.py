#!/usr/bin/env python3
"""Setup-file test: set up the installed system without typing any keys.

Case 1 (one USB port): a Ventoy stick holds both the ISO and the setup file.
Install from it; after the restart, with the same stick still plugged in,
the home screen offers the setup file. Using it sets up Govee and Spotify.

Case 2 (no Ethernet): the setup file is on a stick saved the way Windows
often does it (CRLF line endings, "streamdeck-setup.txt.txt" because
extensions are hidden). The home screen offers it; clicking "Use it" with
the trackpad joins Wi-Fi with the file's password and sets up Govee and
Spotify; nothing is typed. Then Setup file > Save rewrites the file.

Usage: test/seed-test.py <workdir containing streamdeck-os-test.iso>
"""
import os
import re
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(__file__))
import vmlib
from vmlib import (CLIENT_ID, CLIENT_SECRET, GOVEE_KEY, Monitor, Shell, check, choose, click, dismiss, finish,
                   home_open, mark, setup, wait_log)

SEED = "\r\n".join([
    "wifi_name = StreamDeckTest",
    "wifi_password = correct horse",
    f"govee_api_key = {GOVEE_KEY}",
    f"spotify_client_id = {CLIENT_ID}",
    f"spotify_client_secret = {CLIENT_SECRET}",
    "spotify_refresh_token = MOCKREFRESH",
    "spotify_speaker = Kitchen Echo",
]) + "\r\n"


def fat_stick(path, label, files, size="512M"):
    subprocess.run(["truncate", "-s", "0", path], check=True)
    subprocess.run(["truncate", "-s", size, path], check=True)
    subprocess.run(["sfdisk", "-q", path], input="label: dos\nstart=2048, type=c, bootable\n", text=True, check=True)
    img = f"{path}@@1M"
    subprocess.run(["mformat", "-i", img, "-F", "-v", label, "::"], check=True)
    for f in files:
        subprocess.run(["mcopy", "-s", "-i", img, f, "::/"], check=True)


def blank_disk():
    subprocess.run(["truncate", "-s", "0", vmlib.DISK], check=True)
    subprocess.run(["truncate", "-s", "16G", vmlib.DISK], check=True)


def apply_seed(mon, name, after, use_click):
    m = wait_log(r"APP: seed-found (.*)", 60, after)
    check(True, f"{name}: home screen found the setup file ({m.group(1).strip()})")
    vmlib.targets("banner", 30)
    time.sleep(1)
    mon.screenshot(f"{name}-1-banner")
    if use_click:
        hit = click(mon, "banner")
        a = vmlib.LAST_CLICK
        check(hit == "banner", f"{name}: trackpad click on Use it ({hit})")
    else:
        a = mark()
        mon.type("u", enter=False)
    a = choose(mon, "seed-import", "Set up", a)
    wait_log(r"APP: seed-applied", 180, a)
    time.sleep(1)
    mon.screenshot(f"{name}-2-done")
    a = dismiss(mon, "seed-done", a)
    return a


def one_port_case():
    """Install from a Ventoy stick that also holds the setup file, then use it."""
    W = vmlib.WORK
    iso_dir = os.path.join(W, "seed-iso-contents")
    subprocess.run(["rm", "-rf", iso_dir], check=True)
    os.makedirs(iso_dir)
    subprocess.run(["xorriso", "-osirrox", "on", "-indev", vmlib.ISO, "-extract", "/", iso_dir], check=True, capture_output=True)
    subprocess.run(["chmod", "-R", "u+w", iso_dir], check=True)
    boot = os.path.join(iso_dir, "streamdeck", "boot")
    initrd = os.path.join(W, "seed-initrd.img")
    with open(initrd, "wb") as out:
        for part in ("initrd.img", "test.img"):
            out.write(open(os.path.join(boot, part), "rb").read())
    seedfile = os.path.join(W, "streamdeck-setup.txt")
    no_wifi = "\r\n".join(l for l in SEED.split("\r\n") if not l.startswith("wifi_")) + "\r\n"
    open(seedfile, "w", newline="").write(no_wifi)
    ventoy = os.path.join(W, "stick-ventoy-seed.img")
    fat_stick(ventoy, "VENTOY", [vmlib.ISO, seedfile])
    stick = ["-drive", f"if=none,id=onestick,format=raw,file={ventoy}",
             "-device", "usb-storage,bus=xhci.0,drive=onestick,removable=on"]

    blank_disk()
    vm = vmlib.qemu("seed-ventoy-install", usb_stick=False, extra=[
        "-kernel", os.path.join(boot, "vmlinuz"), "-initrd", initrd, "-append", "streamdeck.live quiet loglevel=3"] + stick)
    try:
        mon = Monitor(vmlib.MON)
        wait_log(r"TEST: boot complete", 300)
        r = vmlib.install(mon)
        check(r == "installed nvme0n1", f"one port: installed from the Ventoy stick ({r})")
        check("APP: seed" not in vmlib.log_text(), "one port: the installer doesn't ask about the setup file")
    finally:
        vm.kill()
        vm.wait()

    vm = vmlib.qemu("seed-ventoy-installed", usb_stick=False, extra=stick)
    try:
        mon = Monitor(vmlib.MON)
        wait_log(r"TEST: boot complete mode=installed", 300)
        a = apply_seed(mon, "one-port", 0, use_click=False)
        log = vmlib.log_text()
        check("APP: govee-ok 2" in log and "APP: spotify-ok seeded" in log, "one port: Govee key and Spotify login from the file work")
        asked = re.findall(r"UI: input (\S+)", log)
        check(not asked, f"one port: nothing typed ({asked or 'no text fields'})")
        wait_log(r"config (loaded|reloaded)", 60, 0)
        a = mark()
        vmlib.deck("press 1")
        wait_log(r'key 1 "Party" done', 60, a)
        check("play?device_id=dev-kitchen" in vmlib.log_text()[a:], "one port: deck works, music on the file's speaker")
    finally:
        vm.kill()
        vm.wait()


def wifi_case():
    stick = os.path.join(vmlib.WORK, "stick-seed.img")
    seedfile = os.path.join(vmlib.WORK, "streamdeck-setup.txt.txt")
    open(seedfile, "w", newline="").write(SEED)
    fat_stick(stick, "KEYS", [seedfile], size="64M")
    img = f"{stick}@@1M"

    blank_disk()
    vm = vmlib.qemu("seed-install", usb_stick=True, nic=False)
    try:
        mon = Monitor(vmlib.MON)
        wait_log(r"TEST: boot complete", 300)
        check(vmlib.install(mon) == "installed nvme0n1", "Wi-Fi case: installed")
        vm.wait(timeout=120)
    finally:
        if vm.poll() is None:
            vm.kill()
            vm.wait()

    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("seed-wifi", usb_stick=False, nic=False, shell=sock, extra=[
        "-drive", f"if=none,id=seedstick,format=raw,file={stick}",
        "-device", "usb-storage,bus=xhci.0,drive=seedstick,removable=on",
    ])
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        wait_log(r"TEST: boot complete mode=installed", 300)
        sh.run("/usr/sbin/test-wifi-ap")  # the dongle (and the virtual router)
        wait_log(r"TEST: wifi dongle plugged in", 30)
        apply_seed(mon, "wifi", 0, use_click=True)
        log = vmlib.log_text()
        m = re.search(r"APP: wifi-result (\S+)", log)
        check(m and m.group(1) == "connected", f"Wi-Fi joined with the file's name and password ({m.group(1) if m else '?'})")
        check("APP: govee-ok" in log, "Govee key from the file accepted")
        check("APP: spotify-ok seeded" in log, "Spotify login from the file accepted (no phone step)")
        asked = re.findall(r"UI: input (\S+)", log)
        check(not asked, f"nothing had to be typed ({asked or 'no text fields'})")

        a = home_open(mon, "backup", mark())
        a = choose(mon, "backup", "Save settings", a)
        a = choose(mon, "export-stick", "/dev/sd", a)
        m = wait_log(r"APP: (exported \S+|export-error .*)", 30, a)
        check(m.group(1).startswith("exported /dev/sd"), f"Setup file > Save wrote the file to the USB stick ({m.group(1)})")
        dismiss(mon, "exported", a)
        sh.run("sync")
    finally:
        vm.kill()
        vm.wait()

    out = subprocess.run(["mtype", "-i", img, "::/streamdeck-setup.txt.txt"], capture_output=True, text=True).stdout
    check("# StreamDeck OS setup file" in out and "spotify_refresh_token = MOCKREFRESH" in out
          and "wifi_password = correct horse" in out and f"govee_api_key = {GOVEE_KEY}" in out
          and "spotify_speaker = Kitchen Echo" in out,
          "saved file has Wi-Fi, Govee, Spotify app keys, login and speaker")


def main():
    setup(sys.argv[1], results="results-seed")
    one_port_case()
    wifi_case()
    finish()


if __name__ == "__main__":
    main()
