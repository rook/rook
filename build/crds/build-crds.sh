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

set -o errexit
set -o pipefail

# set BUILD_CRDS_INTO_DIR to build the CRD results into the given dir instead of in-place
: "${BUILD_CRDS_INTO_DIR:=}"

SCRIPT_ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )/../.." && pwd -P)
CONTROLLER_GEN_BIN_PATH=$1
YQ_BIN_PATH=$2
: "${MAX_DESC_LEN:=-1}"
# allowDangerousTypes is used to accept float64
CRD_OPTIONS="crd:maxDescLen=$MAX_DESC_LEN,trivialVersions=true,generateEmbeddedObjectMeta=true,allowDangerousTypes=true"

DESTINATION_ROOT="$SCRIPT_ROOT"
if [[ -n "$BUILD_CRDS_INTO_DIR" ]]; then
  echo "Generating CRDs into dir $BUILD_CRDS_INTO_DIR"
  DESTINATION_ROOT="$BUILD_CRDS_INTO_DIR"
fi
OLM_CATALOG_DIR="${DESTINATION_ROOT}/cluster/olm/ceph/deploy/crds"
CEPH_CRDS_FILE_PATH="${DESTINATION_ROOT}/cluster/examples/kubernetes/ceph/crds.yaml"
CEPH_HELM_CRDS_FILE_PATH="${DESTINATION_ROOT}/cluster/charts/rook-ceph/templates/resources.yaml"

#############
# FUNCTIONS #
#############

copy_ob_obc_crds() {
  mkdir -p "$OLM_CATALOG_DIR"
  cp -f "${SCRIPT_ROOT}/cluster/olm/ceph/assemble/objectbucket.io_objectbucketclaims.yaml" "$OLM_CATALOG_DIR"
  cp -f "${SCRIPT_ROOT}/cluster/olm/ceph/assemble/objectbucket.io_objectbuckets.yaml" "$OLM_CATALOG_DIR"
}

generating_crds_v1() {
  echo "Generating ceph crds"
  "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./pkg/apis/ceph.rook.io/v1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
  # the csv upgrade is failing on the volumeClaimTemplate.metadata.annotations.crushDeviceClass unless we preserve the annotations as an unknown field
  $YQ_BIN_PATH w -i "${OLM_CATALOG_DIR}"/ceph.rook.io_cephclusters.yaml spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.storage.properties.storageClassDeviceSets.items.properties.volumeClaimTemplates.items.properties.metadata.properties.annotations.x-kubernetes-preserve-unknown-fields true
}

generate_vol_rep_crds() {
  echo "Generating volume replication crds in crds.yaml"
  "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="github.com/csi-addons/volume-replication-operator/api/v1alpha1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
}

generating_main_crd() {
  true > "$CEPH_CRDS_FILE_PATH"
  true > "$CEPH_HELM_CRDS_FILE_PATH"
cat <<EOF > "$CEPH_CRDS_FILE_PATH"
##############################################################################
# Create the CRDs that are necessary before creating your Rook cluster.
# These resources *must* be created before the cluster.yaml or their variants.
##############################################################################
EOF
}

build_helm_resources() {
  echo "Generating helm resources.yaml"
  {
    # add header
    echo "{{- if .Values.crds.enabled }}"

    # Add helm annotations to all CRDS and skip the first 4 lines of crds.yaml
    "$YQ_BIN_PATH" w -d'*' "$CEPH_CRDS_FILE_PATH" "metadata.annotations[helm.sh/resource-policy]" keep | tail -n +5

    # DO NOT REMOVE the empty line, it is necessary
    echo ""
    echo "{{- end }}"
  } >>"$CEPH_HELM_CRDS_FILE_PATH"
}

########
# MAIN #
########
generating_crds_v1

if [ -z "$NO_OB_OBC_VOL_GEN" ]; then
  echo "Generating obcs in crds.yaml"
  copy_ob_obc_crds
fi

generate_vol_rep_crds

generating_main_crd

for crd in "$OLM_CATALOG_DIR/"*.yaml; do
  echo "---" >> "$CEPH_CRDS_FILE_PATH" # yq doesn't output doc separators
  # Process each intermediate CRD file with yq to enforce consistent formatting in the final product
  # regardless of whether yq was used in previous steps to alter CRD intermediate files.
  $YQ_BIN_PATH read "$crd" >> "$CEPH_CRDS_FILE_PATH"
done

build_helm_resources
