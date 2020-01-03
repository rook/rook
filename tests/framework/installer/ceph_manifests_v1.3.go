/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package installer

import (
	"fmt"
	"testing"

	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
)

const (
	version1_3 = "master" // v1.3 is not yet released, so test upgrade to master until then
)

// CephManifestsV1_3 wraps rook yaml definitions for Rook-Ceph v1.3 manifests
type CephManifestsV1_3 struct {
	K8sh              *utils.K8sHelper
	Namespace         string
	SystemNamespace   string
	OperatorContainer string
	T                 func() *testing.T
}

// RookImage returns the rook image under test for v1.2
func (m *CephManifestsV1_3) RookImage() string {
	return fmt.Sprintf("rook/ceph:%s", version1_3)
}

// UpgradeToV1_3 performs the steps necessary to upgrade a Rook v1.1 cluster to v1.2. It does not
// verify the upgrade but merely starts the upgrade process.
func (m *CephManifestsV1_3) UpgradeToV1_3() {
	// no special manifest operations (apply, create, delete) yet

	require.NoError(m.T(),
		m.K8sh.SetDeploymentVersion(m.SystemNamespace, m.OperatorContainer, m.OperatorContainer, version1_3))
}
