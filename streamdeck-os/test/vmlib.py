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
        rx = re.compile(rf"^{marker}\r?$", re.M)
        buf, end = "", time.time() + timeout
        while time.time() < end:
            try:
                buf += self.s.recv(65536).decode(errors="replace")
            except socket.timeout:
                pass
            m = rx.search(buf)
            if m:
                return buf[:m.start()].replace("\r", "")
        return buf.replace("\r", "")


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


