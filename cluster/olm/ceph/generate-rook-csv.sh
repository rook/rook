#!/bin/bash
set -e

##################
# INIT VARIABLES #
##################
OLM_CATALOG_DIR=cluster/olm/ceph
CSV_PATH="$OLM_CATALOG_DIR/deploy/olm-catalog"
ASSEMBLE_FILE_COMMON="$OLM_CATALOG_DIR/assemble/metadata-common.yaml"
ASSEMBLE_FILE_K8S="$OLM_CATALOG_DIR/assemble/metadata-k8s.yaml"
ASSEMBLE_FILE_OCP="$OLM_CATALOG_DIR/assemble/metadata-openshift.yaml"
PACKAGE_FILE="$OLM_CATALOG_DIR/assemble/rook-ceph.package.yaml"
SUPPORTED_PLATFORMS='k8s|ocp'

##########
# CHECKS #
##########
if ! command -v operator-sdk &>/dev/null; then
    echo "operator-sdk is not installed"
    echo "follow instructions here: https://github.com/operator-framework/operator-sdk/#quick-start"
    exit 1
fi

if ! command -v yq &>/dev/null; then
    echo "yq is not installed"
    echo "follow instructions here: https://github.com/mikefarah/yq#install"
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

DEFAULT_CSV_FILE_NAME="${CSV_PATH}/ceph/${VERSION}/ceph.v${VERSION}.clusterserviceversion.yaml"
DESIRED_CSV_FILE_NAME="${CSV_PATH}/rook-ceph.v${VERSION}.clusterserviceversion.yaml"
if [[ -f "$DESIRED_CSV_FILE_NAME" ]]; then
    echo "$DESIRED_CSV_FILE_NAME already exists, not doing anything."
    exit 0
fi

#############
# VARIABLES #
#############
YQ_CMD_DELETE=(yq delete -i)
YQ_CMD_MERGE=(yq merge --inplace --overwrite --append)
YQ_CMD_WRITE=(yq write --inplace)
OP_SDK_CMD=(operator-sdk olm-catalog gen-csv --csv-version)
OPERATOR_YAML_FILE_K8S="cluster/examples/kubernetes/ceph/operator.yaml"
OPERATOR_YAML_FILE_OCP="cluster/examples/kubernetes/ceph/operator-openshift.yaml"
COMMON_YAML_FILE="cluster/examples/kubernetes/ceph/common.yaml"
OLM_OPERATOR_YAML_FILE="$OLM_CATALOG_DIR/deploy/operator.yaml"
OLM_ROLE_YAML_FILE="$OLM_CATALOG_DIR/deploy/role.yaml"
OLM_ROLE_BINDING_YAML_FILE="$OLM_CATALOG_DIR/deploy/role_binding.yaml"
OLM_SERVICE_ACCOUNT_YAML_FILE="$OLM_CATALOG_DIR/deploy/service_account.yaml"
CEPH_CRD_YAML_FILE="$OLM_CATALOG_DIR/deploy/crds/ceph_crd.yaml"
CEPH_BLOCK_POOLS_CRD_YAML_FILE="$OLM_CATALOG_DIR/deploy/crds/rookcephblockpools.crd.yaml"
CEPH_OBJECT_STORE_YAML_FILE="$OLM_CATALOG_DIR/deploy/crds/rookcephobjectstores.crd.yaml"
CEPH_OBJECT_STORE_USERS_YAML_FILE="$OLM_CATALOG_DIR/deploy/crds/rookcephobjectstoreusers.crd.yaml"

#############
# FUNCTIONS #
#############
function create_directories(){
    mkdir -p $OLM_CATALOG_DIR/deploy/crds
}

function cleanup() {
    "${YQ_CMD_DELETE[@]}" "$DESIRED_CSV_FILE_NAME" metadata.creationTimestamp
    "${YQ_CMD_DELETE[@]}" "$DESIRED_CSV_FILE_NAME" 'spec.install.spec.deployments[0].spec.template.metadata.creationTimestamp'
}

function generate_csv(){
    pushd "$OLM_CATALOG_DIR" &> /dev/null
    "${OP_SDK_CMD[@]}" "$VERSION"
    popd &> /dev/null
    mv "$DEFAULT_CSV_FILE_NAME" "$DESIRED_CSV_FILE_NAME"
    "${YQ_CMD_MERGE[@]}" "$DESIRED_CSV_FILE_NAME" "$ASSEMBLE_FILE_COMMON"

    if [[ "$PLATFORM" == "k8s" ]]; then
        "${YQ_CMD_MERGE[@]}" "$DESIRED_CSV_FILE_NAME" "$ASSEMBLE_FILE_K8S"
        "${YQ_CMD_WRITE[@]}" "$DESIRED_CSV_FILE_NAME" metadata.name "rook-ceph.v${VERSION}"
        "${YQ_CMD_WRITE[@]}" "$DESIRED_CSV_FILE_NAME" spec.displayName "Rook-Ceph"
        "${YQ_CMD_WRITE[@]}" "$DESIRED_CSV_FILE_NAME" metadata.annotations.createdAt "$(date +"%Y-%m-%dT%H-%M-%SZ")"
    fi

    if [[ "$PLATFORM" == "ocp" ]]; then
        "${YQ_CMD_MERGE[@]}" "$DESIRED_CSV_FILE_NAME" "$ASSEMBLE_FILE_OCP"
    fi

}

function generate_operator_yaml() {
    platform=$2
    operator_file=$OPERATOR_YAML_FILE_K8S
    if [[ "$platform" == "ocp" ]]; then
        operator_file=$OPERATOR_YAML_FILE_OCP
    fi

    sed -n '/^# OLM: BEGIN OPERATOR DEPLOYMENT$/,/# OLM: END OPERATOR DEPLOYMENT$/p' "$operator_file" > "$OLM_OPERATOR_YAML_FILE"
}

function generate_role_yaml() {
    sed -n '/^# OLM: BEGIN OPERATOR ROLE$/,/# OLM: END OPERATOR ROLE$/p' "$COMMON_YAML_FILE" > "$OLM_ROLE_YAML_FILE"
    sed -n '/^# OLM: BEGIN CLUSTER ROLE$/,/# OLM: END CLUSTER ROLE$/p' "$COMMON_YAML_FILE" >> "$OLM_ROLE_YAML_FILE"
}

function generate_role_binding_yaml() {
    sed -n '/^# OLM: BEGIN OPERATOR ROLEBINDING$/,/# OLM: END OPERATOR ROLEBINDING$/p' "$COMMON_YAML_FILE" > "$OLM_ROLE_BINDING_YAML_FILE"
    sed -n '/^# OLM: BEGIN CLUSTER ROLEBINDING$/,/# OLM: END CLUSTER ROLEBINDING$/p' "$COMMON_YAML_FILE" >> "$OLM_ROLE_BINDING_YAML_FILE"
}

function generate_service_account_yaml() {
    sed -n '/^# OLM: BEGIN SERVICE ACCOUNT SYSTEM$/,/# OLM: END SERVICE ACCOUNT SYSTEM$/p' "$COMMON_YAML_FILE" > "$OLM_SERVICE_ACCOUNT_YAML_FILE"
    sed -n '/^# OLM: BEGIN SERVICE ACCOUNT OSD$/,/# OLM: END SERVICE ACCOUNT OSD$/p' "$COMMON_YAML_FILE" >> "$OLM_SERVICE_ACCOUNT_YAML_FILE"
    sed -n '/^# OLM: BEGIN SERVICE ACCOUNT MGR$/,/# OLM: END SERVICE ACCOUNT MGR$/p' "$COMMON_YAML_FILE" >> "$OLM_SERVICE_ACCOUNT_YAML_FILE"
}

function generate_crds_yaml() {
    sed -n '/^# OLM: BEGIN CEPH CRD$/,/# OLM: END CEPH CRD$/p' "$COMMON_YAML_FILE" > "$CEPH_CRD_YAML_FILE"
    sed -n '/^# OLM: BEGIN CEPH OBJECT STORE CRD$/,/# OLM: END CEPH OBJECT STORE CRD$/p' "$COMMON_YAML_FILE" > "$CEPH_OBJECT_STORE_YAML_FILE"
    sed -n '/^# OLM: BEGIN CEPH OBJECT STORE USERS CRD$/,/# OLM: END CEPH OBJECT STORE USERS CRD$/p' "$COMMON_YAML_FILE" > "$CEPH_OBJECT_STORE_USERS_YAML_FILE"
    sed -n '/^# OLM: BEGIN CEPH BLOCK POOL CRD$/,/# OLM: END CEPH BLOCK POOL CRD$/p' "$COMMON_YAML_FILE" > "$CEPH_BLOCK_POOLS_CRD_YAML_FILE"
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

    sed -i 's/rook-ceph-cluster-mgmt-rules/rook-ceph-system/' "$DESIRED_CSV_FILE_NAME"
    sed -i 's/rook-ceph-global-rules/rook-ceph-system/' "$DESIRED_CSV_FILE_NAME"

    sed -i 's/rook-ceph-mgr-system-rules/rook-ceph-mgr/' "$DESIRED_CSV_FILE_NAME"
    sed -i 's/rook-ceph-mgr-cluster-rules/rook-ceph-mgr/' "$DESIRED_CSV_FILE_NAME"
}

function generate_package() {
    "${YQ_CMD_WRITE[@]}" "$PACKAGE_FILE" channels[0].currentCSV "rook-ceph.v${VERSION}"
}

function apply_rook_op_img(){
    "${YQ_CMD_WRITE[@]}" "$DESIRED_CSV_FILE_NAME" metadata.annotations.containerImage "$ROOK_OP_VERSION"
    "${YQ_CMD_WRITE[@]}" "$DESIRED_CSV_FILE_NAME" spec.install.spec.deployments[0].spec.template.spec.containers[0].image "$ROOK_OP_VERSION"
}

########
# MAIN #
########
create_directories
generate_operator_yaml "$@"
generate_role_yaml
generate_role_binding_yaml
generate_service_account_yaml
generate_crds_yaml
generate_csv "$@"
hack_csv
generate_package
apply_rook_op_img
cleanup

echo ""
echo "Congratulations!"
echo "Your Rook CSV $VERSION file for $PLATFORM is ready at: $DESIRED_CSV_FILE_NAME"
echo "Push it to https://github.com/operator-framework/community-operators as well as the CRDs files from $OLM_CATALOG_DIR/deploy/crds and the package file $PACKAGE_FILE."
