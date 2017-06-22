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

	cephosd "github.com/rook/rook/pkg/ceph/osd"
)

func TestStorageSpecMarshal(t *testing.T) {
	specYaml := []byte(`useAllNodes: true
useAllDevices: false
deviceFilter: "^nvme."
metadataDevice:
location: "region=us-west,datacenter=delmar"
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
	rawJson, err := yaml.YAMLToJSON(specYaml)
	assert.Nil(t, err)

	// unmarshal the JSON into a strongly typed storage spec object
	var storageSpec StorageSpec
	err = json.Unmarshal(rawJson, &storageSpec)
	assert.Nil(t, err)

	// the unmarshalled storage spec should equal the expected spec below
	expectedSpec := StorageSpec{
		UseAllNodes: true,
		Selection: Selection{
			UseAllDevices: newBool(false),
			DeviceFilter:  "^nvme.",
		},
		Config: Config{
			Location: "region=us-west,datacenter=delmar",
			StoreConfig: cephosd.StoreConfig{
				DatabaseSizeMB: 1024,
				JournalSizeMB:  1024,
				StoreType:      cephosd.Bluestore,
			},
		},
		Nodes: []Node{
			Node{
				Name:        "node1",
				Directories: []Directory{{Path: "/rook/dir1"}},
				Config: Config{
					StoreConfig: cephosd.StoreConfig{
						StoreType: cephosd.Filestore,
					},
				},
			},
			Node{
				Name: "node2",
				Selection: Selection{
					DeviceFilter: "^sd.",
				},
			},
		},
	}

	assert.Equal(t, expectedSpec, storageSpec)
}
