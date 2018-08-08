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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/spf13/cobra"
	k8smount "k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/version"
	"k8s.io/kubernetes/pkg/volume/util"
)

const (
	rwMask                       = os.FileMode(0660)
	roMask                       = os.FileMode(0440)
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

	log(client, fmt.Sprintf("%#+v", args), false)
	var opts = &flexvolume.AttachOptions{}
	if err := json.Unmarshal([]byte(args[1]), opts); err != nil {
		return fmt.Errorf("Rook: Could not parse options for mounting %s. Got %v", args[1], err)
	}
	opts.MountDir = args[0]
	log(client, fmt.Sprintf("%#+v", opts), false)

	if opts.FsType == cephFS {
		return mountCephFS(client, opts)
	}

	// err = client.Call("Controller.GetAttachInfoFromMountDir", opts.MountDir, &opts)
	// if err != nil {
	// 	log(client, fmt.Sprintf("Attach volume %s/%s failed: %v", opts.Pool, opts.Image, err), true)
	// 	return fmt.Errorf("Rook: Mount volume failed: %v", err)
	// }

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

	// This code works here, but not in mount
	if opts.FsGroup != "" {
		fsGroupInt, err := strconv.Atoi(opts.FsGroup)
		fsGroup := int64(fsGroupInt)
		err = SetVolumeOwnership(client, opts.MountDir, &fsGroup)
		if err != nil {
			log(client, fmt.Sprintf("Rook: chown failed. Cannot set group to: %d, %v", fsGroup, err), true)
			return err
		}
		log(client, fmt.Sprintf("Chowned %s to %d", opts.MountDir, fsGroup), false)
		return nil
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

	// This code will run without error, but it won't actually change permissions
	// if opts.FsGroup != "" {
	// 	fsGroupInt, err := strconv.Atoi(opts.FsGroup)
	// 	fsGroup := int64(fsGroupInt)
	// 	err = SetVolumeOwnership(client, opts.MountDir, &fsGroup)
	// 	if err != nil {
	// 		log(client, fmt.Sprintf("Rook: chown failed. Cannot set group to: %d, %v", fsGroup, err), true)
	// 		return err
	// 	}
	// 	log(client, fmt.Sprintf("Chowned %s to %d", opts.MountDir, fsGroup), false)
	// 	return nil
	// }
	// log(client, fmt.Sprintf("Rook: chown failed. Cannot chown %s to: %s", opts.MountDir, opts.FsGroup), true)
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

func SetVolumeOwnership(client *rpc.Client, path string, fsGroup *int64) error {

	log(client, fmt.Sprintf("Entered SetVolumeOwnership"), false)
	if fsGroup == nil {
		return errors.New("No fsgroup???")
	}

	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		log(client, fmt.Sprintf("Processing %s with fsGroup %d, info %#+v, err %#+v", path, *fsGroup, info, err), false)
		if err != nil {
			log(client, fmt.Sprintf("Returning err %#+v", err), true)
			return err
		}

		// chown and chmod pass through to the underlying file for symlinks.
		// Symlinks have a mode of 777 but this really doesn't mean anything.
		// The permissions of the underlying file are what matter.
		// However, if one reads the mode of a symlink then chmods the symlink
		// with that mode, it changes the mode of the underlying file, overridden
		// the defaultMode and permissions initialized by the volume plugin, which
		// is not what we want; thus, we skip chown/chmod for symlinks.
		if info.Mode()&os.ModeSymlink != 0 {
			log(client, fmt.Sprintf("Skipping chown/chmod for symlinks"), true)
			return errors.New("info.Mode()&os.ModeSymlink failed or whatever")
		}

		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			log(client, fmt.Sprintf("syscall.Stat_t not ok"), true)
			return errors.New("The syscall.Stat_t thing failed")
		}

		if stat == nil {
			log(client, fmt.Sprintf("Got nil stat_t for path %v while setting ownership of volume", path), true)
			glog.Errorf("Got nil stat_t for path %v while setting ownership of volume", path)
			return fmt.Errorf("Got nil stat_t for path %v while setting ownership of volume", path)
		}

		err = os.Chown(path, int(stat.Uid), int(*fsGroup))
		if err != nil {
			log(client, fmt.Sprintf("Returning err %#+v", err), true)
			glog.Errorf("Chown failed on %v: %v", path, err)
			return err
		}

		mask := rwMask
		log(client, fmt.Sprintf("mask:  %#+v", mask), false)

		if info.IsDir() {
			mask |= os.ModeSetgid
			log(client, fmt.Sprintf(" isDir, mask changing to:  %#+v", mask), false)
		}

		err = os.Chmod(path, info.Mode()|mask)
		if err != nil {
			glog.Errorf("Chmod failed on %v: %v", path, err)
			log(client, fmt.Sprintf("Returning err %#+v", err), true)
			return err
		}

		log(client, fmt.Sprintf("path %s chowned to %d", path, *fsGroup), false)
		return nil
	})
}
