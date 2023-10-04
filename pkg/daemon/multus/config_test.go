/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package multus

import (
	_ "embed"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidationTestConfig_YAML(t *testing.T) {
	emptyValidationTestConfig := &ValidationTestConfig{}

	type test struct {
		name   string
		config *ValidationTestConfig
	}
	tests := []test{
		{"empty config", emptyValidationTestConfig},
		{"default config", NewDefaultValidationTestConfig()},
		{"full config", &ValidationTestConfig{
			Namespace:       "my-rook",
			PublicNetwork:   "my-pub",
			ClusterNetwork:  "my-priv",
			ResourceTimeout: 2 * time.Minute,
			NginxImage:      "myorg/nginx:latest",
			NodeTypes: map[string]NodeConfig{
				"osdOnlyNodes": {
					OSDsPerNode:         9,
					OtherDaemonsPerNode: 0,
					Placement: PlacementConfig{
						NodeSelector: map[string]string{
							"osd-node":     "true",
							"storage-node": "true",
						},
						Tolerations: []TolerationType{
							{Key: "storage-node", Operator: "Exists", Effect: "NoSchedule"},
							{Key: "osd-node", Operator: "Exists", Effect: "NoSchedule"},
						},
					},
				},
				"generalStorageNodes": {
					OSDsPerNode:         3,
					OtherDaemonsPerNode: 16,
					Placement: PlacementConfig{
						NodeSelector: map[string]string{
							"storage-node": "true",
						},
						Tolerations: []TolerationType{
							{Key: "storage-node", Operator: "Exists", Effect: "NoSchedule"},
						},
					},
				},
				"workerNodes": {
					OSDsPerNode:         0,
					OtherDaemonsPerNode: 6,
					Placement: PlacementConfig{
						NodeSelector: map[string]string{
							"worker-node": "true",
						},
						Tolerations: []TolerationType{
							{Key: "special-worker", Operator: "Exists"},
						},
					},
				},
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y, err := tt.config.ToYAML()
			assert.NoError(t, err)
			// basic test to ensure config yamls have comments
			assert.Contains(t, y, "# The intended namespace where the validation test will be run.")

			c2, err := ValidationTestConfigFromYAML(y)
			assert.NoError(t, err)

			// config survives round trip to and from yaml
			assert.Equal(t, tt.config, c2)
		})
	}
}

func TestValidationTestConfig_BestNodePlacementForServer(t *testing.T) {
	convergedType := NodeConfig{
		OSDsPerNode:         3,
		OtherDaemonsPerNode: 10,
		Placement: PlacementConfig{
			NodeSelector: map[string]string{"converged": "type"},
		},
	}
	osdType := NodeConfig{
		OSDsPerNode:         3,
		OtherDaemonsPerNode: 0,
		Placement: PlacementConfig{
			NodeSelector: map[string]string{"osd": "type"},
		},
	}
	workerType := NodeConfig{
		OSDsPerNode:         0,
		OtherDaemonsPerNode: 4,
		Placement: PlacementConfig{
			NodeSelector: map[string]string{"worker": "type"},
		},
	}

	tests := []struct {
		name      string
		NodeTypes map[string]NodeConfig
		want      PlacementConfig
		wantErr   bool
	}{
		{"empty", map[string]NodeConfig{}, PlacementConfig{}, true},
		{"converged", map[string]NodeConfig{"converged": convergedType}, convergedType.Placement, false},
		{"worker", map[string]NodeConfig{"worker": workerType}, PlacementConfig{}, true},
		{"worker and osd", map[string]NodeConfig{
			"worker": workerType,
			"osd":    osdType,
		}, osdType.Placement, false},
		{"converged and osd", map[string]NodeConfig{
			"converged": convergedType,
			"osd":       osdType,
		}, convergedType.Placement, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ValidationTestConfig{
				NodeTypes: tt.NodeTypes,
			}
			got, err := c.BestNodePlacementForServer()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidationTestConfig.BestNodePlacementForServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ValidationTestConfig.BestNodePlacementForServer() = %v, want %v", got, tt.want)
			}
		})
	}
}
