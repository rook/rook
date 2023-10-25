#!/usr/bin/env bash
set -e

#############
# VARIABLES #
#############

operator_sdk="${OPERATOR_SDK:-operator-sdk}"
yq="${YQv3:-yq}"
PLATFORM=$(go env GOARCH)

YQ_CMD_DELETE=("$yq" delete -i)
YQ_CMD_MERGE_OVERWRITE=("$yq" merge --inplace --overwrite --prettyPrint)
YQ_CMD_MERGE=("$yq" merge --arrays=append --inplace)
YQ_CMD_WRITE=("$yq" write --inplace -P)
CSV_FILE_NAME="../../build/csv/ceph/$PLATFORM/manifests/rook-ceph.clusterserviceversion.yaml"
CEPH_EXTERNAL_SCRIPT_FILE="../../deploy/examples/create-external-cluster-resources.py"
ASSEMBLE_FILE_COMMON="../../deploy/olm/assemble/metadata-common.yaml"
ASSEMBLE_FILE_OCP="../../deploy/olm/assemble/metadata-ocp.yaml"

#############
# FUNCTIONS #
#############

function generate_csv() {
    kubectl kustomize ../../deploy/examples/ | "$operator_sdk" generate bundle --package="rook-ceph" --output-dir="../../build/csv/ceph/$PLATFORM" --extra-service-accounts=rook-ceph-system,rook-csi-rbd-provisioner-sa,rook-csi-rbd-plugin-sa,rook-csi-cephfs-provisioner-sa,rook-csi-nfs-provisioner-sa,rook-csi-nfs-plugin-sa,rook-csi-cephfs-plugin-sa,rook-ceph-system,rook-ceph-rgw,rook-ceph-purge-osd,rook-ceph-osd,rook-ceph-mgr,rook-ceph-cmd-reporter

    # cleanup to get the expected state before merging the real data from assembles
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.icon[*]'
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.installModes[*]'
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.keywords[0]'
    "${YQ_CMD_DELETE[@]}" "$CSV_FILE_NAME" 'spec.maintainers[0]'

    "${YQ_CMD_MERGE_OVERWRITE[@]}" "$CSV_FILE_NAME" "$ASSEMBLE_FILE_COMMON"
    "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" metadata.annotations.externalClusterScript "$(base64 <$CEPH_EXTERNAL_SCRIPT_FILE)"
    "${YQ_CMD_WRITE[@]}" "$CSV_FILE_NAME" metadata.name "rook-ceph.v${VERSION}"

    "${YQ_CMD_MERGE[@]}" "$CSV_FILE_NAME" "$ASSEMBLE_FILE_OCP"

    # We don't need to include these files in csv as ocs-operator creates its own.
    rm -rf "../../build/csv/ceph/$PLATFORM/manifests/rook-ceph-operator-config_v1_configmap.yaml"

    # This change are just to make the CSV file as it was earlier and as ocs-operator reads.
    # Skipping this change for darwin since `sed -i` doesn't work with darwin properly.
    # and the csv is not ever needed in the mac builds.
    if [[ "$OSTYPE" == "darwin"* ]]; then
        return
    fi

    sed -i 's/image: rook\/ceph:.*/image: {{.RookOperatorImage}}/g' "$CSV_FILE_NAME"
    sed -i 's/name: rook-ceph.v.*/name: rook-ceph.v{{.RookOperatorCsvVersion}}/g' "$CSV_FILE_NAME"
    sed -i 's/version: 0.0.0/version: {{.RookOperatorCsvVersion}}/g' "$CSV_FILE_NAME"

    mv "$CSV_FILE_NAME" "../../build/csv/"
    mv "../../build/csv/ceph/$PLATFORM/manifests/"* "../../build/csv/ceph/"
    rm -rf "../../build/csv/ceph/$PLATFORM"
}

if [ "$PLATFORM" == "amd64" ]; then
    generate_csv
fi
