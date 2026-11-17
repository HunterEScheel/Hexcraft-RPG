#!/usr/bin/env python3
"""Driver-CD dongle test.

Cheap USB Wi-Fi dongles (e.g. generic "AC600" RTL8811CU sticks, USB ID
0bda:1a2b) first appear as a virtual CD-ROM with Windows drivers and only
become a Wi-Fi adapter after that CD is ejected. QEMU can't fake a Realtek
ID, so the test image adds QEMU's own USB storage ID (46f4:0001, CD-ROM only)
to the switch list. A real SCSI eject is checked by QEMU's CD tray opening.

Checks: the dongle hot-plugged after boot is ejected, exactly once, and the
installer USB stick (same QEMU ID, but a disk) is left alone.

Usage: test/modeswitch-test.py <workdir containing streamdeck-os-test.iso>
"""
import os
import re
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(__file__))
import vmlib
from vmlib import Monitor, Shell, check, finish, setup, wait_log


def main():
    setup(sys.argv[1], results="results-modeswitch")
    drv = os.path.join(vmlib.WORK, "driver-cd")
    os.makedirs(drv, exist_ok=True)
    open(os.path.join(drv, "Setup.exe"), "w").write("not really\n")
    cd = os.path.join(vmlib.WORK, "driver-cd.iso")
    subprocess.run(["xorriso", "-as", "mkisofs", "-quiet", "-V", "RTL_DRIVER", "-o", cd, drv], check=True)

    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    vm = vmlib.qemu("modeswitch", usb_stick=True, shell=sock,
                    extra=["-drive", f"if=none,id=fakecd,media=cdrom,readonly=on,file={cd}"])
    try:
        mon = Monitor(vmlib.MON)
        sh = Shell(sock)
        wait_log(r"TEST: boot complete", 300)
        wait_log(r"SETUP: prompt", 60)  # system settled
        check("tray open" not in mon.query("info block fakecd"), "fake dongle CD starts closed")

        mon.query("device_add usb-storage,bus=xhci.0,drive=fakecd,id=dongle")
        ok = False
        for _ in range(30):
            time.sleep(1)
            if "tray open" in mon.query("info block fakecd"):
                ok = True
                break
        check(ok, "dongle plugged in after boot gets the eject command (QEMU CD tray opened)")
        out = sh.run("grep modeswitch /var/log/messages")
        check("switched 46f4:0001" in out, "modeswitch logged the switch")

        time.sleep(10)  # several netwatch rounds
        out = sh.run("grep -c 'modeswitch: switched' /var/log/messages")
        n = re.findall(r"^(\d+)\s*$", out, re.M)
        check(n and n[-1] == "1", f"ejected exactly once ({n[-1] if n else '?'} times)")
        out = sh.run("cat /run/streamdeck-media; grep /media/live /proc/mounts | cut -d' ' -f1; ls /media/live/streamdeck/boot")
        check("initrd.img" in out, "installer USB stick (also QEMU storage, but a disk) left alone and still mounted")
        stick = mon.query("info block stick")
        check("tray open" not in stick, "installer USB stick not ejected")

        # A USB device that is present but whose driver isn't loaded (what the
        # dongle looks like after it re-appears in Wi-Fi mode): unload the
        # keyboard driver; netwatch must load it again and rebind.
        out = sh.run("rmmod usbhid; sleep 0.5; ls /sys/bus/usb/devices/1-1:1.0/driver 2>&1 | head -1")
        check("No such file" in out, "keyboard driver unloaded (device left without a driver)")
        out = sh.run("for i in $(seq 1 20); do [ -e /sys/bus/usb/devices/1-1:1.0/driver ] && break; sleep 1; done; "
                     "echo after=$i; basename $(readlink /sys/bus/usb/devices/1-1:1.0/driver)", timeout=40)
        check("usbhid" in out, f"netwatch loaded the missing driver and the device got it back ({out.strip().splitlines()[-2:]})")

        # The USB Wi-Fi drivers load on this kernel (dependencies, firmware paths).
        mods = "rtw88_8821cu rtw88_8821au rtw88_8822bu rtl8xxxu mt7601u mt76x2u mt7921u rt2800usb ath9k_htc rtw89_8852bu"
        out = sh.run(f"for m in {mods}; do modprobe $m && echo ok:$m || echo FAIL:$m; done", timeout=60)
        bad = [l for l in out.split() if l.startswith("FAIL:")]
        check(not bad and out.count("ok:") == len(mods.split()), f"USB Wi-Fi drivers load ({', '.join(bad) or 'all ' + str(len(mods.split()))})")
    finally:
        vm.kill()
        vm.wait()
    finish()


if __name__ == "__main__":
    main()
