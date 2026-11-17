#!/bin/sh
# Runs before Buildroot's board/raspberrypi/post-image.sh: anything placed in
# rpi-firmware/ ends up on the FAT boot partition, where config.json can be
# edited from any computer.
set -eu
cp -r "${BR2_EXTERNAL_STREAMDECK_OS_PATH}/board/common/bootfs/." "${BINARIES_DIR}/rpi-firmware/"
# Quiet boot so the installer owns the screen.
CMDLINE="${BINARIES_DIR}/rpi-firmware/cmdline.txt"
grep -q 'loglevel=' "$CMDLINE" || sed -i 's/$/ loglevel=3/' "$CMDLINE"
