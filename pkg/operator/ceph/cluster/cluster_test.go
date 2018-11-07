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

package cluster

import (
	"testing"
	"time"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestSetCephVersion(t *testing.T) {
	clientset := testop.New(3)
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Clientset: clientset, Executor: executor}
	cluster := &cephv1beta1.Cluster{}
	c := newCluster(cluster, context)
	c.Namespace = "myns"

	// expect a failure with no default and no version from the batch job
	err := c.setCephMajorVersion(time.Millisecond)
	assert.NotNil(t, err)
	assert.Equal(t, "", cluster.Spec.CephVersion.Name)

	// expect success when the default is set from the crd
	cluster.Spec.CephVersion.Name = "mimic"
	err = c.setCephMajorVersion(time.Millisecond)
	assert.Nil(t, err)
	assert.Equal(t, "mimic", cluster.Spec.CephVersion.Name)
}
