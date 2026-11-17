#!/bin/sh
# Builds the VM test initramfs from:
#  - an Ubuntu generic kernel + its hid/uhid modules and busybox-static (downloaded with apt)
#  - the REAL appliance overlay (Dvorak keymap script/data, default layout)
#  - streamdeckd + the test tools (virtual MK.2, mock APIs), built static
# Usage: test/vm/build-initramfs.sh <workdir>
set -eu
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
WORK="${1:?workdir}"
mkdir -p "$WORK/dl"
cd "$WORK/dl"

KPKG=$(apt-cache depends linux-image-generic | grep -o 'linux-image-[0-9][^ ]*-generic' | head -1)
KVER=${KPKG#linux-image-}
if [ ! -f "$WORK/vmlinuz" ]; then
	apt-get download "$KPKG" "linux-modules-$KVER" "linux-modules-extra-$KVER" busybox-static
	rm -rf x && mkdir x
	for d in *.deb; do dpkg-deb -x "$d" x; done
	cp "x/boot/vmlinuz-$KVER" "$WORK/vmlinuz"
fi

R="$WORK/initramfs"
rm -rf "$R" && mkdir -p "$R"/bin "$R"/sbin "$R"/usr/bin "$R"/usr/sbin "$R"/dev "$R"/proc "$R"/sys "$R"/lib/modules "$R"/etc
cp x/usr/bin/busybox "$R/bin/busybox"
for a in $("$R/bin/busybox" --list-full); do
	[ -e "$R/$a" ] || ln -s /bin/busybox "$R/$a"
done
mknod -m 600 "$R/dev/console" c 5 1
mknod -m 666 "$R/dev/null" c 1 3

M="x/lib/modules/$KVER/kernel/drivers/hid"
for m in hid hid-generic uhid; do zstd -qdc "$M/$m.ko.zst" > "$R/lib/modules/$m.ko"; done

# Real appliance files first, then VM-specific ones on top.
cp -a "$ROOT/buildroot-external/board/common/rootfs-overlay/." "$R/"
cp -a "$HERE/rootfs/." "$R/"

cd "$ROOT/app"
for t in cmd/streamdeckd testtools/fakedeck testtools/mockapi; do
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS=-mod=vendor go build -trimpath -ldflags "-s -w" -o "$R/usr/bin/$(basename $t)" "./$t"
done

cd "$R" && find . | cpio -o -H newc --quiet | gzip -1 > "$WORK/initrd.gz"
echo "kernel: $WORK/vmlinuz ($KVER)"
echo "initrd: $WORK/initrd.gz ($(du -h "$WORK/initrd.gz" | cut -f1))"
