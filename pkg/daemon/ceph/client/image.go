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
package client

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/display"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	MiB          = uint64(1048576) // 1 MiB
	ImageMinSize = MiB
)

type CephBlockImage struct {
	ID       string `json:"id"`
	Name     string `json:"image"`
	Size     uint64 `json:"size"`
	Format   int    `json:"format"`
	InfoName string `json:"name"`
}

type CephBlockImageSnapshot struct {
	Name string `json:"name"`
}

// ListImagesInPool returns a list of images created in a cephblockpool
func ListImagesInPool(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) ([]CephBlockImage, error) {
	return ListImagesInRadosNamespace(context, clusterInfo, poolName, "")
}

// ListImagesInRadosNamespace returns a list if images created in a cephblockpool rados namespace
func ListImagesInRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, namespace string) ([]CephBlockImage, error) {
	args := []string{"ls", "-l", poolName}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list images for pool %s", poolName)
	}

	// The regex expression captures the json result at the end buf
	// When logLevel is DEBUG buf contains log statements of librados (see tests for examples)
	// It can happen that the end of the "real" output doesn't not contain a new line
	// that's why looking for the end isn't an option here (anymore?)
	res := regexp.MustCompile(`(?m)^\[(.*)\]`).FindStringSubmatch(string(buf))
	if len(res) == 0 {
		return []CephBlockImage{}, nil
	}
	buf = []byte(res[0])

	var images []CephBlockImage
	if err = json.Unmarshal(buf, &images); err != nil {
		return nil, errors.Wrapf(err, "unmarshal failed, raw buffer response: %s", string(buf))
	}

	return images, nil
}

// ListSnapshotsInRadosNamespace lists all the snapshots created for an image in a cephblockpool in a given rados namespace
func ListSnapshotsInRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, imageName, namespace string) ([]CephBlockImageSnapshot, error) {
	snapshots := []CephBlockImageSnapshot{}
	args := []string{"snap", "ls", getImageSpec(imageName, poolName)}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true
	buf, err := cmd.Run()
	if err != nil {
		return snapshots, errors.Wrapf(err, "failed to list snapshots of image %q in cephblockpool %q", imageName, poolName)
	}

	if err = json.Unmarshal(buf, &snapshots); err != nil {
		return snapshots, errors.Wrapf(err, "unmarshal failed, raw buffer response: %s", string(buf))
	}
	return snapshots, nil
}

// DeleteSnapshotInRadosNamespace deletes a image snapshot created in block pool in a given rados namespace
func DeleteSnapshotInRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, imageName, snapshot, namespace string) error {
	args := []string{"snap", "rm", getImageSnapshotSpec(poolName, imageName, snapshot)}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	cmd := NewRBDCommand(context, clusterInfo, args)
	_, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete snapshot %q of image %q in cephblockpool %q", snapshot, imageName, poolName)
	}
	return nil
}

// MoveImageToTrashInRadosNamespace moves the cephblockpool image to trash in the rados namespace
func MoveImageToTrashInRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, imageName, namespace string) error {
	args := []string{"trash", "mv", getImageSpec(imageName, poolName)}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	cmd := NewRBDCommand(context, clusterInfo, args)
	_, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to move image %q in cephblockpool %q to trash", imageName, poolName)
	}
	return nil
}

// DeleteImageFromTrashInRadosNamespace deletes the image from trash in the rados namespace
func DeleteImageFromTrashInRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, imageID, namespace string) error {
	args := []string{"rbd", "task", "add", "trash", "remove", getImageSpecInRadosNamespace(poolName, namespace, imageID)}
	cmd := NewCephCommand(context, clusterInfo, args)
	_, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete image %q in cephblockpool %q from trash", imageID, poolName)
	}
	return nil
}

// for a given size in bytes, round up to the nearest number of Mibibytes,
// i. e. return the smallest  number of Mibibytes larger than or equal to size.
func roundupSizeMiB(size uint64) uint64 {
	sizeMiB := (size + MiB - 1) / MiB
	return sizeMiB
}

// CreateImage creates a block storage image.
// If dataPoolName is not empty, the image will use poolName as the metadata pool and the dataPoolname for data.
// If size is zero an empty image will be created. Otherwise, an image will be
// created with a size rounded up to the nearest Mi. The adjusted image size is
// placed in return value CephBlockImage.Size.
func CreateImage(context *clusterd.Context, clusterInfo *ClusterInfo, name, poolName, dataPoolName string, size uint64) (*CephBlockImage, error) {
	if size > 0 && size < ImageMinSize {
		// rbd tool uses MB as the smallest unit for size input.  0 is OK but anything else smaller
		// than 1 MB should just be rounded up to 1 MB.
		logger.Warningf("requested image size %d is less than the minimum size of %d, using the minimum.", size, ImageMinSize)
		size = ImageMinSize
	}

	// Roundup the size of the volume image since we only create images on 1MB boundaries and we should never create an image
	// size that's smaller than the requested one, e.g, requested 1048698 bytes should be 2MB while not be truncated to 1MB
	sizeMB := roundupSizeMiB(size)

	imageSpec := getImageSpec(name, poolName)

	args := []string{"create", imageSpec, "--size", fmt.Sprintf("%d", sizeMB)}

	if dataPoolName != "" {
		args = append(args, fmt.Sprintf("--data-pool=%s", dataPoolName))
	}
	logger.Infof("creating rbd image %q with size %dMB in pool %q", imageSpec, sizeMB, dataPoolName)

	buf, err := NewRBDCommand(context, clusterInfo, args).Run()
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.EEXIST) {
			// Image with the same name already exists in the given rbd pool. Continuing with the link to PV.
			logger.Warningf("Requested image %s exists in pool %s. Continuing", name, poolName)
		} else {
			return nil, errors.Wrapf(err, "failed to create image %s in pool %s of size %d, output: %s",
				name, poolName, size, string(buf))
		}
	}

	// report the adjusted size which will always be >= to the requested size
	var newSizeBytes uint64
	if sizeMB > 0 {
		newSizeBytes = display.MbTob(uint64(sizeMB))
	} else {
		newSizeBytes = 0
	}

	return &CephBlockImage{Name: name, Size: newSizeBytes}, nil
}

func DeleteImageInPool(context *clusterd.Context, clusterInfo *ClusterInfo, name, poolName string) error {
	return DeleteImageInRadosNamespace(context, clusterInfo, name, poolName, "")
}

func DeleteImageInRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, name, poolName, namespace string) error {
	logger.Infof("deleting rbd image %q from pool %q", name, poolName)
	imageSpec := getImageSpec(name, poolName)
	args := []string{"rm", imageSpec}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	buf, err := NewRBDCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete image %s in pool %s, output: %s",
			name, poolName, string(buf))
	}
	return nil
}

func ExpandImage(context *clusterd.Context, clusterInfo *ClusterInfo, name, poolName, monitors, keyring string, size uint64) error {
	logger.Infof("expanding rbd image %q in pool %q to size %dMB", name, poolName, display.BToMb(size))
	imageSpec := getImageSpec(name, poolName)
	args := []string{
		"resize",
		imageSpec,
		fmt.Sprintf("--size=%s", strconv.FormatUint(size, 10)),
		fmt.Sprintf("--cluster=%s", clusterInfo.Namespace),
		fmt.Sprintf("--keyring=%s", keyring),
		"-m", monitors,
	}
	output, err := ExecuteRBDCommandWithTimeout(context, args)
	if err != nil {
		return errors.Wrapf(err, "failed to resize image %s in pool %s, output: %s", name, poolName, string(output))
	}
	return nil
}

// MapImage maps an RBD image using admin cephfx and returns the device path
func MapImage(context *clusterd.Context, clusterInfo *ClusterInfo, imageName, poolName, id, keyring, monitors string) error {
	imageSpec := getImageSpec(imageName, poolName)
	args := []string{
		"map",
		imageSpec,
		fmt.Sprintf("--id=%s", id),
		fmt.Sprintf("--cluster=%s", clusterInfo.Namespace),
		fmt.Sprintf("--keyring=%s", keyring),
		"-m", monitors,
		"--conf=/dev/null", // no config file needed because we are passing all required config as arguments
	}

	output, err := ExecuteRBDCommandWithTimeout(context, args)
	if err != nil {
		return errors.Wrapf(err, "failed to map image %s, output: %s", imageSpec, output)
	}

	return nil
}

// UnMapImage unmap an RBD image from the node
func UnMapImage(context *clusterd.Context, clusterInfo *ClusterInfo, imageName, poolName, id, keyring, monitors string, force bool) error {
	deviceImage := getImageSpec(imageName, poolName)
	args := []string{
		"unmap",
		deviceImage,
		fmt.Sprintf("--id=%s", id),
		fmt.Sprintf("--cluster=%s", clusterInfo.Namespace),
		fmt.Sprintf("--keyring=%s", keyring),
		"-m", monitors,
		"--conf=/dev/null", // no config file needed because we are passing all required config as arguments
	}

	if force {
		args = append(args, "-o", "force")
	}

	output, err := ExecuteRBDCommandWithTimeout(context, args)
	if err != nil {
		return errors.Wrapf(err, "failed to unmap image %s, output: %s", deviceImage, output)
	}

	return nil
}

func getImageSpec(name, poolName string) string {
	return fmt.Sprintf("%s/%s", poolName, name)
}

func getImageSpecInRadosNamespace(poolName, namespace, imageID string) string {
	if namespace == "" {
		return fmt.Sprintf("%s/%s", poolName, imageID)
	}
	return fmt.Sprintf("%s/%s/%s", poolName, namespace, imageID)
}

func getImageSnapshotSpec(poolName, imageName, snapshot string) string {
	return fmt.Sprintf("%s/%s@%s", poolName, imageName, snapshot)
}

type RBDStatus struct {
	Watchers []struct {
		Address string `json:"address"`
	} `json:"watchers"`
}

// GetWatchers returns a list of watchers of the RBD image.
// The RBD status output is parsed to get the desired IP+nonce.
// For example: `192.168.39.137:0/3762982934“ is returned.
func (r RBDStatus) GetWatchers() []string {
	watcherList := []string{}
	for _, watcher := range r.Watchers {
		watcherList = append(watcherList, watcher.Address)
	}
	return watcherList
}

// GetRBDImageStatus returns the status of the RDB image.
func GetRBDImageStatus(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, imageName, namespace string) (RBDStatus, error) {
	args := []string{"status", getImageSpec(imageName, poolName)}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	var rbdStatusObj RBDStatus
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true
	buf, err := cmd.Run()
	if err != nil {
		return rbdStatusObj, errors.Wrapf(err, "failed to get status of the image %q in cephblockpool %q", imageName, poolName)
	}

	err = json.Unmarshal(buf, &rbdStatusObj)
	if err != nil {
		return rbdStatusObj, errors.Wrap(err, "failed to unmarshal rbd status output")
	}

	return rbdStatusObj, nil
}
