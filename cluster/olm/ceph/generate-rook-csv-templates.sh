#!/bin/bash
set -e

export OLM_SKIP_PKG_FILE_GEN="true"
export OLM_INCLUDE_CEPHFS_CSI="true"
export OLM_INCLUDE_RBD_CSI="true"
export OLM_INCLUDE_REPORTER="true"

if [ -f "Dockerfile" ]; then
    # if this is being executed from the images/ceph/ dir,
    # back out to the source dir
    cd ../../
fi

: "${OLM_CATALOG_DIR:=cluster/olm/ceph}"
DEPLOY_DIR="$OLM_CATALOG_DIR/deploy"
CRDS_DIR="$DEPLOY_DIR/crds"

TEMPLATES_DIR="$OLM_CATALOG_DIR/templates"

: "${SED_IN_PLACE:="build/sed-in-place"}"

function generate_template() {
    local provider=$1
    local csv_manifest_path="$DEPLOY_DIR/olm-catalog/${provider}/9999.9999.9999/manifests"
    local tmp_csv_gen_file="$csv_manifest_path/ceph.clusterserviceversion.yaml"
    local csv_template_file="$TEMPLATES_DIR/rook-ceph-${provider}.vVERSION.clusterserviceversion.yaml.in"
    rm -rf $csv_manifest_path

    # v9999.9999.9999 is just a placeholder. operator-sdk requires valid semver here.
    (cluster/olm/ceph/generate-rook-csv.sh "9999.9999.9999" $provider "{{.RookOperatorImage}}")
    mv $tmp_csv_gen_file $csv_template_file

    # replace the placeholder with the templated value
    $SED_IN_PLACE "s/9999.9999.9999/{{.RookOperatorCsvVersion}}/g" $csv_template_file

    echo "Template stored at $csv_template_file"
}

# start clean
if [ -d $TEMPLATES_DIR ]; then
    rm -rf $TEMPLATES_DIR
fi
mkdir -p $TEMPLATES_DIR

generate_template "ocp"
generate_template "k8s"

cp -R $CRDS_DIR $TEMPLATES_DIR/
