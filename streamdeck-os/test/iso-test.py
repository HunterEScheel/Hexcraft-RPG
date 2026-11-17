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
import base64
import html
import os
import re
import shutil
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

WORK = os.path.abspath(sys.argv[1])
ISO = os.path.join(WORK, "streamdeck-os-test.iso")
OUT = os.path.join(WORK, "results")
os.makedirs(OUT, exist_ok=True)
DISK = os.path.join(WORK, "nvme.img")
VARS = os.path.join(WORK, "OVMF_VARS.fd")
MON = os.path.join(WORK, "mon.sock")

GOVEE_KEY = "tEsT-gOvEe-42xq"          # must match testtools/mockapi
CLIENT_ID, CLIENT_SECRET = "mockclientid", "mocksecret"
PORT_WEB, PORT_API, PORT_DECK = 18080, 19999, 17000

QWERTY_ROWS = [
    ("grave 1 2 3 4 5 6 7 8 9 0 minus equal", "`1234567890[]", "~!@#$%^&*(){}"),
    ("q w e r t y u i o p bracket_left bracket_right", "',.pyfgcrl/=", '"<>PYFGCRL?+'),
    ("a s d f g h j k l semicolon apostrophe", "aoeuidhtns-", "AOEUIDHTNS_"),
    ("z x c v b n m comma dot slash", ";qjkxbmwvz", ":QJKXBMWVZ"),
]
DVORAK = {" ": ("spc", False)}
for keys, plain, shifted in QWERTY_ROWS:
    for k, p, s in zip(keys.split(), plain, shifted):
        DVORAK[p] = (k, False)
        DVORAK[s] = (k, True)
QWERTY = {" ": ("spc", False), "-": ("minus", False)}
for c in "abcdefghijklmnopqrstuvwxyz0123456789":
    QWERTY[c] = (c, False)
    if c.isalpha():
        QWERTY[c.upper()] = (c, True)

failures = []
LOG = None


def check(cond, what):
    print(("PASS " if cond else "FAIL ") + what, flush=True)
    if not cond:
        failures.append(what)


def log_text():
    try:
        with open(LOG, errors="replace") as f:
            return f.read()
    except FileNotFoundError:
        return ""


def wait_log(pattern, timeout=120, after=0):
    rx = re.compile(pattern)
    end = time.time() + timeout
    while time.time() < end:
        m = rx.search(log_text(), after)
        if m:
            return m
        time.sleep(0.5)
    print(log_text()[-4000:])
    raise SystemExit(f"timeout waiting for /{pattern}/")


class Monitor:
    def __init__(self, path):
        for _ in range(200):
            try:
                self.s = socket.socket(socket.AF_UNIX)
                self.s.connect(path)
                break
            except OSError:
                time.sleep(0.1)
        self.s.settimeout(0.3)
        self.drain()

    def drain(self):
        try:
            while self.s.recv(65536):
                pass
        except socket.timeout:
            pass

    def cmd(self, c):
        self.s.sendall((c + "\n").encode())
        time.sleep(0.04)
        self.drain()

    def type(self, text, layout=DVORAK, enter=True):
        for ch in text:
            key, shift = layout[ch]
            self.cmd(f"sendkey {'shift-' if shift else ''}{key}")
        if enter:
            self.cmd("sendkey ret")

    def screenshot(self, name):
        time.sleep(1)
        ppm = os.path.join(OUT, name + ".ppm")
        self.cmd(f"screendump {ppm}")
        time.sleep(1.5)
        png = os.path.join(OUT, name + ".png")
        with open(png, "wb") as f:
            subprocess.run(["pnmtopng", ppm], stdout=f, check=True)
        os.remove(ppm)
        print(f"     screenshot {png}")


def deck(cmd):
    with socket.create_connection(("127.0.0.1", PORT_DECK), timeout=30) as s:
        s.sendall((cmd + "\n").encode())
        buf = b""
        while not buf.endswith(b"\n"):
            chunk = s.recv(1 << 20)
            if not chunk:
                break
            buf += chunk
        return buf.decode().strip()


def deck_images():
    return int(re.search(r"images=(\d+)", deck("stats")).group(1))


def wait_images(n, timeout=60):
    end = time.time() + timeout
    while time.time() < end:
        try:
            if deck_images() >= n:
                return True
        except OSError:
            pass
        time.sleep(0.5)
    return False


def save_deck(name):
    path = os.path.join(OUT, name + ".png")
    with open(path, "wb") as f:
        f.write(base64.b64decode(deck("dump")))
    print(f"     deck image {path}")


def qemu(phase, usb_stick):
    global LOG
    LOG = os.path.join(OUT, f"serial-{phase}.log")
    if os.path.exists(LOG):
        os.remove(LOG)
    if os.path.exists(MON):
        os.remove(MON)
    args = [
        "qemu-system-x86_64", "-machine", "q35", "-m", "2048", "-smp", "2",
        "-drive", "if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE_4M.fd",
        "-drive", f"if=pflash,format=raw,file={VARS}",
        "-device", "qemu-xhci,id=xhci",
        "-device", "usb-kbd,bus=xhci.0",
        "-drive", f"if=none,id=disk,file={DISK},format=raw",
        "-device", "nvme,drive=disk,serial=MINIPC0001",
        "-nic", "user,model=e1000e,"
                f"hostfwd=tcp:127.0.0.1:{PORT_WEB}-:8080,"
                f"hostfwd=tcp:127.0.0.1:{PORT_API}-:9999,"
                f"hostfwd=tcp:127.0.0.1:{PORT_DECK}-:7000",
        "-display", "none", "-vga", "std",
        "-monitor", f"unix:{MON},server,nowait",
        "-serial", "null", "-serial", f"file:{LOG}",
        "-no-reboot",
    ]
    if usb_stick:
        args += ["-drive", f"if=none,id=stick,file={ISO},format=raw,readonly=on",
                 "-device", "usb-storage,bus=xhci.0,drive=stick,removable=on,bootindex=0"]
    return subprocess.Popen(args)


def phase1():
    vm = qemu("1-live-usb", usb_stick=True)
    try:
        mon = Monitor(MON)
        t0 = time.time()
        m = wait_log(r"TEST: boot complete mode=(\w+)", 300)
        print(f"     booted from USB in {time.time() - t0:.0f}s")
        check(m.group(1) == "live", "UEFI boots the ISO from a USB stick into live mode")
        wait_log(r"TEST: storage=/dev/sd")
        check(True, "live system found and mounted its own USB stick")
        wait_log(r"TEST: modules=\d+ nvme=1 usbhid=1", 30)
        check(True, "NVMe disk and USB keyboard drivers loaded")
        check(re.search(r"ip=10\.0\.2\.", log_text()) is not None, "Ethernet (e1000e) driver loaded and DHCP got an address")

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

        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, *a, **k):
                return None
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
    out = subprocess.run(["sfdisk", "--dump", DISK], capture_output=True, text=True).stdout
    check("label: gpt" in out and "C12A7328-F81F-11D2-BA4B-00A0C93EC93B" in out, "NVMe disk has a GPT with an EFI System Partition")
    off = int(re.search(r"start=\s*(\d+)", out).group(1)) * 512
    img = f"{DISK}@@{off}"
    files = subprocess.run(["mdir", "-/", "-b", "-i", img, "::"], capture_output=True, text=True).stdout
    for f in ["EFI/BOOT/BOOTX64.EFI", "streamdeck/boot/vmlinuz", "streamdeck/boot/initrd.img",
              "streamdeck/boot/installed.cfg", "streamdeck/config.json", "streamdeck/state/secrets.json"]:
        check("::/" + f in files, f"ESP contains {f}")
    label = subprocess.run(["mlabel", "-s", "-i", img, "::"], capture_output=True, text=True).stdout
    check("STREAMDECK" in label, "ESP is labelled STREAMDECK")


def phase2():
    vm = qemu("2-installed", usb_stick=False)
    try:
        mon = Monitor(MON)
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

        # Settings menu: change the default speaker to Kitchen Echo.
        mon.type("1")
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


def main():
    subprocess.run(["truncate", "-s", "0", DISK], check=True)
    subprocess.run(["truncate", "-s", "16G", DISK], check=True)
    shutil.copy("/usr/share/OVMF/OVMF_VARS_4M.fd", VARS)
    phase1()
    inspect_disk()
    phase2()
    print()
    if failures:
        print(f"{len(failures)} FAILED:", *failures, sep="\n  ")
        sys.exit(1)
    print("ALL PASSED")


if __name__ == "__main__":
    main()
