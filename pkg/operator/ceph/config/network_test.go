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
	"fmt"
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
	ctxt := context.TODO()
	ns := "rook-ceph"
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset:     clientset,
		NetworkClient: fakenetclient.NewSimpleClientset().K8sCniCncfIoV1(),
	}

	//
	// TEST 1: network definition does not exist
	//
	netSelector := map[string]string{
		"public": "public-network-attach-def",
	}

	_, err := generateNetworkSettings(ctx, ns, netSelector)
	assert.Error(t, err)

	//
	// TEST 2: single dedicated networks
	//
	expectedNetworks := []Option{
		{
			Who:    "global",
			Option: "public_network",
			Value:  "192.168.0.0/24",
		},
		{
			Who:    "global",
			Option: "cluster_network",
			Value:  "192.168.0.0/24",
		},
	}

	// this nad uses whereabouts cni
	network := &networkv1.NetworkAttachmentDefinition{
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
				  "subnet": "192.168.0.0/24",
				  "gateway": "172.18.8.1"
				}
			  }`,
		},
	}

	// Create public network definition
	_, err = ctx.NetworkClient.NetworkAttachmentDefinitions(ns).Create(ctxt, network, metav1.CreateOptions{})
	assert.NoError(t, err)

	cephNetwork, err := generateNetworkSettings(ctx, ns, netSelector)
	assert.NoError(t, err)
	assert.ElementsMatch(t, cephNetwork, expectedNetworks, fmt.Sprintf("networks: %+v", cephNetwork))

	//
	// TEST 3: two dedicated networks
	//
	expectedNetworks = []Option{
		{
			Who:    "global",
			Option: "public_network",
			Value:  "192.168.0.0/24",
		},
		{
			Who:    "global",
			Option: "cluster_network",
			Value:  "172.18.0.0/16",
		},
	}

	netSelector = map[string]string{
		"public":  "public-network-attach-def",
		"cluster": "cluster-network-attach-def",
	}
	network2 := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-network-attach-def",
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
				  "subnet": "172.18.0.0/16",
				  "gateway": "172.18.0.1"
				}
			  }`,
		},
	}

	// Create cluster network definition
	_, err = ctx.NetworkClient.NetworkAttachmentDefinitions(ns).Create(ctxt, network2, metav1.CreateOptions{})
	assert.NoError(t, err)

	cephNetwork, err = generateNetworkSettings(ctx, ns, netSelector)
	assert.NoError(t, err)
	assert.ElementsMatch(t, cephNetwork, expectedNetworks, fmt.Sprintf("networks: %+v", cephNetwork))
}

func TestGetNetworkRange(t *testing.T) {
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

	//
	// TEST 3: range is empty and ipam type is whereabouts
	//
	netConfig.Ipam.Type = "whereabouts"
	netConfig.Ipam.Subnet = ""
	networkRange = getNetworkRange(netConfig)
	assert.Empty(t, networkRange)

	//
	// TEST 4: range is not empty and ipam type is whereabouts
	//
	netConfig.Ipam.Type = "whereabouts"
	netConfig.Ipam.Range = "192.168.0.0/24"
	networkRange = getNetworkRange(netConfig)
	assert.Equal(t, "192.168.0.0/24", networkRange)
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
