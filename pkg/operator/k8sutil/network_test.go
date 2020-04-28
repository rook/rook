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

package k8sutil

import (
	"testing"

	netapi "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNetwork_GetMultusIfName(t *testing.T) {
	multusSelector := "macvlan@server1"
	ifName, _ := GetMultusIfName(multusSelector)

	assert.Equal(t, "server1", ifName)
}

func TestNetwork_GetMultusIfNameDefault(t *testing.T) {
	multusSelector := "macvlan"
	_, err := GetMultusIfName(multusSelector)

	assert.Error(t, err)
}

func TestNetwork_parseMultusSelectorJSON(t *testing.T) {
	multusSelector := `{
		"name": "macvlan",
		"interface": "server1",
		"namespace": "rook-edgefs"
	}`

	multusMap, _ := parseMultusSelector(multusSelector)

	expected := map[string]string{
		"name":      "macvlan",
		"interface": "server1",
		"namespace": "rook-edgefs",
	}

	assert.Equal(t, expected, multusMap)
}

func TestNetwork_parseMultusSelectorShort(t *testing.T) {
	multusSelector := "rook-edgefs/macvlan@server1"
	multusMap, _ := parseMultusSelector(multusSelector)

	expected := map[string]string{
		"name":      "macvlan",
		"interface": "server1",
		"namespace": "rook-edgefs",
	}

	assert.Equal(t, expected, multusMap)
}

func TestNetwork_parseMultusSelectorError(t *testing.T) {
	multusSelector := "rook-edgefs/@server1"
	_, err := parseMultusSelector(multusSelector)

	assert.Error(t, err)
}

func TestNetwork_ApplyMultusShort(t *testing.T) {
	net := rookv1.NetworkSpec{
		Provider: "multus",
		Selectors: map[string]string{
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
	net := rookv1.NetworkSpec{
		Provider: "multus",
		Selectors: map[string]string{
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

func TestNetwork_ApplyMultusMixedError(t *testing.T) {
	net := rookv1.NetworkSpec{
		Provider: "multus",
		Selectors: map[string]string{
			"server": `{"name": "macvlan", "interface": "net1"}`,
			"broker": `macvlan@net2`,
		},
	}

	objMeta := metav1.ObjectMeta{}
	err := ApplyMultus(net, &objMeta)

	assert.Error(t, err)
}

func TestGetNetworkAttachmentConfig(t *testing.T) {
	dummyNetAttachDef := netapi.NetworkAttachmentDefinition{
		Spec: netapi.NetworkAttachmentDefinitionSpec{
			Config: `{
				"cniVersion": "0.3.0",
				"type": "macvlan",
				"master": "eth2",
				"mode": "bridge",
				"ipam": {
				  "type": "host-local",
				  "subnet": "172.18.8.0/24",
				  "rangeStart": "172.18.8.200",
				  "rangeEnd": "172.18.8.216",
				  "routes": [
					{
					  "dst": "0.0.0.0/0"
					}
				  ],
				  "gateway": "172.18.8.1"
				}
			  }`,
		},
	}

	config, err := GetNetworkAttachmentConfig(dummyNetAttachDef)
	assert.NoError(t, err)
	assert.Equal(t, "172.18.8.0/24", config.Ipam.Subnet)
}
