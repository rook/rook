#!/usr/bin/env bash
set -eEuo pipefail

DROP_FILE="$1"
KEEP_FILE="$2"

# WRITES TO STDOUT
# DEBUGS TO STDERR

: ${YQ:=yq}

if [[ "$($YQ --version)" != "yq (https://github.com/mikefarah/yq/) version 4."* ]]; then
  echo "yq must be version 4.x"
  exit 1
fi

#
# Create a file for each resource present in the drop set
#
drop_dir="$(mktemp -d)"
pushd "${drop_dir}" &>/dev/stderr

$YQ eval --split-exp '.kind + " " + .metadata.name + " "' "$DROP_FILE" # split into files by <kind> <name> .yaml
# outputting the filenames with spaces after kind and name keeps the same sorting from before

popd &>/dev/stderr

#
# Create a file for each resource present in the keep set
#
keep_dir="$(mktemp -d)"
pushd "${keep_dir}" &>/dev/stderr

$YQ eval --split-exp '.kind + " " + .metadata.name + " "' "$KEEP_FILE" # split into files by <kind> <name> .yaml
# outputting the filenames with spaces after kind and name keeps the same sorting from before

popd &>/dev/stderr

#
# In the keep set, remove every file that also exists in the drop set
#
pushd "${drop_dir}" &>/dev/stderr

find . -type f -name '*.yml' -exec rm "${keep_dir}"/{} \;

popd &>/dev/stderr

#
# Combine the kept files back into one yaml
#
RBAC_FILES=()
while read -r line; do
  RBAC_FILES+=("$line")
done < <(find "${keep_dir}"/. -type f -name '*.yml' | sort)

# use keep-rbac-yaml.sh at the end to strip out only the RBAC, and sort and format it as we want
$YQ eval-all '.' "${RBAC_FILES[@]}" | ./keep-rbac-yaml.sh

rm -rf "${drop_dir}"
rm -rf "${keep_dir}"
