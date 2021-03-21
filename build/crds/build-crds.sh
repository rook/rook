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
set -o nounset
set -o pipefail

SCRIPT_ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )/../.." && pwd -P)
CONTROLLER_GEN_BIN_PATH=$1
# allowDangerousTypes is used to accept float64
CRD_OPTIONS="crd:trivialVersions=true,allowDangerousTypes=true"
OLM_CATALOG_DIR="${SCRIPT_ROOT}/cluster/olm/ceph/deploy/crds"
CRDS_FILE_PATH="${SCRIPT_ROOT}/cluster/examples/kubernetes/ceph/crds.yaml"
HELM_CRDS_FILE_PATH="${SCRIPT_ROOT}/cluster/charts/rook-ceph/templates/resources.yaml"
CRDS_BEFORE_1_16_FILE_PATH="${SCRIPT_ROOT}/cluster/examples/kubernetes/ceph/pre-k8s-1.16/crds.yaml"

#############
# FUNCTIONS #
#############
# TODO: revisit later
# ensures the vendor dir has the right deps, e,g. code-generator
# if [ ! -d vendor/github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1/ ];then
#   echo "Vendoring project"
#   go mod vendor
# fi

copy_ob_obc_crds() {
  mkdir -p "$OLM_CATALOG_DIR"
  cp -f "${SCRIPT_ROOT}/cluster/olm/ceph/assemble/objectbucket.io_objectbucketclaims.yaml" "$OLM_CATALOG_DIR"
  cp -f "${SCRIPT_ROOT}/cluster/olm/ceph/assemble/objectbucket.io_objectbuckets.yaml" "$OLM_CATALOG_DIR"
}

generating_crds() {
  echo "Generating crds.yaml"
  "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./pkg/apis/ceph.rook.io/v1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
  "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./pkg/apis/rook.io/v1alpha2" output:crd:artifacts:config="$OLM_CATALOG_DIR"
  # TODO: revisit later
  # * remove copy_ob_obc_crds()
  # * remove files cluster/olm/ceph/assemble/{objectbucket.io_objectbucketclaims.yaml,objectbucket.io_objectbuckets.yaml}
  # Activate code below
  # "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./vendor/github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
}

generating_main_crd() {
  true > "$CRDS_FILE_PATH"
  true > "$HELM_CRDS_FILE_PATH"
cat <<EOF > "$CRDS_FILE_PATH"
##############################################################################
# Create the CRDs that are necessary before creating your Rook cluster.
# These resources *must* be created before the cluster.yaml or their variants.
##############################################################################
EOF
}

build_helm_resources() {
  # Add helm annotations to all CRDS
  yq w -i -d'*' "$HELM_CRDS_FILE_PATH" "metadata.annotations[helm.sh/resource-policy]" keep

  # add header
  sed -i '1s/^/{{- if semverCompare ">=1.16.0" .Capabilities.KubeVersion.GitVersion }}\n/' "$HELM_CRDS_FILE_PATH"
  sed -i '1s/^/{{- if .Values.crds.enabled }}\n/' "$HELM_CRDS_FILE_PATH"

  echo "Generating helm resources.yaml"
  {
    # add else
    echo "{{- else }}"

    # add footer
    cat "$CRDS_BEFORE_1_16_FILE_PATH"
    # DO NOT REMOVE the empty line, it is necessary
    echo ""
    echo "{{- end }}"
    echo "{{- end }}"
  } >>"$HELM_CRDS_FILE_PATH"
}

########
# MAIN #
########
copy_ob_obc_crds
generating_crds
generating_main_crd

for crd in "$OLM_CATALOG_DIR/"*.yaml; do
  cat "$crd" >> "$CRDS_FILE_PATH"
  cat "$crd" >> "$HELM_CRDS_FILE_PATH"
done

build_helm_resources
