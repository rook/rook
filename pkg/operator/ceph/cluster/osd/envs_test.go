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

	"github.com/stretchr/testify/assert"
)

func TestCephVolumeEnvVar(t *testing.T) {
	cvEnv := cephVolumeEnvVar()
	assert.Equal(t, "CEPH_VOLUME_DEBUG", cvEnv[0].Name)
	assert.Equal(t, "1", cvEnv[0].Value)
	assert.Equal(t, "CEPH_VOLUME_SKIP_RESTORECON", cvEnv[1].Name)
	assert.Equal(t, "1", cvEnv[1].Value)
	assert.Equal(t, "DM_DISABLE_UDEV", cvEnv[2].Name)
	assert.Equal(t, "1", cvEnv[1].Value)
}

func TestOsdActivateEnvVar(t *testing.T) {
	osdActivateEnv := osdActivateEnvVar()
	assert.Equal(t, 5, len(osdActivateEnv))
	assert.Equal(t, "CEPH_VOLUME_DEBUG", osdActivateEnv[0].Name)
	assert.Equal(t, "1", osdActivateEnv[0].Value)
	assert.Equal(t, "CEPH_VOLUME_SKIP_RESTORECON", osdActivateEnv[1].Name)
	assert.Equal(t, "1", osdActivateEnv[1].Value)
	assert.Equal(t, "DM_DISABLE_UDEV", osdActivateEnv[2].Name)
	assert.Equal(t, "1", osdActivateEnv[1].Value)
	assert.Equal(t, "ROOK_CEPH_MON_HOST", osdActivateEnv[3].Name)
	assert.Equal(t, "CEPH_ARGS", osdActivateEnv[4].Name)
	assert.Equal(t, "-m $(ROOK_CEPH_MON_HOST)", osdActivateEnv[4].Value)
}
