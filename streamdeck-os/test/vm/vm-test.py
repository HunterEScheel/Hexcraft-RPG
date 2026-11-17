#!/usr/bin/env python3
"""End-to-end VM test for StreamDeck OS.

Boots the test initramfs in QEMU and plays the user:
  1. types into the installer on tty1 through the VM's PS/2 keyboard
     (QEMU `sendkey` = physical key positions, so Dvorak must be active
     for the right characters to arrive),
  2. completes the Spotify login through the phone page on :8080,
  3. presses keys on the virtual Stream Deck MK.2 and checks the
     Govee/Spotify calls that come out.

Usage: test/vm/vm-test.py <workdir from build-initramfs.sh>
Screenshots and deck images are written to <workdir>/results/.
"""
import html
import os
import re
import socket
import subprocess
import sys
import time
import urllib.parse
import urllib.request

WORK = os.path.abspath(sys.argv[1])
OUT = os.path.join(WORK, "results")
os.makedirs(OUT, exist_ok=True)
LOG = os.path.join(OUT, "vm-serial.log")
MON = os.path.join(WORK, "mon.sock")

GOVEE_KEY = "tEsT-gOvEe-42xq"          # must match testtools/mockapi
CLIENT_ID, CLIENT_SECRET = "mockclientid", "mocksecret"
PORT_WEB, PORT_API, PORT_DECK = 18080, 19999, 17000

# US Dvorak: character -> (physical/QWERTY key name for QEMU, shift)
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
# What the same physical keys would mean on a QWERTY layout.
QWERTY = {" ": ("spc", False), "-": ("minus", False)}
for c in "abcdefghijklmnopqrstuvwxyz0123456789":
    QWERTY[c] = (c, False)
    if c.isalpha():
        QWERTY[c.upper()] = (c, True)

failures = []


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
        txt = log_text()
        m = rx.search(txt, after)
        if m:
            return m
        time.sleep(0.5)
    print(log_text()[-3000:])
    raise SystemExit(f"timeout waiting for /{pattern}/")


class Monitor:
    def __init__(self, path):
        for _ in range(100):
            try:
                self.s = socket.socket(socket.AF_UNIX)
                self.s.connect(path)
                break
            except OSError:
                time.sleep(0.2)
        self.s.settimeout(0.5)
        self.drain()

    def drain(self):
        try:
            while self.s.recv(65536):
                pass
        except socket.timeout:
            pass

    def cmd(self, c):
        self.s.sendall((c + "\n").encode())
        time.sleep(0.05)
        self.drain()

    def type(self, text, layout, enter=True):
        for ch in text:
            key, shift = layout[ch]
            self.cmd(f"sendkey {'shift-' if shift else ''}{key}")
        if enter:
            self.cmd("sendkey ret")

    def screenshot(self, name):
        ppm = os.path.join(OUT, name + ".ppm")
        self.cmd(f"screendump {ppm}")
        time.sleep(1)
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
        if deck_images() >= n:
            return True
        time.sleep(0.5)
    return False


def save_deck(name):
    import base64
    path = os.path.join(OUT, name + ".png")
    with open(path, "wb") as f:
        f.write(base64.b64decode(deck("dump")))
    print(f"     deck image {path}")


def main():
    if os.path.exists(LOG):
        os.remove(LOG)
    qemu = subprocess.Popen([
        "qemu-system-x86_64", "-m", "1024", "-smp", "2",
        "-kernel", os.path.join(WORK, "vmlinuz"),
        "-initrd", os.path.join(WORK, "initrd.gz"),
        "-append", "console=tty1 console=ttyS0 rdinit=/sbin/init loglevel=3",
        "-display", "none", "-vga", "std",
        "-monitor", f"unix:{MON},server,nowait",
        "-serial", "null",                       # ttyS0: debug shell (unused)
        "-serial", f"file:{LOG}",                # ttyS1: logs
        "-nic", "user,model=virtio-net-pci,"
                f"hostfwd=tcp:127.0.0.1:{PORT_WEB}-:8080,"
                f"hostfwd=tcp:127.0.0.1:{PORT_API}-:9999,"
                f"hostfwd=tcp:127.0.0.1:{PORT_DECK}-:7000",
        "-no-reboot",
    ])
    try:
        run()
    finally:
        qemu.kill()
        qemu.wait()
    print()
    if failures:
        print(f"{len(failures)} FAILED:", *failures, sep="\n  ")
        sys.exit(1)
    print("ALL PASSED")


def run():
    mon = Monitor(MON)
    t0 = time.time()
    wait_log(r"VM: boot complete", 240)
    print(f"     booted in {time.time() - t0:.0f}s")
    txt = log_text()
    check("Loading Dvorak keymap: OK" in txt, "Dvorak keymap loads at boot (real S01keymap script)")
    wait_log(r"virtual Stream Deck MK.2 created", 30)
    check(True, "virtual MK.2 created through uhid")

    # ---------- installer on tty1 ----------
    wait_log(r"SETUP: prompt wifi-optional", 120)
    mon.screenshot("01-installer-start")
    mon.type("n", DVORAK)

    wait_log(r"SETUP: prompt govee-key")
    # Typed as if the keyboard were QWERTY: with Dvorak active this must
    # arrive garbled and be rejected.
    mark = len(log_text())
    mon.type(GOVEE_KEY, QWERTY)
    wait_log(r"SETUP: govee-error", 60, mark)
    check(True, "QWERTY-positioned typing is rejected (layout really is not QWERTY)")
    wait_log(r"SETUP: prompt govee-key", 30, mark)
    mon.type(GOVEE_KEY, DVORAK)
    wait_log(r"SETUP: govee-ok 2", 60, mark)
    check(True, "Govee API key typed on Dvorak is accepted; 2 devices found")
    mon.screenshot("02-installer-govee")

    wait_log(r"SETUP: prompt spotify-id")
    mon.type(CLIENT_ID, DVORAK)
    wait_log(r"SETUP: prompt spotify-secret")
    mon.type(CLIENT_SECRET, DVORAK)
    m = wait_log(r"SETUP: spotify-wait state=(\w+)")
    state = m.group(1)
    time.sleep(1)
    mon.screenshot("03-installer-spotify")

    # ---------- the "phone": setup page -> Spotify -> paste address ----------
    page = urllib.request.urlopen(f"http://127.0.0.1:{PORT_WEB}/", timeout=20).read().decode()
    href = html.unescape(re.search(r'href="([^"]+)"', page).group(1))
    q = urllib.parse.parse_qs(urllib.parse.urlparse(href).query)
    check(q.get("client_id") == [CLIENT_ID] and q.get("state") == [state]
          and q.get("redirect_uri") == ["http://127.0.0.1:8888/callback"]
          and "user-modify-playback-state" in q.get("scope", [""])[0],
          "phone page links to Spotify login with client id, state, redirect URI, scopes")
    href = href.replace("127.0.0.1:9999", f"127.0.0.1:{PORT_API}")

    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, *a, **k):
            return None
    try:
        urllib.request.build_opener(NoRedirect).open(href, timeout=20)
        raise SystemExit("expected redirect from mock Spotify")
    except urllib.error.HTTPError as e:
        landed = e.headers["Location"]
    print(f"     browser lands on {landed}")
    body = urllib.parse.urlencode({"url": landed}).encode()
    resp = urllib.request.urlopen(f"http://127.0.0.1:{PORT_WEB}/", data=body, timeout=20).read().decode()
    check("Got it" in resp, "pasting the redirect address into the phone page is accepted")
    wait_log(r"SETUP: spotify-ok")
    check(True, "Spotify code exchanged for a refresh token")

    wait_log(r"SETUP: prompt spotify-device")
    time.sleep(1)
    mon.screenshot("04-installer-device")
    mon.type("1", DVORAK)
    wait_log(r'SETUP: spotify-device "Living Room"')
    wait_log(r"SETUP: saved")
    check(True, "installer saved settings (default speaker: Living Room)")
    time.sleep(3)
    mon.screenshot("05-status-screen")

    # ---------- daemon + virtual MK.2 ----------
    wait_log(r"config loaded: 2 page", 60)
    check(True, "daemon picked up the installer's settings without a restart")
    txt = log_text()
    check("found Stream Deck MK.2 at /dev/hidraw" in txt, "daemon found the MK.2 on /dev/hidraw")
    check("FAKEDECK: brightness 70%" in txt, "brightness feature report applied (70%)")
    check(wait_images(30), "15 placeholder + 15 home-page key images uploaded as valid 72x72 JPEGs")
    stats = deck("stats")
    check(" bad=0 " in stats, f"no malformed images ({stats})")
    save_deck("06-deck-home")

    mark = len(log_text())
    deck("press 1")
    wait_log(r'key 1 "Party" done', 60, mark)
    calls = log_text()[mark:]
    check('"capability":{"instance":"lightScene","type":"devices.capabilities.dynamic_scene","value":{"paramId":102,"id":2}}' in calls
          and '"device":"AA:AA:AA:AA:AA:AA:AA:01"' in calls,
          "Party button: Govee scene 'Party' sent to TV Backlight")
    check('"value":{"paramId":104,"id":4}' in calls and "AA:AA:AA:AA:AA:AA:AA:02" in calls,
          "Party button: Govee scene 'Rainbow' sent to Floor Lamp")
    check("PUT /spotify/api/me/player/volume?device_id=dev-living&volume_percent=60" in calls,
          "Party button: Spotify volume 60 on Living Room")
    check('PUT /spotify/api/me/player/play?device_id=dev-living {"context_uri":"spotify:playlist:37i9dQZF1DXaXB8fQg7xif"}' in calls,
          "Party button: playlist started on Living Room (link converted to spotify: URI)")
    check("PUT /spotify/api/me/player/shuffle?device_id=dev-living&state=true" in calls
          and "POST /spotify/api/me/player/next?device_id=dev-living" in calls,
          "Party button: shuffle on + skip to random start")

    mark = len(log_text())
    deck("press 2")
    wait_log(r'key 2 "Chill" done', 60, mark)
    calls = log_text()[mark:]
    check("device_id=dev-kitchen" in calls, "Chill button: per-button speaker override (Kitchen Echo)")

    n = deck_images()
    deck("press 4")
    check(wait_images(n + 15), "page button switches the deck to the 'music' page")
    time.sleep(1)
    save_deck("07-deck-music")


if __name__ == "__main__":
    main()
