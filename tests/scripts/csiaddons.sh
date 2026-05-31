#!/usr/bin/env bash

# Copyright 2024 The Rook Authors. All rights reserved.
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

set -xEo pipefail

CSIADDONS_VERSION="v0.14.0"
CSIADDONS_CRD_NAME="csiaddonsnodes.csiaddons.openshift.io"
CSIADDONS_CONTAINER_NAME="csi-addons"

function setup_csiaddons() {
  echo "setting up csi-addons"

  echo "deploying controller"
  kubectl create -f https://github.com/csi-addons/kubernetes-csi-addons/releases/download/$CSIADDONS_VERSION/crds.yaml
  kubectl create -f https://github.com/csi-addons/kubernetes-csi-addons/releases/download/$CSIADDONS_VERSION/rbac.yaml
  kubectl create -f https://github.com/csi-addons/kubernetes-csi-addons/releases/download/$CSIADDONS_VERSION/setup-controller.yaml

  echo "enabling csi-addons"
  kubectl patch cm rook-ceph-operator-config -n rook-ceph --type merge -p '{"data":{"CSI_ENABLE_CSIADDONS":"true"}}'

  echo "Successfully created CSI-Addons"
}

function verify_crd_created() {

  crds=$(kubectl get crd "$CSIADDONS_CRD_NAME" -o=jsonpath="{.metadata.name}" 2>/dev/null)

  if [ -n "$crds" ]; then
    echo "CRD '$CSIADDONS_CRD_NAME' exists."
  else
    echo "CRD '$CSIADDONS_CRD_NAME' does not exist!"
    exit 1
  fi
}

function verify_container_in_pod_by_label() {
  label=$1
  pod=$(kubectl get pods -n rook-ceph -l "$label" -o=jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null)
  if ! [ -n "$pod" ]; then
    echo "pod with label $label not found!"
    exit 1
  fi

  container_status=$(kubectl get pod -n rook-ceph "$pod" -o=jsonpath="{.status.containerStatuses[?(@.name=='$CSIADDONS_CONTAINER_NAME')].ready}" 2>/dev/null)
  if [ "$container_status" != "true" ]; then
    echo "csi-addons container not found in $pod pod!"
    exit 1
  fi
  echo "$CSIADDONS_CONTAINER_NAME container is running in $pod"
}

function verify_container_is_running() {
  verify_container_in_pod_by_label app=rook-ceph.rbd.csi.ceph.com-ctrlplugin
  verify_container_in_pod_by_label app=rook-ceph.cephfs.csi.ceph.com-ctrlplugin
}

FUNCTION="$1"
shift # remove function arg now that we've recorded it
# call the function with the remainder of the user-provided args
# -e, -E, and -o=pipefail will ensure this script returns a failure if a part of the function fails
$FUNCTION "$@"
