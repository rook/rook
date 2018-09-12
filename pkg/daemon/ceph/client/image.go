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
	"syscall"

	"strconv"

	"regexp"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	ImageMinSize = uint64(1048576) // 1 MB
)

type CephBlockImage struct {
	Name   string `json:"image"`
	Size   uint64 `json:"size"`
	Format int    `json:"format"`
}

func ListImages(context *clusterd.Context, clusterName, poolName string) ([]CephBlockImage, error) {
	args := []string{"ls", "-l", poolName}
	buf, err := ExecuteRBDCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to list images for pool %s: %+v", poolName, err)
	}

	//The regex expression captures the json result at the end buf
	//When logLevel is DEBUG buf contains log statements of librados (see tests for examples)
	//It can happen that the end of the "real" output doesn't not contain a new line
	//that's why looking for the end isn't an option here (anymore?)
	res := regexp.MustCompile(`(?m)^\[(.*)\]`).FindStringSubmatch(string(buf))
	if len(res) == 0 {
		return []CephBlockImage{}, nil
	}
	buf = []byte(res[0])

	var images []CephBlockImage
	if err = json.Unmarshal(buf, &images); err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v. raw buffer response: %s", err, string(buf))
	}

	return images, nil
}

func CreateImage(context *clusterd.Context, clusterName, name, poolName string, size uint64) (*CephBlockImage, error) {
	if size > 0 && size < ImageMinSize {
		// rbd tool uses MB as the smallest unit for size input.  0 is OK but anything else smaller
		// than 1 MB should just be rounded up to 1 MB.
		logger.Warningf("requested image size %d is less than the minimum size of %d, using the minimum.", size, ImageMinSize)
		size = ImageMinSize
	}

	sizeMB := int(size / 1024 / 1024)
	imageSpec := getImageSpec(name, poolName)

	args := []string{"create", imageSpec, "--size", strconv.Itoa(sizeMB)}
	buf, err := ExecuteRBDCommandNoFormat(context, clusterName, args)
	if err != nil {
		cmdErr, ok := err.(*exec.CommandError)
		if ok && cmdErr.ExitStatus() == int(syscall.EEXIST) {
			// Image with the same name already exists in the given rbd pool. Continuing with the link to PV.
			logger.Warningf("Requested image %s exists in pool %s. Continuing", name, poolName)
		} else {
			return nil, fmt.Errorf("failed to create image %s in pool %s of size %d: %+v. output: %s",
				name, poolName, size, err, string(buf))
		}
	}

	// now that the image is created, retrieve it
	images, err := ListImages(context, clusterName, poolName)
	if err != nil {
		return nil, fmt.Errorf("failed to list images after successfully creating image %s: %v", name, err)
	}
	for i := range images {
		if images[i].Name == name {
			return &images[i], nil
		}
	}

	return nil, fmt.Errorf("failed to find image %s after creating it", name)
}

func DeleteImage(context *clusterd.Context, clusterName, name, poolName string) error {
	imageSpec := getImageSpec(name, poolName)
	args := []string{"rm", imageSpec}
	buf, err := ExecuteRBDCommandNoFormat(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to delete image %s in pool %s: %+v. output: %s",
			name, poolName, err, string(buf))
	}

	return nil
}

// MapImage maps an RBD image using admin cephfx and returns the device path
func MapImage(context *clusterd.Context, imageName, poolName, clusterName, keyring, monitors string) error {
	imageSpec := getImageSpec(imageName, poolName)
	args := []string{
		"map",
		imageSpec,
		"--id", "admin",
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--keyring=%s", keyring),
		"-m", monitors,
		"--conf=/dev/null", // no config file needed because we are passing all required config as arguments
	}

	output, err := ExecuteRBDCommandWithTimeout(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to map image %s: %+v. output: %s", imageSpec, err, output)
	}

	return nil
}

// UnMapImage unmap an RBD image from the node
func UnMapImage(context *clusterd.Context, imageName, poolName, clusterName, keyring, monitors string, force bool) error {
	deviceImage := getImageSpec(imageName, poolName)
	args := []string{
		"unmap",
		deviceImage,
		"--id", "admin",
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--keyring=%s", keyring),
		"-m", monitors,
		"--conf=/dev/null", // no config file needed because we are passing all required config as arguments
	}

	if force {
		args = append(args, "-o", "force")
	}

	output, err := ExecuteRBDCommandWithTimeout(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to unmap image %s: %+v. output: %s", deviceImage, err, output)
	}

	return nil
}

func getImageSpec(name, poolName string) string {
	return fmt.Sprintf("%s/%s", poolName, name)
}
