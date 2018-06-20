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

package cmd

import (
	"fmt"
	"net/rpc"

	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/spf13/cobra"
	k8smount "k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"
)

var (
	unmountCmd = &cobra.Command{
		Use:   "unmount",
		Short: "Unmounts the pod volume",
		RunE:  handleUnmount,
	}
	executor = &exec.CommandExecutor{}
)

func init() {
	RootCmd.AddCommand(unmountCmd)
}

func handleUnmount(cmd *cobra.Command, args []string) error {

	client, err := getRPCClient()
	if err != nil {
		return fmt.Errorf("Rook: Error getting RPC client: %v", err)
	}
	mountDir := args[0]

	log(client, fmt.Sprintf("unmounting mount dir: %s", mountDir), false)

	mounter := getMounter()

	// Check if it's a cephfs
	err = executor.ExecuteCommand(false, "", "df", "--type", cephFS, mountDir)
	if err == nil {
		return unmountCephFS(client, mounter, mountDir)
	}

	var opts = &flexvolume.AttachOptions{
		MountDir: args[0],
	}

	err = client.Call("Controller.GetAttachInfoFromMountDir", opts.MountDir, &opts)
	if err != nil {
		log(client, fmt.Sprintf("Unmount volume at mount dir %s failed: %v", opts.MountDir, err), true)
		return fmt.Errorf("Unmount volume at mount dir %s failed: %v", opts.MountDir, err)
	}

	// construct the input we'll need to get the global mount path
	driverDir, err := getDriverDir()
	if err != nil {
		return err
	}
	globalMountPathInput := flexvolume.GlobalMountPathInput{
		VolumeName: opts.VolumeName,
		DriverDir:  driverDir,
	}

	var globalVolumeMountPath string
	err = client.Call("Controller.GetGlobalMountPath", globalMountPathInput, &globalVolumeMountPath)
	if err != nil {
		log(client, fmt.Sprintf("Detach volume %s/%s failed. Cannot get global volume mount path: %v", opts.Pool, opts.Image, err), true)
		return fmt.Errorf("Rook: Unmount volume failed. Cannot get global volume mount path: %v", err)
	}

	safeToDetach := false
	err = redirectStdout(
		client,
		func() error {

			// Unmount pod mount dir
			if err := util.UnmountPath(opts.MountDir, mounter.Interface); err != nil {
				return fmt.Errorf("failed to unmount volume at %s: %+v", opts.MountDir, err)
			}

			// Remove attachment item from the CRD
			err = client.Call("Controller.RemoveAttachmentObject", opts, &safeToDetach)
			if err != nil {
				log(client, fmt.Sprintf("Unmount volume %s failed: %v", opts.MountDir, err), true)
				// Do not return error. Try detaching first. If error happens during detach, Kubernetes will retry.
			}

			// If safeToDetach is true, then all attachment on this node has been removed
			// Unmount global mount dir
			if safeToDetach {
				if err := util.UnmountPath(globalVolumeMountPath, mounter.Interface); err != nil {
					return fmt.Errorf("failed to unmount volume at %s: %+v", opts.MountDir, err)
				}
			}

			return nil
		},
	)
	if err != nil {
		return err
	}

	if safeToDetach {
		// call detach
		log(client, fmt.Sprintf("calling agent to detach mountDir: %s", opts.MountDir), false)
		err = client.Call("Controller.Detach", opts, nil)
		if err != nil {
			log(client, fmt.Sprintf("Detach volume from %s failed: %v", opts.MountDir, err), true)
			return fmt.Errorf("Rook: Unmount volume failed: %v", err)
		}
		log(client, fmt.Sprintf("volume has been unmounted and detached from %s", opts.MountDir), false)
	}
	log(client, fmt.Sprintf("volume has been unmounted from %s", opts.MountDir), false)
	return nil
}

func unmountCephFS(client *rpc.Client, mounter *k8smount.SafeFormatAndMount, mountDir string) error {
	// Unmount pod mount dir

	err := redirectStdout(
		client,
		func() error {
			// Unmount pod mount dir
			if err := util.UnmountPath(mountDir, mounter.Interface); err != nil {
				return fmt.Errorf("failed to unmount cephfs volume at %s: %+v", mountDir, err)
			}
			return nil
		},
	)
	if err != nil {
		log(client, fmt.Sprintf("failed to mount cephfs volume from %s: %+v", mountDir, err), true)
	} else {
		log(client, fmt.Sprintf("cephfs volume has been unmounted from %s", mountDir), false)
	}
	return err
}
