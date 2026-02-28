// Copyright (c) 2021 Kubernetes Network Plumbing Working Group
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cni100 "github.com/containernetworking/cni/pkg/types/100"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// convertDNS converts CNI's DNS type to client DNS
func convertDNS(dns cnitypes.DNS) *v1.DNS {
	var v1dns v1.DNS

	v1dns.Nameservers = append([]string{}, dns.Nameservers...)
	v1dns.Domain = dns.Domain
	v1dns.Search = append([]string{}, dns.Search...)
	v1dns.Options = append([]string{}, dns.Options...)

	return &v1dns
}

// SetNetworkStatus updates the Pod status
func SetNetworkStatus(client kubernetes.Interface, pod *corev1.Pod, statuses []v1.NetworkStatus) error {
	if client == nil {
		return fmt.Errorf("no client set")
	}

	if pod == nil {
		return fmt.Errorf("no pod set")
	}

	var networkStatus []string
	if statuses != nil {
		for _, status := range statuses {
			data, err := json.MarshalIndent(status, "", "    ")
			if err != nil {
				return fmt.Errorf("SetNetworkStatus: error with Marshal Indent: %v", err)
			}
			networkStatus = append(networkStatus, string(data))
		}
	}

	err := setPodNetworkStatus(client, pod, fmt.Sprintf("[%s]", strings.Join(networkStatus, ",")))
	if err != nil {
		return fmt.Errorf("SetNetworkStatus: failed to update the pod %s in out of cluster comm: %v", pod.Name, err)
	}
	return nil
}

func setPodNetworkStatus(client kubernetes.Interface, pod *corev1.Pod, networkstatus string) error {
	if len(pod.Annotations) == 0 {
		pod.Annotations = make(map[string]string)
	}

	coreClient := client.CoreV1()
	var err error
	name := pod.Name
	namespace := pod.Namespace

	resultErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pod, err = coreClient.Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if len(pod.Annotations) == 0 {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[v1.NetworkStatusAnnot] = networkstatus
		_, err = coreClient.Pods(namespace).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
		return err
	})
	if resultErr != nil {
		return fmt.Errorf("status update failed for pod %s/%s: %v", pod.Namespace, pod.Name, resultErr)
	}
	return nil
}

// GetNetworkStatus returns pod's network status
func GetNetworkStatus(pod *corev1.Pod) ([]v1.NetworkStatus, error) {
	if pod == nil {
		return nil, fmt.Errorf("cannot find pod")
	}
	if pod.Annotations == nil {
		return nil, fmt.Errorf("cannot find pod annotation")
	}

	netStatusesJson, ok := pod.Annotations[v1.NetworkStatusAnnot]
	if !ok {
		return nil, fmt.Errorf("cannot find network status")
	}

	var netStatuses []v1.NetworkStatus
	err := json.Unmarshal([]byte(netStatusesJson), &netStatuses)

	return netStatuses, err
}

// gatewayInterfaceIndex determines the index of the first interface that has a gateway
func gatewayInterfaceIndex(ips []*cni100.IPConfig) int {
	for _, ipConfig := range ips {
		if ipConfig.Gateway != nil && ipConfig.Interface != nil {
			return *ipConfig.Interface
		}
	}
	return -1
}

// CreateNetworkStatuses creates an array of NetworkStatus from CNI result
// Not to be confused with CreateNetworkStatus (singular)
// This is the preferred method and picks up when CNI ADD results contain multiple container interfaces
func CreateNetworkStatuses(r cnitypes.Result, networkName string, defaultNetwork bool, dev *v1.DeviceInfo) ([]*v1.NetworkStatus, error) {
	var networkStatuses []*v1.NetworkStatus
	// indexMap is from original CNI result index to networkStatuses index
	indexMap := make(map[int]int)

	// Convert whatever the IPAM result was into the current Result type
	result, err := cni100.NewResultFromResult(r)
	if err != nil {
		return nil, fmt.Errorf("error converting the type.Result to cni100.Result: %v", err)
	}

	if len(result.Interfaces) == 1 {
		networkStatus, err := CreateNetworkStatus(r, networkName, defaultNetwork, dev)
		return []*v1.NetworkStatus{networkStatus}, err
	}

	// Discover default routes upfront and reuse them if necessary.
	var useDefaultRoute []string
	for _, route := range result.Routes {
		if isDefaultRoute(route) {
			useDefaultRoute = append(useDefaultRoute, route.GW.String())
		}
	}

	// Same for DNS
	v1dns := convertDNS(result.DNS)

	// Check for a gateway-associated interface, we'll use this later if we did to mark as the default.
	gwInterfaceIdx := -1
	if defaultNetwork {
		gwInterfaceIdx = gatewayInterfaceIndex(result.IPs)
	}

	// Initialize NetworkStatus for each container interface (e.g. with sandbox present)
	indexOfFoundPodInterface := 0
	foundFirstSandboxIface := false
	didSetDefault := false
	for i, iface := range result.Interfaces {
		if iface.Sandbox != "" {
			isDefault := false

			// If there's a gateway listed for this interface index found in the ips, we mark that interface as default
			// notably, we use the first one we find.
			if defaultNetwork && i == gwInterfaceIdx && !didSetDefault {
				isDefault = true
				didSetDefault = true
			}

			// Otherwise, if we didn't find it, we use the first sandbox interface.
			if defaultNetwork && gwInterfaceIdx == -1 && !foundFirstSandboxIface {
				isDefault = true
				foundFirstSandboxIface = true
			}

			ns := &v1.NetworkStatus{
				Name:       networkName,
				Default:    isDefault,
				Interface:  iface.Name,
				Mac:        iface.Mac,
				Mtu:        iface.Mtu,
				IPs:        []string{},
				Gateway:    useDefaultRoute,
				DeviceInfo: dev,
				DNS:        *v1dns,
			}
			networkStatuses = append(networkStatuses, ns)
			// Map original index to the new slice index
			indexMap[i] = indexOfFoundPodInterface
			indexOfFoundPodInterface++
		}
	}

	var defaultNetworkStatus *v1.NetworkStatus
	if len(networkStatuses) > 0 {
		// Set the default network status to the last network status.
		defaultNetworkStatus = networkStatuses[len(networkStatuses)-1]
	}

	// Map IPs to network interface based on index
	for _, ipConfig := range result.IPs {
		if ipConfig.Interface != nil {
			originalIndex := *ipConfig.Interface
			if newIndex, ok := indexMap[originalIndex]; ok {
				ns := networkStatuses[newIndex]
				ns.IPs = append(ns.IPs, ipConfig.Address.IP.String())
			}
		} else {
			// If the IPs don't specify the interface assign the IP to the default network status. This keeps the behaviour
			// consistent with previous multus versions.
			if defaultNetworkStatus != nil {
				defaultNetworkStatus.IPs = append(defaultNetworkStatus.IPs, ipConfig.Address.IP.String())
			}
		}
	}

	return networkStatuses, nil
}

// CreateNetworkStatus create NetworkStatus from CNI result
func CreateNetworkStatus(r cnitypes.Result, networkName string, defaultNetwork bool, dev *v1.DeviceInfo) (*v1.NetworkStatus, error) {
	netStatus := &v1.NetworkStatus{}
	netStatus.Name = networkName
	netStatus.Default = defaultNetwork

	// Convert whatever the IPAM result was into the current Result type
	result, err := cni100.NewResultFromResult(r)
	if err != nil {
		return netStatus, fmt.Errorf("error convert the type.Result to cni100.Result: %v", err)
	}

	for _, ifs := range result.Interfaces {
		// Only pod interfaces can have sandbox information
		if ifs.Sandbox != "" {
			netStatus.Interface = ifs.Name
			netStatus.Mac = ifs.Mac
			netStatus.Mtu = ifs.Mtu
		}
	}

	for _, ipconfig := range result.IPs {
		netStatus.IPs = append(netStatus.IPs, ipconfig.Address.IP.String())
	}

	for _, route := range result.Routes {
		if isDefaultRoute(route) {
			netStatus.Gateway = append(netStatus.Gateway, route.GW.String())
		}
	}

	v1dns := convertDNS(result.DNS)
	netStatus.DNS = *v1dns

	if dev != nil {
		netStatus.DeviceInfo = dev
	}

	return netStatus, nil
}

func isDefaultRoute(route *cnitypes.Route) bool {
	return route.Dst.IP == nil && route.Dst.Mask == nil ||
		route.Dst.IP.Equal(net.IPv4zero) ||
		route.Dst.IP.Equal(net.IPv6zero)
}

// ParsePodNetworkAnnotation parses Pod annotation for net-attach-def and get NetworkSelectionElement
func ParsePodNetworkAnnotation(pod *corev1.Pod) ([]*v1.NetworkSelectionElement, error) {
	netAnnot := pod.Annotations[v1.NetworkAttachmentAnnot]
	defaultNamespace := pod.Namespace

	if len(netAnnot) == 0 {
		return nil, &v1.NoK8sNetworkError{Message: "no kubernetes network found"}
	}

	networks, err := ParseNetworkAnnotation(netAnnot, defaultNamespace)
	if err != nil {
		return nil, err
	}
	return networks, nil
}

// ParseNetworkAnnotation parses actual annotation string and get NetworkSelectionElement
func ParseNetworkAnnotation(podNetworks, defaultNamespace string) ([]*v1.NetworkSelectionElement, error) {
	var networks []*v1.NetworkSelectionElement

	if podNetworks == "" {
		return nil, fmt.Errorf("parsePodNetworkAnnotation: pod annotation not having \"network\" as key")
	}

	if strings.IndexAny(podNetworks, "[{\"") >= 0 {
		if err := json.Unmarshal([]byte(podNetworks), &networks); err != nil {
			return nil, fmt.Errorf("parsePodNetworkAnnotation: failed to parse pod Network Attachment Selection Annotation JSON format: %v", err)
		}
	} else {
		// Comma-delimited list of network attachment object names
		for _, item := range strings.Split(podNetworks, ",") {
			// Remove leading and trailing whitespace.
			item = strings.TrimSpace(item)

			// Parse network name (i.e. <namespace>/<network name>@<ifname>)
			netNsName, networkName, netIfName, err := parsePodNetworkObjectText(item)
			if err != nil {
				return nil, fmt.Errorf("parsePodNetworkAnnotation: %v", err)
			}

			networks = append(networks, &v1.NetworkSelectionElement{
				Name:             networkName,
				Namespace:        netNsName,
				InterfaceRequest: netIfName,
			})
		}
	}

	for _, net := range networks {
		if net.Namespace == "" {
			net.Namespace = defaultNamespace
		}
	}

	return networks, nil
}

// parsePodNetworkObjectText parses annotation text and returns
// its triplet, (namespace, name, interface name).
func parsePodNetworkObjectText(podnetwork string) (string, string, string, error) {
	var netNsName string
	var netIfName string
	var networkName string

	slashItems := strings.Split(podnetwork, "/")
	if len(slashItems) == 2 {
		netNsName = strings.TrimSpace(slashItems[0])
		networkName = slashItems[1]
	} else if len(slashItems) == 1 {
		networkName = slashItems[0]
	} else {
		return "", "", "", fmt.Errorf("Invalid network object (failed at '/')")
	}

	atItems := strings.Split(networkName, "@")
	networkName = strings.TrimSpace(atItems[0])
	if len(atItems) == 2 {
		netIfName = strings.TrimSpace(atItems[1])
	} else if len(atItems) != 1 {
		return "", "", "", fmt.Errorf("Invalid network object (failed at '@')")
	}

	// Check and see if each item matches the specification for valid attachment name.
	// "Valid attachment names must be comprised of units of the DNS-1123 label format"
	// [a-z0-9]([-a-z0-9]*[a-z0-9])?
	// And we allow at (@), and forward slash (/) (units separated by commas)
	// It must start and end alphanumerically.
	allItems := []string{netNsName, networkName, netIfName}
	for i := range allItems {
		matched, _ := regexp.MatchString("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$", allItems[i])
		if !matched && len([]rune(allItems[i])) > 0 {
			return "", "", "", fmt.Errorf(fmt.Sprintf("Failed to parse: one or more items did not match comma-delimited format (must consist of lower case alphanumeric characters). Must start and end with an alphanumeric character), mismatch @ '%v'", allItems[i]))
		}
	}

	return netNsName, networkName, netIfName, nil
}
