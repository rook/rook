#!/usr/bin/env bash

# Copyright 2018 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

base_dir="$(cd "${script_dir}/../.."  && pwd)"

# Generate boilerplate with year range (2018 to current year)
current_year=$(date +%Y)
boilerplate_template="${base_dir}"/build/codegen/header.txt
boilerplate="${base_dir}"/build/codegen/header.tmp.txt

# Create temporary boilerplate with year range (replace "Copyright 2018" with "Copyright 2018-YYYY")
sed "s/Copyright 2018 The Rook Authors/Copyright 2018-${current_year} The Rook Authors/" "${boilerplate_template}" > "${boilerplate}"

# Clean up temporary file on exit
cleanup() {
	rm -f "${boilerplate}"
}
trap cleanup EXIT

source "${CODE_GENERATOR}/kube_codegen.sh"
(
  # because apis is a separate go module, code generation
  # must be run from within the corresponding directory.
  cd  ${base_dir}/pkg/apis

  # run code deepcopy generation
  kube::codegen::gen_helpers \
      --boilerplate "${boilerplate}" \
      "${base_dir}/pkg/apis" \

  # run code client,lister,informer generation
  kube::codegen::gen_client \
      --output-dir "${base_dir}/pkg/client" \
      --output-pkg "github.com/rook/rook/pkg/client" \
      --boilerplate "${boilerplate}" \
      --plural-exceptions "CephNFS:CephNFSes" \
      --with-watch \
      "${base_dir}/pkg/apis"
)
