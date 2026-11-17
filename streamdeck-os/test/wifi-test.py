#!/usr/bin/env python3
"""Wi-Fi end-to-end test: a mini PC with NO Ethernet and a "USB Wi-Fi dongle".

The dongle and the home router are virtual radios from the kernel's
mac80211_hwsim (see test/rootfs/usr/sbin/test-wifi-ap): the router runs
hostapd with WPA2 network "StreamDeckTest" / "correct horse" plus DHCP.

Phase 1 (live USB, no network at all):
  - the installer finds no Wi-Fi adapter and explains why
  - the dongle is "plugged in" mid-install; the installer finds it, scans,
    and lists the network
  - a wrong password is called out as a wrong password
  - the right password connects and DHCP gives an address
  - the rest of the install runs over Wi-Fi (Spotify code typed on the console)
Phase 2 (installed, rebooted, still no Ethernet):
  - the dongle appears 5 s after boot; netwatch must reconnect on its own
  - the status screen shows the Wi-Fi connection; a deck key press works

Usage: test/wifi-test.py <workdir containing streamdeck-os-test.iso>
"""
import os
import re
import sys
import time

sys.path.insert(0, os.path.dirname(__file__))
import vmlib
from vmlib import CLIENT_ID, CLIENT_SECRET, GOVEE_KEY, Monitor, Shell, check, finish, setup, wait_log

SHELL = None


def phase1():
    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("w1-live-wifi", usb_stick=True, nic=False, shell=sock)
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        m = wait_log(r"TEST: boot complete mode=(\w+)", 300)
        check(m.group(1) == "live", "boots the ISO with no Ethernet")

        # No adapter yet: the installer must say so instead of failing silently.
        wait_log(r"SETUP: wifi-no-adapter", 120)
        wait_log(r"SETUP: prompt wifi-retry")
        check(True, "no Wi-Fi adapter: installer explains and offers to look again")
        mon.screenshot("w1-01-no-adapter")

        sh.run("/usr/sbin/test-wifi-ap")
        wait_log(r"TEST: wifi dongle plugged in", 30)
        mark = len(vmlib.log_text())
        mon.type("")  # Enter = look again
        m = wait_log(r"SETUP: wifi-adapter (\S+) (\S+)", 60, mark)
        check(m.group(1) == "wlan0" and m.group(2) == "mac80211_hwsim", f"dongle plugged in mid-install is found ({m.group(1)}, {m.group(2)})")
        m = wait_log(r"SETUP: wifi-scan (\d+)", 60, mark)
        check(int(m.group(1)) >= 1, "scan lists the network")
        wait_log(r"SETUP: prompt wifi-ssid", 30, mark)
        mon.screenshot("w1-02-scan")

        # Wrong password first.
        mon.type("1")
        wait_log(r"SETUP: prompt wifi-pass", 30, mark)
        mon.type("wrong password")
        wait_log(r"SETUP: prompt wifi-country", 30, mark)
        mon.type("")
        m = wait_log(r"SETUP: wifi-result (\S+)", 90, mark)
        check(m.group(1) == "wrong-password", f"wrong password is reported as a wrong password ({m.group(1)})")
        mon.screenshot("w1-03-wrong-password")

        # Then the right one (typed on Dvorak like everything else).
        mark = len(vmlib.log_text())
        wait_log(r"SETUP: prompt wifi-ssid", 60, mark)
        mon.type("1")
        wait_log(r"SETUP: prompt wifi-pass", 30, mark)
        mon.type("correct horse")
        wait_log(r"SETUP: prompt wifi-country", 30, mark)
        mon.type("")
        m = wait_log(r"SETUP: wifi-result (\S+) ?(\S*)", 90, mark)
        check(m.group(1) == "connected" and m.group(2).startswith("192.168.77."),
              f"right password connects and gets an address ({m.group(1)} {m.group(2)})")

        # Rest of the installer.
        wait_log(r"SETUP: prompt govee-key", 60, mark)
        mon.type(GOVEE_KEY)
        wait_log(r"SETUP: govee-ok", 60, mark)
        wait_log(r"SETUP: prompt spotify-id", 30, mark)
        mon.type(CLIENT_ID)
        wait_log(r"SETUP: prompt spotify-secret", 30, mark)
        mon.type(CLIENT_SECRET)
        wait_log(r"SETUP: spotify-wait", 30, mark)
        mon.type("MOCKCODE")  # the code can also be typed on the console
        wait_log(r"SETUP: spotify-ok", 30, mark)
        wait_log(r"SETUP: prompt spotify-device", 30, mark)
        mon.type("1")
        wait_log(r"SETUP: saved", 30, mark)
        wait_log(r"SETUP: prompt install-disk", 30, mark)
        mon.type("1")
        wait_log(r"SETUP: prompt install-confirm", 30, mark)
        mon.type("ERASE")
        m = wait_log(r"SETUP: (installed \w+|install-error .*)", 180, mark)
        check(m.group(1) == "installed nvme0n1", "installed over Wi-Fi setup")
        wait_log(r"SETUP: prompt reboot", 30, mark)
        mon.type("")
        vm.wait(timeout=120)
    finally:
        if vm.poll() is None:
            vm.kill()
            vm.wait()


def phase2():
    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("w2-installed-wifi", usb_stick=False, nic=False, shell=sock)
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        m = wait_log(r"TEST: boot complete mode=(\w+)", 300)
        check(m.group(1) == "installed", "installed system boots (no Ethernet)")
        wait_log(r"TEST: wifi dongle plugged in", 60)
        m = wait_log(r"TEST: wifi connected ip=(\S+) after (\d+)s", 90)
        check(m.group(1).startswith("192.168.77."),
              f"dongle that appears late is picked up and reconnects by itself ({m.group(2)} s after it appeared)")
        out = sh.run("cat /var/log/messages | grep netwatch | tail -3")
        check("starting Wi-Fi on wlan0" in out, "netwatch started Wi-Fi on the late adapter")
        time.sleep(16)  # status screen refreshes every 15 s
        mon.screenshot("w2-01-status")

        wait_log(r"config loaded: 2 page", 60)
        mark = len(vmlib.log_text())
        out = sh.run("echo 'press 1' | nc 127.0.0.1 7000")
        wait_log(r'key 1 "Party" done', 60, mark)
        check(True, "deck works on the Wi-Fi-only install")
    finally:
        vm.kill()
        vm.wait()


def main():
    setup(sys.argv[1], results="results-wifi")
    phase1()
    phase2()
    finish()


if __name__ == "__main__":
    main()
