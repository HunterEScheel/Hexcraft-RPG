#!/usr/bin/env python3
"""End-to-end test of the StreamDeck OS installer ISO on a virtual mini PC.

QEMU with UEFI firmware (OVMF), a USB stick holding the test ISO, a USB
keyboard, an NVMe disk and an Intel e1000e network card.

Phase 1 (live USB): the installer is typed on the USB keyboard with QEMU
`sendkey`, which sends physical key positions, so Dvorak has to be active
for the right characters to arrive. Then the Spotify login runs through the
phone page, and the system is installed to the NVMe disk.
Phase 2 (installed): the USB stick is removed and the machine boots from NVMe.
The test checks that the settings survived, presses keys on the virtual
Stream Deck MK.2 and checks the Govee/Spotify calls. Finally it changes the
Spotify speaker in the Settings menu.

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

sys.path.insert(0, __import__("os").path.dirname(__file__))
import vmlib
from vmlib import (CLIENT_ID, CLIENT_SECRET, DVORAK, GOVEE_KEY, PORT_API, PORT_WEB, QWERTY,
                   Monitor, NoRedirect, check, deck, finish, save_deck, setup, wait_images, wait_log)


def vm_start(*a, **k):
    return vmlib.qemu(*a, **k)


def log_text():
    return vmlib.log_text()


def phase1():
    vm = vm_start("1-live-usb", usb_stick=True)
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
        m = wait_log(r"SETUP: network-ip (\S*)", 120)
        check(m.group(1).startswith("10.0.2."), f"VM network (QEMU virtio, built into the kernel) got an address ({m.group(1)})")

        wait_log(r"SETUP: prompt wifi-optional", 120)
        mon.screenshot("p1-01-installer")
        mon.type("n")

        mark = len(log_text())
        wait_log(r"SETUP: prompt govee-key")
        mon.type(GOVEE_KEY, QWERTY)
        wait_log(r"SETUP: govee-error", 60, mark)
        check(True, "typing at QWERTY key positions is rejected (layout is not QWERTY)")
        wait_log(r"SETUP: prompt govee-key", 30, mark + 10)
        mon.type(GOVEE_KEY)
        wait_log(r"SETUP: govee-ok 2", 60, mark)
        check(True, "Govee API key typed on the Dvorak USB keyboard is accepted")

        wait_log(r"SETUP: prompt spotify-id")
        mon.type(CLIENT_ID)
        wait_log(r"SETUP: prompt spotify-secret")
        mon.type(CLIENT_SECRET)
        state = wait_log(r"SETUP: spotify-wait state=(\w+)").group(1)
        mon.screenshot("p1-02-spotify")

        page = urllib.request.urlopen(f"http://127.0.0.1:{PORT_WEB}/", timeout=20).read().decode()
        href = html.unescape(re.search(r'href="([^"]+)"', page).group(1))
        q = urllib.parse.parse_qs(urllib.parse.urlparse(href).query)
        check(q.get("client_id") == [CLIENT_ID] and q.get("state") == [state], "phone page links to Spotify login")
        href = href.replace("127.0.0.1:9999", f"127.0.0.1:{PORT_API}")

        try:
            urllib.request.build_opener(NoRedirect).open(href, timeout=20)
            raise SystemExit("expected redirect")
        except urllib.error.HTTPError as e:
            landed = e.headers["Location"]
        body = urllib.parse.urlencode({"url": landed}).encode()
        resp = urllib.request.urlopen(f"http://127.0.0.1:{PORT_WEB}/", data=body, timeout=20).read().decode()
        check("Got it" in resp, "pasted Spotify redirect address accepted")
        wait_log(r"SETUP: spotify-ok")

        wait_log(r"SETUP: prompt spotify-device")
        mon.type("1")
        wait_log(r"SETUP: saved")
        check(True, "installer saved settings")

        wait_log(r"SETUP: prompt install-disk")
        mon.screenshot("p1-03-install-disk")
        mon.type("1")
        wait_log(r"SETUP: prompt install-confirm")
        mon.type("ERASE")
        m = wait_log(r"SETUP: (installed \w+|install-error .*)", 180)
        check(m.group(1) == "installed nvme0n1", f"installed to the NVMe disk ({m.group(1)})")
        wait_log(r"SETUP: prompt reboot")
        mon.screenshot("p1-04-installed")
        mon.type("")  # Enter -> reboot (QEMU exits because of -no-reboot)
        vm.wait(timeout=120)
        check(True, "installer rebooted the machine")
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
              "streamdeck/boot/installed.cfg", "streamdeck/config.json", "streamdeck/state/secrets.json"]:
        check("::/" + f in files, f"ESP contains {f}")
    label = subprocess.run(["mlabel", "-s", "-i", img, "::"], capture_output=True, text=True).stdout
    check("STREAMDECK" in label, "ESP is labelled STREAMDECK")


def phase2():
    vm = vm_start("2-installed", usb_stick=False)
    try:
        mon = Monitor(vmlib.MON)
        t0 = time.time()
        m = wait_log(r"TEST: boot complete mode=(\w+)", 300)
        print(f"     booted from NVMe in {time.time() - t0:.0f}s")
        check(m.group(1) == "installed", "boots from the NVMe disk with the USB stick removed")
        m = wait_log(r"TEST: storage=\S* boot=(\S*)")
        check(m.group(1).startswith("/dev/nvme0n1p1"), "STREAMDECK partition mounted at /boot")
        wait_log(r"TEST: state .*secrets\.json", 30)
        check(True, "saved settings present on the disk after reboot")

        wait_log(r"config loaded: 2 page", 90)
        check("layout /boot/streamdeck/config.json" in log_text(), "daemon uses the layout on the disk")
        check(wait_images(15), "15 home-page key images uploaded to the MK.2")
        save_deck("p2-01-deck-home")
        mon.screenshot("p2-02-status")

        mark = len(log_text())
        deck("press 1")
        wait_log(r'key 1 "Party" done', 60, mark)
        calls = log_text()[mark:]
        check('"value":{"paramId":102,"id":2}' in calls, "Party: Govee scene sent")
        check('PUT /spotify/api/me/player/play?device_id=dev-living {"context_uri":"spotify:playlist:37i9dQZF1DXaXB8fQg7xif"}' in calls,
              "Party: playlist started on Living Room (settings survived reboot)")

        # Settings menu (status screen option 2): change the default speaker to Kitchen Echo.
        mon.type("2")
        wait_log(r"SETUP: prompt settings", 30, mark)
        mon.screenshot("p2-03-settings")
        mark = len(log_text())
        mon.type("4")
        wait_log(r"SETUP: prompt spotify-device", 30, mark)
        mon.type("2")
        wait_log(r'SETUP: spotify-device "Kitchen Echo"', 30, mark)
        wait_log(r"SETUP: settings-saved", 30, mark)
        mon.type("")              # Press Enter to continue
        wait_log(r"config reloaded", 30, mark)
        check(True, "Settings menu changed the Spotify speaker; daemon reloaded")
        mon.type("0")

        mark = len(log_text())
        deck("press 1")
        wait_log(r'key 1 "Party" done', 60, mark)
        check("play?device_id=dev-kitchen" in log_text()[mark:], "Party now plays on Kitchen Echo")
    finally:
        vm.kill()
        vm.wait()


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
    vm = vm_start("3-update", usb_stick=True)
    try:
        mon = Monitor(vmlib.MON)
        wait_log(r"TEST: boot complete mode=live", 300)
        wait_log(r"SETUP: prompt update", 60)
        check(True, "USB stick on an installed PC offers to update it")
        mon.screenshot("p3-update-offer")
        mon.type("1")
        m = wait_log(r"SETUP: (updated|update-error.*)", 120)
        check(m.group(1) == "updated", f"update in place ({m.group(1)})")
        mon.type("")
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
    vm = vm_start("4-after-update", usb_stick=False)
    try:
        wait_log(r"TEST: boot complete mode=installed", 300)
        wait_log(r"config loaded: 2 page", 90)
        check(wait_images(15), "deck works after the update")
        mark = len(log_text())
        deck("press 1")
        wait_log(r'key 1 "Party" done', 60, mark)
        check("play?device_id=dev-kitchen" in log_text()[mark:], "settings survived the update (Party still plays on Kitchen Echo)")
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
