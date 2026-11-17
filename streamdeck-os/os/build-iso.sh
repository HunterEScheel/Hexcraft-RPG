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
need xorriso xorriso; need mformat mtools; need depmod kmod; need zstd zstd; need cpio cpio; need go golang; need git git; need make make; need patch patch; need nm binutils
[ "$TEST" = 0 ] || [ -x /usr/sbin/hostapd ] || { echo "missing hostapd — apt-get install hostapd" >&2; exit 1; }
[ -x /usr/sbin/wpa_supplicant ] || { echo "missing wpa_supplicant — apt-get install wpasupplicant" >&2; exit 1; }
[ -f /etc/ssl/certs/ca-certificates.crt ] || { echo "missing CA bundle — apt-get install ca-certificates" >&2; exit 1; }

# ---- 1. download kernel, busybox, firmware ----
# The HWE kernel: newer USB Wi-Fi drivers (e.g. Realtek RTL8812AU/8821AU/8814AU,
# RTL8851BU/8852BU) that the 6.8 GA kernel lacks.
KERNEL_META=${KERNEL_META:-linux-image-generic-hwe-24.04}
KPKG=$(apt-cache depends "$KERNEL_META" | grep -o 'linux-image-[0-9][^ ]*-generic' | head -1)
KVER=${KPKG#linux-image-}
KPKGS="$KPKG linux-modules-$KVER"
apt-cache show "linux-modules-extra-$KVER" >/dev/null 2>&1 && KPKGS="$KPKGS linux-modules-extra-$KVER"
FW_PKGS="linux-firmware-intel-wireless linux-firmware-realtek linux-firmware-mediatek linux-firmware-qualcomm-wireless linux-firmware-broadcom-wireless linux-firmware-misc wireless-regdb"
X="$WORK/extract-$KVER"
if [ ! -f "$X/.done" ]; then
	rm -f "$WORK"/dl/linux-image-* "$WORK"/dl/linux-modules-*
	(cd "$WORK/dl" && apt-get download $KPKGS busybox-static $FW_PKGS)
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
# Networking is Wi-Fi only: no wired Ethernet chips and no USB Ethernet
# adapters (drivers/net/ethernet, drivers/net/usb and their PHY helpers).
# Storage: NVMe, SATA, eMMC and USB sticks only; server SCSI/FC/iSCSI cards
# (which would drag in server Ethernet drivers) are left out. SCSI disk/CD
# support itself is built into the kernel.
KEEP='^kernel/(drivers/(hid|usb|input|net/wireless|ata|nvme|mmc|cdrom|platform/x86/(wmi|intel)|thunderbolt|pinctrl|gpio|i2c)|net/(wireless|mac80211|rfkill|packet)|fs/(isofs|fat|nls|exfat|ntfs3|udf)|crypto|lib)'
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
if grep -qE '^kernel/drivers/net/(ethernet|usb)/' "$WORK/modules.list"; then
	echo "Ethernet drivers crept in via dependencies:" >&2
	grep -E '^kernel/drivers/net/(ethernet|usb)/' "$WORK/modules.list" >&2
	exit 1
fi
# Debug/legacy drivers that would grab the keyboard or spam the log.
sed -i -e '/\/evbug\.ko/d' -e '/\/usbkbd\.ko/d' -e '/\/usbmouse\.ko/d' "$WORK/modules.list"
while read -r m; do
	dst="$R/lib/modules/$KVER/${m%.zst}"
	mkdir -p "$(dirname "$dst")"
	zstd -qdc "$MODS/$m" > "$dst"
done < "$WORK/modules.list"
# ---- kernel driver patches (os/kernel-patches) ----
# rtw88-8821c-rfe7.patch: generic "AC600" RTL8811CU dongles (0bda:c811) report
# RFE type 7, which the in-kernel rtw88_8821c driver rejects ("rfe 7 isn't
# supported"). Only that module is rebuilt, from the upstream source of this
# kernel version with Ubuntu's headers and the same gcc as the kernel.
patch_rtw8821c() {
	local tag base cc hdr src b ko
	base=${KVER%%-*}                       # 7.0.0
	tag=v${base%.0}                        # v7.0 (v7.0.N for point releases)
	cc=$(sed -n 's/^CONFIG_CC_VERSION_TEXT="\([^ ]*\).*/\1/p' "$X/boot/config-$KVER")   # x86_64-linux-gnu-gcc-13
	command -v "$cc" >/dev/null || { echo "missing $cc (the kernel's compiler) — apt-get install $cc" >&2; exit 1; }
	hdr="$WORK/headers-$KVER"
	if [ ! -d "$hdr/usr/src/linux-headers-$KVER" ]; then
		mkdir -p "$hdr/dl"
		(cd "$hdr/dl" && apt-get download "linux-headers-$KVER" \
			$(apt-cache depends "linux-headers-$KVER" | sed -n 's/^ *Depends: \(linux-.*headers.*\)/\1/p'))
		for d in "$hdr"/dl/*.deb; do dpkg-deb -x "$d" "$hdr"; done
	fi
	src="$WORK/linux-src-$tag"
	if [ ! -d "$src/drivers/net/wireless/realtek/rtw88" ]; then
		rm -rf "$src"
		git clone -q --depth 1 --filter=blob:none --sparse --branch "$tag" https://github.com/torvalds/linux.git "$src"
		git -C "$src" sparse-checkout set drivers/net/wireless/realtek/rtw88
	fi
	b="$WORK/build-rtw88"
	rm -rf "$b" && cp -r "$src/drivers/net/wireless/realtek/rtw88" "$b"
	patch -s -d "$b" -p6 < "$HERE/kernel-patches/rtw88-8821c-rfe7.patch"
	rm -f "$b/Makefile"
	printf 'obj-m := rtw88_8821c.o\nrtw88_8821c-y := rtw8821c.o rtw8821c_table.o\n' > "$b/Kbuild"
	make -s -C "$hdr/usr/src/linux-headers-$KVER" M="$b" CC="$cc" modules 2>&1 | grep -v -e pahole -e 'BTF generation' -e 'built with' -e 'You are using' || true
	# Sanity check: the RFE table must have grown from 7 to 8 entries.
	zstd -qdcf "$MODS/kernel/drivers/net/wireless/realtek/rtw88/rtw88_8821c.ko.zst" > "$b/shipped.ko"
	old=$(nm -S "$b/shipped.ko" | awk '/rtw8821c_rfe_defs/{print $2}')
	new=$(nm -S "$b/rtw88_8821c.ko" | awk '/rtw8821c_rfe_defs/{print $2}')
	[ $((0x$new * 7)) -eq $((0x$old * 8)) ] || { echo "rtw88_8821c patch check failed ($old -> $new)" >&2; exit 1; }
	ko="$R/lib/modules/$KVER/kernel/drivers/net/wireless/realtek/rtw88/rtw88_8821c.ko"
	cp "$b/rtw88_8821c.ko" "$ko"
	echo "patched rtw88_8821c: RFE 7 support"
}
patch_rtw8821c

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

# Plus every firmware file a network driver in the image says it needs
# (modinfo), e.g. rt2870.bin (Ralink/MediaTek USB) or htc_9271.fw (Atheros
# AR9271 USB). Symlinks bring their targets along.
(cd "$MODS" && grep -E '^kernel/drivers/(net|usb)/' "$WORK/modules.list" | xargs modinfo -F firmware 2>/dev/null) | sort -u \
	| python3 -c '
import glob, os, sys
fw = sys.argv[1]
out = set()
def add(rel):
    p = os.path.join(fw, rel)
    if os.path.lexists(p):
        out.add("./" + rel)
        if os.path.islink(p):
            add(os.path.normpath(os.path.join(os.path.dirname(rel), os.readlink(p))))
for name in sys.stdin.read().split():
    for cand in glob.glob(os.path.join(fw, name)) + glob.glob(os.path.join(fw, name + ".zst")):
        add(os.path.relpath(cand, fw))
print("\n".join(sorted(out)))
' "$FW" | (cd "$FW" && cpio -pdmu --quiet "$R/lib/firmware")
# Wi-Fi regulatory database (allowed channels per country).
cp "$FW/regulatory.db" "$R/lib/firmware/regulatory.db"
cp "$FW/regulatory.db.p7s" "$R/lib/firmware/regulatory.db.p7s"

# wpa_supplicant/wpa_cli and their libraries from the build host (Ubuntu 24.04).
cp /usr/sbin/wpa_supplicant /usr/sbin/wpa_cli "$R/usr/sbin/"
for lib in $(for b in /usr/sbin/wpa_supplicant /usr/sbin/wpa_cli; do ldd "$b"; done | grep -o '/[^ :]*\.so[^ ]*' | sort -u); do
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
	# hostapd + libs: the virtual Wi-Fi router for the Wi-Fi test.
	cp /usr/sbin/hostapd "$T/usr/sbin/"
	for lib in $(ldd /usr/sbin/hostapd | grep -o '/[^ :]*\.so[^ ]*'); do
		mkdir -p "$T$(dirname "$lib")"; cp -L "$lib" "$T$lib"
	done
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
