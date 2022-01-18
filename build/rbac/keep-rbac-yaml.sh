#!/usr/bin/env bash
set -eEuo pipefail

# READS FROM STDIN
# WRITES TO STDOUT
# DEBUGS TO STDERR

: ${YQ:=yq}

if [[ "$($YQ --version)" != "yq (https://github.com/mikefarah/yq/) version 4."* ]]; then
  echo "yq must be version 4.x"
  exit 1
fi

temp_dir="$(mktemp -d)"
pushd "${temp_dir}" &>/dev/stderr

# Output the RBAC into separate temporary files named with Kind and Name so that the filesystem can
# sort the files, and we can keep the same resource ordering as before for easy diffing. Then we
# just read in the files, sorted by the fs for final output.

$YQ eval '
    select(.kind == "PodSecurityPolicy"),
    select(.kind == "ServiceAccount"),
    select(.kind == "ClusterRole"),
    select(.kind == "ClusterRoleBinding"),
    select(.kind == "Role"),
    select(.kind == "RoleBinding")
  ' - | # select all RBAC resource Kinds
$YQ eval 'del(.metadata.labels."helm.sh/chart")' - | # remove the 'helm.sh/chart' label that only applies to Helm-managed resources
$YQ eval 'del(.metadata.labels."app.kubernetes.io/managed-by")' - | # remove the 'labels.app.kubernetes.io/managed-by' label that only applies to Helm-managed resources
$YQ eval 'del(.metadata.labels."app.kubernetes.io/created-by")' - | # remove the 'app.kubernetes.io/created-by' label that only applies to Helm-managed resources
sed '/^$/d' | # remove empty lines caused by yq's display of header/footer comments
sed '/^# Source: /d' | # helm adds '# Source: <file>' comments atop of each yaml doc. Strip these
$YQ eval --split-exp '.kind + " " + .metadata.name + " "' - # split into files by <kind> <name> .yaml
# outputting the filenames with spaces after kind and name keeps the same sorting from before

RBAC_FILES=()
while read -r line; do
  RBAC_FILES+=("$line")
done < <(find . -type f -name '*.yml' | sort)

# For debugging, output the resource kinds and names we processed and the number we are keeping
for file in "${RBAC_FILES[@]}"; do
  # basename to get rid of the leading './' which find adds; %.yml to remove the .yml suffix
  basename "${file%.yml}" >/dev/stderr
done
# shellcheck disable=SC2012 # we know filenames are alphanumeric from being k8s resources
echo "Number of RBAC resources: ${#RBAC_FILES[@]}" >/dev/stderr

$YQ eval-all '.' "${RBAC_FILES[@]}" | # output all files, now sorted by Kind and Name by the fs
sed '/^$/d' # remove empty lines caused by yq's display of header/footer comments

rm -rf "${temp_dir}"
popd &>/dev/stderr
