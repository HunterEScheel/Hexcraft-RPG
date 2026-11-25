#!/usr/bin/env python3
"""End-to-end test of the StreamDeck OS installer ISO on a virtual mini PC.

QEMU with UEFI firmware (OVMF), a USB stick holding the test ISO, a USB
keyboard with a "trackpad" (usb-mouse), an NVMe disk and a virtio NIC.

Phase 1 (USB stick): only the installer runs. Nothing is typed but menu
choices: install to the NVMe disk, restart.
Phase 2 (installed, stick removed): everything is set up on the installed
system's graphical home screen, with the keyboard (QEMU `sendkey` sends key
positions, so Dvorak must be active) and the trackpad:
  - Govee key (a QWERTY-typed key is rejected, the Dvorak-typed one works)
  - Spotify app keys + phone login + speaker (opened with a trackpad click)
  - the button editor: new folder on key 8, a key inside it built from the
    account's lights and scenes, Try it, folder navigation, live deck preview
  - the deck: Party key, folder key, the new key inside the folder
  - changing the speaker afterwards
Then: power cut check, update from the USB stick, settings survive.

Usage: test/iso-test.py <workdir containing streamdeck-os-test.iso>
"""
import html
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

sys.path.insert(0, os.path.dirname(__file__))
import vmlib
from vmlib import (CLIENT_ID, CLIENT_SECRET, GOVEE_KEY, PORT_API, PORT_WEB, QWERTY, Monitor, NoRedirect, Shell,
                   check, choose, click, deck, dismiss, enter_text, finish, home_open, mark, save_deck, setup,
                   wait_images, wait_log)


def log_text():
    return vmlib.log_text()


def phase1():
    vm = vmlib.qemu("1-live-usb", usb_stick=True)
    try:
        mon = Monitor(vmlib.MON)
        t0 = time.time()
        m = wait_log(r"TEST: boot complete mode=(\w+)", 300)
        print(f"     booted from USB in {time.time() - t0:.0f}s")
        check(m.group(1) == "live", "UEFI boots the ISO from a USB stick into live mode")
        wait_log(r"TEST: storage=/dev/sd")
        check(True, "live system found and mounted its own USB stick")
        wait_log(r"TEST: modules=\d+ nvme=1 usbhid=1", 30)
        check(True, "NVMe disk and USB keyboard drivers loaded")
        m = wait_log(r"TEST: netdrivers=(.*)", 60)
        check("ethernet" not in m.group(1).split() and "usb" not in m.group(1).split(),
              f"image has no Ethernet drivers (net drivers: {m.group(1).strip()})")
        m = wait_log(r"UI: renderer (\w+)", 120)
        check(m.group(1) == "gfx", "graphical screen on the framebuffer (not the text console)")
        wait_log(r"UI: menu installer ", 120)
        time.sleep(1)
        mon.screenshot("p1-01-installer")
        r = vmlib.install(mon)
        check(r == "installed nvme0n1", f"installed to the NVMe disk with menu choices only ({r})")
        asked = re.findall(r"UI: input (\S+)", log_text())
        check(not asked, f"the USB stick asks for no keys, passwords or Wi-Fi ({asked or 'nothing typed'})")
        vm.wait(timeout=120)  # restart -> QEMU exits because of -no-reboot
        check(True, "installer restarted the machine")
    finally:
        if vm.poll() is None:
            vm.kill()
            vm.wait()


def inspect_disk():
    out = subprocess.run(["sfdisk", "--dump", vmlib.DISK], capture_output=True, text=True).stdout
    check("label: gpt" in out and "C12A7328-F81F-11D2-BA4B-00A0C93EC93B" in out, "NVMe disk has a GPT with an EFI System Partition")
    off = int(re.search(r"start=\s*(\d+)", out).group(1)) * 512
    img = f"{vmlib.DISK}@@{off}"
    files = subprocess.run(["mdir", "-/", "-b", "-i", img, "::"], capture_output=True, text=True).stdout
    for f in ["EFI/BOOT/BOOTX64.EFI", "streamdeck/boot/vmlinuz", "streamdeck/boot/initrd.img",
              "streamdeck/boot/installed.cfg", "streamdeck/config.json"]:
        check("::/" + f in files, f"ESP contains {f}")
    check("secrets.json" not in files, "no keys on the disk yet (they're set up after installing)")
    label = subprocess.run(["mlabel", "-s", "-i", img, "::"], capture_output=True, text=True).stdout
    check("STREAMDECK" in label, "ESP is labelled STREAMDECK")


def spotify_phone_login(state_after):
    """What the user does on their phone: open the page, log in, paste the address."""
    state = wait_log(r"APP: spotify-wait state=(\w+)", 60, state_after).group(1)
    page = urllib.request.urlopen(f"http://127.0.0.1:{PORT_WEB}/", timeout=20).read().decode()
    href = html.unescape(re.search(r'href="([^"]+)"', page).group(1))
    q = urllib.parse.parse_qs(urllib.parse.urlparse(href).query)
    check(q.get("client_id") == [CLIENT_ID] and q.get("state") == [state], "phone page links to the Spotify login")
    href = href.replace("127.0.0.1:9999", f"127.0.0.1:{PORT_API}")
    try:
        urllib.request.build_opener(NoRedirect).open(href, timeout=20)
        raise SystemExit("expected redirect")
    except urllib.error.HTTPError as e:
        landed = e.headers["Location"]
    body = urllib.parse.urlencode({"url": landed}).encode()
    resp = urllib.request.urlopen(f"http://127.0.0.1:{PORT_WEB}/", data=body, timeout=20).read().decode()
    check("Got it" in resp, "pasted Spotify redirect address accepted")


def phase2():
    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("2-installed", usb_stick=False, shell=sock)
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        t0 = time.time()
        m = wait_log(r"TEST: boot complete mode=(\w+)", 300)
        print(f"     booted from NVMe in {time.time() - t0:.0f}s")
        check(m.group(1) == "installed", "boots from the NVMe disk with the USB stick removed")
        m = wait_log(r"TEST: storage=\S* boot=(\S*)")
        check(m.group(1).startswith("/dev/nvme0n1p1"), "STREAMDECK partition mounted at /boot")
        m = wait_log(r"UI: renderer (\w+) (\S+)", 120)
        check(m.group(1) == "gfx", f"graphical home screen ({m.group(2)})")
        wait_log(r"UI: home sel=", 60)
        time.sleep(2)
        mon.screenshot("p2-01-home")

        # Govee key, with the keyboard.
        a = home_open(mon, "govee", mark())
        a = choose(mon, "govee", "Govee API key", a)
        a = enter_text(mon, "govee-key", GOVEE_KEY, a, layout=QWERTY)
        a = dismiss(mon, "govee-error", a)
        check(True, "a key typed at QWERTY positions is rejected (the layout is Dvorak)")
        a = enter_text(mon, "govee-key", GOVEE_KEY, a)
        wait_log(r"APP: govee-ok 2", 60, a)
        time.sleep(1)
        mon.screenshot("p2-02-govee-ok")
        a = dismiss(mon, "govee-ok", a)
        wait_log(r"APP: settings-saved", 30, a)
        check(True, "Govee key typed on the Dvorak keyboard is checked and saved")

        # Lights on the network (Govee LAN API): find, blink, name.
        a = choose(mon, "govee", "Find lights on my network", a)
        m = wait_log(r"APP: lan-found (\d+)", 30, a)
        check(m.group(1) == "2", f"LAN scan finds the H7065 spotlights and H7061 flood lights ({m.group(1)})")
        time.sleep(1)
        mon.screenshot("p2-02b-lan-found")
        a = choose(mon, "lan-found", "H7065", a)
        wait_log(r'LAN D7:65:AA:BB:CC:DD:00:01 \{"msg":\{"cmd":"turn"', 30, a)
        a = enter_text(mon, "lan-name", "Spotlights", a)
        wait_log(r"APP: lan-added Spotlights", 30, a)
        check(True, "the chosen light blinks (LAN turn commands) and is named")
        a = choose(mon, "lan-found", "H7061", a)
        a = enter_text(mon, "lan-name", "Flood Lights", a)
        wait_log(r"APP: lan-added Flood Lights", 30, a)
        sel, items = vmlib.menu("lan-found", a)
        check(not any(i.startswith("H70") for i in items), f"named lights leave the list ({items})")
        a = mark()
        mon.key("esc")
        sel, items = vmlib.menu("govee", a)
        check("Spotlights" in items and "Flood Lights" in items, "Lights screen lists both LAN lights")
        time.sleep(1)
        mon.screenshot("p2-02c-lights")
        a = mark()
        mon.key("esc")
        wait_log(r"UI: home sel=", 30, a)

        # Spotify, opened with the trackpad.
        hit = click(mon, "tile:spotify")
        a = vmlib.LAST_CLICK
        check(hit == "tile:spotify", f"trackpad click opens the Music tile ({hit or 'nothing'})")
        a = enter_text(mon, "spotify-id", CLIENT_ID, a)
        a = enter_text(mon, "spotify-secret", CLIENT_SECRET, a)
        wait_log(r"UI: info spotify-phone", 30, a)
        time.sleep(1)
        mon.screenshot("p2-03-spotify-phone")
        spotify_phone_login(a)
        wait_log(r"APP: spotify-ok", 30, a)
        a = choose(mon, "spotify-speaker", "Living Room", a)
        wait_log(r'APP: spotify-device "Living Room"', 30, a)
        vmlib.menu("spotify", a)
        check(True, "Spotify app keys, phone login and default speaker set up after installing")
        a = mark()
        mon.key("esc")
        wait_log(r"UI: home sel=", 30, a)
        wait_log(r"config (loaded|reloaded)", 60, 0)
        check(wait_images(15), "with the keys set up, the layout loads and 15 key images reach the MK.2")

        a = mark()
        deck("press 1")
        wait_log(r'key 1 "Party" done', 60, a)
        calls = log_text()[a:]
        check('"value":{"paramId":102,"id":2}' in calls, "Party: Govee scene sent")
        check('PUT /spotify/api/me/player/play?device_id=dev-living {"context_uri":"spotify:playlist:37i9dQZF1DXaXB8fQg7xif"}' in calls,
              "Party: playlist started on Living Room")

        editor(mon, sh)
        speaker(mon)
    finally:
        vm.kill()
        vm.wait()


def editor(mon, sh):
    """The button editor on the installed system, including folders."""
    hit = click(mon, "tile:buttons")
    a = vmlib.LAST_CLICK
    wait_log(r"EDITOR: open", 30, a)
    check(hit == "tile:buttons", "trackpad click on Buttons opens the editor on the installed system")
    wait_log(r"button editor connected", 30, a)
    check(True, "editor took over the Stream Deck (live preview)")
    time.sleep(2)
    mon.screenshot("p2-04-editor")

    a = mark()
    deck("press 7")
    wait_log(r"EDITOR: deck-select 7", 30, a)
    check(True, "pressing a deck key selects it in the editor")
    a = mark()
    mon.type("n", enter=False)  # New folder here
    a = enter_text(mon, "folder-name", "Lights", a)
    wait_log(r"EDITOR: folder-open Lights", 30, a)
    wait_log(r"UI: grid page=Lights", 30, a)
    check(True, "new folder created on key 8 and opened in edit mode")
    time.sleep(2)
    save_deck("p2-05-deck-in-folder")
    mon.screenshot("p2-06-folder")

    hit = click(mon, "cell:1")
    b = vmlib.LAST_CLICK
    wait_log(r"UI: grid page=Lights sel=1", 30, b)
    check(hit == "cell:1", "trackpad click selects key 2 inside the folder")
    b = mark()
    mon.key("ret")
    b = choose(mon, "key", "Label", b)
    b = enter_text(mon, "label", "Lamp On", b)
    wait_log(r'EDITOR: label "Lamp On"', 30, b)
    c = choose(mon, "key", "+ Add an action", b)
    c = choose(mon, "add-action", "Set a scene", c)
    c = choose(mon, "pick-light", "Floor Lamp", c)
    c = choose(mon, "pick-light", "Done", c)
    vmlib.menu("pick-scene", c)
    mon.type("rain", enter=False)  # type to search
    time.sleep(1)
    mon.screenshot("p2-07-pick-scene")
    c = mark()
    mon.key("ret")
    wait_log(r"EDITOR: action govee_scene", 30, c)
    vmlib.menu("key", c)
    time.sleep(1)
    mon.screenshot("p2-08-key")
    c = mark()
    mon.key("esc")  # done -> saves
    wait_log(r"EDITOR: saved", 30, c)
    check(True, "key inside the folder built from the account's lights and scenes and saved")

    # Key 3 in the folder: Flood Lights (LAN) to orange, picked on the colour wheel.
    c = mark()
    mon.key("right", "ret")
    c = choose(mon, "key", "Label", c)
    c = enter_text(mon, "label", "Flood", c)
    c = choose(mon, "key", "+ Add an action", c)
    c = choose(mon, "add-action", "Set a colour", c)
    sel, items = vmlib.menu("pick-light", c)
    check("Flood Lights" in items and "Spotlights" in items, f"light picker offers the LAN lights ({items})")
    c = choose(mon, "pick-light", "Flood Lights", c)
    c = choose(mon, "pick-light", "Spotlights", c)
    time.sleep(1)
    mon.screenshot("p2-08a-pick-lights")
    c = choose(mon, "pick-light", "Done", c)
    wait_log(r"EDITOR: lights Spotlights\|Flood Lights", 30, c)
    wait_log(r"UI: color color value=", 30, c)
    mon.key(*(["up"] * 20))
    mon.key(*(["right"] * 6))
    wait_log(r"UI: color color value=#ff8000", 30, c)
    time.sleep(1)
    mon.screenshot("p2-08b-color-wheel")
    c = mark()
    mon.key("ret")
    wait_log(r"EDITOR: action govee_color", 30, c)
    c = mark()
    mon.key("esc")
    wait_log(r"EDITOR: saved", 30, c)
    check(True, "one colour action for two lights (Spotlights and Flood Lights), #ff8000 from the colour wheel; key saved")
    # Key 4: white light (brightest), cloud and network lights together.
    c = mark()
    mon.key("right", "ret")
    c = choose(mon, "key", "Label", c)
    c = enter_text(mon, "label", "Bright", c)
    c = choose(mon, "key", "+ Add an action", c)
    c = choose(mon, "add-action", "Set white temperature", c)
    c = choose(mon, "pick-light", "Floor Lamp", c)
    c = choose(mon, "pick-light", "Flood Lights", c)
    c = choose(mon, "pick-light", "Done", c)
    c = choose(mon, "kelvin", "Daylight", c)
    wait_log(r"EDITOR: action govee_color_temp", 30, c)
    time.sleep(1)
    mon.screenshot("p2-08c-white")
    c = mark()
    mon.key("esc")
    wait_log(r"EDITOR: saved", 30, c)
    check(True, "white temperature action (Daylight) for Floor Lamp and Flood Lights saved")
    c = mark()
    mon.key("left", "left")
    wait_log(r"UI: grid page=Lights sel=1", 30, c)

    c = mark()
    mon.type("t", enter=False)  # Try it
    wait_log(r"EDITOR: try-(ok|error)", 60, c)
    calls = log_text()[c:]
    check("EDITOR: try-ok" in calls and '"value":{"paramId":104,"id":4}' in calls, "Try it runs Floor Lamp → Rainbow")
    dismiss(mon, "try-ok", c)

    c = mark()
    mon.key("esc")
    wait_log(r"EDITOR: folder-up home", 30, c)
    check(True, "Esc goes back up from the folder")
    time.sleep(2.5)  # so the next two presses count as a fresh double press
    c = mark()
    deck("press 7")
    deck("press 7")
    wait_log(r"EDITOR: folder-open Lights", 30, c)
    check(True, "pressing the folder key twice on the deck opens the folder in the editor")
    c = mark()
    mon.key("esc")
    wait_log(r"EDITOR: folder-up home", 30, c)
    c = mark()
    mon.key("esc")
    wait_log(r"EDITOR: closed", 30, c)
    wait_log(r"button editor closed", 30, c)
    wait_log(r"UI: home sel=", 30, c)

    out = sh.run("cat /boot/streamdeck/config.json")
    check('"Lights"' in out and '"label": "Lamp On"' in out and '"scene": "Rainbow"' in out and "‹ Back" in out
          and "api_key" not in out and "refresh_token" not in out,
          "layout on the disk has the folder, its Back key and the new key, and no credentials")

    time.sleep(2)
    c = mark()
    deck("press 7")
    wait_log(r'key 7 on "home": "Lights"', 60, c)
    time.sleep(1)
    deck("press 1")
    wait_log(r'key 1 "Lamp On" done', 60, c)
    calls = log_text()[c:]
    check('"value":{"paramId":104,"id":4}' in calls and "AA:AA:AA:AA:AA:AA:AA:02" in calls,
          "on the deck: folder key opens the folder, Lamp On sets Floor Lamp → Rainbow")
    c2 = mark()
    deck("press 2")
    wait_log(r'key 2 "Flood" done', 60, c2)
    orange = r' \{"msg":\{"cmd":"colorwc","data":\{"color":\{"b":0,"g":128,"r":255\}'
    check(re.search(r'LAN D7:61:AA:BB:CC:DD:00:02' + orange, log_text()[c2:]) is not None
          and re.search(r'LAN D7:65:AA:BB:CC:DD:00:01' + orange, log_text()[c2:]) is not None
          and "/govee/device/control" not in log_text()[c2:],
          "Flood key sends the colour to both the H7061 and the H7065 over the LAN (no cloud call)")
    c3 = mark()
    deck("press 3")
    wait_log(r'key 3 "Bright" done', 60, c3)
    out = log_text()[c3:]
    check(re.search(r'LAN D7:61:AA:BB:CC:DD:00:02 \{"msg":\{"cmd":"colorwc","data":\{"color":\{"b":0,"g":0,"r":0\},"colorTemInKelvin":6500\}', out) is not None
          and re.search(r'/govee/device/control.*"instance":"colorTemperatureK".*"value":6500', out) is not None,
          "Bright key: 6500 K white to the flood lights over the LAN and to Floor Lamp through the cloud")
    save_deck("p2-09-deck-folder")
    deck("press 0")
    wait_log(r'key 0 on "Lights": "‹ Back"', 60, c)
    time.sleep(1)
    check(True, "the folder's Back key returns to the main keys")


def speaker(mon):
    a = home_open(mon, "spotify", mark())
    a = choose(mon, "spotify", "Default speaker", a)
    a = choose(mon, "spotify-speaker", "Kitchen Echo", a)
    wait_log(r'APP: spotify-device "Kitchen Echo"', 30, a)
    wait_log(r"APP: settings-saved", 30, a)
    wait_log(r"config reloaded", 30, a)
    check(True, "Music > Default speaker changed to Kitchen Echo; daemon reloaded")
    mon.key("esc")
    a = mark()
    deck("press 1")
    wait_log(r'key 1 "Party" done', 60, a)
    check("play?device_id=dev-kitchen" in log_text()[a:], "Party now plays on Kitchen Echo")


def check_fat(when):
    """The settings partition must be consistent after the VM's power is cut."""
    out = subprocess.run(["sfdisk", "--dump", vmlib.DISK], capture_output=True, text=True).stdout
    start = int(re.search(r"start=\s*(\d+)", out).group(1))
    size = int(re.search(r"size=\s*(\d+)", out).group(1))
    part = os.path.join(vmlib.WORK, "esp-check.img")
    subprocess.run(["dd", f"if={vmlib.DISK}", f"of={part}", "bs=512", f"skip={start}", f"count={size}", "status=none"], check=True)
    # Same repair the system runs at boot (S05storage), then it must be clean.
    subprocess.run(["fsck.fat", "-a", "-w", part], capture_output=True, text=True)
    r = subprocess.run(["fsck.fat", "-n", part], capture_output=True, text=True)
    sec = subprocess.run(["mtype", "-i", part, "::/streamdeck/state/secrets.json"], capture_output=True, text=True).stdout
    check(r.returncode == 0 and "Kitchen Echo" in sec,
          f"{when}: after the boot-time repair the FAT is clean and the last saved setting is on disk (fsck rc={r.returncode})")
    os.remove(part)


def phase3_update():
    """Boot the USB stick again on the installed PC: update in place."""
    vm = vmlib.qemu("3-update", usb_stick=True)
    try:
        mon = Monitor(vmlib.MON)
        wait_log(r"TEST: boot complete mode=live", 300)
        sel, items = vmlib.menu("installer", 0, 120)
        check(items[0] == "Update StreamDeck OS", f"USB stick on an installed PC offers to update it ({items[0]})")
        mon.screenshot("p3-update-offer")
        a = choose(mon, "installer", "Update StreamDeck OS")
        m = wait_log(r"APP: (updated|update-error.*)", 120, a)
        check(m.group(1) == "updated", f"update in place ({m.group(1)})")
        dismiss(mon, "install-done", a)
        vm.wait(timeout=120)
    finally:
        if vm.poll() is None:
            vm.kill()
            vm.wait()
    out = subprocess.run(["sfdisk", "--dump", vmlib.DISK], capture_output=True, text=True).stdout
    off = int(re.search(r"start=\s*(\d+)", out).group(1)) * 512
    a, b = os.path.join(vmlib.WORK, "cmp-esp.img"), os.path.join(vmlib.WORK, "cmp-iso.img")
    for f in (a, b):
        if os.path.exists(f):
            os.remove(f)
    subprocess.run(["mcopy", "-n", "-i", f"{vmlib.DISK}@@{off}", "::/streamdeck/boot/initrd.img", a], check=True)
    subprocess.run(["xorriso", "-osirrox", "on", "-indev", vmlib.ISO, "-extract", "/streamdeck/boot/initrd.img", b],
                   check=True, capture_output=True)
    check(open(a, "rb").read() == open(b, "rb").read(), "installed initrd now matches the ISO")


def phase4_after_update():
    vm = vmlib.qemu("4-after-update", usb_stick=False)
    try:
        wait_log(r"TEST: boot complete mode=installed", 300)
        wait_log(r"config loaded: 3 page", 90)
        check(wait_images(15), "deck works after the update")
        a = mark()
        deck("press 1")
        wait_log(r'key 1 "Party" done', 60, a)
        check("play?device_id=dev-kitchen" in log_text()[a:], "settings survived the update (Party still plays on Kitchen Echo)")
        deck("press 7")
        wait_log(r'key 7 on "home": "Lights"', 60, a)
        time.sleep(1)
        deck("press 1")
        wait_log(r'key 1 "Lamp On" done', 60, a)
        deck("press 2")
        wait_log(r'key 2 "Flood" done', 60, a)
        check("LAN D7:61:AA:BB:CC:DD:00:02" in log_text()[a:], "buttons and LAN lights survived the update")
    finally:
        vm.kill()
        vm.wait()


def main():
    setup(sys.argv[1])
    phase1()
    inspect_disk()
    phase2()
    check_fat("power cut right after saving a setting")
    phase3_update()
    phase4_after_update()
    finish()


if __name__ == "__main__":
    main()
