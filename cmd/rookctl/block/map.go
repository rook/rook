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
package block

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	rc "github.com/rook/rook/cmd/rookctl/client"
	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/ceph/util"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/spf13/cobra"
)

const (
	rbdKernelModuleName      = "rbd"
	devicePathPrefix         = "/dev/"
	rbdAddNode               = "add"
	rbdAddSingleMajorNode    = "add_single_major"
	rbdRemoveNode            = "remove"
	rbdRemoveSingleMajorNode = "remove_single_major"
)

var (
	mapImageName       string
	mapImagePoolName   string
	mapImagePath       string
	mapFormatRequested bool
)

var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Maps a block image from the cluster as a local block device and optionally formats and mounts it with the given file system path",
}

func init() {
	mapCmd.Flags().StringVar(&mapImageName, "name", "", "Name of block image to map (required)")
	mapCmd.Flags().StringVar(&mapImagePoolName, "pool-name", "rbd", "Name of storage pool that contains block image to map")
	mapCmd.Flags().BoolVar(&mapFormatRequested, "format", false, "Format a filesystem after mapping")
	mapCmd.Flags().StringVar(&mapImagePath, "mount", "", "Mount a filesystem on the indicated path")

	mapCmd.MarkFlagRequired("name")

	mapCmd.RunE = mapBlockEntry
}

func mapBlockEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name"}); err != nil {
		return err
	}
	c := rook.NewRookNetworkRestClient()
	e := &exec.CommandExecutor{}
	out, err := mapBlock(mapImageName, mapImagePoolName, mapImagePath, util.RBDSysBusPathDefault, util.RBDDevicePathPrefix, mapFormatRequested, c, e)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func mapBlock(name, poolName, mountPoint, rbdSysBusPath, rbdDevicePathPrefix string, formatRequested bool, c client.RookRestClient, executor exec.Executor) (string, error) {
	clientAccessInfo, err := c.GetClientAccessInfo()
	if err != nil {
		return "", err
	}

	hasSingleMajor := checkRBDSingleMajor(executor)

	var options []string
	if hasSingleMajor {
		options = []string{"single_major=Y"}
	}

	// load the rbd kernel module with options
	if err := sys.LoadKernelModule(rbdKernelModuleName, options, executor); err != nil {
		return "", err
	}

	addSingleMajorPath := filepath.Join(rbdSysBusPath, rbdAddSingleMajorNode)
	addPath := filepath.Join(rbdSysBusPath, rbdAddNode)

	addFile, err := openRBDFile(hasSingleMajor, addSingleMajorPath, addPath)
	if err != nil {
		return "", err
	}
	defer addFile.Close()

	// generate the data string that will be written to the rbd add path
	rbdAddData, err := getRBDAddData(name, poolName, clientAccessInfo)
	if err != nil {
		return "", fmt.Errorf("failed to generate rbd add data: %+v", err)
	}

	// write the rbd data string to the rbd add path
	if _, err := addFile.Write([]byte(rbdAddData)); err != nil {
		return "", fmt.Errorf("failed to write rbd add data: %+v", err)
	}

	// wait for the device to become available so we can find out its name/ID
	devicePath, err := waitForDevicePath(name, poolName, rbdSysBusPath, rbdDevicePathPrefix, 10, 1)
	if err != nil {
		return "", err
	}

	successMessage := fmt.Sprintf("succeeded mapping image %s on device %s", name, devicePath)

	if formatRequested {
		// format the device with a default file system
		if err := sys.FormatDevice(devicePath, executor); err != nil {
			return "", fmt.Errorf("%s but failed to format device %s: %+v", successMessage, devicePath, err)
		}
		successMessage += ", formatted"
	}

	if len(mountPoint) > 0 {
		// mount the device at the given mount point
		if err := sys.MountDevice(devicePath, mountPoint, executor); err != nil {
			return "", fmt.Errorf("%s but failed to mount device %s at '%s': %+v", successMessage, devicePath, mountPoint, err)
		}
		successMessage += fmt.Sprintf(", and mounted at %s", mountPoint)
	}

	return successMessage, nil
}

func getRBDAddData(name, poolName string, clientAccessInfo model.ClientAccessInfo) (string, error) {
	if err := rc.VerifyClientAccessInfo(clientAccessInfo); err != nil {
		return "", err
	}

	monAddrs := rc.ProcessMonAddresses(clientAccessInfo)

	// mon address list (comma separated), user name, secret, pool name, image name
	rbdAddData := fmt.Sprintf(
		"%s name=%s,secret=%s %s %s",
		strings.Join(monAddrs, ","),
		clientAccessInfo.UserName,
		clientAccessInfo.SecretKey,
		poolName,
		name)

	return rbdAddData, nil
}

func checkRBDSingleMajor(executor exec.Executor) bool {
	// check to see if the rbd kernel module has single_major support
	hasSingleMajor, err := sys.CheckKernelModuleParam(rbdKernelModuleName, "single_major", executor)
	if err != nil {
		logger.Noticef("failed %s single_major check, assuming it's unsupported: %+v", rbdKernelModuleName, err)
		hasSingleMajor = false
	}

	return hasSingleMajor
}

func openRBDFile(hasSingleMajor bool, singleMajorPath, path string) (*os.File, error) {
	var fd *os.File
	var err error

	// attempt to open single_major if its supported, but fall back if needed
	if hasSingleMajor {
		fd, err = os.OpenFile(singleMajorPath, os.O_WRONLY, 0200)
		if err != nil {
			logger.Noticef("failed to open %s, falling back to %s: %+v", singleMajorPath, path, err)
			fd = nil
		}
	}

	// still don't have an open file handle, try the regular path
	if fd == nil {
		fd, err = os.OpenFile(path, os.O_WRONLY, 0200)
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %+v", path, err)
		}
	}

	return fd, nil
}

func waitForDevicePath(imageName, poolName, rbdSysBusPath, rbdDevicePathPrefix string, maxRetries, sleepSecs int) (string, error) {
	retryCount := 0
	for {
		mappedFile, err := util.FindRBDMappedFile(imageName, poolName, rbdSysBusPath)
		if err != nil {
			return "", fmt.Errorf("failed to find mapped image: %+v", err)
		}

		if mappedFile != "" {
			devicePath := rbdDevicePathPrefix + mappedFile
			if _, err := os.Lstat(devicePath); err != nil {
				return "", fmt.Errorf("sysfs information for image '%s' in pool '%s' found but the rbd device path %s does not exist", imageName, poolName, devicePath)
			}
			return devicePath, nil
		}

		retryCount++
		if retryCount >= maxRetries {
			return "", fmt.Errorf("exceeded retry count while finding device path: %+v", err)
		}

		logger.Noticef("failed to find device path, sleeping %d seconds: %+v", sleepSecs, err)
		<-time.After(time.Duration(sleepSecs) * time.Second)
	}
}
