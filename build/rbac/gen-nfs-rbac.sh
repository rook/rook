#!/usr/bin/env bash
set -xeEuo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
pushd "$SCRIPT_DIR" &>/dev/stderr

NFS_RBAC_YAML_FILE="$SCRIPT_DIR/../../deploy/examples/csi/nfs/rbac.yaml"

tmpdir="$(mktemp -d)"
WITHOUT_FILE="${tmpdir}"/without-nfs.yaml # intermediate file of yaml that doesn't include NFS RBAC
WITH_FILE="${tmpdir}"/with-nfs.yaml # intermediate file of yaml that includes previous plus NFS RBAC

./get-helm-rbac.sh > "$WITHOUT_FILE"

export ADDITIONAL_HELM_CLI_OPTIONS="--set csi.nfs.enabled=true"
./get-helm-rbac.sh > "$WITH_FILE"

rm -f "$NFS_RBAC_YAML_FILE"
cat nfs-rbac.yaml.header >> "$NFS_RBAC_YAML_FILE"
./keep-added.sh "$WITHOUT_FILE" "$WITH_FILE" >> "$NFS_RBAC_YAML_FILE"

rm -rf "$tmpdir"
popd &>/dev/stderr
