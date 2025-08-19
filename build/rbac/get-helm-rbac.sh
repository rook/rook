#!/usr/bin/env bash
set -eEuox pipefail

: "${HELM:=helm}"

if ! command -v "$HELM" &>/dev/null; then
  echo "Helm not found. Please install it: https://helm.sh/docs/intro/install/"
  exit 1
fi

# Supply additional CLI options to the helm command used for generating RBAC.
# e.g., '--set key=value'
: "${ADDITIONAL_HELM_CLI_OPTIONS:=""}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
pushd "$SCRIPT_DIR" &>/dev/stderr

options=(
  --namespace rook-ceph
  --set crds.enabled=false
  --set csi.csiAddons.enabled=true
  --set csi.rookUseCsiOperator=false
)

for option in ${ADDITIONAL_HELM_CLI_OPTIONS}; do
  options+=("$option")
done

echo "generating Helm template with options: ${options[*]}" &>/dev/stderr

${HELM} template ../../deploy/charts/rook-ceph "${options[@]}" --debug | ./keep-rbac-yaml.sh

popd &>/dev/stderr
