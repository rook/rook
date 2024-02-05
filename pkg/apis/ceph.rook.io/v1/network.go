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

package v1

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
	"github.com/pkg/errors"
)

// IsMultus get whether to use multus network provider
func (n *NetworkSpec) IsMultus() bool {
	return n.Provider == NetworkProviderMultus
}

// IsHost get whether to use host network provider. This method also preserve
// compatibility with the old HostNetwork field.
func (n *NetworkSpec) IsHost() bool {
	return (n.HostNetwork && n.Provider == NetworkProviderDefault) || n.Provider == NetworkProviderHost
}

func ValidateNetworkSpec(clusterNamespace string, spec NetworkSpec) error {
	if spec.HostNetwork && (spec.Provider != NetworkProviderDefault) {
		return errors.Errorf(`the legacy hostNetwork setting is only valid with the default network provider ("") and not with '%q'`, spec.Provider)
	}
	if spec.IsMultus() {
		if len(spec.Selectors) == 0 {
			return errors.Errorf("at least one network selector must be specified when using the %q network provider", NetworkProviderMultus)
		}

		if _, err := spec.GetNetworkSelection(clusterNamespace, CephNetworkPublic); err != nil {
			return errors.Wrap(err, "ceph public network selector provided for multus is invalid")
		}
		if _, err := spec.GetNetworkSelection(clusterNamespace, CephNetworkCluster); err != nil {
			return errors.Wrap(err, "ceph cluster network selector provided for multus is invalid")
		}
	}

	if !spec.AddressRanges.IsEmpty() {
		if !spec.IsMultus() && !spec.IsHost() {
			// TODO: be sure to update docs that AddressRanges can be specified for host networking  as
			// well as multus so that the override configmap doesn't need to be set
			return errors.Errorf("network ranges can only be specified for %q and %q network providers", NetworkProviderHost, NetworkProviderMultus)
		}
		if spec.IsMultus() {
			if len(spec.AddressRanges.Public) > 0 && !spec.NetworkHasSelection(CephNetworkPublic) {
				return errors.Errorf("public address range can only be specified for multus if there is a public network selection")
			}
			if len(spec.AddressRanges.Cluster) > 0 && !spec.NetworkHasSelection(CephNetworkCluster) {
				return errors.Errorf("cluster address range can only be specified for multus if there is a cluster network selection")
			}
		}
	}

	if err := spec.AddressRanges.Validate(); err != nil {
		return err
	}

	return nil
}

func ValidateNetworkSpecUpdate(clusterNamespace string, oldSpec, newSpec NetworkSpec) error {
	// Allow an attempt to enable or disable host networking, but not other provider changes
	oldProvider := oldSpec.Provider
	newProvider := newSpec.Provider
	if oldProvider != newProvider && oldProvider != "host" && newProvider != "host" {
		return errors.Errorf("invalid update: network provider change from %q to %q is not allowed", oldProvider, newProvider)
	}

	return ValidateNetworkSpec(clusterNamespace, newSpec)
}

// NetworkHasSelection returns true if the given Ceph network has a selection.
func (n *NetworkSpec) NetworkHasSelection(network CephNetworkType) bool {
	s, ok := n.Selectors[network]
	if !ok || s == "" {
		return false
	}
	return true
}

// GetNetworkSelection gets the network selection for a given Ceph network, or nil if the network
// doesn't have a selection.
func (n *NetworkSpec) GetNetworkSelection(clusterNamespace string, network CephNetworkType) (*nadv1.NetworkSelectionElement, error) {
	if !n.NetworkHasSelection(network) {
		return nil, nil // no selection for network
	}
	s := n.Selectors[network]
	// From documentation of the "k8s.v1.cni.cncf.io/network-status" annotation, valid JSON inputs
	// must be in list form, surrounded with brackets. The NAD utility library will only parse
	// list-format JSON input. However, old versions of Rook code allowed non-list JSON objects.
	// In order to support legacy users, make an attempt to turn single-JSON-object inputs into
	// len(1) lists so that they parse correctly by the util library. Do not advertise this
	// "feature" in documentation since it is not technically the correct format.
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		s = "[" + s + "]"
	}
	selection, err := nadutils.ParseNetworkAnnotation(s, clusterNamespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q network selector %q", network, s)
	}
	if len(selection) != 1 {
		return nil, errors.Errorf("%q network selector %q has multiple (%d) selections, which is not supported", network, s, len(selection))
	}
	return selection[0], nil
}

// NetworkSelectionsToAnnotationValue converts NetworkAttachmentDefinition network selection
// elements to an annotation value for the "k8s.v1.cni.cncf.io/networks" annotation key.
func NetworkSelectionsToAnnotationValue(selections ...*nadv1.NetworkSelectionElement) (string, error) {
	reduced := []*nadv1.NetworkSelectionElement{}
	for _, s := range selections {
		if s != nil {
			reduced = append(reduced, s)
		}
	}
	if len(reduced) == 0 {
		return "", nil
	}
	b, err := json.Marshal(reduced)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert network selections to annotation value")
	}
	return string(b), nil
}

func (n *AddressRangesSpec) IsEmpty() bool {
	return n == nil || len(n.Public) == 0 && len(n.Cluster) == 0
}

func (n *AddressRangesSpec) Validate() error {
	if n.IsEmpty() {
		return nil
	}

	allRanges := append(n.Public, n.Cluster...)
	invalid := []string{}
	for _, cidr := range allRanges {
		_, _, err := net.ParseCIDR(string(cidr))
		if err != nil {
			// returned err is "invalid CIDR: <addr>" & not more useful than invalid list below
			invalid = append(invalid, string(cidr))
		}
	}
	if len(invalid) == 0 {
		return nil
	}

	return fmt.Errorf("%d network ranges are invalid: %v", len(invalid), invalid)
}

// String turns a CIDR list into a comma-delimited string of CIDRs
func (l *CIDRList) String() string {
	sl := []string{}
	for _, c := range *l {
		sl = append(sl, string(c))
	}
	return strings.Join(sl, ", ")
}
