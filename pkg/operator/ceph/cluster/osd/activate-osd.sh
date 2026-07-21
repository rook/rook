#!/usr/bin/env bash
set -o errexit
set -o pipefail
set -o nounset # fail if variables are unset
set -o xtrace

OSD_ID="$ROOK_OSD_ID"
OSD_UUID="$ROOK_OSD_UUID"
OSD_STORE_FLAG="$ROOK_OSD_STORE_FLAG"
OSD_DATA_DIR=/var/lib/ceph/osd/ceph-"$OSD_ID"
CV_MODE="$ROOK_CV_MODE"
DEVICE="$ROOK_BLOCK_PATH"
ENCRYPTED="$ROOK_ENCRYPTED_DEVICE"

# copy the latest lockbox keys to the keyring file as they might have been rotated
if [ "$ENCRYPTED" == "true" ] ; then
	LOCKBOX_KEYRING_FILE="$OSD_DATA_DIR"/lockbox.keyring
	LOCKBOX_USER=client.osd-lockbox."$OSD_UUID"

	if ! ceph --name client.admin auth get-or-create "$LOCKBOX_USER" \
			mon 'allow command "config-key get" with key="dm-crypt/osd/'$OSD_UUID'/luks"' \
			--keyring /etc/ceph/admin-keyring-store/keyring > /tmp/lockbox.keyring; then
		echo "failed to get latest cephx lockbox key for OSD. continuing OSD startup using on-disk key" >/dev/stderr
		# allowing OSD to attempt to start could avoid full OSD outage due to mon/system issues
	else
		echo "got latest cephx lockbox key for OSD successfully. updating on-disk key" >/dev/stderr
		mv /tmp/lockbox.keyring "$LOCKBOX_KEYRING_FILE"
	fi
fi

# active the osd with ceph-volume
if [[ "$CV_MODE" == "lvm" ]]; then
	TMP_DIR=$(mktemp -d)

	# prevent LVM from trying to scan RBD volumes that may be unable to serve reads without this OSD up
	echo 'devices { filter = ["r|/dev/rbd.*|"] }' >> /etc/lvm/lvm.conf

	# activate osd
	ceph-volume lvm activate --no-systemd "$OSD_STORE_FLAG" "$OSD_ID" "$OSD_UUID"

	# For encrypted LVM OSDs, resize the full stack: PV → LV → LUKS.
	# All operations are no-ops if the underlying device hasn't grown.
	if [ "$ENCRYPTED" == "true" ]; then
		VG_NAME=$(dirname "$DEVICE" | xargs basename)
		pvresize "$(pvs --noheadings -o pv_name -S vg_name="$VG_NAME" 2>/dev/null | tr -d ' ')"
		# With osd_per_device > 1, multiple LVs share the same VG. Absolute target
		# (total/count) divides space equally and is a no-op on restarts.
		OSD_COUNT=$(vgs --noheadings --nosuffix -o lv_count "$VG_NAME" 2>/dev/null | tr -d ' ')
		TOTAL_EXTENTS=$(vgs --noheadings --nosuffix -o vg_extent_count "$VG_NAME" 2>/dev/null | tr -d ' ')
		lvextend -l "$((TOTAL_EXTENTS / OSD_COUNT))" "$DEVICE" || true

		DM_NAME=$(basename "$(readlink "$OSD_DATA_DIR/block")")
		KEY_FILE=$(mktemp /tmp/.luks_key.XXXXXX)
		trap 'rm -f "$KEY_FILE"' EXIT
		ceph --cluster ceph --name "$LOCKBOX_USER" \
			--keyring "$LOCKBOX_KEYRING_FILE" \
			config-key get "dm-crypt/osd/$OSD_UUID/luks" > "$KEY_FILE"
		cryptsetup --verbose resize "$DM_NAME" --key-file "$KEY_FILE"
	fi

	# copy the tmpfs directory to a temporary directory
	# this is needed because when the init container exits, the tmpfs goes away and its content with it
	# this will result in the emptydir to be empty when accessed by the main osd container
	cp --verbose --no-dereference "$OSD_DATA_DIR"/* "$TMP_DIR"/

	# unmount the tmpfs since we don't need it anymore
	umount "$OSD_DATA_DIR"

	# copy back the content of the tmpfs into the original osd directory
	cp --verbose --no-dereference "$TMP_DIR"/* "$OSD_DATA_DIR"

	# retain ownership of files to the ceph user/group
	chown --verbose --recursive ceph:ceph "$OSD_DATA_DIR"

	# remove the temporary directory
	rm --recursive --force "$TMP_DIR"
else
	# 'ceph-volume raw list' (which the osd-prepare job uses to report OSDs on nodes)
	#  returns user-friendly device names which can change when systems reboot. To
	# keep OSD pods from crashing repeatedly after a reboot, we need to check if the
	# block device we have is still correct, and if it isn't correct, we need to
	# scan all the disks to find the right one.
	OSD_LIST="$(mktemp)"

	function find_device() {
		# jq would be preferable, but might be removed for hardened Ceph images
		# python3 should exist in all containers having Ceph
		python3 -c "
import sys, json
for _, info in json.load(sys.stdin).items():
	if info['osd_id'] == $OSD_ID:
		print(info['device'], end='')
		print('found device: ' + info['device'], file=sys.stderr) # log the disk we found to stderr
		sys.exit(0)  # don't keep processing once the disk is found
sys.exit('no disk found with OSD ID $OSD_ID')
"
	}

	if ! ceph-volume raw list "$DEVICE" > "$OSD_LIST"; then
		# if the command fails, the disk may be renamed
		echo '' > "$OSD_LIST"
	fi
	cat "$OSD_LIST"

	if ! find_device < "$OSD_LIST"; then
		# The disk may have been renamed, so scan disks to find the right one. Build the
		# scan list explicitly instead of letting a bare 'ceph-volume raw list' scan every
		# block device: opening an rbd device can block in uninterruptible sleep while this
		# OSD is down (the same deadlock the lvm filter above prevents for lvm-mode OSDs).
		# Skip rbd, nbd, and drbd (network-backed devices that can hang when their backing
		# storage is unavailable) and zram (volatile RAM); Rook never provisions OSDs on any
		# of these. loop devices are intentionally kept, since OSDs on loop devices are
		# supported for CI and local testing.
		SCAN_DEVICES="$(lsblk --noheadings --paths --list --output NAME,TYPE | awk '$2 == "disk" || $2 == "part" {print $1}' | grep -vE '^/dev/(rbd|nbd|zram|drbd)' || true)"
		[[ -z "$SCAN_DEVICES" ]] && { echo "no devices to scan for OSD $OSD_ID" ; exit 1 ; }
		# shellcheck disable=SC2086 # word splitting of the device list is intended
		ceph-volume raw list $SCAN_DEVICES > "$OSD_LIST"
		cat "$OSD_LIST"

		DEVICE="$(find_device < "$OSD_LIST")"
	fi
	[[ -z "$DEVICE" ]] && { echo "no device" ; exit 1 ; }

	# If a kernel device name change happens and a block device file
	# in the OSD directory becomes missing, this OSD fails to start
	# continuously. This problem can be resolved by confirming
	# the validity of the device file and recreating it if necessary.
	OSD_BLOCK_PATH=/var/lib/ceph/osd/ceph-$OSD_ID/block
	if [ -L "$OSD_BLOCK_PATH" ] && [ "$(readlink "$OSD_BLOCK_PATH")" != "$DEVICE" ] ; then
		rm $OSD_BLOCK_PATH
	fi

	# ceph-volume raw mode only supports bluestore so we don't need to pass a store flag
	ceph-volume raw activate --device "$DEVICE" --no-systemd --no-tmpfs
fi
