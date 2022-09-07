#!/usr/bin/env bash

# Copyright 2021 The Rook Authors. All rights reserved.
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

set -ex

#############
# VARIABLES #
#############
SIZE=2048M

#############
# FUNCTIONS #
#############

function usage {
  echo "Use me like this to create bluestore partitions:"
  echo "$0 --disk /dev/sda --bluestore-type block.db"
  echo ""
  echo "Use me like this to create multiple OSDs:"
  echo "$0 --disk /dev/sda --osd-count 2"
  echo ""
}

function wipe_disk {
  sudo sgdisk --zap-all -- "$DISK"
  sudo dd if=/dev/zero of="$DISK" bs=1M count=10
  if [ -n "$WIPE_ONLY" ]; then
    # `parted "$DISK -s print" exits with 1 if the partition label doesn't exist.
    # It's no problem in "--wipe-only" mode
    sudo parted "$DISK" -s print || :
    return
  fi
  set +e
  sudo parted -s "$DISK" mklabel gpt
  set -e
  sudo partprobe -d "$DISK"
  sudo udevadm settle
  sudo parted "$DISK" -s print
}

function create_partition {
  sudo sgdisk --new=0:0:+"$SIZE" --change-name=0:"$1" --mbrtogpt -- "$DISK"
}

function create_block_partition {
  local osd_count=$1
  if [ "$osd_count" -eq 1 ]; then
    sudo sgdisk --largest-new=0 --change-name=0:'block' --mbrtogpt -- "$DISK"
  elif [ "$osd_count" -gt 1 ]; then
    SIZE=6144M
    for osd in $(seq 1 "$osd_count"); do
      echo "$osd"
      create_partition osd-"$osd"
      echo "SUBSYSTEM==\"block\", ATTR{size}==\"12582912\", ATTR{partition}==\"$osd\", ACTION==\"add\", RUN+=\"/bin/chown 167:167 ${DISK}${osd}\"" | sudo tee -a /etc/udev/rules.d/01-rook-"$osd".rules
    done
  fi
}

########
# MAIN #
########
if [ ! "$#" -ge 1 ]; then
  exit 1
fi

while [ "$1" != "" ]; do
  case $1 in
  --disk)
    shift
    DISK="$1"
    ;;
  --bluestore-type)
    shift
    BLUESTORE_TYPE="$1"
    ;;
  --osd-count)
    shift
    OSD_COUNT="$1"
    ;;
  --wipe-only)
    WIPE_ONLY=1
    ;;
  -h | --help)
    usage
    exit
    ;;
  *)
    usage
    exit 1
    ;;
  esac
  shift
done

# First wipe the disk
wipe_disk

if [ -n "$WIPE_ONLY" ]; then
  exit
fi

if [ -z "$WIPE_ONLY" ]; then
  if [ -n "$BLUESTORE_TYPE" ]; then
    case "$BLUESTORE_TYPE" in
    block.db)
      create_partition block.db
      ;;
    block.wal)
      create_partition block.db
      create_partition block.wal
      ;;
    *)
      printf "invalid bluestore configuration %q" "$BLUESTORE_TYPE" >&2
      exit 1
      ;;
    esac
  fi

  # Create final block partitions
  create_block_partition "$OSD_COUNT"
fi

# Inform the kernel of partition table changes
sudo partprobe "$DISK"

# Wait the udev event queue, and exits if all current events are handled.
sudo udevadm settle

# Print drives
sudo lsblk
sudo parted "$DISK" -s print
