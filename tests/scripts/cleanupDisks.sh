#!/bin/bash

test_scratch_device=/dev/nvme0n1
test_scratch_device2=/dev/nvme1n1
if [ $# -ge 1 ] ; then
  test_scratch_device=$1
fi
if [ $# -ge 2 ] ; then
  test_scratch_device2=$2
fi

sudo dd if=/dev/zero of=${test_scratch_device} bs=1M count=100 oflag=direct
sudo dd if=/dev/zero of=${test_scratch_device2} bs=1M count=100 oflag=direct
