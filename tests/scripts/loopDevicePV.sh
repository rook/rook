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
osd_count=$1
sudo lsblk

#############
# FUNCTIONS #
#############

function add_loop_dev_pvc() {
  local osd_count=$1
  local storage=6Gi

  for osd in $(seq 1 "$osd_count"); do
    path=/dev/loop${osd}

    cat <<eof | kubectl apply -f -
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-vol-loop-dev$osd
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
add_loop_dev_pvc "$osd_count"

kubectl get pv -o wide
