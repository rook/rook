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

SCRIPT_ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )/../.." && pwd -P)
CONTROLLER_GEN_BIN_PATH=$1
YQ_BIN_PATH=$2
: "${MAX_DESC_LEN:=-1}"
# allowDangerousTypes is used to accept float64
CRD_OPTIONS="crd:maxDescLen=$MAX_DESC_LEN,trivialVersions=true,allowDangerousTypes=true"
OLM_CATALOG_DIR="${SCRIPT_ROOT}/cluster/olm/ceph/deploy/crds"
CRDS_FILE_PATH="${SCRIPT_ROOT}/cluster/examples/kubernetes/ceph/crds.yaml"
HELM_CRDS_FILE_PATH="${SCRIPT_ROOT}/cluster/charts/rook-ceph/templates/resources.yaml"
CRDS_BEFORE_1_16_FILE_PATH="${SCRIPT_ROOT}/cluster/examples/kubernetes/ceph/pre-k8s-1.16/crds.yaml"

#############
# FUNCTIONS #
#############
# ensures the vendor dir has the right deps, e,g. volume replication controller
if [ ! -d vendor/github.com/csi-addons/volume-replication-operator/api/v1alpha1 ];then
  echo "Vendoring project"
  go mod vendor
fi

copy_ob_obc_crds() {
  mkdir -p "$OLM_CATALOG_DIR"
  cp -f "${SCRIPT_ROOT}/cluster/olm/ceph/assemble/objectbucket.io_objectbucketclaims.yaml" "$OLM_CATALOG_DIR"
  cp -f "${SCRIPT_ROOT}/cluster/olm/ceph/assemble/objectbucket.io_objectbuckets.yaml" "$OLM_CATALOG_DIR"
}

generating_crds_v1() {
  echo "Generating v1 in crds.yaml"
  "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./pkg/apis/ceph.rook.io/v1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
  $YQ_BIN_PATH w -i cluster/olm/ceph/deploy/crds/ceph.rook.io_cephclusters.yaml spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.mon.properties.stretchCluster.properties.zones.items.properties.volumeClaimTemplate.properties.metadata.x-kubernetes-preserve-unknown-fields true
  $YQ_BIN_PATH w -i cluster/olm/ceph/deploy/crds/ceph.rook.io_cephclusters.yaml spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.mon.properties.volumeClaimTemplate.properties.metadata.x-kubernetes-preserve-unknown-fields true
  $YQ_BIN_PATH w -i cluster/olm/ceph/deploy/crds/ceph.rook.io_cephclusters.yaml spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.storage.properties.volumeClaimTemplates.items.properties.metadata.x-kubernetes-preserve-unknown-fields true
  $YQ_BIN_PATH w -i cluster/olm/ceph/deploy/crds/ceph.rook.io_cephclusters.yaml spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.storage.properties.nodes.items.properties.volumeClaimTemplates.items.properties.metadata.x-kubernetes-preserve-unknown-fields true
  $YQ_BIN_PATH w -i cluster/olm/ceph/deploy/crds/ceph.rook.io_cephclusters.yaml spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.storage.properties.storageClassDeviceSets.items.properties.volumeClaimTemplates.items.properties.metadata.x-kubernetes-preserve-unknown-fields true
  # fixes a bug in yq: https://github.com/mikefarah/yq/issues/351 where the '---' gets removed
  sed -i'' -e '1i\
---
' cluster/olm/ceph/deploy/crds/ceph.rook.io_cephclusters.yaml
}

generating_crds_v1alpha2() {
  "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./pkg/apis/rook.io/v1alpha2" output:crd:artifacts:config="$OLM_CATALOG_DIR"
  # TODO: revisit later
  # * remove copy_ob_obc_crds()
  # * remove files cluster/olm/ceph/assemble/{objectbucket.io_objectbucketclaims.yaml,objectbucket.io_objectbuckets.yaml}
  # Activate code below
  # "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./vendor/github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
}

generate_vol_rep_crds() {
  echo "Generating volume replication crds in crds.yaml"
  "$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./vendor/github.com/csi-addons/volume-replication-operator/api/v1alpha1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
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
  echo "Generating helm resources.yaml"
  {
    # add header
    echo "{{- if .Values.crds.enabled }}"
    echo "{{- if semverCompare \">=1.16.0\" .Capabilities.KubeVersion.GitVersion }}"
    
    # Add helm annotations to all CRDS and skip the first 4 lines of crds.yaml
    "$YQ_BIN_PATH" w -d'*' "$CRDS_FILE_PATH" "metadata.annotations[helm.sh/resource-policy]" keep | tail -n +5
    
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
generating_crds_v1

if [ -z "$NO_OB_OBC_VOL_GEN" ]; then
  echo "Generating v1alpha2 in crds.yaml"
  copy_ob_obc_crds
  generating_crds_v1alpha2
fi

generate_vol_rep_crds

generating_main_crd

for crd in "$OLM_CATALOG_DIR/"*.yaml; do
  cat "$crd" >> "$CRDS_FILE_PATH"
done

build_helm_resources
