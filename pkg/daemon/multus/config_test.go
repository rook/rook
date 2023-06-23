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
			Namespace: "my-rook", PublicNetwork: "my-pub", ClusterNetwork: "my-priv",
			DaemonsPerNode: 12, ResourceTimeout: 2 * time.Minute, NginxImage: "myorg/nginx:latest",
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
