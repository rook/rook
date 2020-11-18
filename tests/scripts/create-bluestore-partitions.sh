#!/bin/bash
set -ex

#############
# VARIABLES #
#############

: "${BLUESTORE_TYPE:=${2}}"
: "${DISK:=${1}}"
SIZE=2048M

#############
# FUNCTIONS #
#############

function wipe_disk {
    sudo sgdisk --zap-all --clear --mbrtogpt -g -- "$DISK"
    sudo dd if=/dev/zero of="$DISK" bs=1M count=10
    sudo parted -s "$DISK" mklabel gpt
    sudo partprobe "$DISK"
    sudo udevadm settle
    sudo parted "$DISK" -s print
}

function create_partition {
    sudo sgdisk --new=0:0:+"$SIZE" --change-name=0:"$1" --mbrtogpt -- "$DISK"
}

function create_block_partition {
    sudo sgdisk --largest-new=0 --change-name=0:'block' --mbrtogpt -- "$DISK"
}

########
# MAIN #
########

# First wipe the disk
wipe_disk

case "$BLUESTORE_TYPE" in
  block.db)
    create_partition block.db
    ;;
  block.wal)
    create_partition block.db
    create_partition block.wal
    ;;
  *)
    echo "invalid bluestore configuration $BLUESTORE_TYPE" >&2
    exit 1
  esac

# Create final block partitions
create_block_partition

# Inform the kernel of partition table changes
sudo partprobe "$DISK"

# Wait the udev event queue, and exits if all current events are handled.
sudo udevadm settle

# Print drives
sudo lsblk
sudo parted "$DISK" -s print