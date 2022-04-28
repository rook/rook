#!/usr/bin/env bash
set -e

##################
# INIT VARIABLES #
##################
: "${OLM_CATALOG_DIR:=deploy/olm}"
ASSEMBLE_FILE_COMMON="$OLM_CATALOG_DIR/assemble/metadata-common.yaml"
ASSEMBLE_FILE_K8S="$OLM_CATALOG_DIR/assemble/metadata-k8s.yaml"
ASSEMBLE_FILE_OCP="$OLM_CATALOG_DIR/assemble/metadata-ocp.yaml"
ASSEMBLE_FILE_OKD="$OLM_CATALOG_DIR/assemble/metadata-okd.yaml"
PACKAGE_FILE="$OLM_CATALOG_DIR/assemble/rook-ceph.package.yaml"
CRDS_FILE="deploy/examples/crds.yaml"
SUPPORTED_PLATFORMS='k8s|ocp|okd'

operator_sdk="${OPERATOR_SDK:-operator-sdk}"
yq="${YQv3:-yq}"

# Default CSI to true
: "${OLM_INCLUDE_CEPHFS_CSI:=true}"
: "${OLM_INCLUDE_RBD_CSI:=true}"
: "${OLM_INCLUDE_REPORTER:=true}"

##########
# CHECKS #
##########
if ! command -v "$operator_sdk" >/dev/null && [ ! -f "$operator_sdk" ]; then
    echo "operator-sdk is not installed $operator_sdk"
    echo "follow instructions here: https://github.com/operator-framework/operator-sdk/#quick-start"
    exit 1
fi

if ! command -v "$yq" >/dev/null && [ ! -f "$yq" ]; then
    echo "yq is not installed"
    echo "follow instructions here: https://github.com/mikefarah/yq#install"
    exit 1
fi

if [[ "$("$yq" --version)" != "yq version 3."* ]]; then
    echo "yq must be version 3" >/dev/stderr
    exit 1
fi

if [[ -z "$1" ]]; then
    echo "Please provide a version, e.g:"
    echo ""
    echo "ARGUMENT'S ORDER MATTERS"
    echo ""
    echo "make csv-ceph CSV_VERSION=1.0.1 CSV_PLATFORM=k8s ROOK_OP_VERSION=rook/ceph:v1.0.1"
    exit 1
fi
VERSION=$1

if [[ -z $2 ]]; then
    echo "Please provide a platform, choose one of these: $SUPPORTED_PLATFORMS, e.g:"
    echo ""
    echo "ARGUMENT'S ORDER MATTERS"
    echo ""
    echo "make csv-ceph CSV_VERSION=1.0.1 CSV_PLATFORM=k8s ROOK_OP_VERSION=rook/ceph:v1.0.1"
    exit 1
fi

if [[ -n $2 ]]; then
    if [[ ! $2 =~ $SUPPORTED_PLATFORMS ]]; then
        echo "Platform $2 is not supported"
        echo "Please choose one of these: $SUPPORTED_PLATFORMS"
        exit 1
    fi
    PLATFORM=$2
fi

if [[ -z $3 ]]; then
    echo "Please provide an operator version, e.g:"
    echo ""
    echo "ARGUMENT'S ORDER MATTERS"
    echo ""
    echo "make csv-ceph CSV_VERSION=1.0.1 CSV_PLATFORM=k8s ROOK_OP_VERSION=rook/ceph:v1.0.1"
    exit 1
fi
ROOK_OP_VERSION=$3

#############
# VARIABLES #
#############
: "${SED_IN_PLACE:="build/sed-in-place"}"
YQ_CMD_DELETE=($yq delete -i)
YQ_CMD_MERGE_OVERWRITE=($yq merge --inplace --overwrite --prettyPrint)
YQ_CMD_MERGE=($yq merge --inplace --append -P )
YQ_CMD_WRITE=($yq write --inplace -P )
OPERATOR_YAML_FILE_K8S="deploy/examples/operator.yaml"
OPERATOR_YAML_FILE_OCP="deploy/examples/operator-openshift.yaml"
CSV_PATH="$OLM_CATALOG_DIR/deploy/olm-catalog/${PLATFORM}/${VERSION}"
CSV_BUNDLE_PATH="${CSV_PATH}/manifests"
CSV_FILE_NAME="$CSV_BUNDLE_PATH/ceph.clusterserviceversion.yaml"
OP_SDK_CMD=($operator_sdk generate csv --output-dir="deploy/olm-catalog/${PLATFORM}/${VERSION}" --csv-version)
OLM_OPERATOR_YAML_FILE="$OLM_CATALOG_DIR/deploy/operator.yaml"
OLM_RBAC_YAML_FILE="$OLM_CATALOG_DIR/deploy/rbac.yaml"
CEPH_EXTERNAL_SCRIPT_FILE="deploy/examples/create-external-cluster-resources.py"

if [[ -d "$CSV_BUNDLE_PATH" ]]; then
    echo "$CSV_BUNDLE_PATH already exists, not doing anything."
    exit 0
fi

#############
# FUNCTIONS #
#############
function create_directories(){
    mkdir -p "$CSV_PATH"
    mkdir -p "$OLM_CATALOG_DIR/deploy/crds"
}

function cleanup() {
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" metadata.creationTimestamp
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.install.spec.deployments[0].spec.template.metadata.creationTimestamp'
}

function generate_csv(){
    pushd "$OLM_CATALOG_DIR" &> /dev/null
    "${OP_SDK_CMD[@]}" "$VERSION"
    popd &> /dev/null

    mv "$CSV_BUNDLE_PATH/olm.clusterserviceversion.yaml" "$CSV_FILE_NAME"

    # cleanup to get the expected state before merging the real data from assembles
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.icon[*]'
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.installModes[*]'
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.keywords[0]'
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.maintainers[0]'

    "${YQ_CMD_MERGE_OVERWRITE[@]}" "$CSV_FILE_NAME" "$ASSEMBLE_FILE_COMMON"
    "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" metadata.annotations.externalClusterScript "$(base64 <$CEPH_EXTERNAL_SCRIPT_FILE)"
    "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" metadata.name "rook-ceph.v${VERSION}"

    if [[ "$PLATFORM" == "k8s" ]]; then
        "${YQ_CMD_MERGE_OVERWRITE[@]}" "$CSV_FILE_NAME" "$ASSEMBLE_FILE_K8S"
        "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" spec.displayName "Rook-Ceph"
        "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" metadata.annotations.createdAt "$(date +"%Y-%m-%dT%H-%M-%SZ")"
    fi

    if [[ "$PLATFORM" == "ocp" ]]; then
        "${YQ_CMD_MERGE[@]}" "$CSV_FILE_NAME" "$ASSEMBLE_FILE_OCP"
    fi

    if [[ "$PLATFORM" == "okd" ]]; then
        "${YQ_CMD_MERGE[@]}" "$CSV_FILE_NAME" "$ASSEMBLE_FILE_OKD"
        "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" spec.displayName "Rook-Ceph"
        "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" metadata.annotations.createdAt "$(date +"%Y-%m-%dT%H-%M-%SZ")"
    fi
}

function generate_operator_yaml() {
    platform=$2
    operator_file=$OPERATOR_YAML_FILE_K8S
    if [[ "$platform" == "ocp" ]]; then
        operator_file=$OPERATOR_YAML_FILE_OCP
    fi
    if [[ "$platform" == "okd" ]]; then
        operator_file=$OPERATOR_YAML_FILE_OCP
    fi

    sed -n '/^# OLM: BEGIN OPERATOR DEPLOYMENT$/,/# OLM: END OPERATOR DEPLOYMENT$/p' "$operator_file" > "$OLM_OPERATOR_YAML_FILE"
}

function hack_csv() {
    # Let's respect the following mapping
    # somehow the operator-sdk command generates serviceAccountNames suffixed with '-rules'
    # instead of the service account name
    # So that function fixes that

    # rook-ceph-system --> serviceAccountName
    #     rook-ceph-cluster-mgmt --> rule
    #     rook-ceph-system
    #     rook-ceph-global

    # rook-ceph-mgr --> serviceAccountName
    #     rook-ceph-mgr --> rule
    #     rook-ceph-mgr-system --> rule
    #     rook-ceph-mgr-cluster

    # rook-ceph-osd --> serviceAccountName
    #     rook-ceph-osd --> rule

    $SED_IN_PLACE 's/rook-ceph-global/rook-ceph-system/' "$CSV_FILE_NAME"
    $SED_IN_PLACE 's/rook-ceph-object-bucket/rook-ceph-system/' "$CSV_FILE_NAME"
    $SED_IN_PLACE 's/rook-ceph-cluster-mgmt/rook-ceph-system/' "$CSV_FILE_NAME"

    $SED_IN_PLACE 's/rook-ceph-mgr-cluster/rook-ceph-mgr/' "$CSV_FILE_NAME"
    $SED_IN_PLACE 's/rook-ceph-mgr-system/rook-ceph-mgr/' "$CSV_FILE_NAME"

    $SED_IN_PLACE 's/cephfs-csi-nodeplugin/rook-csi-cephfs-plugin-sa/' "$CSV_FILE_NAME"
    $SED_IN_PLACE 's/cephfs-external-provisioner-runner/rook-csi-cephfs-provisioner-sa/' "$CSV_FILE_NAME"

    $SED_IN_PLACE 's/ceph-nfs-csi-nodeplugin/rook-csi-nfs-plugin-sa/' "$CSV_FILE_NAME"
    $SED_IN_PLACE 's/ceph-nfs-external-provisioner-runner/rook-csi-nfs-provisioner-sa/' "$CSV_FILE_NAME"

    $SED_IN_PLACE 's/rbd-csi-nodeplugin/rook-csi-rbd-plugin-sa/' "$CSV_FILE_NAME"
    $SED_IN_PLACE 's/rbd-external-provisioner-runner/rook-csi-rbd-provisioner-sa/' "$CSV_FILE_NAME"
    # The operator-sdk also does not properly respect when
    # Roles differ from the Service Account name
    # The operator-sdk instead assumes the Role/ClusterRole is the ServiceAccount name
    #
    # To account for these mappings, we have to replace Role/ClusterRole names with
    # the corresponding ServiceAccount.
    $SED_IN_PLACE 's/cephfs-external-provisioner-cfg/rook-csi-cephfs-provisioner-sa/' "$CSV_FILE_NAME"
    $SED_IN_PLACE 's/rbd-external-provisioner-cfg/rook-csi-rbd-provisioner-sa/' "$CSV_FILE_NAME"
}

function generate_package() {
    "${YQ_CMD_WRITE[@]}" "$PACKAGE_FILE" channels[0].currentCSV "rook-ceph.v${VERSION}"
}

function apply_rook_op_img(){
    "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" metadata.annotations.containerImage "$ROOK_OP_VERSION"
    "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" spec.install.spec.deployments[0].spec.template.spec.containers[0].image "$ROOK_OP_VERSION"
}

function validate_crds() {
    crds=$(awk '/Kind:/ {print $2}' $CRDS_FILE | grep -vE "ObjectBucketList|ObjectBucketClaimList" | sed 's/List//' | sort)
    csv_crds=$(awk '/kind:/ {print $3}' "$CSV_FILE_NAME" | sort)
    if [ "$crds" != "$csv_crds" ]; then
        echo "CRDs in $CSV_FILE_NAME do not match CRDs in $CRDS_FILE, see the diff below"
        echo ""
        diff <(echo "$crds") <(echo "$csv_crds")
        exit 1
    fi
}

########
# MAIN #
########
create_directories
generate_operator_yaml "$@"

# Do not include Pod Security Policy (PSP) resources for CSV generation since OLM uses
# Security Context Constraints (SCC).
export DO_NOT_INCLUDE_POD_SECURITY_POLICY_RESOURCES=true
# Generate csi nfs rbac too.
export ADDITIONAL_HELM_CLI_OPTIONS="--set csi.nfs.enabled=true"
./build/rbac/get-helm-rbac.sh > "$OLM_RBAC_YAML_FILE"

# TODO: do we need separate clusterrole/clusterrolebinding/role/rolebinding/servicaccount files, or
# can these just stay in rbac.yaml? If they need to be separate, we can do that here with YQ.

generate_csv "$@"
hack_csv
if [ -z "${OLM_SKIP_PKG_FILE_GEN}" ]; then
    generate_package
fi
apply_rook_op_img
cleanup

echo ""
echo "Congratulations!"
echo "Your Rook CSV $VERSION manifest for $PLATFORM is ready at: $CSV_BUNDLE_PATH"
echo "Push it to https://github.com/operator-framework/community-operators as well as the CRDs from the same folder and the package file $PACKAGE_FILE."
