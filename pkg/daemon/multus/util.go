/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package multus

import (
	"fmt"
	"os"

	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Logger defines the log types that this library will use for outputting status information to the
// user. Because this library may be called interactively by a user or programmatically by another
// application, we allow the calling application to make a very simple Logger type that will be
// compatible with this library.
type Logger interface {
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Warningf(format string, args ...interface{})
}

type SimpleStderrLogger struct{}

func (*SimpleStderrLogger) Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, " INFO: "+format+"\n", args...)
}

func (*SimpleStderrLogger) Debugf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "DEBUG: "+format+"\n", args...)
}

func (*SimpleStderrLogger) Warningf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, " WARN: "+format+"\n", args...)
}

func getNetworksFromPod(
	pod *core.Pod,
	desiredPublicNet, desiredClusterNet *types.NamespacedName,
) (
	publicAddr, clusterAddr string, suggestions []string, err error,
) {
	nets, err := nadutils.GetNetworkStatus(pod)
	if err != nil {
		return "", "", unableToProvideAddressSuggestions,
			fmt.Errorf("pod has no network status yet: %w", err)
	}
	if len(nets) == 0 {
		return "", "", unableToProvideAddressSuggestions,
			fmt.Errorf("pod has no attached networks yet")
	}

	// if the pod has networks attached, parse all of them, and report any debugging suggestions
	// associated for each one
	publicAddr, clusterAddr = "", ""
	suggestions = []string{}
	for _, net := range nets {
		nsName, err := networkNamespacedName(net.Name, pod.Namespace)
		if err != nil {
			suggestions = append(suggestions,
				fmt.Sprintf("pod has an attached network with an un-parse-able network name [%s]; "+
					"not sure what to do; this is unlikely to resolve", net.Name),
			)
		}
		if len(net.IPs) == 0 {
			suggestions = append(suggestions,
				fmt.Sprintf("pod has an attached network [%s] with no IP; "+
					"could this be an IPAM issue?", net.Name))
		}

		ip := net.IPs[0] // any IP should work
		if desiredPublicNet != nil && nsName == *desiredPublicNet {
			// FOUND DESIRED PUBLIC ADDR
			publicAddr = ip
		}
		if desiredClusterNet != nil && nsName == *desiredClusterNet {
			// FOUND DESIRED CLUSTER ADDR
			clusterAddr = ip
		}
	}

	if desiredPublicNet != nil && publicAddr == "" {
		suggestions = append(suggestions, "pod does not have a public network attachment")
	}
	if desiredClusterNet != nil && clusterAddr == "" {
		suggestions = append(suggestions, "pod does not have a cluster network attachment")
	}

	// if there are any suggestions, something isn't right
	if len(suggestions) > 0 {
		suggestions = append(suggestions, unableToProvideAddressSuggestions...)
		return publicAddr, clusterAddr, suggestions, fmt.Errorf("did not find all desired networks")
	}

	// found what we were looking for
	return publicAddr, clusterAddr, []string{}, nil
}

func podIsRunning(pod core.Pod) bool {
	return pod.Status.Phase == core.PodRunning
}

func podIsReady(pod core.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == core.PodReady {
			return c.Status == core.ConditionTrue
		}
	}
	return false
}

func networkNamespacedName(netName, podNamespace string) (types.NamespacedName, error) {
	nsName := types.NamespacedName{}
	// what we really want is the result of parsePodNetworkObjectText(), a private method.
	// because the network name is in the same format as network annotations, we can at
	// at least use ParseNetworkAnnotation() to parse the network name
	netSelections, err := nadutils.ParseNetworkAnnotation(netName, podNamespace)
	if err != nil {
		return nsName, fmt.Errorf("failed to parse network name %q into namespaced name: %w", netName, err)
	}

	if len(netSelections) != 1 {
		return nsName, fmt.Errorf("failed to parse network name %q into a single namespaced network name, instead: %v", netName, netSelections)
	}

	net := netSelections[0]
	nsName.Namespace = net.Namespace
	nsName.Name = net.Name
	return nsName, nil
}
