#!/usr/bin/env bash
set -o errexit
set -o pipefail

# Close any dm-crypt LUKS mapping, wipe the backing block device, and remove the LSO symlink.

if [ "$ENCRYPTED" = "true" ] && [ -n "$DM_NAME" ]; then
	if cryptsetup status "$DM_NAME" > /dev/null 2>&1; then
		cryptsetup close "$DM_NAME"
	fi
fi

if [ -L "$SYMLINK_PATH" ]; then
	# Verify the symlink goes through a stable device ID path (WWN).
	# If LSO was configured with unstable device names (e.g. /dev/sdb), that name
	# could point to a different physical disk after a reboot. Refusing here prevents
	# wiping the wrong disk in that case.
	LINK_TARGET=$(readlink "$SYMLINK_PATH")
	if [[ "$LINK_TARGET" != /dev/disk/by-id/* ]]; then
		echo "ERROR: symlink target '$LINK_TARGET' does not go through a stable device ID path"
		echo "Cannot confirm this is the correct disk. Refusing to wipe."
		exit 1
	fi

	REAL_DEVICE=$(readlink -f "$SYMLINK_PATH")
	wipefs -a "$REAL_DEVICE"
	rm -f "$SYMLINK_PATH"
elif [ -n "$SYMLINK_PATH" ] && [ -b "$SYMLINK_PATH" ]; then
	wipefs -a "$SYMLINK_PATH"
fi
