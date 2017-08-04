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
package provisioner

import (
	"fmt"
	"testing"

	"k8s.io/api/core/v1"

	"strings"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProcessMonAddresses(t *testing.T) {
	clientset := test.New(3)
	context := &clusterd.Context{Clientset: clientset}
	ns := "myns"
	config := provisionerConfig{clusterName: ns}
	p := &RookVolumeProvisioner{context: context, provConfig: config}

	// create the test set of mons
	testConfigMap := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: mon.EndpointConfigMapName},
		Data: map[string]string{mon.EndpointDataKey: "mon1=10.0.0.1:6790,mon2=10.0.0.2:8125,mon3=10.0.0.3:6790"}}
	_, err := clientset.CoreV1().ConfigMaps(ns).Create(testConfigMap)
	require.Nil(t, err)

	formattedMons, err := p.getMonitorEndpoints()
	require.Nil(t, err)
	findMon(t, "10.0.0.1:6790", formattedMons)
	findMon(t, "10.0.0.2:8125", formattedMons)
	findMon(t, "10.0.0.3:6790", formattedMons)
}

func findMon(t *testing.T, expected string, mons []string) {
	for _, mon := range mons {
		if expected == mon {
			return
		}
	}
	assert.Fail(t, fmt.Sprintf("expected: %s in %v", expected, mons))
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

func TestCreateImageName(t *testing.T) {
	// use a PV name that is typical, it should not be truncated because the resultant image name is not over max length
	pvName := "pvc-023d0ff3-261d-11e7-aa63-001c42669caf"
	imageName := createImageName(pvName)
	assert.True(t, strings.HasPrefix(imageName, "k8s-dynamic-pvc-023d0ff3-261d-11e7-aa63-001c42669caf-"))
	assert.Equal(t, 89, len(imageName))

	// now try a PV name that is too long, it should be properly truncated
	pvName = "pvc-0fd5988f-2516-11e7-aa55-02420ac00002-2d8-this-is-where-it-gets-too-long"
	imageName = createImageName(pvName)
	assert.True(t, strings.HasPrefix(imageName, "k8s-dynamic-pvc-0fd5988f-2516-11e7-aa55-02420ac00002-2d8-"))

	// length of our image name plus the rbd ID prefix that the RBD kernel module will add should be equal to the max allowed
	assert.Equal(t, imageNameMaxLen, len(imageName)+len(rbdIDPrefix),
		fmt.Sprintf("unexpected image name length: %d, image name: '%s'", len(imageName), imageName))
}
