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
	"encoding/json"
	"errors"
	"fmt"
	"net/rpc"
	"os"
	"strings"

	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/spf13/cobra"
	k8smount "k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/version"
	"k8s.io/kubernetes/pkg/volume/util"
)

const (
	mds_namespace_kernel_support = "4.7"
)

var (
	mountCmd = &cobra.Command{
		Use:   "mount",
		Short: "Mounts the volume to the pod volume",
		RunE:  handleMount,
	}
)

func init() {
	RootCmd.AddCommand(mountCmd)
}

func handleMount(cmd *cobra.Command, args []string) error {

	client, err := getRPCClient()
	if err != nil {
		return fmt.Errorf("Rook: Error getting RPC client: %v", err)
	}

	var opts = &flexvolume.AttachOptions{}
	if err := json.Unmarshal([]byte(args[1]), opts); err != nil {
		return fmt.Errorf("Rook: Could not parse options for mounting %s. Got %v", args[1], err)
	}
	opts.MountDir = args[0]

	if opts.FsType == cephFS {
		return mountCephFS(client, opts)
	}

	err = client.Call("Controller.GetAttachInfoFromMountDir", opts.MountDir, &opts)
	if err != nil {
		log(client, fmt.Sprintf("Attach volume %s/%s failed: %v", opts.Pool, opts.Image, err), true)
		return fmt.Errorf("Rook: Mount volume failed: %v", err)
	}

	// Attach volume to node
	devicePath, err := attach(client, opts)
	if err != nil {
		return err
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

	// Get global mount path
	var globalVolumeMountPath string
	err = client.Call("Controller.GetGlobalMountPath", globalMountPathInput, &globalVolumeMountPath)
	if err != nil {
		log(client, fmt.Sprintf("Attach volume %s/%s failed. Cannot get global volume mount path: %v", opts.Pool, opts.Image, err), true)
		return fmt.Errorf("Rook: Mount volume failed. Cannot get global volume mount path: %v", err)
	}

	mounter := getMounter()
	// Mount the volume to a global volume path
	err = mountDevice(client, mounter, devicePath, globalVolumeMountPath, opts)
	if err != nil {
		return err
	}

	// Mount the global mount path to pod mount dir
	err = mount(client, mounter, globalVolumeMountPath, opts)
	if err != nil {
		return err
	}
	log(client, fmt.Sprintf("volume %s/%s has been attached and mounted", opts.Pool, opts.Image), false)
	return nil
}

func attach(client *rpc.Client, opts *flexvolume.AttachOptions) (string, error) {

	log(client, fmt.Sprintf("calling agent to attach volume %s/%s", opts.Pool, opts.Image), false)
	var devicePath string
	err := client.Call("Controller.Attach", opts, &devicePath)
	if err != nil {
		log(client, fmt.Sprintf("Attach volume %s/%s failed: %v", opts.Pool, opts.Image, err), true)
		return "", fmt.Errorf("Rook: Mount volume failed: %v", err)
	}
	return devicePath, err
}

func mountDevice(client *rpc.Client, mounter *k8smount.SafeFormatAndMount, devicePath, globalVolumeMountPath string, opts *flexvolume.AttachOptions) error {
	notMnt, err := mounter.Interface.IsLikelyNotMountPoint(globalVolumeMountPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(globalVolumeMountPath, 0750); err != nil {
				return fmt.Errorf("Rook: Mount volume failed. Cannot create global volume mount path dir: %v", err)
			}
			notMnt = true
		} else {
			return fmt.Errorf("Rook: Mount volume failed. Error checking if %s is a mount point: %v", globalVolumeMountPath, err)
		}
	}
	options := []string{opts.RW}
	if notMnt {
		err = redirectStdout(
			client,
			func() error {
				if err = mounter.FormatAndMount(devicePath, globalVolumeMountPath, opts.FsType, options); err != nil {
					return fmt.Errorf("failed to mount volume %s [%s] to %s, error %v", devicePath, opts.FsType, globalVolumeMountPath, err)
				}
				return nil
			},
		)
		if err != nil {
			log(client, fmt.Sprintf("mount volume %s/%s failed: %v", opts.Pool, opts.Image, err), true)
			os.Remove(globalVolumeMountPath)
			return err
		}
		log(client,
			"Ignore error about Mount failed: exit status 32. Kubernetes does this to check whether the volume has been formatted. It will format and retry again. https://github.com/kubernetes/kubernetes/blob/release-1.7/pkg/util/mount/mount_linux.go#L360",
			false)
		log(client, fmt.Sprintf("formatting volume %v devicePath %v deviceMountPath %v fs %v with options %+v", opts.VolumeName, devicePath, globalVolumeMountPath, opts.FsType, options), false)
	}
	return nil
}

func mount(client *rpc.Client, mounter *k8smount.SafeFormatAndMount, globalVolumeMountPath string, opts *flexvolume.AttachOptions) error {

	log(client, fmt.Sprintf("mounting global mount path %s on %s", globalVolumeMountPath, opts.MountDir), false)
	// Perform a bind mount to the full path to allow duplicate mounts of the same volume. This is only supported for RO attachments.
	options := []string{opts.RW, "bind"}
	err := redirectStdout(
		client,
		func() error {
			err := mounter.Interface.Mount(globalVolumeMountPath, opts.MountDir, "", options)
			if err != nil {
				notMnt, mntErr := mounter.Interface.IsLikelyNotMountPoint(opts.MountDir)
				if mntErr != nil {
					return fmt.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
				}
				if !notMnt {
					if mntErr = mounter.Interface.Unmount(opts.MountDir); mntErr != nil {
						return fmt.Errorf("Failed to unmount: %v", mntErr)
					}
					notMnt, mntErr := mounter.Interface.IsLikelyNotMountPoint(opts.MountDir)
					if mntErr != nil {
						return fmt.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
					}
					if !notMnt {
						// This is very odd, we don't expect it.  We'll try again next sync loop.
						return fmt.Errorf("%s is still mounted, despite call to unmount().  Will try again next sync loop", opts.MountDir)
					}
				}
				os.Remove(opts.MountDir)
				return fmt.Errorf("failed to mount volume %s to %s, error %v", globalVolumeMountPath, opts.MountDir, err)
			}
			return nil
		},
	)
	if err != nil {
		log(client, fmt.Sprintf("mount volume %s/%s failed: %v", opts.Pool, opts.Image, err), true)
	}
	return err
}

func mountCephFS(client *rpc.Client, opts *flexvolume.AttachOptions) error {

	if opts.FsName == "" {
		return errors.New("Rook: Attach filesystem failed: Filesystem name is not provided")
	}

	log(client, fmt.Sprintf("mounting ceph filesystem %s on %s", opts.FsName, opts.MountDir), false)

	if opts.ClusterNamespace == "" {
		if opts.ClusterName == "" {
			return fmt.Errorf("Rook: Attach filesystem %s failed: cluster namespace is not provided", opts.FsName)
		} else {
			opts.ClusterNamespace = opts.ClusterName
		}
	}

	// Get client access info
	var clientAccessInfo flexvolume.ClientAccessInfo
	err := client.Call("Controller.GetClientAccessInfo", opts.ClusterNamespace, &clientAccessInfo)
	if err != nil {
		errorMsg := fmt.Sprintf("Attach filesystem %s on cluster %s failed: %v", opts.FsName, opts.ClusterNamespace, err)
		log(client, errorMsg, true)
		return fmt.Errorf("Rook: %v", errorMsg)
	}

	// if a path has not been provided, just use the root of the filesystem.
	// otherwise, ensure that the provided path starts with the path separator char.
	path := string(os.PathSeparator)
	if opts.Path != "" {
		path = opts.Path
		if !strings.HasPrefix(path, string(os.PathSeparator)) {
			path = string(os.PathSeparator) + path
		}
	}

	options := []string{fmt.Sprintf("name=%s", clientAccessInfo.UserName), fmt.Sprintf("secret=%s", clientAccessInfo.SecretKey)}

	// Get kernel version
	var kernelVersion string
	err = client.Call("Controller.GetKernelVersion", struct{}{} /* no inputs */, &kernelVersion)
	if err != nil {
		log(client, fmt.Sprintf("WARNING: The node kernel version cannot be detected. The kernel version has to be at least %s in order to specify a filesystem namespace."+
			" If you have multiple ceph filesystems, the result could be inconsistent", mds_namespace_kernel_support), false)
	} else {
		kernelVersionParsed, err := version.ParseGeneric(kernelVersion)
		if err != nil {
			log(client, fmt.Sprintf("WARNING: The node kernel version %s cannot be parsed. The kernel version has to be at least %s in order to specify a filesystem namespace."+
				" If you have multiple ceph filesystems, the result could be inconsistent", kernelVersion, mds_namespace_kernel_support), false)
		} else {
			if kernelVersionParsed.AtLeast(version.MustParseGeneric(mds_namespace_kernel_support)) {
				options = append(options, fmt.Sprintf("mds_namespace=%s", opts.FsName))
			} else {
				log(client,
					fmt.Sprintf("WARNING: The node kernel version is %s, which do not support multiple ceph filesystems. "+
						"The kernel version has to be at least %s. If you have multiple ceph filesystems, the result could be inconsistent",
						kernelVersion, mds_namespace_kernel_support), false)
			}
		}
	}

	devicePath := fmt.Sprintf("%s:%s", strings.Join(clientAccessInfo.MonAddresses, ","), path)

	log(client, fmt.Sprintf("mounting ceph filesystem %s on %s to %s", opts.FsName, devicePath, opts.MountDir), false)
	mounter := getMounter()
	err = redirectStdout(
		client,
		func() error {

			notMnt, err := mounter.Interface.IsLikelyNotMountPoint(opts.MountDir)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if !notMnt {
				// Directory is already mounted
				return nil
			}
			os.MkdirAll(opts.MountDir, 0750)

			err = mounter.Interface.Mount(devicePath, opts.MountDir, cephFS, options)
			if err != nil {
				// cleanup upon failure
				util.UnmountPath(opts.MountDir, mounter.Interface)
				return fmt.Errorf("failed to mount filesystem %s to %s with monitor %s and options %v: %+v", opts.FsName, opts.MountDir, devicePath, options, err)
			}
			return nil
		},
	)
	if err != nil {
		log(client, err.Error(), true)
	} else {
		log(client, fmt.Sprintf("ceph filesystem %s has been attached and mounted", opts.FsName), false)
	}

	return err
}
