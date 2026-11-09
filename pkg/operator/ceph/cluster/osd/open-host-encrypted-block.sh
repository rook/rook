#!/usr/bin/env bash

# Copyright 2026 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Open the LUKS device of an encrypted host-based OSD with its key from the KMS, so that
# "ceph-volume raw activate" finds the OSD. ceph-volume cannot open it: it finds OSDs by their
# BlueStore label, which is only readable once the device is open.

set -o errexit
set -o pipefail
set -o nounset

DM_PATH="$ROOK_BLOCK_PATH"
DM_NAME="$(basename "$DM_PATH")"
KEY_FILE="$ROOK_ENCRYPTION_KEY_DIR/luks_key"

if [ -b "$DM_PATH" ]; then
	echo "$DM_PATH is already open"
	exit 0
fi

# the prepare job opens a closed OSD under the kernel name the device has now, which may differ from
# the one in the name the OSD was created with
for mapper in /dev/mapper/ceph-"$ROOK_OSD_UUID"-*-block-dmcrypt; do
	if [ -b "$mapper" ]; then
		# no udev in the container to wait for, so create the device node directly
		dmsetup rename --noudevsync "$(basename "$mapper")" "$DM_NAME"
		dmsetup mknodes "$DM_NAME"
		if [ ! -b "$DM_PATH" ]; then
			echo "failed to rename $mapper to $DM_PATH" >&2
			exit 1
		fi
		echo "renamed $mapper to $DM_PATH"
		exit 0
	fi
done

# ceph-volume records the OSD fsid in the LUKS2 subsystem, so the device is found even if its
# kernel name changed since prepare
DEVICE=""
for dev in $(lsblk --noheadings --paths --list --output NAME,TYPE | awk '$2 == "disk" || $2 == "part" || $2 == "loop" || $2 == "mpath" {print $1}'); do
	# a path of a multipath device shows the same header, but must be opened through the mpath
	if [ -n "$(ls "/sys/class/block/$(basename "$dev")/holders" 2>/dev/null)" ]; then
		continue
	fi
	if cryptsetup luksDump "$dev" 2>/dev/null | grep -q "ceph_fsid=$ROOK_OSD_UUID"; then
		DEVICE="$dev"
		break
	fi
done
if [ -z "$DEVICE" ]; then
	echo "no LUKS device found for OSD $ROOK_OSD_UUID" >&2
	exit 1
fi

trap 'rm -f "$KEY_FILE"' EXIT
rook key-management get "$ROOK_OSD_UUID" "$KEY_FILE"
cryptsetup luksOpen --verbose --disable-keyring --allow-discards --key-file "$KEY_FILE" "$DEVICE" "$DM_NAME"
echo "opened $DEVICE as $DM_PATH"
