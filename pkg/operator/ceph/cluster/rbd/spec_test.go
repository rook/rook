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

package rbd

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpec(t *testing.T) {
	c := New(&clusterd.Context{},
		"ns",
		"rook/rook:myversion",
		cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"},
		rookalpha.Placement{},
		false,
		cephv1.RBDMirroringSpec{Workers: 2},
		v1.ResourceRequirements{},
		metav1.OwnerReference{},
	)

	d := c.makeDeployment("rname", "dname")
	assert.Equal(t, "rname", d.Name)
	spec := d.Spec.Template.Spec
	require.Equal(t, 1, len(spec.Containers))
	assert.Equal(t, 1, len(spec.InitContainers))
	assert.Equal(t, 3, len(spec.Volumes))
	cont := spec.Containers[0]
	assert.Equal(t, "rbd-mirror", cont.Command[0])
	assert.Equal(t, 7, len(cont.Args))
	assert.Equal(t, "--foreground", cont.Args[0])
	assert.Equal(t, "-n", cont.Args[1])
	assert.Equal(t, "client.rbd-mirror.dname", cont.Args[2])
}
