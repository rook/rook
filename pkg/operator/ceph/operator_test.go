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

package operator

import (
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/object/user"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
)

func TestOperator(t *testing.T) {
	clientset := test.New(3)
	context := &clusterd.Context{Clientset: clientset}
	o := New(context, &attachment.MockAttachment{}, "", "")

	assert.NotNil(t, o)
	assert.NotNil(t, o.clusterController)
	assert.NotNil(t, o.resources)
	assert.Equal(t, context, o.context)
	assert.Equal(t, len(o.resources), 6)
	for _, r := range o.resources {
		if r.Name != cluster.ClusterResource.Name && r.Name != pool.PoolResource.Name && r.Name != object.ObjectStoreResource.Name &&
			r.Name != file.FilesystemResource.Name && r.Name != attachment.VolumeResource.Name && r.Name != objectuser.ObjectStoreUserResource.Name {
			assert.Fail(t, fmt.Sprintf("Resource %s is not valid", r.Name))
		}
	}
}
