/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package config

import (
	"context"
	"testing"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	fakenetclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateNetworkSettings(t *testing.T) {
	t.Run("no network definition exists", func(*testing.T) {
		ctx := context.TODO()
		ns := "rook-ceph"
		clientset := testop.New(t, 1)
		clusterdContext := &clusterd.Context{
			Clientset:     clientset,
			NetworkClient: fakenetclient.NewSimpleClientset().K8sCniCncfIoV1(),
		}
		netSelector := map[string]string{"public": "public-network-attach-def"}
		_, err := generateNetworkSettings(ctx, clusterdContext, ns, netSelector)
		assert.Error(t, err)

	})

	t.Run("only cluster network", func(*testing.T) {
		netSelector := map[string]string{"cluster": "cluster-network-attach-def"}
		networks := []networkv1.NetworkAttachmentDefinition{getClusterNetwork()}
		ctxt := context.TODO()
		ns := "rook-ceph"
		clientset := testop.New(t, 1)
		clusterdContext := &clusterd.Context{
			Clientset:     clientset,
			NetworkClient: fakenetclient.NewSimpleClientset().K8sCniCncfIoV1(),
		}

		for i := range networks {
			_, err := clusterdContext.NetworkClient.NetworkAttachmentDefinitions(ns).Create(ctxt, &networks[i], metav1.CreateOptions{})
			assert.NoError(t, err)
		}
		cephNetwork, err := generateNetworkSettings(context.TODO(), clusterdContext, ns, netSelector)
		assert.NoError(t, err)
		assert.Equal(t, "172.18.0.0/16", cephNetwork["cluster_network"])
		assert.Equal(t, 1, len(cephNetwork))
	})

	t.Run("only public network", func(*testing.T) {
		ctx := context.TODO()
		ns := "rook-ceph"
		clientset := testop.New(t, 1)
		clusterdContext := &clusterd.Context{
			Clientset:     clientset,
			NetworkClient: fakenetclient.NewSimpleClientset().K8sCniCncfIoV1(),
		}
		netSelector := map[string]string{"public": "public-network-attach-def"}
		networks := []networkv1.NetworkAttachmentDefinition{getPublicNetwork()}
		for i := range networks {
			_, err := clusterdContext.NetworkClient.NetworkAttachmentDefinitions(ns).Create(ctx, &networks[i], metav1.CreateOptions{})
			assert.NoError(t, err)
		}
		cephNetwork, err := generateNetworkSettings(ctx, clusterdContext, ns, netSelector)
		assert.NoError(t, err)
		assert.Equal(t, "192.168.0.0/24", cephNetwork["public_network"])
		assert.Equal(t, 1, len(cephNetwork))
	})

	t.Run("public and cluster network", func(*testing.T) {
		ctx := context.TODO()
		ns := "rook-ceph"
		clientset := testop.New(t, 1)
		clusterdContext := &clusterd.Context{
			Clientset:     clientset,
			NetworkClient: fakenetclient.NewSimpleClientset().K8sCniCncfIoV1(),
		}
		netSelector := map[string]string{
			"public":  "public-network-attach-def",
			"cluster": "cluster-network-attach-def",
		}
		networks := []networkv1.NetworkAttachmentDefinition{getPublicNetwork(), getClusterNetwork()}
		for i := range networks {
			_, err := clusterdContext.NetworkClient.NetworkAttachmentDefinitions(ns).Create(ctx, &networks[i], metav1.CreateOptions{})
			assert.NoError(t, err)
		}
		cephNetwork, err := generateNetworkSettings(ctx, clusterdContext, ns, netSelector)
		assert.NoError(t, err)
		assert.Equal(t, "172.18.0.0/16", cephNetwork["cluster_network"])
		assert.Equal(t, "192.168.0.0/24", cephNetwork["public_network"])
		assert.Equal(t, 2, len(cephNetwork))
	})
}

func getPublicNetwork() networkv1.NetworkAttachmentDefinition {
	return networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "public-network-attach-def",
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{
				"cniVersion": "0.3.0",
				"type": "macvlan",
				"master": "eth2",
				"mode": "bridge",
				"ipam": {
				  "type": "host-local",
				  "subnet": "192.168.0.0/24",
				  "gateway": "172.18.8.1"
				}
			  }`,
		},
	}
}

func getClusterNetwork() networkv1.NetworkAttachmentDefinition {
	return networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-network-attach-def",
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: `{
				"cniVersion": "0.3.0",
				"type": "macvlan",
				"master": "eth2",
				"mode": "bridge",
				"ipam": {
				  "type": "host-local",
				  "subnet": "172.18.0.0/16",
				  "gateway": "172.18.0.1"
				}
			  }`,
		},
	}
}

func TestGetNetworkRange(t *testing.T) {
	t.Run("simple host-local IPAM test", func(t *testing.T) {
		ns := "rook-ceph"
		nad := &networkv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "public-network-attach-def",
				Namespace: ns,
			},
			Spec: networkv1.NetworkAttachmentDefinitionSpec{
				Config: `{
				"cniVersion": "0.3.0",
				"type": "macvlan",
				"master": "eth2",
				"mode": "bridge",
				"ipam": {
				  "type": "host-local",
				  "subnet": "",
				  "gateway": "172.18.8.1"
				}
			  }`,
			},
		}

		netConfig, err := k8sutil.GetNetworkAttachmentConfig(*nad)
		assert.NoError(t, err)

		//
		// TEST 1: subnet/range is empty
		//
		networkRange := getNetworkRange(netConfig)
		assert.Empty(t, networkRange)

		//
		// TEST 2: subnet is not empty
		//
		netConfig.Ipam.Type = "host-local"
		netConfig.Ipam.Subnet = "192.168.0.0/24"
		networkRange = getNetworkRange(netConfig)
		assert.Equal(t, "192.168.0.0/24", networkRange)
	})

	t.Run("advanced host-local IPAM test", func(t *testing.T) {
		ns := "rook-ceph"
		nad := &networkv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "public-network-attach-def",
				Namespace: ns,
			},
			Spec: networkv1.NetworkAttachmentDefinitionSpec{
				Config: `{
	"ipam": {
		"type": "host-local",
		"ranges": [
			[
				{
					"subnet": "10.10.0.0/16",
					"rangeStart": "10.10.1.20",
					"rangeEnd": "10.10.3.50",
					"gateway": "10.10.0.254"
				},
				{
					"subnet": "172.16.5.0/24"
				}
			],
			[
				{
					"subnet": "3ffe:ffff:0:01ff::/64",
					"rangeStart": "3ffe:ffff:0:01ff::0010",
					"rangeEnd": "3ffe:ffff:0:01ff::0020"
				}
			]
		],
		"routes": [
			{ "dst": "0.0.0.0/0" },
			{ "dst": "192.168.0.0/16", "gw": "10.10.5.1" },
			{ "dst": "3ffe:ffff:0:01ff::1/64" }
		],
		"dataDir": "/run/my-orchestrator/container-ipam-state"
	}
}`,
			},
		}

		netConfig, err := k8sutil.GetNetworkAttachmentConfig(*nad)
		assert.NoError(t, err)
		networkRange := getNetworkRange(netConfig)
		assert.Equal(t, "10.10.0.0/16,172.16.5.0/24,3ffe:ffff:0:01ff::/64", networkRange)
	})

	t.Run("advanced static IPAM test", func(t *testing.T) {
		ns := "rook-ceph"
		nad := &networkv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "public-network-attach-def",
				Namespace: ns,
			},
			Spec: networkv1.NetworkAttachmentDefinitionSpec{
				Config: `{
	"ipam": {
		"type": "static",
		"addresses": [
			{
				"address": "10.10.0.1/24",
				"gateway": "10.10.0.254"
			},
			{
				"address": "3ffe:ffff:0:01ff::1/64",
				"gateway": "3ffe:ffff:0::1"
			}
		],
		"routes": [
			{ "dst": "0.0.0.0/0" },
			{ "dst": "192.168.0.0/16", "gw": "10.10.5.1" },
			{ "dst": "3ffe:ffff:0:01ff::1/64" }
		],
		"dns": {
			"nameservers" : ["8.8.8.8"],
			"domain": "example.com",
			"search": [ "example.com" ]
		}
	}
}`,
			},
		}
		netConfig, err := k8sutil.GetNetworkAttachmentConfig(*nad)
		assert.NoError(t, err)
		networkRange := getNetworkRange(netConfig)
		assert.Equal(t, "10.10.0.1/24,3ffe:ffff:0:01ff::1/64", networkRange)
	})

	t.Run("advanced whereabouts IPAM test", func(t *testing.T) {
		ns := "rook-ceph"
		nad := &networkv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "public-network-attach-def",
				Namespace: ns,
			},
			Spec: networkv1.NetworkAttachmentDefinitionSpec{
				Config: `{
      "cniVersion": "0.3.0",
      "name": "whereaboutsexample",
      "type": "macvlan",
      "master": "eth0",
      "mode": "bridge",
      "ipam": {
        "type": "whereabouts",
        "range": "192.168.2.225/28",
        "exclude": [
           "192.168.2.229/30",
           "192.168.2.236/32"
        ]
      }
}`,
			},
		}
		netConfig, err := k8sutil.GetNetworkAttachmentConfig(*nad)
		assert.NoError(t, err)
		networkRange := getNetworkRange(netConfig)
		assert.Equal(t, "192.168.2.225/28", networkRange)
	})
}

func TestGetMultusNamespace(t *testing.T) {
	// TEST 1: When namespace is specified with the NAD
	namespace, nad := GetMultusNamespace("multus-ns/public-nad")
	assert.Equal(t, "multus-ns", namespace)
	assert.Equal(t, "public-nad", nad)

	// TEST 2: When only NAD is specified
	namespace, nad = GetMultusNamespace("public-nad")
	assert.Empty(t, namespace)
	assert.Equal(t, "public-nad", nad)
}
