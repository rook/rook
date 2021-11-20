/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

func Setup(multusPods *corev1.PodList) error {
	// Get all IPs of pods with app: rook-ceph-multus
	// Look for namespace on node that has interface with the given pod IP
	// Migrate all interfaces not named eth0 or lo.
	logger.Info("starting multus interface migration")

	var podIPs []string
	for _, pod := range multusPods.Items {
		podIPs = append(podIPs, pod.Status.PodIP)
	}

	holderNS, err := determineHolderNS(podIPs)
	if err != nil {
		return errors.Wrap(err, "failed to determine the holder network namespace id")
	}

	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		return errors.Wrap(err, "failed to get the host network namespace")
	}

	logger.Info("ok, here I am")
	err = migrateInterfaces(hostNS, holderNS)
	if err != nil {
		return errors.Wrap(err, "failed to migrate multus interfaces to the host network namespace")
	}

	logger.Info("interface migration complete!")

	return nil
}
