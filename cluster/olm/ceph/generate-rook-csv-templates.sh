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

OLM_CATALOG_DIR=cluster/olm/ceph
DEPLOY_DIR="$OLM_CATALOG_DIR/deploy"
CRDS_DIR="$DEPLOY_DIR/crds"

TMP_CSV_GEN_FILE="$OLM_CATALOG_DIR/deploy/olm-catalog/rook-ceph.v9999.9999.9999.clusterserviceversion.yaml"
TEMPLATES_DIR="$OLM_CATALOG_DIR/templates"

function generate_template() {
    local provider=$1
    local csv_template_file="$TEMPLATES_DIR/rook-ceph-${provider}.vVERSION.clusterserviceversion.yaml.in"

    # v9999.9999.9999 is just a placeholder. operator-sdk requires valid semver here.
    (cluster/olm/ceph/generate-rook-csv.sh "9999.9999.9999" $provider "{{.RookOperatorImage}}")
    mv $TMP_CSV_GEN_FILE $csv_template_file

    # replace the placeholder with the templated value
    sed -i "s/9999.9999.9999/{{.RookOperatorCsvVersion}}/g" $csv_template_file

    echo "Template stored at $csv_template_file"
}

# start clean
rm -f $TMP_CSV_GEN_FILE
if [ -d $TEMPLATES_DIR ]; then
    rm -rf $TEMPLATES_DIR
fi
mkdir -p $TEMPLATES_DIR

generate_template "ocp"
generate_template "k8s"

cp -R $CRDS_DIR $TEMPLATES_DIR/
