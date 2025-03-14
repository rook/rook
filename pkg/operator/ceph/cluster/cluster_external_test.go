/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package cluster

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/stretchr/testify/assert"
)

func TestValidateExternalClusterSpec(t *testing.T) {
	c := &cluster{Spec: &cephv1.ClusterSpec{}, mons: &mon.Cluster{}}
	err := validateExternalClusterSpec(c)
	assert.NoError(t, err)

	c.Spec.CephVersion.Image = "quay.io/ceph/ceph:v15"
	err = validateExternalClusterSpec(c)
	assert.Error(t, err)

	c.Spec.DataDirHostPath = "path"
	err = validateExternalClusterSpec(c)
	assert.NoError(t, err, err)
	assert.Equal(t, uint16(0), c.Spec.Monitoring.ExternalMgrPrometheusPort)

	c.Spec.Monitoring.Enabled = true
	err = validateExternalClusterSpec(c)
	assert.NoError(t, err, err)
	assert.Equal(t, uint16(9283), c.Spec.Monitoring.ExternalMgrPrometheusPort)
}
