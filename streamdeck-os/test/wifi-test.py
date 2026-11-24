#!/usr/bin/env python3
"""Wi-Fi end-to-end test: a mini PC with NO Ethernet and a "USB Wi-Fi dongle".

The dongle and the home router are virtual radios from the kernel's
mac80211_hwsim (see test/rootfs/usr/sbin/test-wifi-ap): the router runs
hostapd with WPA2 network "StreamDeckTest" / "correct horse" plus DHCP.

Phase 1 (USB stick, no network at all): install; no network is needed.
Phase 2 (installed, no network yet), on the home screen's Wi-Fi tile:
  - no adapter: the screen explains why and offers to look again
  - the dongle is "plugged in"; Look again finds it, scans, lists the network
  - a wrong password is called out as a wrong password
  - the right password connects and DHCP gives an address
  - Govee and Spotify are set up over Wi-Fi (the Spotify code typed on screen)
Phase 3 (rebooted, still no Ethernet):
  - the dongle appears 5 s after boot; netwatch must reconnect on its own
  - the home screen shows the connection; a deck key press works

Usage: test/wifi-test.py <workdir containing streamdeck-os-test.iso>
"""
import os
import sys
import time

sys.path.insert(0, os.path.dirname(__file__))
import vmlib
from vmlib import (CLIENT_ID, CLIENT_SECRET, GOVEE_KEY, Monitor, Shell, check, choose, dismiss, enter_text, finish,
                   home_open, mark, setup, wait_log)


def phase1():
    vm = vmlib.qemu("w1-install", usb_stick=True, nic=False)
    try:
        mon = Monitor(vmlib.MON)
        m = wait_log(r"TEST: boot complete mode=(\w+)", 300)
        check(m.group(1) == "live", "boots the ISO with no Ethernet")
        r = vmlib.install(mon)
        check(r == "installed nvme0n1", f"installs without any network ({r})")
        vm.wait(timeout=120)
    finally:
        if vm.poll() is None:
            vm.kill()
            vm.wait()


def join(mon, password, after):
    a = choose(mon, "wifi-networks", "StreamDeckTest", after, 90)
    a = enter_text(mon, "wifi-pass", password, a)
    return wait_log(r"APP: wifi-result (\S+) ?(\S*)", 90, a), a


def phase2():
    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("w2-installed-setup", usb_stick=False, nic=False, shell=sock)
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        m = wait_log(r"TEST: boot complete mode=(\w+)", 300)
        check(m.group(1) == "installed", "installed system boots (no Ethernet)")
        a = home_open(mon, "wifi", mark(), 120)
        wait_log(r"APP: wifi-no-adapter", 60, a)
        vmlib.menu("wifi-no-adapter", a)
        check(True, "no Wi-Fi adapter: the Wi-Fi screen explains and offers to look again")
        time.sleep(1)
        mon.screenshot("w2-01-no-adapter")

        sh.run("/usr/sbin/test-wifi-ap")
        wait_log(r"TEST: wifi dongle plugged in", 30)
        a = choose(mon, "wifi-no-adapter", "Look again", a)
        m = wait_log(r"APP: wifi-adapter (\S+) (\S+)", 60, a)
        check(m.group(1) == "wlan0" and m.group(2) == "mac80211_hwsim", f"dongle plugged in later is found ({m.group(1)}, {m.group(2)})")
        m = wait_log(r"APP: wifi-scan (\d+)", 60, a)
        check(int(m.group(1)) >= 1, "scan lists the network")
        vmlib.menu("wifi-networks", a)
        time.sleep(1)
        mon.screenshot("w2-02-networks")

        m, a = join(mon, "wrong password", a)
        check(m.group(1) == "wrong-password", f"wrong password is reported as a wrong password ({m.group(1)})")
        time.sleep(1)
        mon.screenshot("w2-03-wrong-password")
        a = dismiss(mon, "wifi-wrong-password", a)

        m, a = join(mon, "correct horse", a)  # typed on Dvorak like everything else
        check(m.group(1) == "connected" and m.group(2).startswith("192.168.77."),
              f"right password connects and gets an address ({m.group(1)} {m.group(2)})")
        a = dismiss(mon, "wifi-connected", a)
        wait_log(r"UI: home sel=", 30, a)

        # Keys over Wi-Fi; the Spotify code is typed on screen this time.
        a = home_open(mon, "govee", mark())
        a = choose(mon, "govee", "Govee API key", a)
        a = enter_text(mon, "govee-key", GOVEE_KEY, a)
        wait_log(r"APP: govee-ok", 60, a)
        a = dismiss(mon, "govee-ok", a)
        vmlib.menu("govee", a)
        a = mark()
        mon.key("esc")
        a = home_open(mon, "spotify", a)
        a = enter_text(mon, "spotify-id", CLIENT_ID, a)
        a = enter_text(mon, "spotify-secret", CLIENT_SECRET, a)
        wait_log(r"UI: info spotify-phone", 30, a)
        mon.type("t", enter=False)
        a = enter_text(mon, "spotify-code", "MOCKCODE", a)
        wait_log(r"APP: spotify-ok", 30, a)
        a = choose(mon, "spotify-speaker", "Living Room", a)
        vmlib.menu("spotify", a)
        mon.key("esc")
        check(True, "Govee and Spotify set up over Wi-Fi (Spotify code typed on screen)")
        time.sleep(3)
        sh.run("sync")
    finally:
        vm.kill()
        vm.wait()


def phase3():
    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("w3-reboot-wifi", usb_stick=False, nic=False, shell=sock)
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        wait_log(r"TEST: boot complete mode=installed", 300)
        wait_log(r"TEST: wifi dongle plugged in", 60)
        m = wait_log(r"TEST: wifi connected ip=(\S+) after (\d+)s", 90)
        check(m.group(1).startswith("192.168.77."),
              f"dongle that appears late is picked up and reconnects by itself ({m.group(2)} s after it appeared)")
        out = sh.run("cat /var/log/messages | grep netwatch | tail -3")
        check("starting Wi-Fi on wlan0" in out, "netwatch started Wi-Fi on the late adapter")
        time.sleep(5)  # home screen refreshes every few seconds
        mon.screenshot("w3-01-home")

        wait_log(r"config loaded: 2 page", 60)
        a = mark()
        sh.run("echo 'press 1' | nc 127.0.0.1 7000")
        wait_log(r'key 1 "Party" done', 60, a)
        check(True, "deck works on the Wi-Fi-only install")
    finally:
        vm.kill()
        vm.wait()


def main():
    setup(sys.argv[1], results="results-wifi")
    phase1()
    phase2()
    phase3()
    finish()


if __name__ == "__main__":
    main()
