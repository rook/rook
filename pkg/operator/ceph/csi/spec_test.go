/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package csi

import (
	"testing"

	"github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"
)

func TestStartCSI(t *testing.T) {
	RBDPluginTemplatePath = "csi-rbdplugin.yaml"
	RBDProvisionerSTSTemplatePath = "csi-rbdplugin-provisioner-sts.yaml"
	RBDProvisionerDepTemplatePath = "csi-rbdplugin-provisioner-dep.yaml"
	CephFSPluginTemplatePath = "csi-cephfsplugin.yaml"
	CephFSProvisionerSTSTemplatePath = "csi-cephfsplugin-provisioner-sts.yaml"
	CephFSProvisionerDepTemplatePath = "csi-cephfsplugin-provisioner-dep.yaml"

	CSIParam = Param{
		CSIPluginImage:   "image",
		RegistrarImage:   "image",
		ProvisionerImage: "image",
		AttacherImage:    "image",
		SnapshotterImage: "image",
	}
	clientset := test.New(3)
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		assert.Nil(t, err)
	}
	err = StartCSIDrivers("ns", clientset, serverVersion)
	assert.Nil(t, err)
}
