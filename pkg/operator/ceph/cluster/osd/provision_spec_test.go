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

package osd

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDriveGroups(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}

	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap("ns", "/var/lib/rook"),
	}

	driveGroups := cephv1.DriveGroupsSpec{
		cephv1.DriveGroup{
			Name: "group1",
			Spec: cephv1.DriveGroupSpec{
				"data_devices": map[string]string{
					"all": "true",
				},
			},
			Placement: rookv1.Placement{},
		},
	}
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns"}
	spec := cephv1.ClusterSpec{
		DataDirHostPath: "/var/lib/rook",
		CephVersion:     cephv1.CephVersionSpec{Image: "test-image"},
		DriveGroups:     driveGroups,
		Storage: rookv1.StorageScopeSpec{
			UseAllNodes: true,
			Selection:   rookv1.Selection{},
			Nodes:       []rookv1.Node{{Name: "node1"}, {Name: "node2"}},
		},
	}
	c := New(context, clusterInfo, spec, "rook/rook:myversion")

	n := c.spec.Storage.ResolveNode("node1")

	osdProp := osdProperties{
		crushHostname: n.Name,
	}

	assertNoDriveGroups := func(e []v1.EnvVar) {
		for _, v := range e {
			if v.Name == "ROOK_DRIVE_GROUPS" {
				t.Helper()
				t.Errorf("ROOK_DRIVE_GROUPS should not be specified to provisioning pod but is: %+v", v)
			}
		}
	}

	assertDriveGroups := func(e []v1.EnvVar) {
		for _, v := range e {
			if v.Name == "ROOK_DRIVE_GROUPS" {
				return
			}
		}
		t.Helper()
		t.Errorf("ROOK_DRIVE_GROUPS should be specified to provisioning pod but is not: %+v", e)
	}

	// Drive groups are not specified; should NOT have env var specified
	clusterInfo.CephVersion = cephver.CephVersion{Major: 15, Minor: 2, Extra: 5}
	spec.DriveGroups = cephv1.DriveGroupsSpec{}
	job, err := c.makeJob(osdProp, dataPathMap)
	assert.NotNil(t, job)
	assert.NoError(t, err)
	assertNoDriveGroups(job.Spec.Template.Spec.Containers[0].Env)

	// Drive groups are specified; should have env var specified
	osdProp.driveGroups = driveGroups
	spec.DriveGroups = driveGroups
	job, err = c.makeJob(osdProp, dataPathMap)
	assert.NotNil(t, job)
	assert.NoError(t, err)
	assertDriveGroups(job.Spec.Template.Spec.Containers[0].Env)
}
