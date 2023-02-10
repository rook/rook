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

test_scratch_device=/dev/nvme0n1
if [ $# -ge 1 ] ; then
  test_scratch_device=$1
fi

#############
# VARIABLES #
#############
osd_count=1
sudo lsblk
sudo test ! -b "${test_scratch_device}" && echo "invalid scratch device, not a block device: ${test_scratch_device}" >&2 && exit 1

#############
# FUNCTIONS #
#############

function create_osd_pvc() {
  local osd_count=$1
  local storage=6Gi

  for osd in $(seq 1 "$osd_count"); do
    path=${test_scratch_device}${osd}
    if [ "$osd_count" -eq 1 ]; then
      path=$test_scratch_device
      storage=10Gi
    fi

    cat <<eof | kubectl apply -f -
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-vol$((4 + osd))
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
  done
}

########
# MAIN #
########

create_osd_pvc "$osd_count"

kubectl get pv -o wide
