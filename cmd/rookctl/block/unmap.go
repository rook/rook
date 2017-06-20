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

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/spf13/cobra"
)

var (
	unmapDeviceName string
	unmapPath       string
)

var unmapCmd = &cobra.Command{
	Use:   "unmap",
	Short: "Unmap and destroys a block device from the local machine",
}

func init() {
	unmapCmd.Flags().StringVar(&unmapDeviceName, "device", "", "Name of device to unmap (e.g., rbd0)")
	unmapCmd.Flags().StringVar(&unmapPath, "mount", "", "File system mount point to unmount before unmapping")

	unmapCmd.RunE = unmapBlockEntry
}

func unmapBlockEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	e := &exec.CommandExecutor{}
	out, err := unmapBlock(unmapDeviceName, unmapPath, rbdSysBusPathDefault, e)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func unmapBlock(device, path, rbdSysBusPath string, executor exec.Executor) (string, error) {
	if device == "" && path == "" {
		return "", fmt.Errorf("device and path are not specified, one of them must be specified")
	}

	if device == "" && path != "" {
		// we only have the mount path, convert that into a device name
		var err error
		device, err = sys.GetDeviceFromMountPoint(path, executor)
		if err != nil || device == "" {
			return "", fmt.Errorf("failed to get device from mount point %s: %+v", path, err)
		}
	}

	if device != "" && path == "" {
		// we only have the device name, convert that into its mount point, an error here is OK,
		// we won't use the path from here on it except for logging
		path, _ = sys.GetDeviceMountPoint(strings.TrimPrefix(device, devicePathPrefix), executor)
	}

	// ensure the device path is fully rooted in the /dev tree (user may have supplied "rbd0" instead of "/dev/rbd0")
	if !strings.HasPrefix(device, devicePathPrefix) {
		device = filepath.Join(devicePathPrefix, device)
	}

	// unmount the device from the file system before attempting to remove it
	if err := sys.UnmountDevice(device, executor); err != nil {
		return "", fmt.Errorf("failed to unmount rbd device %s: %+v", device, err)
	}

	rbdNum := strings.TrimPrefix(device, rbdDevicePathPrefix)
	logger.Infof("removing rbd device %s (%s)", rbdNum, device)

	// determine if the rbd kernel module supports single_major and open the
	// correct file handle to write the rbd remove command to
	hasSingleMajor := checkRBDSingleMajor(executor)
	removeSingleMajorPath := filepath.Join(rbdSysBusPath, rbdRemoveSingleMajorNode)
	removePath := filepath.Join(rbdSysBusPath, rbdRemoveNode)
	removeFile, err := openRBDFile(hasSingleMajor, removeSingleMajorPath, removePath)
	if err != nil {
		return "", err
	}
	defer removeFile.Close()

	// note we don't load the kernel module here because if it's not loaded then there
	// shouldn't be any rbd devices to unmount

	// write the remove command to the rbd remove file handle
	if _, err := removeFile.Write([]byte(rbdNum)); err != nil {
		return "", fmt.Errorf("failed to write rbd remove data: %+v", err)
	}

	return fmt.Sprintf("succeeded removing rbd device %s from '%s'", device, path), nil
}
