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
	"path/filepath"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestGetEncryptionVolume(t *testing.T) {
	var m int32 = 0o400
	c := &Cluster{}

	// No KMS
	osdProps := osdProperties{pvc: v1.PersistentVolumeClaimVolumeSource{ClaimName: "set1-data-1-bbgcw"}}
	v, vM := c.getEncryptionVolume(osdProps)
	assert.Equal(t, v1.Volume{Name: "osd-encryption-key", VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{SecretName: "rook-ceph-osd-encryption-key-set1-data-1-bbgcw", Items: []v1.KeyToPath{{Key: "dmcrypt-key", Path: "luks_key"}}, DefaultMode: &m}}}, v)
	assert.Equal(t, v1.VolumeMount{Name: "osd-encryption-key", ReadOnly: true, MountPath: "/etc/ceph"}, vM)

	// With KMS
	c.spec.Security = cephv1.SecuritySpec{
		KeyManagementService: cephv1.KeyManagementServiceSpec{
			ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault"},
		},
	}
	v, vM = c.getEncryptionVolume(osdProps)
	assert.Equal(t, v1.Volume{Name: "osd-encryption-key", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{Medium: "Memory"}}}, v)
	assert.Equal(t, v1.VolumeMount{Name: "osd-encryption-key", ReadOnly: false, MountPath: "/etc/ceph"}, vM)
}

func TestGetDataBridgeVolumeSource(t *testing.T) {
	claimName := "test-claim"
	configDir := "/var/lib/rook"
	namespace := "rook-ceph"

	source := getDataBridgeVolumeSource(claimName, configDir, namespace, true)
	assert.Equal(t, v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{Medium: "Memory"}}, source)
	hostPathType := v1.HostPathDirectoryOrCreate
	source = getDataBridgeVolumeSource(claimName, configDir, namespace, false)
	assert.Equal(t, v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: filepath.Join(configDir, namespace, claimName), Type: &hostPathType}}, source)
}
