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

// Package config provides default configurations which Rook will set in Ceph clusters.
package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// PublicNetworkSelectorKeyName is the network selector key for the ceph public network
	PublicNetworkSelectorKeyName = "public"
	// ClusterNetworkSelectorKeyName is the network selector key for the ceph cluster network
	ClusterNetworkSelectorKeyName = "cluster"
	// WhereaboutsIPAMType is Whereabouts IPAM type
	WhereaboutsIPAMType = "whereabouts"
	hostLocalIPAMType   = "host-local"
	staticIPAMType      = "static"
)

var (
	// NetworkSelectors is a slice of ceph network selector key name
	NetworkSelectors = []string{PublicNetworkSelectorKeyName, ClusterNetworkSelectorKeyName}
)

func generateNetworkSettings(ctx context.Context, clusterdContext *clusterd.Context, namespace string, networkSelectors map[string]string) ([]Option, error) {
	cephNetworks := []Option{}

	for _, selectorKey := range NetworkSelectors {
		// skip if selector is not specified
		if _, ok := networkSelectors[selectorKey]; !ok {
			continue
		}

		multusNamespace, nad := GetMultusNamespace(networkSelectors[selectorKey])
		if multusNamespace == "" {
			multusNamespace = namespace
		}
		// Get network attachment definition
		netDefinition, err := clusterdContext.NetworkClient.NetworkAttachmentDefinitions(multusNamespace).Get(ctx, nad, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				return []Option{}, errors.Wrapf(err, "specified network attachment definition %q in namespace %q for selector %q does not exist", nad, namespace, selectorKey)
			}
			return []Option{}, errors.Wrapf(err, "failed to fetch network attachment definition for selector %q", selectorKey)
		}

		// Get network attachment definition configuration
		netConfig, err := k8sutil.GetNetworkAttachmentConfig(*netDefinition)
		if err != nil {
			return []Option{}, errors.Wrapf(err, "failed to get network attachment definition configuration for selector %q", selectorKey)
		}

		networkRange := getNetworkRange(netConfig)
		if networkRange != "" {
			cephNetworks = append(cephNetworks, configOverride("global", fmt.Sprintf("%s_network", selectorKey), networkRange))
		} else {
			return []Option{}, errors.Errorf("empty subnet from network attachment definition %q", networkSelectors[selectorKey])
		}
	}

	return cephNetworks, nil
}

func GetMultusNamespace(nad string) (string, string) {
	tmp := strings.Split(nad, "/")
	if len(tmp) == 2 {
		return tmp[0], tmp[1]
	}
	return "", nad
}

func getNetworkRange(netConfig k8sutil.NetworkAttachmentConfig) string {
	var subnets []string

	switch netConfig.Ipam.Type {
	case hostLocalIPAMType:
		if netConfig.Ipam.Subnet != "" {
			return netConfig.Ipam.Subnet
		}
		for _, netRanges := range netConfig.Ipam.Ranges {
			for _, netRange := range netRanges {
				subnets = append(subnets, netRange.Subnet)
			}
		}
		return strings.Join(subnets, ",")

	case staticIPAMType:
		for _, subnet := range netConfig.Ipam.Addresses {
			subnets = append(subnets, subnet.Address)
		}

		return strings.Join(subnets, ",")

	case WhereaboutsIPAMType:
		return netConfig.Ipam.Range

	default:
		return ""
	}
}
