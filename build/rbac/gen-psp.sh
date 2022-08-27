#!/usr/bin/env bash
set -xeEuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
pushd "$SCRIPT_DIR" &>/dev/stderr

PSP_YAML_FILE="$SCRIPT_DIR/../../deploy/examples/psp.yaml"

tmpdir="$(mktemp -d)"
WITHOUT_FILE="${tmpdir}"/without-psp.yaml # intermediate file of yaml that doesn't include PSP
WITH_FILE="${tmpdir}"/with-psp.yaml       # intermediate file of yaml that includes previous plus PSP

export ADDITIONAL_HELM_CLI_OPTIONS="--set pspEnable=false"
./get-helm-rbac.sh >"$WITHOUT_FILE"

export ADDITIONAL_HELM_CLI_OPTIONS="--set pspEnable=true"
./get-helm-rbac.sh >"$WITH_FILE"

rm -f "$PSP_YAML_FILE"
cat psp.yaml.header >>"$PSP_YAML_FILE"
./keep-added.sh "$WITHOUT_FILE" "$WITH_FILE" >>"$PSP_YAML_FILE"

rm -rf "$tmpdir"
popd &>/dev/stderr
