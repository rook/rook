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
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	sysconfig = []byte(`# /etc/sysconfig/ceph
#
# Environment file for ceph daemon systemd unit files.
#

# Increase tcmalloc cache size
TCMALLOC_MAX_TOTAL_THREAD_CACHE_BYTES=134217728

## automatically restart systemd units on upgrade
#
# By default, it is left to the administrator to restart
# ceph daemons (or their related systemd units) manually
# when the 'ceph' package is upgraded. By setting this
# parameter to "yes", package upgrade will trigger a
# "systemctl try-restart" on all the ceph systemd units
# currently active on the node.
#
CEPH_AUTO_RESTART_ON_UPGRADE=no`)
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

func TestGetTcmallocMaxTotalThreadCacheBytes(t *testing.T) {
	// No file, nothing
	v := getTcmallocMaxTotalThreadCacheBytes("")
	assert.Equal(t, "", v.Value)

	// File and arg are empty so we can an empty value
	file, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	defer os.Remove(file.Name())
	cephEnvConfigFile = file.Name()
	v = getTcmallocMaxTotalThreadCacheBytes("")
	assert.Equal(t, "", v.Value)

	// Arg is not empty
	v = getTcmallocMaxTotalThreadCacheBytes("67108864")
	assert.Equal(t, "67108864", v.Value)

	// Read the file now
	err = ioutil.WriteFile(file.Name(), sysconfig, 0444)
	assert.NoError(t, err)
	v = getTcmallocMaxTotalThreadCacheBytes("")
	assert.Equal(t, "134217728", v.Value)
}
