#!/usr/bin/env bash
set -xe

CEPH_FSID="$ROOK_CEPH_FSID"
PVC_NAME="$ROOK_PVC_NAME"
KEY_FILE_PATH="$ROOK_ENCRYPTION_KEY_FILE_PATH"
BLOCK_PATH="$ROOK_ENCRYPTION_BLOCK_PATH"
DM_NAME="$ROOK_ENCRYPTION_DM_NAME"
DM_PATH="$ROOK_ENCRYPTION_DM_PATH"

# Helps debugging
dmsetup version

function open_encrypted_block {
	echo "Opening encrypted device $BLOCK_PATH at $DM_PATH"
	cryptsetup luksOpen --verbose --disable-keyring --allow-discards --key-file "$KEY_FILE_PATH" "$BLOCK_PATH" "$DM_NAME"
}

# This is done for upgraded clusters that did not have the subsystem and label set by the prepare job
function set_luks_subsystem_and_label {
	echo "setting LUKS label and subsystem"
	cryptsetup config $BLOCK_PATH --subsystem ceph_fsid="$CEPH_FSID" --label pvc_name="$PVC_NAME"
}

if [ -b "$DM_PATH" ]; then
	echo "Encrypted device $BLOCK_PATH already opened at $DM_PATH"
	for field in $(dmsetup table "$DM_NAME"); do
		if [[ "$field" =~ ^[0-9]+\:[0-9]+ ]]; then
			underlaying_block="/sys/dev/block/$field"
			if [ ! -d "$underlaying_block" ]; then
				echo "Underlying block device $underlaying_block of crypt $DM_NAME disappeared!"
				echo "Removing stale dm device $DM_NAME"
				dmsetup remove --force "$DM_NAME"
				open_encrypted_block
			fi
		fi
	done
else
	open_encrypted_block
fi

# Setting label and subsystem on LUKS1 is not supported and the command will fail
if cryptsetup luksDump $BLOCK_PATH|grep -qEs "Version:.*2"; then
	set_luks_subsystem_and_label
else
	echo "LUKS version is not 2 so not setting label and subsystem"
fi
