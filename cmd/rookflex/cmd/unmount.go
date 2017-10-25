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

	"github.com/rook/rook/pkg/agent/flexvolume"
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/volume/util"
)

var (
	unmountCmd = &cobra.Command{
		Use:   "unmount",
		Short: "Unmounts the pod volume",
		RunE:  handleUnmount,
	}
)

func init() {
	RootCmd.AddCommand(unmountCmd)
}

func handleUnmount(cmd *cobra.Command, args []string) error {

	client, err := getRPCClient()
	if err != nil {
		return fmt.Errorf("Rook: Error getting RPC client: %v", err)
	}

	var opts = &flexvolume.AttachOptions{
		MountDir: args[0],
	}

	log(client, fmt.Sprintf("unmounting mount dir: %s", opts.MountDir), false)

	err = client.Call("FlexvolumeController.GetAttachInfoFromMountDir", opts.MountDir, &opts)
	if err != nil {
		log(client, fmt.Sprintf("Unmount volume at mount dir %s failed: %v", opts.MountDir, err), true)
		return fmt.Errorf("Unmount volume at mount dir %s failed: %v", opts.MountDir, err)
	}

	mounter := getMounter()

	var globalVolumeMountPath string
	err = client.Call("FlexvolumeController.GetGlobalMountPath", opts.VolumeName, &globalVolumeMountPath)
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
			err = client.Call("FlexvolumeController.RemoveAttachmentObject", opts, &safeToDetach)
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
		err = client.Call("FlexvolumeController.Detach", opts, nil)
		if err != nil {
			log(client, fmt.Sprintf("Detach volume from %s failed: %v", opts.MountDir, err), true)
			return fmt.Errorf("Rook: Unmount volume failed: %v", err)
		}
		log(client, fmt.Sprintf("volume has been unmounted and detached from %s", opts.MountDir), false)
	}
	log(client, fmt.Sprintf("volume has been unmounted from %s", opts.MountDir), false)
	return nil
}
