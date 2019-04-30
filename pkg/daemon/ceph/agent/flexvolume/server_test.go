/*
Copyright 2017 The Rook Authors. All rights reserved.

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

// Package flexvolume to manage Kubernetes storage attach events.
package flexvolume

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/volume/flexvolume"
)

func TestConfigureFlexVolume(t *testing.T) {
	driverDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(driverDir)
	driverFile := path.Join(driverDir, flexvolumeDriverFileName)
	os.OpenFile(driverFile, os.O_RDONLY|os.O_CREATE, 0755)

	driverName := "rook"
	os.Setenv("POD_NAMESPACE", driverName)
	defer os.Setenv("POD_NAMESPACE", "")
	os.Setenv(agent.RookEnableSelinuxRelabelingEnv, "false")
	defer os.Setenv(agent.RookEnableSelinuxRelabelingEnv, "")
	os.Setenv(agent.RookEnableFSGroupEnv, "false")
	defer os.Setenv(agent.RookEnableFSGroupEnv, "")
	err := configureFlexVolume(driverFile, driverDir, driverName)
	assert.Nil(t, err)
	_, err = os.Stat(path.Join(driverDir, "rook"))
	assert.False(t, os.IsNotExist(err))

	// verify the non-default settings
	settings := LoadFlexSettings(driverDir)
	var status flexvolume.DriverStatus
	err = json.Unmarshal(settings, &status)
	assert.Nil(t, err)
	assert.False(t, status.Capabilities.FSGroup)
	assert.False(t, status.Capabilities.SELinuxRelabel)
}

func TestGetFlexDriverInfo(t *testing.T) {
	// empty string, can't do anything with that, this is an error
	vendor, driver, err := getFlexDriverInfo("")
	assert.NotNil(t, err)

	// no driver dir found, this is an error
	vendor, driver, err = getFlexDriverInfo("/a/b/c")
	assert.NotNil(t, err)

	// well formed flex driver path, driver dir is last dir
	vendor, driver, err = getFlexDriverInfo("/usr/libexec/kubernetes/kubelet-plugins/volume/exec/foo.bar.baz~biz")
	assert.Nil(t, err)
	assert.Equal(t, "foo.bar.baz", vendor)
	assert.Equal(t, "biz", driver)

	// well formed flex driver path, driver dir is last dir but there's a trailing path separator
	vendor, driver, err = getFlexDriverInfo("/usr/libexec/kubernetes/kubelet-plugins/volume/exec/foo.bar.baz~biz/")
	assert.Nil(t, err)
	assert.Equal(t, "foo.bar.baz", vendor)
	assert.Equal(t, "biz", driver)

	// well formed flex driver path, driver dir is not the last dir in the path
	vendor, driver, err = getFlexDriverInfo("/usr/libexec/kubernetes/kubelet-plugins/volume/exec/foo.bar.baz~biz/another-folder")
	assert.Nil(t, err)
	assert.Equal(t, "foo.bar.baz", vendor)
	assert.Equal(t, "biz", driver)

	// more flex volume info items than expected, this is an error
	vendor, driver, err = getFlexDriverInfo("/usr/libexec/kubernetes/kubelet-plugins/volume/exec/foo.bar.baz~biz~buzz/")
	assert.NotNil(t, err)
}
