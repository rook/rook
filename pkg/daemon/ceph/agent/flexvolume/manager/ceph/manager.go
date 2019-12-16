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
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	findDevicePathMaxRetries = 10
	rbdKernelModuleName      = "rbd"
	keyringTemplate          = `
[client.%s]
key = %s
`
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-volumeattacher")

// VolumeManager represents an object for perform volume attachment requests for Ceph volumes
type VolumeManager struct {
	context          *clusterd.Context
	devicePathFinder pathFinder
}

type devicePathFinder struct{}

// DevicePathFinder is used to find the device path after the volume has been attached
type pathFinder interface {
	FindDevicePath(image, pool, clusterNamespace string) (string, error)
}

// NewVolumeManager create attacher for ceph volumes
func NewVolumeManager(context *clusterd.Context) (*VolumeManager, error) {
	vm := &VolumeManager{
		context:          context,
		devicePathFinder: &devicePathFinder{},
	}
	err := vm.Init()
	return vm, err
}

// Init the ceph volume manager
func (vm *VolumeManager) Init() error {
	// check if the rbd is a builtin kernel module, if it is then we don't need to load it manually
	in, err := sys.IsBuiltinKernelModule(rbdKernelModuleName, vm.context.Executor)
	if err != nil {
		return err
	}
	if in == true {
		logger.Noticef("volume manager is a builtin kernel module, don't load it manually")
		return nil
	}

	// check to see if the rbd kernel module has single_major support
	hasSingleMajor, err := sys.CheckKernelModuleParam(rbdKernelModuleName, "single_major", vm.context.Executor)
	if err != nil {
		logger.Noticef("failed %q single_major check, assuming it's unsupported. %v", rbdKernelModuleName, err)
		hasSingleMajor = false
	}

	opts := []string{}
	if hasSingleMajor {
		opts = append(opts, "single_major=Y")
	}

	// load the rbd kernel module with options
	if err := sys.LoadKernelModule(rbdKernelModuleName, opts, vm.context.Executor); err != nil {
		logger.Noticef("failed to load kernel module %q. %v", rbdKernelModuleName, err)
		return err
	}

	return nil
}

// Attach a ceph image to the node
func (vm *VolumeManager) Attach(image, pool, id, key, clusterNamespace string) (string, error) {
	// Check if the volume is already attached
	devicePath, err := vm.isAttached(image, pool, clusterNamespace)
	if err != nil {
		return "", errors.Wrapf(err, "failed to check if volume %s/%s is already attached", pool, image)
	}
	if devicePath != "" {
		logger.Infof("volume %s/%s is already attached. The device path is %s", pool, image, devicePath)
		return devicePath, nil
	}

	if id == "" && key == "" {
		return "", errors.New("no id nor keyring given, can't mount without credentials")
	}

	// Attach and poll until volume is mapped
	logger.Infof("attaching volume %s/%s cluster %s", pool, image, clusterNamespace)
	monitors, keyring, err := getClusterInfo(vm.context, clusterNamespace)
	defer os.Remove(keyring)
	if err != nil {
		return "", errors.Wrapf(err, "failed to load cluster information from cluster %s", clusterNamespace)
	}

	// Write the user given key to the keyring file
	if key != "" {
		keyringEval := func(key string) string {
			r := fmt.Sprintf(keyringTemplate, id, key)
			return r
		}
		if err = cephconfig.WriteKeyring(keyring, key, keyringEval); err != nil {
			return "", errors.Wrapf(err, "failed writing custom keyring for id %s", id)
		}
	}

	err = cephclient.MapImage(vm.context, image, pool, id, keyring, clusterNamespace, monitors)
	if err != nil {
		return "", errors.Wrapf(err, "failed to map image %s/%s cluster %s", pool, image, clusterNamespace)
	}

	// Poll for device path
	retryCount := 0
	for {
		devicePath, err := vm.devicePathFinder.FindDevicePath(image, pool, clusterNamespace)
		if err != nil {
			return "", errors.Wrapf(err, "failed to poll for mapped image %s/%s cluster %s", pool, image, clusterNamespace)
		}

		if devicePath != "" {
			return devicePath, nil
		}

		retryCount++
		if retryCount >= findDevicePathMaxRetries {
			return "", errors.Wrapf(err, "exceeded retry count while finding device path")
		}

		logger.Infof("failed to find device path, sleeping 1 second. %v", err)
		<-time.After(time.Second)
	}
}

func (vm *VolumeManager) Expand(image, pool, clusterNamespace string, size uint64) error {
	monitors, keyring, err := getClusterInfo(vm.context, clusterNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to resize volume %s/%s cluster %s", pool, image, clusterNamespace)
	}
	err = cephclient.ExpandImage(vm.context, clusterNamespace, image, pool, monitors, keyring, size)
	if err != nil {
		return errors.Wrapf(err, "failed to resize volume %s/%s cluster %s", pool, image, clusterNamespace)
	}
	return nil
}

// Detach the volume
func (vm *VolumeManager) Detach(image, pool, id, key, clusterNamespace string, force bool) error {
	// check if the volume is attached
	devicePath, err := vm.isAttached(image, pool, clusterNamespace)
	if err != nil {
		return errors.Errorf("failed to check if volume %s/%s is attached cluster %s", pool, image, clusterNamespace)
	}
	if devicePath == "" {
		logger.Infof("volume %s/%s is already detached cluster %s", pool, image, clusterNamespace)
		return nil
	}

	if id == "" && key == "" {
		return errors.New("no id nor keyring given, can't unmount without credentials")
	}

	logger.Infof("detaching volume %s/%s cluster %s", pool, image, clusterNamespace)
	monitors, keyring, err := getClusterInfo(vm.context, clusterNamespace)
	defer os.Remove(keyring)
	if err != nil {
		return errors.Wrapf(err, "failed to load cluster information from cluster %s", clusterNamespace)
	}

	// Write the user given key to the keyring file
	if key != "" {
		keyringEval := func(key string) string {
			r := fmt.Sprintf(keyringTemplate, id, key)
			return r
		}

		if err = cephconfig.WriteKeyring(keyring, key, keyringEval); err != nil {
			return errors.Wrapf(err, "failed writing custom keyring for id %s", id)
		}
	}

	err = cephclient.UnMapImage(vm.context, image, pool, id, keyring, clusterNamespace, monitors, force)
	if err != nil {
		return errors.Wrapf(err, "failed to detach volume %s/%s cluster %s", pool, image, clusterNamespace)
	}
	logger.Infof("detached volume %s/%s", pool, image)
	return nil
}

// Check if the volume is attached
func (vm *VolumeManager) isAttached(image, pool, clusterNamespace string) (string, error) {
	devicePath, err := vm.devicePathFinder.FindDevicePath(image, pool, clusterNamespace)
	if err != nil {
		return "", err
	}
	return devicePath, nil
}

func getClusterInfo(context *clusterd.Context, clusterNamespace string) (string, string, error) {
	clusterInfo, _, _, err := mon.LoadClusterInfo(context, clusterNamespace)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to load cluster information from cluster %s", clusterNamespace)
	}

	// create temp keyring file
	keyringFile, err := ioutil.TempFile("", clusterNamespace+".keyring")
	if err != nil {
		return "", "", err
	}

	keyring := cephconfig.AdminKeyring(clusterInfo)
	if err := ioutil.WriteFile(keyringFile.Name(), []byte(keyring), 0644); err != nil {
		return "", "", errors.Errorf("failed to write monitor keyring to %s", keyringFile.Name())
	}

	monEndpoints := make([]string, 0, len(clusterInfo.Monitors))
	for _, monitor := range clusterInfo.Monitors {
		monEndpoints = append(monEndpoints, monitor.Endpoint)
	}
	return strings.Join(monEndpoints, ","), keyringFile.Name(), nil
}

// FindDevicePath polls and wait for the mapped ceph image device to show up
func (f *devicePathFinder) FindDevicePath(image, pool, clusterNamespace string) (string, error) {
	mappedFile, err := cephutil.FindRBDMappedFile(image, pool, cephutil.RBDSysBusPathDefault)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find mapped image")
	}

	if mappedFile != "" {
		devicePath := cephutil.RBDDevicePathPrefix + mappedFile
		if _, err := os.Lstat(devicePath); err != nil {
			return "", errors.Errorf("sysfs information for image %q in pool %q found but the rbd device path %s does not exist", image, pool, devicePath)
		}
		return devicePath, nil
	}
	return "", nil
}
