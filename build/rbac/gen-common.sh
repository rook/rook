#!/usr/bin/env bash
set -xeEuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
pushd "$SCRIPT_DIR" &>/dev/stderr

COMMON_YAML_FILE="$SCRIPT_DIR/../../deploy/examples/common.yaml"

export ADDITIONAL_HELM_CLI_OPTIONS="--set pspEnable=false"

rm -f "$COMMON_YAML_FILE"
cat common.yaml.header >>"$COMMON_YAML_FILE"
./get-helm-rbac.sh >>"$COMMON_YAML_FILE"

popd &>/dev/stderr
