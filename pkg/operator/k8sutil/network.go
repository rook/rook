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
	"net/netip"
	"strings"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplyMultus apply multus selector to Pods
// Multus supports short and json syntax, use only one kind at a time.
func ApplyMultus(clusterNamespace string, netSpec *cephv1.NetworkSpec, objectMeta *metav1.ObjectMeta) error {
	app, ok := objectMeta.Labels["app"]
	if !ok {
		app = "" // unknown app
	}

	netSelections := []*nadv1.NetworkSelectionElement{}

	// all apps get the public network
	pub, err := netSpec.GetNetworkSelection(clusterNamespace, cephv1.CephNetworkPublic)
	if err != nil {
		return errors.Wrapf(err, "failed to get %q network selection for app %q", cephv1.CephNetworkPublic, app)
	}
	netSelections = append(netSelections, pub)

	if isClusterNetApp(app) {
		cluster, err := netSpec.GetNetworkSelection(clusterNamespace, cephv1.CephNetworkCluster)
		if err != nil {
			return errors.Wrapf(err, "failed to get %q network selection for app %q", cephv1.CephNetworkCluster, app)
		}
		netSelections = append(netSelections, cluster)
	}

	annotationValue, err := cephv1.NetworkSelectionsToAnnotationValue(netSelections...)
	if err != nil {
		return errors.Wrapf(err, "failed to generate annotation value to apply nets %v to app %q", netSelections, app)
	}

	t := cephv1.Annotations{
		"k8s.v1.cni.cncf.io/networks": annotationValue,
	}
	t.ApplyToObjectMeta(objectMeta)

	return nil
}

func isClusterNetApp(app string) bool {
	return app == "rook-ceph-osd"
}

// ParseNetworkStatusAnnotation takes the annotation value from k8s.v1.cni.cncf.io/network-status
// and returns the network status struct.
func ParseNetworkStatusAnnotation(annotationValue string) ([]nadv1.NetworkStatus, error) {
	netStatuses := []nadv1.NetworkStatus{}
	if err := json.Unmarshal([]byte(annotationValue), &netStatuses); err != nil {
		return nil, errors.Wrapf(err, "failed to parse network status annotation %q", annotationValue)
	}
	return netStatuses, nil
}

func FindNetworkStatusByInterface(statuses []nadv1.NetworkStatus, ifaceName string) (nadv1.NetworkStatus, bool) {
	for _, s := range statuses {
		if s.Interface == ifaceName {
			return s, true
		}
	}
	return nadv1.NetworkStatus{}, false
}

// LinuxIpAddrResult provides a pared down Go struct for codifying json-formatted output from
// `ip --json address show`. Each result contains addresses associated with a single interface.
type LinuxIpAddrResult struct {
	InterfaceName string            `json:"ifname"`
	AddrInfo      []LinuxIpAddrInfo `json:"addr_info"`
}

type LinuxIpAddrInfo struct {
	Local     string `json:"local"`
	PrefixLen int    `json:"prefixlen"`
}

// ParseLinuxIpAddrOutput parses raw json-encoded `ip --json address show` output
func ParseLinuxIpAddrOutput(rawOutput string) ([]LinuxIpAddrResult, error) {
	results := []LinuxIpAddrResult{}
	if strings.TrimSpace(rawOutput) == "" {
		return results, errors.Errorf("cannot parse empty 'ip --json address' output")
	}
	if err := json.Unmarshal([]byte(rawOutput), &results); err != nil {
		return []LinuxIpAddrResult{}, errors.Wrap(err, "failed to parse 'ip --json address' output")
	}
	return results, nil
}

func GetIpAddressType(addresses []string) (discoveryv1.AddressType, error) {
	var addressType discoveryv1.AddressType

	if len(addresses) == 0 {
		return addressType, errors.New("failed to get the address type for ip address, address list is empty type")
	}
	ip, err := netip.ParseAddr(addresses[0])
	if err != nil {
		return addressType, errors.Wrapf(err, "failed to get the address type for ip address %q", addresses[0])
	}
	addressType = discoveryv1.AddressTypeIPv4
	if ip.Is6() {
		addressType = discoveryv1.AddressTypeIPv6
	}

	// validate all addresses are of the same type
	for _, addr := range addresses[1:] {
		ip, err := netip.ParseAddr(addr)
		if err != nil {
			return addressType, errors.Wrapf(err, "failed to get the address type for ip address %q", addr)
		}

		if ip.Is6() {
			if addressType != discoveryv1.AddressTypeIPv6 {
				return addressType, errors.Errorf("all ip address are not of same type %v", addresses)
			}
		} else {
			if addressType != discoveryv1.AddressTypeIPv4 {
				return addressType, errors.Errorf("all ip address are not of same type %v", addresses)
			}
		}
	}

	return addressType, nil
}
