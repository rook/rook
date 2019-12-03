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

package crash

import (
	"testing"

	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/stretchr/testify/assert"
)

func TestGenerateCrashEnvVar(t *testing.T) {
	v := cephver.Nautilus
	env := generateCrashEnvVar(v)
	assert.Equal(t, "CEPH_ARGS", env.Name)
	assert.Equal(t, "-m $(ROOK_CEPH_MON_HOST) -k /etc/ceph/admin-keyring-store/keyring", env.Value)

	v = cephver.CephVersion{Major: 14, Minor: 2, Extra: 5}
	env = generateCrashEnvVar(v)
	assert.Equal(t, "CEPH_ARGS", env.Name)
	assert.Equal(t, "-m $(ROOK_CEPH_MON_HOST) -k /etc/ceph/crash-collector-keyring-store/keyring", env.Value)
}

func TestGetVolumes(t *testing.T) {
	v := cephver.Nautilus
	vol := getVolumes(v)
	assert.Equal(t, "rook-ceph-admin-keyring", vol.Name)

	v = cephver.CephVersion{Major: 14, Minor: 2, Extra: 5}
	vol = getVolumes(v)
	assert.Equal(t, "rook-ceph-crash-collector-keyring", vol.Name)
}

func TestGetVolumeMount(t *testing.T) {
	v := cephver.Nautilus
	volMounts := getVolumeMounts(v)
	assert.Equal(t, "rook-ceph-admin-keyring", volMounts.Name)

	v = cephver.CephVersion{Major: 14, Minor: 2, Extra: 5}
	volMounts = getVolumeMounts(v)
	assert.Equal(t, "rook-ceph-crash-collector-keyring", volMounts.Name)
}
