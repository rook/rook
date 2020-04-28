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
	"strings"

	netapi "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkAttachmentConfig represents the configuration of the NetworkAttachmentDefinitions object
type NetworkAttachmentConfig struct {
	CniVersion string `json:"cniVersion"`
	Type       string `json:"type"`
	Master     string `json:"master"`
	Mode       string `json:"mode"`
	Ipam       struct {
		Type       string `json:"type"`
		Subnet     string `json:"subnet"`
		RangeStart string `json:"rangeStart"`
		RangeEnd   string `json:"rangeEnd"`
		Routes     []struct {
			Dst string `json:"dst"`
		} `json:"routes"`
		Gateway string `json:"gateway"`
	} `json:"ipam"`
}

// parseMultusSelector will parse short and JSON form of individual multus
// network attachment selection annotation. Valid JSON will be unmarshalled and
// return as is, while invalid JSON will be tried using
// <namespace>/<name>@<interface> short syntax.
func parseMultusSelector(selector string) (map[string]string, error) {
	rawMap := make(map[string]string)

	err := json.Unmarshal([]byte(selector), &rawMap)

	if err != nil {
		// it can be in short form
		nsEndIndex := strings.IndexAny(selector, "/")
		if nsEndIndex != -1 {
			rawMap["namespace"] = selector[:nsEndIndex]
		}

		ifStartIndex := strings.LastIndexAny(selector, "@")
		if ifStartIndex != -1 && len(selector)-ifStartIndex > 1 {
			rawMap["interface"] = selector[ifStartIndex+1:]
		}

		if nsEndIndex != -1 && ifStartIndex != -1 && ifStartIndex-nsEndIndex > 1 {
			rawMap["name"] = selector[nsEndIndex+1 : ifStartIndex]
		} else if nsEndIndex == -1 && ifStartIndex != -1 {
			rawMap["name"] = selector[:ifStartIndex]
		} else if nsEndIndex != -1 && ifStartIndex == -1 {
			rawMap["name"] = selector[nsEndIndex+1:]
		}
	}

	if name, ok := rawMap["name"]; !ok || name == "" {
		return nil, fmt.Errorf("parseMultusSelector: missing name")
	}

	return rawMap, nil
}

// GetMultusIfName return a network interface name that multus will assign when
// connected to the multus network.
func GetMultusIfName(selector string) (string, error) {
	multusMap, _ := parseMultusSelector(selector)
	var ifName string

	if name, ok := multusMap["interfaceRequest"]; ok {
		ifName = name
	}
	if name, ok := multusMap["interface"]; ok {
		ifName = name
	}

	// fail selector without interface name
	if ifName == "" {
		return "", fmt.Errorf("GetMultusIfname: missing interface")
	}

	return ifName, nil
}

// ApplyMultus apply multus selector to Pods
// Multus supports short and json syntax, use only one kind at a time.
func ApplyMultus(net rookv1.NetworkSpec, objectMeta *metav1.ObjectMeta) error {
	v := make([]string, 0, 2)
	shortSyntax := false
	jsonSyntax := false

	for _, ns := range net.Selectors {
		var multusMap map[string]string
		err := json.Unmarshal([]byte(ns), &multusMap)

		if err == nil {
			jsonSyntax = true
		} else {
			shortSyntax = true
		}

		v = append(v, string(ns))
	}

	if shortSyntax && jsonSyntax {
		return fmt.Errorf("ApplyMultus: Can't mix short and JSON form")
	}

	networks := strings.Join(v, ", ")
	if jsonSyntax {
		networks = "[" + networks + "]"
	}

	t := rookv1.Annotations{
		"k8s.v1.cni.cncf.io/networks": networks,
	}
	t.ApplyToObjectMeta(objectMeta)

	return nil
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
