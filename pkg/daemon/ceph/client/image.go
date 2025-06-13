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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
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
