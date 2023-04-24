/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package v1

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestClusterSpecMarshal(t *testing.T) {
	specYaml := []byte(`
dataDirHostPath: /var/lib/rook
mon:
  count: 5
  allowMultiplePerNode: false
network:
  hostNetwork: true
storage:
  useAllNodes: false
  useAllDevices: false
  deviceFilter: "^sd."
  devicePathFilter: "^/dev/disk/by-path/pci-.*"
  location: "region=us-west,datacenter=delmar"
  config:
    metadataDevice: "nvme01"
    databaseSizeMB: "1024"
  nodes:
  - name: "node2"
    deviceFilter: "^foo*"
    devicePathFilter: "^/dev/disk/by-id/.*foo.*"`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.ToJSON(specYaml)
	assert.Nil(t, err)
	fmt.Printf("rawJSON: %s\n", string(rawJSON))

	// unmarshal the JSON into a strongly typed storage spec object
	var clusterSpec ClusterSpec
	err = json.Unmarshal(rawJSON, &clusterSpec)
	assert.Nil(t, err)

	// the unmarshalled storage spec should equal the expected spec below
	useAllDevices := false
	expectedSpec := ClusterSpec{
		Mon: MonSpec{
			Count:                5,
			AllowMultiplePerNode: false,
		},
		DataDirHostPath: "/var/lib/rook",
		Network: NetworkSpec{
			HostNetwork: true,
		},
		Storage: StorageScopeSpec{
			UseAllNodes: false,
			Selection: Selection{
				UseAllDevices:    &useAllDevices,
				DeviceFilter:     "^sd.",
				DevicePathFilter: "^/dev/disk/by-path/pci-.*",
			},
			Config: map[string]string{
				"metadataDevice": "nvme01",
				"databaseSizeMB": "1024",
			},
			Nodes: []Node{
				{
					Name: "node2",
					Selection: Selection{
						DeviceFilter:     "^foo*",
						DevicePathFilter: "^/dev/disk/by-id/.*foo.*",
					},
				},
			},
		},
	}

	assert.Equal(t, expectedSpec, clusterSpec)
}
