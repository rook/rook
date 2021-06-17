/*
Copyright 2020 The Rook Authors. All rights reserved.

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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

// GetDeviceClasses gets the available device classes.
func GetDeviceClasses(context *clusterd.Context, clusterInfo *ClusterInfo) ([]string, error) {
	args := []string{"osd", "crush", "class", "ls"}
	cmd := NewCephCommand(context, clusterInfo, args)
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get deviceclasses. %s", string(buf))
	}

	var deviceclass []string
	if err := json.Unmarshal(buf, &deviceclass); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal osd crush class list response")
	}

	return deviceclass, nil
}

// GetDeviceClassOSDs gets the OSDs associated with a device class.
func GetDeviceClassOSDs(context *clusterd.Context, clusterInfo *ClusterInfo, deviceClass string) ([]int, error) {
	args := []string{"osd", "crush", "class", "ls-osd", deviceClass}
	cmd := NewCephCommand(context, clusterInfo, args)
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get device class osd. %s", string(buf))
	}

	var osds []int
	if err := json.Unmarshal(buf, &osds); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal device class osd response")
	}

	return osds, nil
}
