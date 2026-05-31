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
osd_count=2
db_device=$2
wal_device=$3
sudo lsblk
sudo test ! -b "${test_scratch_device}" && echo "invalid scratch device, not a block device: ${test_scratch_device}" >&2 && exit 1

#############
# FUNCTIONS #
#############

function prepare_node() {
  sudo rm -rf /var/lib/rook/rook-integration-test
  sudo mkdir -p /var/lib/rook/rook-integration-test/mon1 /var/lib/rook/rook-integration-test/mon2 /var/lib/rook/rook-integration-test/mon3
  node_name=$(kubectl get nodes -o jsonpath='{.items[*].metadata.name}')
  kubectl label nodes "${node_name}" rook.io/has-disk=true
  kubectl delete pv -l type=local
}

function create_mon_pvc() {
cat <<eof | kubectl apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-vol1
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 5Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  volumeMode: Filesystem
  local:
    path: "/var/lib/rook/rook-integration-test/mon1"
  nodeAffinity:
      required:
        nodeSelectorTerms:
          - matchExpressions:
              - key: rook.io/has-disk
                operator: In
                values:
                - "true"
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-vol2
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 5Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  volumeMode: Filesystem
  local:
    path: "/var/lib/rook/rook-integration-test/mon2"
  nodeAffinity:
      required:
        nodeSelectorTerms:
          - matchExpressions:
              - key: rook.io/has-disk
                operator: In
                values:
                - "true"
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-vol3
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 5Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  volumeMode: Filesystem
  local:
    path: "/var/lib/rook/rook-integration-test/mon3"
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

function create_create_sc() {
cat <<eof | kubectl apply -f -
---
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: manual
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
eof
}

function add_db_pvc {
cat <<eof | kubectl apply -f -
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-vol8
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 2Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  volumeMode: Block
  local:
    path: "${db_device}"
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

function add_wal_pvc {
cat <<eof | kubectl apply -f -
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-vol9
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 2Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  volumeMode: Block
  local:
    path: "${wal_device}"
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
prepare_node
create_mon_pvc


# Add a db device if needed
if [ -n "$db_device" ]; then
  osd_count=1
  add_db_pvc
fi

# Add a wal device if needed
if [ -n "$wal_device" ]; then
  osd_count=1
  add_wal_pvc
fi

# For the LVM scenario
scratch_dev_type=$(lsblk --noheadings --output TYPE "$test_scratch_device")
if [[ "$scratch_dev_type" == "lvm" ]]; then
  osd_count=1
fi

# For the ceph_integration suite
# It's an env var set by the gitaction action when running TestCephMultiClusterDeploySuite not a misspell
# shellcheck disable=SC2153
if [[ -n "$TEST_SCRATCH_DEVICE" ]]; then
  osd_count=1
fi

create_osd_pvc "$osd_count"
create_create_sc

kubectl get pv -o wide
