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

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd "${scriptdir}/../../vendor/k8s.io/code-generator" && ./generate-groups.sh \
    all \
    github.com/rook/rook/pkg/client \
    github.com/rook/rook/pkg/apis \
    "rook.io:v1alpha2 ceph.rook.io:v1beta1,v1 cockroachdb.rook.io:v1alpha1 minio.rook.io:v1alpha1 nfs.rook.io:v1alpha1 cassandra.rook.io:v1alpha1 edgefs.rook.io:v1alpha1"
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
