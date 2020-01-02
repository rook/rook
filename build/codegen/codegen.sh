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

# shellcheck disable=SC2086,SC2089,SC2090
# Disables quote checks, which is needed because of the SED variable here.

KUBE_CODE_GEN_VERSION="kubernetes-1.16.4"

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# TODO Should we clone the k8s.io/code-generator repository to the `.cache` dir?
GO111MODULE=off go get -u k8s.io/code-generator/cmd/client-gen
cd "${GOPATH}/src/k8s.io/code-generator" || { echo ""; exit 1; }
git reset --hard
git checkout "${KUBE_CODE_GEN_VERSION}"

./generate-groups.sh \
    all \
    github.com/rook/rook/pkg/client \
    github.com/rook/rook/pkg/apis \
    "rook.io:v1alpha2 ceph.rook.io:v1 cockroachdb.rook.io:v1alpha1 nfs.rook.io:v1alpha1 cassandra.rook.io:v1alpha1 edgefs.rook.io:v1 yugabytedb.rook.io:v1alpha1"
# this seems busted in the release-1.8 branch
#  --go-header-file ${SCRIPT_ROOT}/build/codegen/header.txt

SED="sed -i.bak"

# workaround https://github.com/openshift/origin/issues/10357
find "${scriptdir}/../../pkg/client" -name "clientset_generated.go" -exec \
    $SED 's/fakePtr := testing.Fake\([{]\)}/cs := \&Clientset\1}/g' {} +
find "${scriptdir}/../../pkg/client" -name "clientset_generated.go" -exec \
    $SED 's/fakePtr.AddReactor/cs.Fake.AddReactor/g' {} +
find "${scriptdir}/../../pkg/client" -name "clientset_generated.go" -exec \
    $SED 's/fakePtr.AddWatchReactor/cs.Fake.AddWatchReactor/g' {} +
# shellcheck disable=SC1004
# Disables backslash+linefeed is literal check.
find "${scriptdir}/../../pkg/client" -name "clientset_generated.go" -exec \
    $SED 's/return \&Clientset{fakePtr, \&fakediscovery.FakeDiscovery{Fake: \&fakePtr}}/cs.discovery = \&fakediscovery.FakeDiscovery{Fake: \&cs.Fake}\
	return cs/g' {} +
find "${scriptdir}/../../pkg/client" -name "clientset_generated.go.bak" -delete

# Code generation does not respect the plural version of the CRD name unless it simply appends "s".
# In this case the plural for cephnfs should be cephnfses.
find "${scriptdir}/../../pkg/client" -name "*.go" -exec \
    $SED 's/cephnfss/cephnfses/g' {} +
find "${scriptdir}/../../pkg/client" -name "*.go" -exec \
    $SED 's/CephNFSs/CephNFSes/g' {} +
find "${scriptdir}/../../pkg/client" -name "*.go" -exec \
    $SED 's/cephNFSs/cephNFSes/g' {} +
find "${scriptdir}/../../pkg/client" -name "*.go.bak" -delete
