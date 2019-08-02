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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IsHost get whether to use host network provider
func (net *NetworkSpec) IsHost() bool {
	return net.Provider == "host"
}

// IsMultus get whether to use multus network provider
func (net *NetworkSpec) IsMultus() bool {
	return net.Provider == "multus"
}

// parseMultusSelector will parse short and JSON form of individual multus
// network attachment selection annotation. Valid JSON will be unmarshaled and
// return as is, while invalid JSON will be tried using
// <namespace>/<name>@<interface> short syntax.
// BUG(giovanism): There are no data (name, interface, namespace) validation
func parseMultusSelector(ns NetworkSelector) map[string]string {
	rawMap := make(map[string]string)

	err := json.Unmarshal([]byte(ns), &rawMap)

	if err != nil {
		// it can be in short form
		multusSelectorString := string(ns)

		nsEndIndex := strings.IndexAny(multusSelectorString, "/")
		if nsEndIndex != -1 {
			rawMap["namespace"] = multusSelectorString[:nsEndIndex]
		}

		ifStartIndex := strings.LastIndexAny(multusSelectorString, "@")
		if ifStartIndex != -1 && len(multusSelectorString)-ifStartIndex > 1 {
			rawMap["interface"] = multusSelectorString[ifStartIndex+1:]
		}

		if nsEndIndex != -1 && ifStartIndex != -1 && ifStartIndex-nsEndIndex > 1 {
			rawMap["name"] = multusSelectorString[nsEndIndex+1 : ifStartIndex]
		} else if nsEndIndex == -1 && ifStartIndex != -1 {
			rawMap["name"] = multusSelectorString[:ifStartIndex]
		} else if nsEndIndex != -1 && ifStartIndex == -1 {
			rawMap["name"] = multusSelectorString[nsEndIndex+1:]
		}
	}

	return rawMap
}

// GetMultusIfName return a network interface name that multus will assign when
// connected to the multus network.
// BUG(giovanism): Can't relliably tell the network interface name using a
// network selector alone if the interface name is omitted.
func GetMultusIfName(ns NetworkSelector) string {
	multusMap := parseMultusSelector(ns)
	ifName := "net1"

	if name, ok := multusMap["interfaceRequest"]; ok {
		ifName = name
	}
	if name, ok := multusMap["interface"]; ok {
		ifName = name
	}

	return ifName
}

// ApplyMultus apply multus selector to Pods
// Multus supports short and json syntax, use only one kind at a time.
// BUG(giovanism): Can't mix short and JSON form of multus network selection
// annotation.
func ApplyMultus(net NetworkSpec, objectMeta *metav1.ObjectMeta) {
	v := make([]string, 0, 2)
	useSquareBrackets := false

	for _, ns := range net.Selectors {
		if !useSquareBrackets {
			var multusMap map[string]string
			err := json.Unmarshal([]byte(ns), &multusMap)

			if err == nil {
				useSquareBrackets = true
			}
		}

		v = append(v, string(ns))
	}

	networks := strings.Join(v, ", ")
	if useSquareBrackets {
		networks = "[" + networks + "]"
	}

	t := Annotations{
		"k8s.v1.cni.cncf.io/networks": networks,
	}
	t.ApplyToObjectMeta(objectMeta)
}
