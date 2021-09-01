/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"time"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

var HolderPodNotReadyResult = controllerruntime.Result{RequeueAfter: 2 * time.Second}

func GetHolderPodIP(pod *corev1.Pod) (string, error) {
	ip := pod.Status.PodIP
	if ip == "" {
		return "", errors.New("failed to get IP address info for pod")
	}
	return ip, nil
}

// GetHolderPodMultusNetwork returns the network status of the multus network attached to a holder pod
func GetHolderPodMultusNetwork(pod *corev1.Pod) (nettypes.NetworkStatus, error) {
	emptyNetStat := nettypes.NetworkStatus{}

	// get names of networks attached to the pod
	nets, err := nadutils.ParsePodNetworkAnnotation(pod)
	if err != nil {
		return emptyNetStat, errors.Wrap(err, "failed to determine multus network attached to pod")
	}
	if len(nets) != 1 {
		return emptyNetStat, errors.Errorf("failed to determine multus network attached to pod. "+
			"holder pod should have exactly one attached network (found %d): %v", len(nets), nets)
	}
	net := nets[0]
	// in the network status below, networks named by "<namespace>/<name>" in all cases for Rook (by experimental observation)
	multusNetName := fmt.Sprintf("%s/%s", net.Namespace, net.Name)

	netStats, err := nadutils.GetNetworkStatus(pod)
	if err != nil {
		return emptyNetStat, errors.Wrapf(err, "failed to get pod network status while looking for multus network %q", multusNetName)
	}

	for _, netStat := range netStats {
		if netStat.Name == multusNetName {
			return netStat, nil
		}
	}
	return emptyNetStat, errors.Errorf("failed to find network stats for multus network %q attached to pod", multusNetName)
}
