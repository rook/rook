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

function prepare_node() {
  sudo rm -rf /var/lib/rook/rook-integration-test
  sudo mkdir -p /var/lib/rook/rook-integration-test/monitoring-stack
  node_name=$(kubectl get nodes -o jsonpath='{.items[*].metadata.name}' | cut -d " " -f1)
  kubectl label nodes "${node_name}" rook.io/monitoring-stack=true
  kubectl delete pv -l type=local
}

function create_monitoring_stack_sc() {
cat <<eof | kubectl apply -f -
---
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: ceph-storage
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
eof
}

function create_monitoring_stack_pv() {
cat <<eof | kubectl apply -f -
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-pv-grafana
  labels:
    app: grafana
    type: local
spec:
  capacity:
    storage: 1Gi
  storageClassName: ceph-storage
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  local:
    path: /var/lib/rook/rook-integration-test/monitoring-stack
  nodeAffinity:
    required:
      nodeSelectorTerms:
        - matchExpressions:
            - key: rook.io/monitoring-stack
              operator: In
              values:
                        - "true"
eof
}

########
# MAIN #
########
prepare_node
create_monitoring_stack_sc
create_monitoring_stack_pv

kubectl get pv -o wide