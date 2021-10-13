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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	netapi "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// publicNetworkSelectorKeyName is the network selector key for the ceph public network
	publicNetworkSelectorKeyName = "public"
	// clusterNetworkSelectorKeyName is the network selector key for the ceph cluster network
	clusterNetworkSelectorKeyName = "cluster"
)

// NetworkAttachmentConfig represents the configuration of the NetworkAttachmentDefinitions object
type NetworkAttachmentConfig struct {
	CniVersion string `json:"cniVersion,omitempty"`
	Type       string `json:"type,omitempty"`
	Master     string `json:"master,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Ipam       struct {
		Type      string `json:"type,omitempty"`
		Subnet    string `json:"subnet,omitempty"`
		Addresses []struct {
			Address string `json:"address,omitempty"`
			Gateway string `json:"gateway,omitempty"`
		} `json:"addresses,omitempty"`
		Ranges [][]struct {
			Subnet     string `json:"subnet,omitempty"`
			RangeStart string `json:"rangeStart,omitempty"`
			RangeEnd   string `json:"rangeEnd,omitempty"`
			Gateway    string `json:"gateway,omitempty"`
		} `json:"ranges,omitempty"`
		Range      string `json:"range,omitempty"`
		RangeStart string `json:"rangeStart,omitempty"`
		RangeEnd   string `json:"rangeEnd,omitempty"`
		Routes     []struct {
			Dst string `json:"dst,omitempty"`
		} `json:"routes,omitempty"`
		Gateway string `json:"gateway,omitempty"`
	} `json:"ipam,omitempty"`
}

// ApplyMultus apply multus selector to Pods
// Multus supports short and json syntax, use only one kind at a time.
func ApplyMultus(net cephv1.NetworkSpec, objectMeta *metav1.ObjectMeta) error {
	v := make([]string, 0, 2)
	shortSyntax := false
	jsonSyntax := false

	for k, ns := range net.Selectors {
		var multusMap map[string]string
		err := json.Unmarshal([]byte(ns), &multusMap)

		if err == nil {
			jsonSyntax = true
		} else {
			shortSyntax = true
		}

		app, ok := objectMeta.Labels["app"]
		if !ok {
			app = "" // unknown app
		}
		isClusterNetApp := false
		for _, clusterNetworkApp := range getClusterNetworkApps() {
			if app == clusterNetworkApp {
				isClusterNetApp = true
				break
			}
		}
		if isClusterNetApp {
			// append all networks to apps that are cluster network apps
			v = append(v, string(ns))
		} else {
			// only append public networks to apps that are not cluster network apps
			if k == publicNetworkSelectorKeyName {
				v = append(v, string(ns))
			}
		}
	}

	if shortSyntax && jsonSyntax {
		return fmt.Errorf("ApplyMultus: Can't mix short and JSON form")
	}

	// Sort network strings so that pods/deployments won't need updated in a loop if nothing changes
	sort.Strings(v)

	networks := strings.Join(v, ", ")
	if jsonSyntax {
		networks = "[" + networks + "]"
	}

	t := cephv1.Annotations{
		"k8s.v1.cni.cncf.io/networks": networks,
	}
	t.ApplyToObjectMeta(objectMeta)

	return nil
}

// getClusterNetworkApps returns the list of ceph apps that utilize cluster network
func getClusterNetworkApps() []string {
	return []string{"rook-ceph-osd"}
}

// GetNetworkAttachmentConfig returns the NetworkAttachmentDefinitions configuration
func GetNetworkAttachmentConfig(n netapi.NetworkAttachmentDefinition) (NetworkAttachmentConfig, error) {
	netConfigJSON := n.Spec.Config
	var netConfig NetworkAttachmentConfig

	err := json.Unmarshal([]byte(netConfigJSON), &netConfig)
	if err != nil {
		return netConfig, fmt.Errorf("failed to unmarshal netconfig json %q. %v", netConfigJSON, err)
	}

	return netConfig, nil
}
