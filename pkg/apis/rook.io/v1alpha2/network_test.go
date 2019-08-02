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

package v1alpha2

import (
	"encoding/json"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNetwork_Spec(t *testing.T) {
	netSpecYAML := []byte(`
provider: host
selectors:
  server: enp2s0f0
  broker: enp2s0f0`)

	rawJSON, err := yaml.YAMLToJSON(netSpecYAML)
	assert.Nil(t, err)

	var net NetworkSpec

	err = json.Unmarshal(rawJSON, &net)
	assert.Nil(t, err)

	expected := NetworkSpec{
		Provider: "host",
		Selectors: map[string]NetworkSelector{
			"server": NetworkSelector("enp2s0f0"),
			"broker": NetworkSelector("enp2s0f0"),
		},
	}

	assert.Equal(t, expected, net)
}

func TestNetwork_GetMultusIfName(t *testing.T) {
	multusSelector := NetworkSelector("macvlan@server1")
	ifName := GetMultusIfName(multusSelector)

	assert.Equal(t, "server1", ifName)
}

func TestNetwork_GetMultusIfNameDefault(t *testing.T) {
	multusSelector := NetworkSelector("macvlan")
	ifName := GetMultusIfName(multusSelector)

	assert.Equal(t, "net1", ifName)
}

func TestNetwork_parseMultusSelectorJSON(t *testing.T) {
	multusSelector := NetworkSelector(`{
		"name": "macvlan",
		"interface": "server1",
		"namespace": "rook-edgefs"
	}`)

	multusMap := parseMultusSelector(multusSelector)

	expected := map[string]string{
		"name":      "macvlan",
		"interface": "server1",
		"namespace": "rook-edgefs",
	}

	assert.Equal(t, expected, multusMap)
}

func TestNetwork_parseMultusSelectorShort(t *testing.T) {
	multusSelector := NetworkSelector("rook-edgefs/macvlan@server1")
	multusMap := parseMultusSelector(multusSelector)

	expected := map[string]string{
		"name":      "macvlan",
		"interface": "server1",
		"namespace": "rook-edgefs",
	}

	assert.Equal(t, expected, multusMap)
}

func TestNetwork_ApplyMultusShort(t *testing.T) {
	net := NetworkSpec{
		Provider: "multus",
		Selectors: map[string]NetworkSelector{
			"server": "macvlan@net1",
			"broker": "macvlan@net2",
		},
	}

	objMeta := metav1.ObjectMeta{}
	ApplyMultus(net, &objMeta)

	assert.Contains(t, objMeta.Annotations, "k8s.v1.cni.cncf.io/networks")
	assert.Contains(t, objMeta.Annotations["k8s.v1.cni.cncf.io/networks"], "macvlan@net1")
	assert.Contains(t, objMeta.Annotations["k8s.v1.cni.cncf.io/networks"], "macvlan@net2")
}

func TestNetwork_ApplyMultusJSON(t *testing.T) {
	net := NetworkSpec{
		Provider: "multus",
		Selectors: map[string]NetworkSelector{
			"server": `{"name": "macvlan", "interface": "net1"}`,
			"broker": `{"name": "macvlan", "interface": "net2"}`,
		},
	}

	objMeta := metav1.ObjectMeta{}
	ApplyMultus(net, &objMeta)

	assert.Contains(t, objMeta.Annotations, "k8s.v1.cni.cncf.io/networks")
	assert.Contains(t, objMeta.Annotations["k8s.v1.cni.cncf.io/networks"], `{"name": "macvlan", "interface": "net1"}`)
	assert.Contains(t, objMeta.Annotations["k8s.v1.cni.cncf.io/networks"], `{"name": "macvlan", "interface": "net2"}`)
}
