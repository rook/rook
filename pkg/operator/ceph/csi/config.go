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

	err := c.Get(ctx, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = c.Create(ctx, csiOpClientProfile)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph-csi clientProfile cr for RBD %q", csiOpClientProfile.Name)
			}
			logger.Infof("successfully created ceph-csi clientProfile CR for RBD %q", csiOpClientProfile.Name)
			return nil
		}
		return err
	}

	err = c.Update(ctx, csiOpClientProfile)
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph-csi clientProfile cr for RBD %q", csiOpClientProfile.Name)
	}
	logger.Infof("successfully updated ceph-csi clientProfile CR for RBD %q", csiOpClientProfile.Name)

	return nil
}

func CreateUpdateClientProfileSubVolumeGroup(ctx context.Context, c client.Client, clusterInfo *cephclient.ClusterInfo, cephFilesystemSubVolumeGroupName, clusterID string) error {
	logger.Info("Creating ceph-csi clientProfile CR for subvolume group")

	csiOpClientProfile := generateProfileSubVolumeGroupSpec(clusterInfo, cephFilesystemSubVolumeGroupName, clusterID)

	err := c.Get(ctx, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = c.Create(ctx, csiOpClientProfile)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph-csi clientProfile cr for subVolGrp %q", csiOpClientProfile.Name)
			}
			logger.Infof("successfully created ceph-csi clientProfile CR for subVolGrp %q", csiOpClientProfile.Name)
			return nil
		}
		return err
	}

	err = c.Update(ctx, csiOpClientProfile)
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph-csi clientProfile cr for subVolGrp %q", csiOpClientProfile.Name)
	}
	logger.Infof("successfully updated ceph-csi clientProfile CR for subVolGrp %q", csiOpClientProfile.Name)

	return nil
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

	if !reflect.DeepEqual(clusterInfo.CSIDriverSpec.CephFS, cephv1.CSICephFSSpec{}) {
		if clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions != "" {
			kernelMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions, "=")
			csiOpClientProfile.Spec.CephFs.KernelMountOptions = map[string]string{kernelMountKeyVal[0]: kernelMountKeyVal[1]}
		} else if clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions != "" {
			fuseMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions, "=")
			csiOpClientProfile.Spec.CephFs.FuseMountOptions = map[string]string{fuseMountKeyVal[0]: fuseMountKeyVal[1]}
		}
	}

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

	if !reflect.DeepEqual(clusterInfo.CSIDriverSpec.CephFS, cephv1.CSICephFSSpec{}) {
		if clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions != "" {
			kernelMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions, "=")
			if len(kernelMountKeyVal) >= 2 {
				csiOpClientProfile.Spec.CephFs = &csiopv1.CephFsConfigSpec{
					KernelMountOptions: map[string]string{kernelMountKeyVal[0]: kernelMountKeyVal[1]},
				}
			}
		} else if clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions != "" {
			fuseMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions, "=")
			if len(fuseMountKeyVal) >= 2 {
				csiOpClientProfile.Spec.CephFs = &csiopv1.CephFsConfigSpec{
					FuseMountOptions: map[string]string{fuseMountKeyVal[0]: fuseMountKeyVal[1]},
				}
			}
		}
	}

	// set cephFS ControllerPublish Secret
	if csiOpClientProfile.Spec.CephFs == nil {
		csiOpClientProfile.Spec.CephFs = &csiopv1.CephFsConfigSpec{}
	}

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

	err := c.Get(clusterInfo.Context, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = c.Create(clusterInfo.Context, csiOpClientProfile)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph-csi for default clientProfile CR %q", csiOpClientProfile.Name)
			}
			logger.Infof("successfully created ceph-csi for default clientProfile CR %q", csiOpClientProfile.Name)
			return nil
		}
		return err
	}

	err = c.Update(clusterInfo.Context, csiOpClientProfile)
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph-csi for default clientProfile CR %q", csiOpClientProfile.Name)
	}
	logger.Infof("successfully updated ceph-csi for default clientProfile CR %q", csiOpClientProfile.Name)

	return nil
}
