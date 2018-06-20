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
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephmon "github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	findDevicePathMaxRetries = 10
	rbdKernelModuleName      = "rbd"
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
func NewVolumeManager(context *clusterd.Context) *VolumeManager {
	vm := &VolumeManager{
		context:          context,
		devicePathFinder: &devicePathFinder{},
	}
	vm.Init()
	return vm
}

// Init the ceph volume manager
func (vm *VolumeManager) Init() error {

	// check to see if the rbd kernel module has single_major support
	hasSingleMajor, err := sys.CheckKernelModuleParam(rbdKernelModuleName, "single_major", vm.context.Executor)
	if err != nil {
		logger.Noticef("failed %s single_major check, assuming it's unsupported: %+v", rbdKernelModuleName, err)
		hasSingleMajor = false
	}

	opts := []string{}
	if hasSingleMajor {
		opts = append(opts, "single_major=Y")
	}

	// load the rbd kernel module with options
	// TODO: should this fail if modprobe fails?
	if err := sys.LoadKernelModule(rbdKernelModuleName, opts, vm.context.Executor); err != nil {
		logger.Noticef("failed to load kernel module %s: %+v", rbdKernelModuleName, err)
	}

	return nil
}

// Attach a ceph image to the node
func (vm *VolumeManager) Attach(image, pool, clusterNamespace string) (string, error) {

	// check if the volume is already attached
	devicePath, err := vm.isAttached(image, pool, clusterNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to check if volume %s/%s is already attached: %+v", pool, image, err)
	}
	if devicePath != "" {
		logger.Infof("volume %s/%s is already attached. The device path is %s", pool, image, devicePath)
		return devicePath, nil
	}

	// attach and poll until volume is mapped
	logger.Infof("attaching volume %s/%s cluster %s", pool, image, clusterNamespace)
	monitors, keyring, err := getClusterInfo(vm.context, clusterNamespace)
	defer os.Remove(keyring)
	if err != nil {
		return "", fmt.Errorf("failed to load cluster information from cluster %s: %+v", clusterNamespace, err)
	}

	err = cephclient.MapImage(vm.context, image, pool, clusterNamespace, keyring, monitors)
	if err != nil {
		return "", fmt.Errorf("failed to map image %s/%s cluster %s. %+v", pool, image, clusterNamespace, err)
	}

	// poll for device path
	retryCount := 0
	for {
		devicePath, err := vm.devicePathFinder.FindDevicePath(image, pool, clusterNamespace)
		if err != nil {
			return "", fmt.Errorf("failed to poll for mapped image %s/%s cluster %s. %+v", pool, image, clusterNamespace, err)
		}

		if devicePath != "" {
			return devicePath, nil
		}

		retryCount++
		if retryCount >= findDevicePathMaxRetries {
			return "", fmt.Errorf("exceeded retry count while finding device path: %+v", err)
		}

		logger.Infof("failed to find device path, sleeping 1 second: %+v", err)
		<-time.After(time.Second)
	}
}

// Detach the volume
func (vm *VolumeManager) Detach(image, pool, clusterNamespace string, force bool) error {
	// check if the volume is attached
	devicePath, err := vm.isAttached(image, pool, clusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to check if volume %s/%s is attached cluster %s", pool, image, clusterNamespace)
	}
	if devicePath == "" {
		logger.Infof("volume %s/%s is already detached cluster %s", pool, image, clusterNamespace)
		return nil
	}

	logger.Infof("detaching volume %s/%s cluster %s", pool, image, clusterNamespace)
	monitors, keyring, err := getClusterInfo(vm.context, clusterNamespace)
	defer os.Remove(keyring)
	if err != nil {
		return fmt.Errorf("failed to load cluster information from cluster %s: %+v", clusterNamespace, err)
	}

	err = cephclient.UnMapImage(vm.context, image, pool, clusterNamespace, keyring, monitors, force)
	if err != nil {
		return fmt.Errorf("failed to detach volume %s/%s cluster %s. %+v", pool, image, clusterNamespace, err)
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
		return "", "", fmt.Errorf("failed to load cluster information from cluster %s: %+v", clusterNamespace, err)
	}

	// create temp keyring file
	keyringFile, err := ioutil.TempFile("", clusterNamespace+".keyring")
	if err != nil {
		return "", "", err
	}

	keyring := fmt.Sprintf(cephmon.AdminKeyringTemplate, clusterInfo.AdminSecret)
	if err := ioutil.WriteFile(keyringFile.Name(), []byte(keyring), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write monitor keyring to %s: %+v", keyringFile.Name(), err)
	}

	monEndpoints := make([]string, 0, len(clusterInfo.Monitors))
	for _, monitor := range clusterInfo.Monitors {
		monEndpoints = append(monEndpoints, monitor.Endpoint)
	}
	return strings.Join(monEndpoints, ","), keyringFile.Name(), nil
}

// FindDevicePath polls and wait for the mapped ceph image device to show up
func (f *devicePathFinder) FindDevicePath(image, pool, clusterNamespace string) (string, error) {
	mappedFile, err := util.FindRBDMappedFile(image, pool, util.RBDSysBusPathDefault)
	if err != nil {
		return "", fmt.Errorf("failed to find mapped image: %+v", err)
	}

	if mappedFile != "" {
		devicePath := util.RBDDevicePathPrefix + mappedFile
		if _, err := os.Lstat(devicePath); err != nil {
			return "", fmt.Errorf("sysfs information for image '%s' in pool '%s' found but the rbd device path %s does not exist", image, pool, devicePath)
		}
		return devicePath, nil
	}
	return "", nil
}
