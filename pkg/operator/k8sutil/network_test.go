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
	"strings"
	"testing"

	netapi "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNetwork_ApplyMultusShort(t *testing.T) {
	net := cephv1.NetworkSpec{
		Provider: "multus",
		Selectors: map[string]string{
			publicNetworkSelectorKeyName:  "macvlan@net1",
			clusterNetworkSelectorKeyName: "macvlan@net2",
		},
	}

	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{
			name: "for a non osd pod",
			want: []string{"macvlan@net1"},
		},
		{
			name:   "for an osd pod",
			labels: map[string]string{"app": "rook-ceph-osd"},
			want:   []string{"macvlan@net1", "macvlan@net2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objMeta := metav1.ObjectMeta{}
			objMeta.Labels = test.labels
			err := ApplyMultus(net, &objMeta)
			assert.NoError(t, err)
			appliedNetworksList := strings.Split(objMeta.Annotations["k8s.v1.cni.cncf.io/networks"], ", ")
			assert.ElementsMatch(t, appliedNetworksList, test.want)
		})

	}
}

func TestNetwork_ApplyMultusJSON(t *testing.T) {
	net := cephv1.NetworkSpec{
		Provider: "multus",
		Selectors: map[string]string{
			"server": `{"name": "macvlan", "interface": "net1"}`,
			"broker": `{"name": "macvlan", "interface": "net2"}`,
		},
	}

	objMeta := metav1.ObjectMeta{}
	objMeta.Labels = map[string]string{
		"app": "rook-ceph-osd",
	}
	err := ApplyMultus(net, &objMeta)
	assert.NoError(t, err)

	assert.Contains(t, objMeta.Annotations, "k8s.v1.cni.cncf.io/networks")
	assert.Contains(t, objMeta.Annotations["k8s.v1.cni.cncf.io/networks"], `{"name": "macvlan", "interface": "net1"}`)
	assert.Contains(t, objMeta.Annotations["k8s.v1.cni.cncf.io/networks"], `{"name": "macvlan", "interface": "net2"}`)
}

func TestNetwork_ApplyMultusMixedError(t *testing.T) {
	net := cephv1.NetworkSpec{
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
