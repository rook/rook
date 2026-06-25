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
# The argument is a device path (/dev/...); create a single volumeMode: Block PV
# backed by that whole device, for use as an extra OSD by a PVC-based cluster.
device=$1
sudo lsblk

#############
# FUNCTIONS #
#############

# add_block_dev_pvc <device-path> <name-suffix>
# Create one volumeMode: Block PersistentVolume backed by the given device.
function add_block_dev_pvc() {
  local path=$1
  local name=$2
  local storage=6Gi

  cat <<eof | kubectl apply -f -
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-vol-$name
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: "$storage"
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  volumeMode: Block
  local:
    path: "$path"
  nodeAffinity:
      required:
        nodeSelectorTerms:
          - matchExpressions:
              - key: rook.io/has-disk
                operator: In
                values:
                - "true"
eof
}

########
# MAIN #
########
add_block_dev_pvc "$device" "$(basename "$device")"

kubectl get pv -o wide
