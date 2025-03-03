/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package csi

import (
	"os"

	"github.com/pkg/errors"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"

	csiopv1a1 "github.com/ceph/ceph-csi-operator/api/v1alpha1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateUpdateCephConnection(c client.Client, clusterInfo *cephclient.ClusterInfo, clusterSpec cephv1.ClusterSpec) error {

	logger.Infof("Configuring ceph connection CR %q in namespace %q", clusterInfo.NamespacedName().Name, clusterInfo.NamespacedName().Namespace)
	csiCephConnection := &csiopv1a1.CephConnection{}

	csiCephConnection.Name = clusterInfo.NamespacedName().Name
	csiCephConnection.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)

	spec, err := generateCephConnSpec(c, clusterInfo, csiCephConnection.Spec, clusterSpec)
	if err != nil {
		return errors.Wrapf(err, "failed to set ceph connection CR %q in namespace %q", csiCephConnection.Name, clusterInfo.Namespace)
	}

	err = c.Get(clusterInfo.Context, types.NamespacedName{Name: csiCephConnection.Name, Namespace: csiCephConnection.Namespace}, csiCephConnection)
	if err != nil {
		if kerrors.IsNotFound(err) {
			csiCephConnection.Spec = spec
			err = c.Create(clusterInfo.Context, csiCephConnection)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph connection CR %q", csiCephConnection.Name)
			}

			logger.Infof("Successfully created ceph connection CR %q", csiCephConnection.Name)
			return nil
		}
		return errors.Wrap(err, "failed to get ceph connection CR")
	}

	csiCephConnection.Spec = spec
	err = c.Update(clusterInfo.Context, csiCephConnection)
	if err != nil {
		return errors.Wrapf(err, "failed to update ceph connection CR %q", csiCephConnection.Name)
	}

	logger.Infof("Successfully updated ceph connection CR %q", csiCephConnection.Name)
	return nil
}

func generateCephConnSpec(c client.Client, clusterInfo *cephclient.ClusterInfo, csiClusterConnSpec csiopv1a1.CephConnectionSpec, clusterSpec cephv1.ClusterSpec) (csiopv1a1.CephConnectionSpec, error) {
	if clusterSpec.CSI.ReadAffinity.Enabled {
		csiClusterConnSpec = csiopv1a1.CephConnectionSpec{
			ReadAffinity: &csiopv1a1.ReadAffinitySpec{
				CrushLocationLabels: clusterSpec.CSI.ReadAffinity.CrushLocationLabels,
			},
		}
	}

	cephRBDMirrorList := &cephv1.CephRBDMirrorList{}
	err := c.List(clusterInfo.Context, cephRBDMirrorList, &client.ListOptions{Namespace: clusterInfo.Namespace})
	if err != nil {
		return csiClusterConnSpec, errors.Wrapf(err, "failed to list CephRBDMirror resource")
	}

	if len(cephRBDMirrorList.Items) == 0 {
		logger.Debug("no ceph CephRBDMirror found")
	} else {
		// Currently, only single RBD mirror is supported
		csiClusterConnSpec.RbdMirrorDaemonCount = cephRBDMirrorList.Items[0].Spec.Count
	}

	for _, mon := range clusterInfo.AllMonitors() {
		csiClusterConnSpec.Monitors = append(csiClusterConnSpec.Monitors, mon.Endpoint)
	}

	return csiClusterConnSpec, nil
}
