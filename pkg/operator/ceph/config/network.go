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
	"fmt"

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
)

var (
	// NetworkSelectors is a slice of ceph network selector key name
	NetworkSelectors = []string{PublicNetworkSelectorKeyName, ClusterNetworkSelectorKeyName}
)

func generateNetworkSettings(context *clusterd.Context, namespace string, networkSelectors map[string]string) ([]Option, error) {
	cephNetworks := []Option{}

	for _, selectorKey := range NetworkSelectors {
		// This means only "public" was specified and thus we use the same subnet for cluster too
		if _, ok := networkSelectors[selectorKey]; !ok {
			cephNetworks = append(cephNetworks, cephNetworks[0])
			cephNetworks[1].Option = fmt.Sprintf("%s_network", selectorKey)
			continue
		}

		// Get network attachment definition
		netDefinition, err := context.NetworkClient.NetworkAttachmentDefinitions(namespace).Get(networkSelectors[selectorKey], metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				return []Option{}, errors.Wrapf(err, "specified network attachment definition for selector %q does not exist", selectorKey)
			}
			return []Option{}, errors.Wrapf(err, "failed to fetch network attachment definition for selector %q", selectorKey)
		}

		// Get network attachment definition configuration
		netConfig, err := k8sutil.GetNetworkAttachmentConfig(*netDefinition)
		if err != nil {
			return []Option{}, errors.Wrapf(err, "failed to get network attachment definition configuration for selector %q", selectorKey)
		}

		if netConfig.Ipam.Subnet != "" {
			cephNetworks = append(cephNetworks, configOverride("global", fmt.Sprintf("%s_network", selectorKey), netConfig.Ipam.Subnet))
		} else {
			return []Option{}, errors.Errorf("empty subnet from network attachment definition %q", networkSelectors[selectorKey])
		}
	}

	return cephNetworks, nil
}
