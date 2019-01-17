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
package ceph

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	cephtest "github.com/rook/rook/pkg/daemon/ceph/test"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeDevicePathFinder struct {
	response []string
	called   int
}

func (f *fakeDevicePathFinder) FindDevicePath(image, pool, clusterNamespace string) (string, error) {
	response := f.response[f.called]
	f.called++
	return response, nil
}

func TestInitLoadRBDModSingleMajor(t *testing.T) {
	modInfoCalled := false
	modprobeCalled := false

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			assert.Equal(t, "modinfo", command)
			assert.Equal(t, "rbd", args[2])
			modInfoCalled = true
			return "single_major:Use a single major number for all rbd devices (default: false) (bool)", nil
		},
		MockExecuteCommand: func(debug bool, actionName string, command string, args ...string) error {
			assert.Equal(t, "modprobe", command)
			assert.Equal(t, "rbd", args[0])
			assert.Equal(t, "single_major=Y", args[1])
			modprobeCalled = true
			return nil
		},
	}

	context := &clusterd.Context{
		Executor: executor,
	}
	NewVolumeManager(context)
	assert.True(t, modInfoCalled)
	assert.True(t, modprobeCalled)
}

func TestInitLoadRBDModNoSingleMajor(t *testing.T) {
	modInfoCalled := false
	modprobeCalled := false

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			assert.Equal(t, "modinfo", command)
			assert.Equal(t, "rbd", args[2])
			modInfoCalled = true
			return "", nil
		},
		MockExecuteCommand: func(debug bool, actionName string, command string, args ...string) error {
			assert.Equal(t, "modprobe", command)
			assert.Equal(t, 1, len(args))
			assert.Equal(t, "rbd", args[0])
			modprobeCalled = true
			return nil
		},
	}

	context := &clusterd.Context{
		Executor: executor,
	}
	NewVolumeManager(context)
	assert.True(t, modInfoCalled)
	assert.True(t, modprobeCalled)
}

func TestAttach(t *testing.T) {
	clientset := test.New(3)
	clusterNamespace := "testCluster"
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	cm := &v1.ConfigMap{
		Data: map[string]string{
			"data": "rook-ceph-mon0=10.0.0.1:6789,rook-ceph-mon1=10.0.0.2:6789,rook-ceph-mon2=10.0.0.3:6789",
		},
	}
	cm.Name = "rook-ceph-mon-endpoints"
	clientset.CoreV1().ConfigMaps(clusterNamespace).Create(cm)

	runCount := 1

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(configDir, clusterNamespace))
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(debug bool, timeout time.Duration, actionName string, command string, args ...string) (string, error) {
			assert.Equal(t, "rbd", command)
			assert.Equal(t, "map", args[0])
			assert.Equal(t, fmt.Sprintf("testpool/image%d", runCount), args[1])
			if runCount == 1 {
				assert.Equal(t, "--id=admin", args[2])
			} else {
				assert.Equal(t, "--id=user1", args[2])
			}
			assert.Equal(t, "--cluster=testCluster", args[3])
			assert.True(t, strings.HasPrefix(args[4], "--keyring="))
			assert.Contains(t, args[6], "10.0.0.1:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.1:6789"))
			assert.Contains(t, args[6], "10.0.0.2:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.2:6789"))
			assert.Contains(t, args[6], "10.0.0.3:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.3:6789"))
			runCount++
			return "", nil
		},
	}

	context := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
		ConfigDir: configDir,
	}
	vm := &VolumeManager{
		context: context,
		devicePathFinder: &fakeDevicePathFinder{
			response: []string{"", "/dev/rbd3"},
			called:   0,
		},
	}
	mon.CreateOrLoadClusterInfo(context, clusterNamespace, &metav1.OwnerReference{})

	devicePath, err := vm.Attach("image1", "testpool", "admin", "never-gonna-give-you-up", clusterNamespace)
	assert.Equal(t, "/dev/rbd3", devicePath)
	assert.Nil(t, err)

	vm = &VolumeManager{
		context: context,
		devicePathFinder: &fakeDevicePathFinder{
			response: []string{"", "/dev/rbd4"},
			called:   0,
		},
	}

	devicePath, err = vm.Attach("image2", "testpool", "user1", "never-gonna-let-you-down", clusterNamespace)
	assert.Equal(t, "/dev/rbd4", devicePath)
	assert.Nil(t, err)
}

func TestAttachAlreadyExists(t *testing.T) {
	vm := &VolumeManager{
		context: &clusterd.Context{},
		devicePathFinder: &fakeDevicePathFinder{
			response: []string{"/dev/rbd3"},
			called:   0,
		},
	}
	devicePath, err := vm.Attach("image1", "testpool", "admin", "never-gonna-run-around-and-desert-you ", "testCluster")
	assert.Equal(t, "/dev/rbd3", devicePath)
	assert.Nil(t, err)
}

func TestDetach(t *testing.T) {
	clientset := test.New(3)
	clusterNamespace := "testCluster"
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	cm := &v1.ConfigMap{
		Data: map[string]string{
			"data": "rook-ceph-mon0=10.0.0.1:6789,rook-ceph-mon1=10.0.0.2:6789,rook-ceph-mon2=10.0.0.3:6789",
		},
	}
	cm.Name = "rook-ceph-mon-endpoints"
	clientset.CoreV1().ConfigMaps(clusterNamespace).Create(cm)

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(configDir, clusterNamespace))
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(debug bool, timeout time.Duration, actionName string, command string, args ...string) (string, error) {
			assert.Equal(t, "rbd", command)
			assert.Equal(t, "unmap", args[0])
			assert.Equal(t, "testpool/image1", args[1])
			assert.Equal(t, "--id=admin", args[2])
			assert.Equal(t, "--cluster=testCluster", args[3])
			assert.True(t, strings.HasPrefix(args[4], "--keyring="))
			assert.Contains(t, args[6], "10.0.0.1:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.1:6789"))
			assert.Contains(t, args[6], "10.0.0.2:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.2:6789"))
			assert.Contains(t, args[6], "10.0.0.3:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.3:6789"))
			return "", nil
		},
	}

	context := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
		ConfigDir: configDir,
	}
	vm := &VolumeManager{
		context: context,
		devicePathFinder: &fakeDevicePathFinder{
			response: []string{"/dev/rbd3"},
			called:   0,
		},
	}
	mon.CreateOrLoadClusterInfo(context, clusterNamespace, &metav1.OwnerReference{})
	err := vm.Detach("image1", "testpool", "admin", "", clusterNamespace, false)
	assert.Nil(t, err)
}

func TestDetachCustomKeyring(t *testing.T) {
	clientset := test.New(3)
	clusterNamespace := "testCluster"
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	cm := &v1.ConfigMap{
		Data: map[string]string{
			"data": "rook-ceph-mon0=10.0.0.1:6789,rook-ceph-mon1=10.0.0.2:6789,rook-ceph-mon2=10.0.0.3:6789",
		},
	}
	cm.Name = "rook-ceph-mon-endpoints"
	clientset.CoreV1().ConfigMaps(clusterNamespace).Create(cm)

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(configDir, clusterNamespace))
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(debug bool, timeout time.Duration, actionName string, command string, args ...string) (string, error) {
			assert.Equal(t, "rbd", command)
			assert.Equal(t, "unmap", args[0])
			assert.Equal(t, "testpool/image1", args[1])
			assert.Equal(t, "--id=user1", args[2])
			assert.Equal(t, "--cluster=testCluster", args[3])
			assert.True(t, strings.HasPrefix(args[4], "--keyring="))
			assert.Contains(t, args[6], "10.0.0.1:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.1:6789"))
			assert.Contains(t, args[6], "10.0.0.2:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.2:6789"))
			assert.Contains(t, args[6], "10.0.0.3:6789", fmt.Sprintf("But '%s' does contain '%s'", args[6], "10.0.0.3:6789"))
			return "", nil
		},
	}

	context := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
		ConfigDir: configDir,
	}
	vm := &VolumeManager{
		context: context,
		devicePathFinder: &fakeDevicePathFinder{
			response: []string{"/dev/rbd3"},
			called:   0,
		},
	}
	mon.CreateOrLoadClusterInfo(context, clusterNamespace, &metav1.OwnerReference{})
	err := vm.Detach("image1", "testpool", "user1", "", clusterNamespace, false)
	assert.Nil(t, err)
}

func TestAlreadyDetached(t *testing.T) {
	vm := &VolumeManager{
		context: &clusterd.Context{},
		devicePathFinder: &fakeDevicePathFinder{
			response: []string{""},
			called:   0,
		},
	}
	err := vm.Detach("image1", "testpool", "admin", "", "testCluster", false)
	assert.Nil(t, err)
}
