#!/usr/bin/env -S bash -e

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

# outputs absolute path for given relative path.
function toAbsolutePath(){
  echo "$(cd "$(dirname "$1")"; pwd)/$(basename "$1")"
}


GROUP_VERSIONS="ceph.rook.io:v1"

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

output_base="$(dirname "${BASH_SOURCE[0]}")/../../../../.."
output_pkg="github.com/rook/rook/pkg/client"
apis_pkg="github.com/rook/rook/pkg/apis"

# check that output directory for generated packages already exists.
if [ ! -d "$output_base/$apis_pkg" ]; then
  echo "apis pkg $(toAbsolutePath $output_base/$apis_pkg) does not exist."
  echo "please make sure that project located in 'github.com/rook/rook' directory"
  exit 1
fi

echo "Generate code for packages: 
  - $(toAbsolutePath $output_base/$output_pkg)
  - $(toAbsolutePath $output_base/$apis_pkg)"

# CODE GENERATION
# we run deepcopy and client,lister,informer generations separately so we can use the flag "--plural-exceptions"
# which is only known by client,lister,informer binary and not the deepcopy binary

# run code deepcopy generation
bash ${CODE_GENERATOR}/generate-groups.sh \
    deepcopy \
    $output_pkg \
    $apis_pkg \
    "${GROUP_VERSIONS}" \
    --output-base $output_base \
    --go-header-file "${scriptdir}/boilerplate.go.txt"

# run code client,lister,informer generation
bash ${CODE_GENERATOR}/generate-groups.sh \
    client,lister,informer \
    $output_pkg \
    $apis_pkg \
    "${GROUP_VERSIONS}" \
    --output-base $output_base \
    --go-header-file "${scriptdir}/boilerplate.go.txt" \
    --plural-exceptions "CephNFS:CephNFSes" \
