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

SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
CONTROLLER_GEN_BIN_PATH=$1
YQ_BIN_PATH=$2
: "${MAX_DESC_LEN:=100}"
# allowDangerousTypes is used to accept float64
CRD_OPTIONS="crd:maxDescLen=$MAX_DESC_LEN,generateEmbeddedObjectMeta=true,allowDangerousTypes=true"

DESTINATION_ROOT="$SCRIPT_ROOT"
if [[ -n "$BUILD_CRDS_INTO_DIR" ]]; then
  echo "Generating CRDs into dir $BUILD_CRDS_INTO_DIR"
  DESTINATION_ROOT="$BUILD_CRDS_INTO_DIR"
fi
OLM_CATALOG_DIR="${DESTINATION_ROOT}/deploy/olm/deploy/crds"
CEPH_CRDS_FILE_PATH="${DESTINATION_ROOT}/deploy/examples/crds.yaml"
CEPH_HELM_CRDS_FILE_PATH="${DESTINATION_ROOT}/deploy/charts/rook-ceph/templates/resources.yaml"

if [[ "$($YQ_BIN_PATH --version)" != "yq (https://github.com/mikefarah/yq/) version 4."* ]]; then
  echo "yq must be version 4.x"
  exit 1
fi

#############
# FUNCTIONS #
#############

copy_ob_obc_crds() {
  mkdir -p "$OLM_CATALOG_DIR"
  cp -f "${SCRIPT_ROOT}/deploy/olm/assemble/objectbucket.io_objectbucketclaims.yaml" "$OLM_CATALOG_DIR"
  cp -f "${SCRIPT_ROOT}/deploy/olm/assemble/objectbucket.io_objectbuckets.yaml" "$OLM_CATALOG_DIR"
}

generating_crds_v1() {
  echo "Generating ceph crds"
  "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./pkg/apis/ceph.rook.io/v1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
  # the csv upgrade is failing on the volumeClaimTemplate.metadata.annotations.crushDeviceClass unless we preserve the annotations as an unknown field
  $YQ_BIN_PATH eval --inplace '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.storage.properties.storageClassDeviceSets.items.properties.volumeClaimTemplates.items.properties.metadata.properties.annotations.x-kubernetes-preserve-unknown-fields = true' "${OLM_CATALOG_DIR}"/ceph.rook.io_cephclusters.yaml
}

generating_main_crd() {
  true >"$CEPH_CRDS_FILE_PATH"
  true >"$CEPH_HELM_CRDS_FILE_PATH"
  cat <<EOF >"$CEPH_CRDS_FILE_PATH"
##############################################################################
# Create the CRDs that are necessary before creating your Rook cluster.
# These resources *must* be created before the cluster.yaml or their variants.
##############################################################################
EOF
}

build_helm_resources() {
  TMP_FILE=$(mktemp -q /tmp/resources.XXXXXX || exit 1)
  echo "Generating helm resources.yaml to temp file: $TMP_FILE"
  {
    # add header
    echo "{{- if .Values.crds.enabled }}"

    # Add helm annotations to all CRDS, remove empty lines in the output
    # skip the comment lines of crds.yaml as well as the yaml doc header
    "$YQ_BIN_PATH" eval-all '.metadata.annotations["helm.sh/resource-policy"] = "keep"' "$CEPH_CRDS_FILE_PATH" | tail -n +6

    # DO NOT REMOVE the empty line, it is necessary
    echo ""
    echo "{{- end }}"
  } >>"$TMP_FILE"
  echo "updating helm crds file $CEPH_HELM_CRDS_FILE_PATH from temp file"
  mv "$TMP_FILE" "$CEPH_HELM_CRDS_FILE_PATH"
}

########
# MAIN #
########
# clean the directory where CRDs are generated
rm -fr "$OLM_CATALOG_DIR"

# generate the CRDs
generating_crds_v1

# get the OBC CRDs
if [ -z "$NO_OB_OBC_VOL_GEN" ]; then
  echo "Generating obcs in crds.yaml"
  copy_ob_obc_crds
fi

generating_main_crd

CRD_FILES=()
while read -r line; do
  CRD_FILES+=("$line")
done < <(find "$OLM_CATALOG_DIR" -type f -name '*.yaml' | sort)

echo "---" >>"$CEPH_CRDS_FILE_PATH" # yq doesn't output the first doc separator
$YQ_BIN_PATH eval-all '.' "${CRD_FILES[@]}" >>"$CEPH_CRDS_FILE_PATH"

build_helm_resources
