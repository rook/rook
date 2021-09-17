/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestNetworkCephSpecLegacy(t *testing.T) {
	netSpecYAML := []byte(`hostNetwork: true`)

	rawJSON, err := yaml.ToJSON(netSpecYAML)
	assert.Nil(t, err)

	var net NetworkSpec

	err = json.Unmarshal(rawJSON, &net)
	assert.Nil(t, err)

	expected := NetworkSpec{HostNetwork: true}

	assert.Equal(t, expected, net)
}

func TestNetworkCephIsHostLegacy(t *testing.T) {
	net := NetworkSpec{HostNetwork: true}

	assert.True(t, net.IsHost())
}

func TestNetworkSpec(t *testing.T) {
	netSpecYAML := []byte(`
provider: host
selectors:
  server: enp2s0f0
  broker: enp2s0f0`)

	rawJSON, err := yaml.ToJSON(netSpecYAML)
	assert.Nil(t, err)

	var net NetworkSpec

	err = json.Unmarshal(rawJSON, &net)
	assert.Nil(t, err)

	expected := NetworkSpec{
		Provider: "host",
		Selectors: map[string]string{
			"server": "enp2s0f0",
			"broker": "enp2s0f0",
		},
	}

	assert.Equal(t, expected, net)
}
