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
	"context"
	"os"
	"reflect"
	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateUpdateClientProfileRadosNamespace(ctx context.Context, c client.Client, clusterInfo *cephclient.ClusterInfo, cephBlockPoolRadosNamespaceName, clusterID string) error {
	logger.Info("creating ceph-csi clientProfile CR for rados namespace")

	csiOpClientProfile := &csiopv1.ClientProfile{}
	csiOpClientProfile.Name = clusterID
	csiOpClientProfile.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
	csiOpClientProfile.Spec = csiopv1.ClientProfileSpec{
		CephConnectionRef: v1.LocalObjectReference{
			Name: clusterInfo.Namespace,
		},
		Rbd: &csiopv1.RbdConfigSpec{
			RadosNamespace: cephBlockPoolRadosNamespaceName,
			CephCsiSecrets: &csiopv1.CephCsiSecretsSpec{
				ControllerPublishSecret: v1.SecretReference{
					Name:      CsiRBDProvisionerSecret,
					Namespace: clusterInfo.Namespace,
				},
			},
		},
	}

	return createUpdateClientProfile(c, clusterInfo, csiOpClientProfile)
}

func CreateUpdateClientProfileSubVolumeGroup(ctx context.Context, c client.Client, clusterInfo *cephclient.ClusterInfo, cephFilesystemSubVolumeGroupName, clusterID string) error {
	logger.Info("Creating ceph-csi clientProfile CR for subvolume group")

	csiOpClientProfile := generateProfileSubVolumeGroupSpec(clusterInfo, cephFilesystemSubVolumeGroupName, clusterID)

	return createUpdateClientProfile(c, clusterInfo, csiOpClientProfile)
}

func generateProfileSubVolumeGroupSpec(clusterInfo *cephclient.ClusterInfo, cephFilesystemSubVolumeGroupName, clusterID string) *csiopv1.ClientProfile {
	csiOpClientProfile := &csiopv1.ClientProfile{}
	csiOpClientProfile.Name = clusterID
	csiOpClientProfile.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
	csiOpClientProfile.Spec = csiopv1.ClientProfileSpec{
		CephConnectionRef: v1.LocalObjectReference{
			Name: clusterInfo.Namespace,
		},
		CephFs: &csiopv1.CephFsConfigSpec{
			SubVolumeGroup: cephFilesystemSubVolumeGroupName,
			CephCsiSecrets: &csiopv1.CephCsiSecretsSpec{
				ControllerPublishSecret: v1.SecretReference{
					Name:      CsiCephFSProvisionerSecret,
					Namespace: clusterInfo.Namespace,
				},
			},
		},
	}

	applyCephFSMountOptions(clusterInfo, csiOpClientProfile.Spec.CephFs)

	return csiOpClientProfile
}

// CreateDefaultClientProfile creates a default client profile for csi-operator to connect driver
func CreateDefaultClientProfile(c client.Client, clusterInfo *cephclient.ClusterInfo) error {
	logger.Info("Creating ceph-csi clientProfile default CR")
	csiOpClientProfile := &csiopv1.ClientProfile{}
	csiOpClientProfile.Name = clusterInfo.Namespace
	csiOpClientProfile.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
	csiOpClientProfile.Spec = csiopv1.ClientProfileSpec{
		CephConnectionRef: v1.LocalObjectReference{
			Name: clusterInfo.Namespace,
		},
	}

	// set cephFS ControllerPublish Secret
	if csiOpClientProfile.Spec.CephFs == nil {
		csiOpClientProfile.Spec.CephFs = &csiopv1.CephFsConfigSpec{}
	}

	applyCephFSMountOptions(clusterInfo, csiOpClientProfile.Spec.CephFs)

	csiOpClientProfile.Spec.CephFs.CephCsiSecrets = &csiopv1.CephCsiSecretsSpec{
		ControllerPublishSecret: v1.SecretReference{
			Name:      CsiCephFSProvisionerSecret,
			Namespace: clusterInfo.Namespace,
		},
	}
	if csiOpClientProfile.Spec.Rbd == nil {
		csiOpClientProfile.Spec.Rbd = &csiopv1.RbdConfigSpec{}
	}

	// set RBD ControllerPublish Secret
	csiOpClientProfile.Spec.Rbd.CephCsiSecrets = &csiopv1.CephCsiSecretsSpec{
		ControllerPublishSecret: v1.SecretReference{
			Name:      CsiRBDProvisionerSecret,
			Namespace: clusterInfo.Namespace,
		},
	}

	return createUpdateClientProfile(c, clusterInfo, csiOpClientProfile)
}

func createUpdateClientProfile(c client.Client, clusterInfo *cephclient.ClusterInfo, clientProfile *csiopv1.ClientProfile) error {
	existingCsiOpClientProfile := &csiopv1.ClientProfile{}
	err := c.Get(clusterInfo.Context, types.NamespacedName{Name: clientProfile.Name, Namespace: clientProfile.Namespace}, existingCsiOpClientProfile)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = c.Create(clusterInfo.Context, clientProfile)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph-csi for clientProfile CR %q", clientProfile.Name)
			}
			logger.Infof("successfully created ceph-csi for clientProfile CR %q", clientProfile.Name)
			return nil
		}
		return err
	}

	existingCsiOpClientProfile.Spec = clientProfile.Spec
	err = c.Update(clusterInfo.Context, existingCsiOpClientProfile)
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph-csi for clientProfile CR %q", clientProfile.Name)
	}
	logger.Infof("successfully updated ceph-csi for clientProfile CR %q", clientProfile.Name)

	return nil
}

func applyCephFSMountOptions(clusterInfo *cephclient.ClusterInfo, cephFs *csiopv1.CephFsConfigSpec) {
	if reflect.DeepEqual(clusterInfo.CSIDriverSpec.CephFS, cephv1.CSICephFSSpec{}) {
		return
	}

	if clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions != "" {
		cephFs.KernelMountOptions = parseMountOptions(clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions)
	} else if clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions != "" {
		cephFs.FuseMountOptions = parseMountOptions(clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions)
	}
}

func parseMountOptions(options string) map[string]string {
	logger.Debugf("parsing CephFS mount options : %s ", options)
	result := map[string]string{}
	// Example: SplitSeq("ms_mode=prefer-secure,recover_session=clean", ",") iterates over ["ms_mode=prefer-secure", "recover_session=clean"]
	for part := range strings.SplitSeq(options, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Example: SplitN("ms_mode=prefer-secure", "=", 2) returns ["ms_mode", "prefer-secure"]
		keyVal := strings.SplitN(part, "=", 2)
		if len(keyVal) == 2 {
			result[strings.TrimSpace(keyVal[0])] = strings.TrimSpace(keyVal[1])
		}
	}
	logger.Debugf("parsed CephFS mount options: %v", result)
	return result
}
