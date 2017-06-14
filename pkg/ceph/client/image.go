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

	"strconv"

	"github.com/rook/rook/pkg/clusterd"
)

type Image interface {
	Open(args ...interface{}) error
	Close() error
	Remove() error
	Stat() (info *ImageInfo, err error)
	Name() string
}

type ImageInfo struct {
	Size              uint64
	Obj_size          uint64
	Num_objs          uint64
	Order             int
	Block_name_prefix string
	Parent_pool       int64
	Parent_name       string
}

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

	var images []CephBlockImage
	err = json.Unmarshal(buf, &images)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, string(buf))
	}

	return images, nil
}

func CreateImage(context *clusterd.Context, clusterName, name, poolName string, size uint64) (*CephBlockImage, error) {
	imageSpec := getImageSpec(name, poolName)
	sizeMB := int(size / 1024 / 1024)
	if sizeMB <= 0 {
		return nil, fmt.Errorf("invalid size: %d", size)
	}

	args := []string{"create", imageSpec, "--size", strconv.Itoa(sizeMB)}
	buf, err := ExecuteRBDCommandNoFormat(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to create image %s in pool %s of size %d: %+v. output: %s",
			name, poolName, size, err, string(buf))
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

func getImageSpec(name, poolName string) string {
	return fmt.Sprintf("%s/%s", poolName, name)
}
