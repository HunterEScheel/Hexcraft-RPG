#!/bin/sh
# Hand the HDMI console to the installer/status screen and run the daemon.
set -eu
INITTAB="${TARGET_DIR}/etc/inittab"
# Drop any login prompt on tty1 (upstream Raspberry Pi scripts add one).
sed -i '/^tty1::/d' "$INITTAB"
grep -q streamdeckd "$INITTAB" || cat >> "$INITTAB" <<'EOT'

# StreamDeck OS
tty1::respawn:/usr/bin/streamdeckd console
::respawn:/usr/sbin/streamdeckd-service
EOT
