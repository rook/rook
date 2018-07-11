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
package v1beta1

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
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
  location: "region=us-west,datacenter=delmar"
  config:
    metadataDevice: "nvme01"
    journalSizeMB: "1024"
    databaseSizeMB: "1024"
  directories:
  - path: "/rook/dir2"
  nodes:
  - name: "node1"
    config:
      storeType: filestore
    directories:
    - path: "/rook/dir1"
  - name: "node2"
    deviceFilter: "^foo*"`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.YAMLToJSON(specYaml)
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
		Network: rookalpha.NetworkSpec{
			HostNetwork: true,
		},
		Storage: rookalpha.StorageScopeSpec{
			UseAllNodes: false,
			Location:    "region=us-west,datacenter=delmar",
			Selection: rookalpha.Selection{
				UseAllDevices: &useAllDevices,
				DeviceFilter:  "^sd.",
				Directories:   []rookalpha.Directory{{Path: "/rook/dir2"}},
			},
			Config: map[string]string{
				"metadataDevice": "nvme01",
				"journalSizeMB":  "1024",
				"databaseSizeMB": "1024",
			},
			Nodes: []rookalpha.Node{
				{
					Name: "node1",
					Selection: rookalpha.Selection{
						Directories: []rookalpha.Directory{{Path: "/rook/dir1"}},
					},
					Config: map[string]string{
						"storeType": "filestore",
					},
				},
				{
					Name: "node2",
					Selection: rookalpha.Selection{
						DeviceFilter: "^foo*",
					},
				},
			},
		},
	}

	assert.Equal(t, expectedSpec, clusterSpec)
}
