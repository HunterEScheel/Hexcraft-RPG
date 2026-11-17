#!/bin/bash
# Builds the StreamDeck OS installer ISO for x86_64 mini PCs (UEFI or BIOS).
#
# Run on Ubuntu 24.04 (or in the Dockerfile next to this script):
#   os/build-iso.sh [workdir] [--test]
# Output: <workdir>/streamdeck-os.iso  (--test: streamdeck-os-test.iso, which
# adds the virtual Stream Deck + mock Govee/Spotify used by test/iso-test.py)
#
# The system is an Ubuntu generic kernel + busybox + streamdeckd, running from
# RAM. Kernel modules and firmware are trimmed to keyboards/USB/storage and
# Ethernet/Wi-Fi.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
WORK="$(realpath -m "${1:-$ROOT/.iso}")"
TEST=0; [ "${2:-}" = "--test" ] && TEST=1
mkdir -p "$WORK/dl"

need() { command -v "$1" >/dev/null || { echo "missing $1 — apt-get install $2" >&2; exit 1; }; }
need grub-mkrescue "grub-common grub-efi-amd64-bin grub-pc-bin"
need xorriso xorriso; need mformat mtools; need depmod kmod; need zstd zstd; need cpio cpio; need go golang
[ -x /usr/sbin/wpa_supplicant ] || { echo "missing wpa_supplicant — apt-get install wpasupplicant" >&2; exit 1; }
[ -f /etc/ssl/certs/ca-certificates.crt ] || { echo "missing CA bundle — apt-get install ca-certificates" >&2; exit 1; }

# ---- 1. download kernel, busybox, firmware ----
KPKG=$(apt-cache depends linux-image-generic | grep -o 'linux-image-[0-9][^ ]*-generic' | head -1)
KVER=${KPKG#linux-image-}
FW_PKGS="linux-firmware-intel-wireless linux-firmware-realtek linux-firmware-mediatek linux-firmware-qualcomm-wireless linux-firmware-broadcom-wireless"
X="$WORK/extract-$KVER"
if [ ! -f "$X/.done" ]; then
	(cd "$WORK/dl" && apt-get download "$KPKG" "linux-modules-$KVER" "linux-modules-extra-$KVER" busybox-static $FW_PKGS)
	rm -rf "$X" && mkdir -p "$X"
	for d in "$WORK"/dl/*.deb; do dpkg-deb -x "$d" "$X"; done
	depmod -b "$X" "$KVER"
	touch "$X/.done"
fi
MODS="$X/lib/modules/$KVER"

# ---- 2. root filesystem ----
R="$WORK/rootfs"
rm -rf "$R" && mkdir -p "$R"/{bin,sbin,usr/bin,usr/sbin,dev,proc,sys,lib/modules,lib/firmware,etc/ssl/certs}
cp "$X/usr/bin/busybox" "$R/bin/busybox"
for a in $("$R/bin/busybox" --list-full); do [ -e "$R/$a" ] || ln -s /bin/busybox "$R/$a"; done
ln -sf /bin/busybox "$R/init"
mknod -m 600 "$R/dev/console" c 5 1
mknod -m 666 "$R/dev/null" c 1 3

# Kernel modules: the directories below plus everything they depend on.
KEEP='^kernel/(drivers/(hid|usb|input|net/(ethernet|wireless|usb|phy|mdio|pcs|mii\.ko)|ata|nvme|scsi|mmc|cdrom|platform/x86/(wmi|intel)|thunderbolt|pinctrl|gpio|i2c)|net/(wireless|mac80211|rfkill|packet)|fs/(isofs|fat|nls)|crypto|lib)'
python3 - "$MODS" "$KEEP" > "$WORK/modules.list" <<'PY'
import re, sys
mods, keep = sys.argv[1], re.compile(sys.argv[2])
deps = {}
for line in open(f"{mods}/modules.dep"):
    m, _, d = line.partition(":")
    deps[m] = d.split()
todo = [m for m in deps if keep.match(m)]
seen = set()
while todo:
    m = todo.pop()
    if m not in seen:
        seen.add(m); todo += deps.get(m, [])
print("\n".join(sorted(seen)))
PY
# Debug/legacy drivers that would grab the keyboard or spam the log.
sed -i -e '/\/evbug\.ko/d' -e '/\/usbkbd\.ko/d' -e '/\/usbmouse\.ko/d' "$WORK/modules.list"
while read -r m; do
	dst="$R/lib/modules/$KVER/${m%.zst}"
	mkdir -p "$(dirname "$dst")"
	zstd -qdc "$MODS/$m" > "$dst"
done < "$WORK/modules.list"
cp "$MODS"/modules.{builtin,builtin.modinfo,order} "$R/lib/modules/$KVER/"
depmod -b "$R" "$KVER"

# Firmware: Wi-Fi + Ethernet only. For Intel Wi-Fi keep, per chip, the newest
# firmware API the kernel's iwlwifi driver can load (it asks for max..min).
FW="$X/lib/firmware"
IWL_MAX=$(modinfo -F firmware "$MODS/kernel/drivers/net/wireless/intel/iwlwifi/iwlwifi.ko.zst")
(cd "$FW" && find . \( -path './rtl_nic/*' -o -path './rtw88/*' -o -path './rtw89/*' -o -path './rtlwifi/*' \
	-o -path './mediatek/*' -o -name 'mt76*' -o -path './ath10k/*' -o -path './ath11k/*' -o -path './ath12k/*' \
	-o -path './brcm/*' -o -path './intel/iwlwifi/*' -o -name 'iwlwifi-*' \) \( -type f -o -type l \) \
	| grep -v -e '/rtl_bt/' -e 'BT_' -e '/qca/' -e 'mediatek/mt799' -e 'mediatek/mt8' -e 'mediatek/sof' \
	| IWL_MAX="$IWL_MAX" python3 -c '
import os, re, sys
rx = re.compile(r"iwlwifi-(.+)-(\w+?)\.ucode")
maxapi = {}
for name in os.environ["IWL_MAX"].split():
    m = rx.match(name)
    if m and m.group(2).isdigit():
        maxapi[m.group(1)] = int(m.group(2))
files = [l.strip() for l in sys.stdin]
best = {}
for f in files:
    m = rx.match(os.path.basename(f))
    if m and m.group(2).isdigit() and m.group(1) in maxapi and int(m.group(2)) <= maxapi[m.group(1)]:
        k = (os.path.dirname(f), m.group(1))
        best[k] = max(best.get(k, 0), int(m.group(2)))
for f in files:
    m = rx.match(os.path.basename(f))
    if not m:
        print(f)
    elif m.group(2).isdigit() and best.get((os.path.dirname(f), m.group(1))) == int(m.group(2)):
        print(f)
    # other versions and chips this driver cannot use are dropped
' | cpio -pdm --quiet "$R/lib/firmware")

# wpa_supplicant and its libraries from the build host (Ubuntu 24.04).
cp /usr/sbin/wpa_supplicant "$R/usr/sbin/"
for lib in $(ldd /usr/sbin/wpa_supplicant | grep -o '/[^ ]*'); do
	mkdir -p "$R$(dirname "$lib")"; cp -L "$lib" "$R$lib"
done
cp /etc/ssl/certs/ca-certificates.crt "$R/etc/ssl/certs/"

cp -a "$HERE/rootfs/." "$R/"

cd "$ROOT/app"
CGO_ENABLED=0 GOFLAGS=-mod=vendor go build -trimpath -ldflags "-s -w -X main.version=$(git describe --always --dirty 2>/dev/null || echo dev)" \
	-o "$R/usr/bin/streamdeckd" ./cmd/streamdeckd

# ---- 3. images ----
ISO="$WORK/iso"
rm -rf "$ISO" && mkdir -p "$ISO/streamdeck/boot" "$ISO/boot/grub"
cp "$X/boot/vmlinuz-$KVER" "$ISO/streamdeck/boot/vmlinuz"
(cd "$R" && find . | cpio -o -H newc --quiet | zstd -q -10 -T0) > "$ISO/streamdeck/boot/initrd.img"
INITRDS=/streamdeck/boot/initrd.img
NAME=streamdeck-os
if [ "$TEST" = 1 ]; then
	T="$WORK/testfs"
	rm -rf "$T" && mkdir -p "$T/usr/bin"
	cp -a "$ROOT/test/rootfs/." "$T/"
	for t in fakedeck mockapi; do
		CGO_ENABLED=0 GOFLAGS=-mod=vendor go build -trimpath -ldflags "-s -w" -o "$T/usr/bin/$t" "./testtools/$t"
	done
	(cd "$T" && find . | cpio -o -H newc --quiet | zstd -q -10) > "$ISO/streamdeck/boot/test.img"
	INITRDS="$INITRDS /streamdeck/boot/test.img"
	NAME=streamdeck-os-test
fi

# Boot loader for the installed system (copied to EFI/BOOT/BOOTX64.EFI by the installer).
grub-mkstandalone -O x86_64-efi -o "$ISO/streamdeck/boot/BOOTX64.EFI" \
	--modules="part_gpt part_msdos fat search search_fs_file normal linux configfile all_video echo" \
	--locales= --fonts= "boot/grub/grub.cfg=$HERE/grub/embedded.cfg"

sed "s#@INITRDS@#$INITRDS#" "$HERE/grub/live.cfg" > "$ISO/boot/grub/grub.cfg"
grub-mkrescue -o "$WORK/$NAME.iso" "$ISO" -- -volid STREAMDECK_LIVE 2>&1 | grep -v '^xorriso : UPDATE' || true

echo
echo "kernel $KVER, $(wc -l < "$WORK/modules.list") modules, firmware $(du -sh "$R/lib/firmware" | cut -f1)"
echo "initrd $(du -h "$ISO/streamdeck/boot/initrd.img" | cut -f1)"
echo "ISO: $WORK/$NAME.iso ($(du -h "$WORK/$NAME.iso" | cut -f1))"
