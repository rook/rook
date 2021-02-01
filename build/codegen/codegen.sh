#!/bin/bash -e

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

GROUP_VERSIONS="rook.io:v1 rook.io:v1alpha2 ceph.rook.io:v1 nfs.rook.io:v1alpha1 cassandra.rook.io:v1alpha1 yugabytedb.rook.io:v1alpha1"

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
codegendir="${scriptdir}/../../vendor/k8s.io/code-generator"

# ensures the vendor dir has the right deps, e,g. code-generator
go mod vendor

# run code generation
bash ${codegendir}/generate-groups.sh \
    all \
    github.com/rook/rook/pkg/client \
    github.com/rook/rook/pkg/apis \
    "${GROUP_VERSIONS}" \
    --output-base "$(dirname "${BASH_SOURCE[0]}")/../../../../.." \
    --go-header-file "${scriptdir}/boilerplate.go.txt"
