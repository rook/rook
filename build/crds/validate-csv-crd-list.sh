#!/usr/bin/env bash

script_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

list_of_crd_in_crd_yaml=$(grep -oE '[^ ]*\.ceph\.rook\.io' "${script_root}/deploy/examples/crds.yaml" | sort)
list_of_csv_in_csv_yaml=$(grep -oE '[^ ]*\.ceph\.rook\.io' "${script_root}/deploy/olm/assemble/metadata-common.yaml" | sort)

if [ "$list_of_crd_in_crd_yaml" != "$list_of_csv_in_csv_yaml" ]; then
    echo "CRD list in crds.yaml file and metadata-common.yaml is different. Make sure to add crd in metadata-common.yaml."
    echo -e "crd file list in crd.yaml:\n$list_of_crd_in_crd_yaml"
    echo -e "crd file list in csv.yaml:\n$list_of_csv_in_csv_yaml"
fi
