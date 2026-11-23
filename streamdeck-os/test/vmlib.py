"""Shared helpers for the StreamDeck OS VM tests (QEMU + OVMF virtual mini PC)."""
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

WORK = OUT = ISO = DISK = VARS = MON = None


def setup(workdir, results="results"):
    """Point the helpers at a work dir holding streamdeck-os-test.iso; fresh disk + firmware vars."""
    global WORK, OUT, ISO, DISK, VARS, MON
    WORK = os.path.abspath(workdir)
    ISO = os.path.join(WORK, "streamdeck-os-test.iso")
    OUT = os.path.join(WORK, results)
    os.makedirs(OUT, exist_ok=True)
    DISK = os.path.join(WORK, "nvme.img")
    VARS = os.path.join(WORK, "OVMF_VARS.fd")
    MON = os.path.join(WORK, "mon.sock")
    subprocess.run(["truncate", "-s", "0", DISK], check=True)
    subprocess.run(["truncate", "-s", "16G", DISK], check=True)
    shutil.copy("/usr/share/OVMF/OVMF_VARS_4M.fd", VARS)


class Shell:
    """Root shell on the test image's ttyS0, for actions a person would do
    physically (e.g. plugging in a Wi-Fi dongle)."""

    def __init__(self, path):
        for _ in range(200):
            try:
                self.s = socket.socket(socket.AF_UNIX)
                self.s.connect(path)
                break
            except OSError:
                time.sleep(0.1)
        self.s.settimeout(1)

    def run(self, cmd, timeout=60):
        """Run cmd in the VM's shell and return its output."""
        marker = f"__done_{time.time_ns()}__"
        try:  # discard anything left over
            while self.s.recv(65536):
                pass
        except socket.timeout:
            pass
        self.s.sendall(f"{cmd}; echo {marker}\n".encode())
        rx = re.compile(rf"^{marker}\r?$".encode(), re.M)
        buf, end = b"", time.time() + timeout
        while time.time() < end:
            try:
                buf += self.s.recv(65536)
            except socket.timeout:
                pass
            m = rx.search(buf)
            if m:
                return buf[:m.start()].decode(errors="replace").replace("\r", "")
        return buf.decode(errors="replace").replace("\r", "")


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
LAST_CLICK = 0


def finish():
    print()
    if failures:
        print(f"{len(failures)} FAILED:", *failures, sep="\n  ")
        sys.exit(1)
    print("ALL PASSED")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **k):
        return None


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

    def query(self, c):
        """Run a monitor command and return its output."""
        self.drain()
        self.s.sendall((c + "\n").encode())
        time.sleep(0.5)
        out = b""
        try:
            while True:
                out += self.s.recv(65536)
        except socket.timeout:
            pass
        return out.decode(errors="replace")

    def key(self, *names):
        """Special keys by QEMU name: up, down, left, right, ret, esc, delete, pgup, pgdn."""
        for n in names:
            self.cmd(f"sendkey {n}")
            time.sleep(0.05)

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


def qemu(phase, usb_stick, nic=True, shell=None, extra=()):
    """Start the virtual mini PC. nic=False: no Ethernet at all.
    shell: path of a unix socket connected to ttyS0 (the test image's root shell)."""
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
        "-device", "usb-mouse,bus=xhci.0",   # the keyboard's trackpad
        "-drive", f"if=none,id=disk,file={DISK},format=raw",
        "-device", "nvme,drive=disk,serial=MINIPC0001",
        "-display", "none", "-vga", "std",
        "-monitor", f"unix:{MON},server,nowait",
        "-serial", f"unix:{shell},server,nowait" if shell else "null",
        "-serial", f"file:{LOG}",
        "-no-reboot",
    ]
    args += list(extra)
    if nic:
        args += ["-nic", "user,model=virtio-net-pci,"
                 f"hostfwd=tcp:127.0.0.1:{PORT_WEB}-:8080,"
                 f"hostfwd=tcp:127.0.0.1:{PORT_API}-:9999,"
                 f"hostfwd=tcp:127.0.0.1:{PORT_DECK}-:7000"]
    else:
        args += ["-nic", "none"]
    if usb_stick:
        args += ["-drive", f"if=none,id=stick,file={ISO},format=raw,readonly=on",
                 "-device", "usb-storage,bus=xhci.0,drive=stick,removable=on,bootindex=0"]
    return subprocess.Popen(args)




# ---- driving the on-screen app (it traces "UI: ..." lines to ttyS1) ----

def mark():
    """Position in the log; pass as `after` to only look at what follows."""
    return len(log_text())


def _last(pattern, after, timeout, settle=0.4):
    wait_log(pattern, timeout, after)
    time.sleep(settle)  # let the screen finish (later traces win)
    ms = list(re.finditer(pattern, log_text()[after:]))
    return ms[-1]


def menu(mid, after=0, timeout=60):
    """(sel, items) of the menu with this ID as last drawn."""
    m = _last(rf"UI: menu {re.escape(mid)} sel=(\d+) items=(.*)", after, timeout)
    return int(m.group(1)), m.group(2).rstrip("\r").split("|")


def choose(mon, mid, label, after=0, timeout=60):
    """Pick the row whose label starts with `label` using the arrow keys."""
    sel, items = menu(mid, after, timeout)
    idx = [i for i, it in enumerate(items) if not it.startswith("#") and it.startswith(label)]
    if not idx:
        raise SystemExit(f"menu {mid}: no {label!r} in {items}")
    t = idx[0]
    lo, hi = sorted((sel, t))
    n = sum(1 for i in range(lo + 1, hi + 1) if not items[i].startswith("#"))
    if sel != t:
        mon.key(*(["down" if t > sel else "up"] * n))
    m = mark()
    mon.key("ret")
    return m


def enter_text(mon, iid, text, after=0, timeout=60, layout=DVORAK):
    """Type into text field `iid` and press Enter; returns the mark before Enter."""
    _last(rf"UI: input {re.escape(iid)}\b", after, timeout)
    mon.type(text, layout, enter=False)
    m = mark()
    mon.key("ret")
    return m


def dismiss(mon, nid, after=0, timeout=60):
    """Wait for message `nid` and press Enter on it."""
    _last(rf"UI: message {re.escape(nid)}\b", after, timeout)
    m = mark()
    mon.key("ret")
    return m


def home_open(mon, tile, after=0, timeout=60):
    """Open a home-screen tile with the arrow keys."""
    m = _last(r"UI: home sel=(\d+) tiles=(\S+)", after, timeout)
    tiles = m.group(2).split(",")
    n = (tiles.index(tile) - int(m.group(1))) % len(tiles)
    mon.key(*(["right"] * n))
    m = mark()
    mon.key("ret")
    return m


def targets(want=None, timeout=30):
    """Clickable things on the current screen: name -> centre (x, y)."""
    end = time.time() + timeout
    while True:
        time.sleep(0.6)
        ms = re.findall(r"UI: targets (.*)", log_text())
        out = {}
        if ms:
            for part in ms[-1].rstrip("\r").split(";"):
                name, _, xy = part.rpartition("=")
                if name:
                    x, y = xy.split(",")
                    out[name] = (int(x), int(y))
        if want is None or want in out or time.time() > end:
            return out


def click(mon, target, after=0, timeout=30):
    """Move the (virtual) trackpad pointer onto `target` and click it.

    The pointer is first pushed into the top-left corner, then moved in small
    steps (no pointer acceleration), like a finger on a touchpad."""
    t = targets(target, timeout)
    if target not in t:
        raise SystemExit(f"no {target!r} on screen: {sorted(t)}")
    x, y = t[target]
    m0 = mark()
    for _ in range(14):
        mon.cmd("mouse_move -120 -120")
    cx = cy = 0
    while cx < x or cy < y:
        dx, dy = min(4, x - cx), min(4, y - cy)
        mon.cmd(f"mouse_move {dx} {dy}")
        cx, cy = cx + dx, cy + dy
    time.sleep(0.3)
    global LAST_CLICK
    LAST_CLICK = mark()
    mon.cmd("mouse_button 1")
    time.sleep(0.1)
    mon.cmd("mouse_button 0")
    m = wait_log(r"UI: click (.*?) at \d+,\d+", 10, m0)
    return m.group(1)


def install(mon, after=0, label="Install StreamDeck OS"):
    """Live USB stick: install to the first disk; returns the APP: result."""
    m = choose(mon, "installer", label, after, 180)
    m = choose(mon, "install-disk", "", m)
    m = choose(mon, "install-confirm", "Erase and install", m)
    r = wait_log(r"APP: (installed \w+|install-error .*)", 180, m)
    if r.group(1).startswith("installed"):
        dismiss(mon, "install-done", m)
    return r.group(1)
