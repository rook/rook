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
package osd

import (
	"encoding/json"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	cephosd "github.com/rook/rook/pkg/daemon/ceph/osd"
)

func TestStorageSpecMarshal(t *testing.T) {
	specYaml := []byte(`useAllNodes: true
useAllDevices: false
deviceFilter: "^nvme."
metadataDevice:
location: "region=us-west,datacenter=delmar"
directories:
- path: "/rook/dir2"
storeConfig:
  storeType: bluestore
  journalSizeMB: 1024
  databaseSizeMB: 1024
nodes:
- name: "node1"
  storeConfig:
    storeType: filestore
  directories:
  - path: "/rook/dir1"
- name: "node2"
  deviceFilter: "^sd."`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.YAMLToJSON(specYaml)
	assert.Nil(t, err)

	// unmarshal the JSON into a strongly typed storage spec object
	var storageSpec rookalpha.StorageSpec
	err = json.Unmarshal(rawJSON, &storageSpec)
	assert.Nil(t, err)

	// the unmarshalled storage spec should equal the expected spec below
	useAllDevices := false
	expectedSpec := rookalpha.StorageSpec{
		UseAllNodes: true,
		Selection: rookalpha.Selection{
			UseAllDevices: &useAllDevices,
			DeviceFilter:  "^nvme.",
			Directories:   []rookalpha.Directory{{Path: "/rook/dir2"}},
		},
		Config: rookalpha.Config{
			Location: "region=us-west,datacenter=delmar",
			StoreConfig: rookalpha.StoreConfig{
				DatabaseSizeMB: 1024,
				JournalSizeMB:  1024,
				StoreType:      cephosd.Bluestore,
			},
		},
		Nodes: []rookalpha.Node{
			{
				Name: "node1",
				Selection: rookalpha.Selection{
					Directories: []rookalpha.Directory{{Path: "/rook/dir1"}},
				},
				Config: rookalpha.Config{
					StoreConfig: rookalpha.StoreConfig{
						StoreType: cephosd.Filestore,
					},
				},
				Resources: v1.ResourceRequirements{},
			},
			{
				Name: "node2",
				Selection: rookalpha.Selection{
					DeviceFilter: "^sd.",
				},
				Resources: v1.ResourceRequirements{},
			},
		},
	}

	assert.Equal(t, expectedSpec, storageSpec)
}
