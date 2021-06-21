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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyMultus(t *testing.T) {
	t.Run("short format", func(t *testing.T) {
		tests := []struct {
			name         string
			netSelectors map[string]string
			labels       map[string]string
			want         string
		}{
			{
				name: "no applicable networks for non-osd pod",
				netSelectors: map[string]string{
					"unknown": "macvlan@net1",
				},
				want: "",
			},
			{
				name: "for a non-osd pod",
				netSelectors: map[string]string{
					publicNetworkSelectorKeyName:  "macvlan@net1",
					clusterNetworkSelectorKeyName: "macvlan@net2",
				},
				want: "macvlan@net1",
			},
			{
				name: "for an osd pod",
				netSelectors: map[string]string{
					publicNetworkSelectorKeyName:  "macvlan@net1",
					clusterNetworkSelectorKeyName: "macvlan@net2",
				},
				labels: map[string]string{"app": "rook-ceph-osd"},
				want:   "macvlan@net1, macvlan@net2",
			},
			{
				name: "for an osd pod (reverse ordering)",
				netSelectors: map[string]string{
					publicNetworkSelectorKeyName:  "macvlan@net2",
					clusterNetworkSelectorKeyName: "macvlan@net1",
				},
				labels: map[string]string{"app": "rook-ceph-osd"},
				want:   "macvlan@net1, macvlan@net2", // should not change the order of output
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				net := cephv1.NetworkSpec{
					Provider:  "multus",
					Selectors: test.netSelectors,
				}
				objMeta := metav1.ObjectMeta{}
				objMeta.Labels = test.labels
				err := ApplyMultus(net, &objMeta)
				assert.NoError(t, err)
				assert.Equal(t, test.want, objMeta.Annotations["k8s.v1.cni.cncf.io/networks"])
			})
		}
	})

	t.Run("JSON format", func(t *testing.T) {
		json1 := `{"name": "macvlan", "interface": "net1"}`
		json2 := `{"name": "macvlan", "interface": "net2"}`

		t.Run("no applicable networks for non-osd pod", func(t *testing.T) {
			net := cephv1.NetworkSpec{
				Provider: "multus",
				Selectors: map[string]string{
					"server": json1,
					"broker": json2,
				},
			}
			objMeta := metav1.ObjectMeta{}
			err := ApplyMultus(net, &objMeta)
			assert.NoError(t, err)
			// non-osd pods should not get any network annotations here
			assert.Equal(t, "[]", objMeta.Annotations["k8s.v1.cni.cncf.io/networks"])
		})

		t.Run("for a non-osd pod", func(t *testing.T) {
			net := cephv1.NetworkSpec{
				Provider: "multus",
				Selectors: map[string]string{
					"public":  json1,
					"cluster": json2,
				},
			}
			objMeta := metav1.ObjectMeta{}
			err := ApplyMultus(net, &objMeta)
			assert.NoError(t, err)
			// non-osd pods should only get public networks
			assert.Equal(t, "["+json1+"]", objMeta.Annotations["k8s.v1.cni.cncf.io/networks"])
		})

		t.Run("for an osd pod", func(t *testing.T) {
			net := cephv1.NetworkSpec{
				Provider: "multus",
				Selectors: map[string]string{
					"server": json1,
					"broker": json2,
				},
			}
			objMeta := metav1.ObjectMeta{
				Labels: map[string]string{
					"app": "rook-ceph-osd",
				},
			}
			err := ApplyMultus(net, &objMeta)
			assert.NoError(t, err)
			assert.Equal(t, "["+json1+", "+json2+"]", objMeta.Annotations["k8s.v1.cni.cncf.io/networks"])
		})

		t.Run("for an osd pod (reverse ordering)", func(t *testing.T) {
			net := cephv1.NetworkSpec{
				Provider: "multus",
				Selectors: map[string]string{
					"server": json2,
					"broker": json1,
				},
			}
			objMeta := metav1.ObjectMeta{
				Labels: map[string]string{
					"app": "rook-ceph-osd",
				},
			}
			err := ApplyMultus(net, &objMeta)
			assert.NoError(t, err)
			// should not change the order of output
			assert.Equal(t, "["+json1+", "+json2+"]", objMeta.Annotations["k8s.v1.cni.cncf.io/networks"])
		})
	})

	t.Run("mixed format (error)", func(t *testing.T) {
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
	})
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
