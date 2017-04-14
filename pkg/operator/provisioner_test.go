/*
Copyright 2017 The Rook Authors. All rights reserved.

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
package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessMonAddresses(t *testing.T) {
	unformattedMons := []string{"10.0.0.1:8124/0", "10.0.0.2:8125", "10.0.0.3/24"}
	formattedMons := processMonAddresses(unformattedMons)

	assert.Equal(t, "10.0.0.1:8124", formattedMons[0])
	assert.Equal(t, "10.0.0.2:8125", formattedMons[1])
	assert.Equal(t, "10.0.0.3", formattedMons[2])
}

func TestParseClassParameters(t *testing.T) {
	cfg := make(map[string]string)
	cfg["pool"] = "testPool"
	cfg["clusterNamespace"] = "mynamespace"
	cfg["clustername"] = "myname"

	provConfig, err := parseClassParameters(cfg)
	assert.Nil(t, err)

	assert.Equal(t, "testPool", provConfig.pool)
	assert.Equal(t, "mynamespace", provConfig.clusterNamespace)
	assert.Equal(t, "myname", provConfig.clusterName)
}

func TestParseClassParametersDefault(t *testing.T) {
	cfg := make(map[string]string)
	cfg["pool"] = "testPool"

	provConfig, err := parseClassParameters(cfg)
	assert.Nil(t, err)

	assert.Equal(t, "testPool", provConfig.pool)
	assert.Equal(t, "rook", provConfig.clusterNamespace)
	assert.Equal(t, "rook", provConfig.clusterName)
}

func TestParseClassParametersNoPool(t *testing.T) {
	cfg := make(map[string]string)
	cfg["clusterNamespace"] = "mynamespace"
	cfg["clustername"] = "myname"

	_, err := parseClassParameters(cfg)
	assert.EqualError(t, err, "StorageClass for provisioner rookVolumeProvisioner must contain 'pool' parameter")

}

func TestParseClassParametersInvalidOption(t *testing.T) {
	cfg := make(map[string]string)
	cfg["pool"] = "testPool"
	cfg["foo"] = "bar"

	_, err := parseClassParameters(cfg)
	assert.EqualError(t, err, "invalid option \"foo\" for volume plugin rookVolumeProvisioner")
}
