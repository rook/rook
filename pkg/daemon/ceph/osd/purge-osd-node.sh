#!/usr/bin/env bash
set -o errexit
set -o pipefail

# Close any dm-crypt LUKS mapping and wipe the backing block device.
if [ "$ENCRYPTED" = "true" ] && [ -n "$DM_NAME" ]; then
	if cryptsetup status "$DM_NAME" > /dev/null 2>&1; then
		cryptsetup close "$DM_NAME"
	fi
fi

DEVICE_ID=$(basename "$DEVICE_PATH")
if [[ "$DEVICE_ID" =~ ^(sd[a-z]|nvme[0-9]+n[0-9]+|vd[a-z]|hd[a-z])$ ]]; then
	echo "ERROR: Device identifier '$DEVICE_ID' is not a stable ID"
	echo "Plain device names are unstable and may change on reboot. Use a stable identifier (wwn-*, scsi-*, nvme-*) instead."
	echo "Cannot confirm this is the correct disk. Refusing to wipe."
	exit 1
fi

REAL_DEVICE=$(readlink -f "$DEVICE_PATH")

#Wipe filesystem signatures and Ceph metadata at strategic offsets
wipefs -fa "$REAL_DEVICE"

# Zero backup BlueStore labels at 0, 1, 10, 100, and 1000 GiB.
size=$(blockdev --getsize64 "$REAL_DEVICE" 2>/dev/null || echo 0)
for gb in 0 1 10 100 1000; do
	offset=$((gb * 1024 * 1024 * 1024))
	if [ "$size" -gt $((offset + 1048576)) ]; then
		dd if=/dev/zero of="$REAL_DEVICE" bs=1K count=200 oflag=direct,dsync seek=$((gb * 1024 * 1024 / 1)) 2>/dev/null || true
	fi
done
