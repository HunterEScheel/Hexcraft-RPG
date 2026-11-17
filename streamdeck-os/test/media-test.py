#!/usr/bin/env python3
"""Installer media test: every common way of writing the USB stick.

Field report: "install failed: open /media/live/streamdeck/boot: no such file
or directory". The stick booted but the live system only looked for a raw
ISO 9660 filesystem, so sticks made with Rufus "ISO mode" (files copied to
FAT32) or Ventoy (the .iso kept as a file) were never found, and the
installer had already wiped the disk before noticing.

The VM boots the kernel directly (OVMF direct boot), so the stick under test
is the only installer media present:
  A. Rufus "ISO mode": ISO contents copied onto an MBR + FAT32 stick
  B. Ventoy-style: the .iso file on an MBR + FAT32 stick (loop-mounted)
  C. a stick without StreamDeck OS files: the installer must refuse and the
     NVMe disk must be left untouched

Usage: test/media-test.py <workdir containing streamdeck-os-test.iso>
"""
import os
import re
import subprocess
import sys

sys.path.insert(0, os.path.dirname(__file__))
import vmlib
from vmlib import Monitor, Shell, check, finish, setup, wait_log


def sh(*cmd, **kw):
    return subprocess.run(cmd, check=True, capture_output=True, text=True, **kw).stdout


def fat_stick(path, label, files):
    """MBR disk image with one FAT32 partition holding `files` (dir or file paths)."""
    subprocess.run(["truncate", "-s", "0", path], check=True)
    subprocess.run(["truncate", "-s", "512M", path], check=True)
    subprocess.run(["sfdisk", "-q", path], input="label: dos\nstart=2048, type=c, bootable\n", text=True, check=True)
    img = f"{path}@@1M"
    sh("mformat", "-i", img, "-F", "-v", label, "::")
    for f in files:
        sh("mcopy", "-s", "-i", img, f, "::/")


def run_case(name, stick, kernel, initrd, expect_media):
    sock = os.path.join(vmlib.WORK, "ttyS0.sock")
    subprocess.run(["truncate", "-s", "0", vmlib.DISK], check=True)
    subprocess.run(["truncate", "-s", "16G", vmlib.DISK], check=True)
    vm = vmlib.qemu(f"media-{name}", usb_stick=False, shell=sock, extra=[
        "-kernel", kernel, "-initrd", initrd, "-append", "streamdeck.live quiet loglevel=3",
        "-drive", f"if=none,id=testmedia,format=raw,readonly=on,file={stick}",
        "-device", "usb-storage,bus=xhci.0,drive=testmedia,removable=on",
    ])
    try:
        mon = Monitor(vmlib.MON)
        shell = Shell(sock)
        wait_log(r"TEST: boot complete", 300)
        out = shell.run("cat /run/streamdeck-media 2>&1; ls /media/live/streamdeck/boot 2>&1")
        if expect_media:
            check(expect_media in out and "initrd.img" in out, f"{name}: installer files found ({out.split()[0] if out.split() else '?'})")
        # Skip Wi-Fi, Govee and Spotify; go straight to the install step.
        wait_log(r"SETUP: prompt wifi-optional", 120)
        mon.type("n")
        wait_log(r"SETUP: prompt govee-key")
        mon.type("")
        wait_log(r"SETUP: prompt spotify-id")
        mon.type("")
        m = wait_log(r"SETUP: (prompt install-disk|install-no-media)", 90)
        if expect_media:
            check(m.group(1) == "prompt install-disk", f"{name}: installer offers the disk")
            mon.type("1")
            wait_log(r"SETUP: prompt install-confirm")
            mon.type("ERASE")
            m = wait_log(r"SETUP: (installed \w+|install-error .*)", 180)
            check(m.group(1) == "installed nvme0n1", f"{name}: installed to NVMe ({m.group(1)})")
        else:
            check(m.group(1) == "install-no-media", f"{name}: installer refuses when its files can't be found")
            mon.screenshot(f"media-{name}")
    finally:
        vm.kill()
        vm.wait()

    if expect_media:
        out = sh("sfdisk", "--dump", vmlib.DISK)
        off = int(re.search(r"start=\s*(\d+)", out).group(1)) * 512
        files = sh("mdir", "-/", "-b", "-i", f"{vmlib.DISK}@@{off}", "::")
        check(all(f"::/{f}" in files for f in ["EFI/BOOT/BOOTX64.EFI", "streamdeck/boot/vmlinuz", "streamdeck/boot/initrd.img"]),
              f"{name}: system files copied from the stick")
    else:
        with open(vmlib.DISK, "rb") as f:
            head = f.read(4 << 20)
        check(head == bytes(len(head)), f"{name}: NVMe disk untouched (first 4 MB still zero)")


def main():
    setup(sys.argv[1], results="results-media")
    W = vmlib.WORK
    iso_dir = os.path.join(W, "media-iso-contents")
    subprocess.run(["rm", "-rf", iso_dir], check=True)
    os.makedirs(iso_dir)
    sh("xorriso", "-osirrox", "on", "-indev", vmlib.ISO, "-extract", "/", iso_dir)
    subprocess.run(["chmod", "-R", "u+w", iso_dir], check=True)
    boot = os.path.join(iso_dir, "streamdeck", "boot")
    kernel = os.path.join(boot, "vmlinuz")
    initrd = os.path.join(W, "media-initrd.img")
    with open(initrd, "wb") as out:
        for part in ("initrd.img", "test.img"):
            out.write(open(os.path.join(boot, part), "rb").read())

    rufus = os.path.join(W, "stick-rufus.img")
    fat_stick(rufus, "RUFUS_USB", [os.path.join(iso_dir, "streamdeck"), os.path.join(iso_dir, "boot"), os.path.join(iso_dir, "efi")])
    ventoy = os.path.join(W, "stick-ventoy.img")
    fat_stick(ventoy, "VENTOY", [vmlib.ISO])
    other = os.path.join(W, "stick-other.img")
    note = os.path.join(W, "README.txt")
    open(note, "w").write("just some files\n")
    fat_stick(other, "PHOTOS", [note])

    run_case("rufus-iso-mode", rufus, kernel, initrd, "/dev/sd")
    run_case("ventoy-iso-file", ventoy, kernel, initrd, ":streamdeck-os-test.iso")
    run_case("no-installer-files", other, kernel, initrd, None)
    finish()


if __name__ == "__main__":
    main()
