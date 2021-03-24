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
FSSIZE=1024M

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

########
# MAIN #
########
if [ "$#" -ne 1 ]; then
  exit 1
fi

DISK="$1"

# First wipe the disk
wipe_disk

# Some github actions runners seem to delete and re-create block devices
# if there is no mounted filesystem on it. It causes test failure.
# So let's bypass this problem by mounting a fake filesystem.
# For more details, see the following PR.
# https://github.com/rook/rook/pull/7404
sudo sgdisk --new=0:0:+"$FSSIZE" -- "$DISK"
FAKEFSPART=${DISK}1
sudo mkfs.ext4 ${FAKEFSPART}
sudo mount ${FAKEFSPART} /mnt

# Create a partition for OSD
sudo sgdisk --largest-new=0 -- "$DISK"

# Inform the kernel of partition table changes
sudo partprobe "$DISK"

# Wait the udev event queue, and exits if all current events are handled.
sudo udevadm settle

# Print drives
sudo lsblk
sudo parted "$DISK" -s print
