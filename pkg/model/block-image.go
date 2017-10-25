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
package model

type BlockImage struct {
	Name       string `json:"imageName"`
	PoolName   string `json:"poolName"`
	Size       uint64 `json:"size"`
	Device     string `json:"device"`
	MountPoint string `json:"mountPoint"`
}

// DevicePathFinder is used to find the device path after the volume has been attached
type DevicePathFinder interface {
	FindDevicePath(image, pool, clusterName string) (string, error)
}
