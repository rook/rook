<<<<<<< HEAD
#!/usr/bin/env bash
=======
#!/usr/bin/env -S bash -e
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

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

<<<<<<< HEAD
set -o errexit
set -o nounset
set -o pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

base_dir="$(cd "${script_dir}/../.."  && pwd)"

boilerplate="${base_dir}"/build/codegen/header.txt

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
=======
GROUP_VERSIONS="ceph.rook.io:v1"

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# CODE GENERATION
# we run deepcopy and client,lister,informer generations separately so we can use the flag "--plural-exceptions"
# which is only known by client,lister,informer binary and not the deepcopy binary

# run code deepcopy generation
bash ${CODE_GENERATOR}/generate-groups.sh \
    deepcopy \
    github.com/rook/rook/pkg/client \
    github.com/rook/rook/pkg/apis \
    "${GROUP_VERSIONS}" \
    --output-base "$(dirname "${BASH_SOURCE[0]}")/../../../../.." \
    --go-header-file "${scriptdir}/boilerplate.go.txt"

# run code client,lister,informer generation
bash ${CODE_GENERATOR}/generate-groups.sh \
    client,lister,informer \
    github.com/rook/rook/pkg/client \
    github.com/rook/rook/pkg/apis \
    "${GROUP_VERSIONS}" \
    --output-base "$(dirname "${BASH_SOURCE[0]}")/../../../../.." \
    --go-header-file "${scriptdir}/boilerplate.go.txt" \
    --plural-exceptions "CephNFS:CephNFSes" \
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
