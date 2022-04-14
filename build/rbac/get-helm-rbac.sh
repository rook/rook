#!/usr/bin/env bash
set -eEuox pipefail

: "${HELM:=helm}"

if ! command -v "$HELM" &>/dev/null; then
  echo "Helm not found. Please install it: https://helm.sh/docs/intro/install/"
  exit 1
fi

# Whether to include Pod Security Policy (PSP) related resources in the RBAC output.
# Empty string means DO include PSP resources. Any other value means do NOT include PSP resources.
: "${DO_NOT_INCLUDE_POD_SECURITY_POLICY_RESOURCES:=""}"

# Supply additional CLI options to the helm command used for generating RBAC.
# e.g., '--set key=value'
: "${ADDITIONAL_HELM_CLI_OPTIONS:=""}"

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
pushd "$SCRIPT_DIR" &>/dev/stderr

options=(
  --namespace rook-ceph
  --set crds.enabled=false
  --set csi.csiAddons.enabled=true
)
if [[ -z "${DO_NOT_INCLUDE_POD_SECURITY_POLICY_RESOURCES}" ]]; then
  options+=(--set pspEnable=true)
else
  options+=(--set pspEnable=false)
fi

for option in ${ADDITIONAL_HELM_CLI_OPTIONS}; do
  options+=("$option")
done

echo "generating Helm template with options: ${options[*]}" &>/dev/stderr

${HELM} template ../../deploy/charts/rook-ceph "${options[@]}" | ./keep-rbac-yaml.sh

popd &>/dev/stderr
