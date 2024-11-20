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
<<<<<<< HEAD
	"os"
	"reflect"
	"runtime/debug"
	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
=======
	"strings"

	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

	csiopv1a1 "github.com/ceph/ceph-csi-operator/api/v1alpha1"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateUpdateClientProfileRadosNamespace(ctx context.Context, c client.Client, clusterInfo *cephclient.ClusterInfo, cephBlockPoolRadosNamespacedName types.NamespacedName, clusterID, clusterName string) error {

	logger.Info("creating ceph-csi clientProfile CR for rados namespace")

	csiOpClientProfile := &csiopv1a1.ClientProfile{}
	csiOpClientProfile.Name = clusterID
<<<<<<< HEAD
	csiOpClientProfile.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
=======
	csiOpClientProfile.Namespace = cephBlockPoolRadosNamespacedName.Namespace
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	csiOpClientProfile.Spec = csiopv1a1.ClientProfileSpec{
		CephConnectionRef: v1.LocalObjectReference{
			Name: clusterName,
		},
		Rbd: &csiopv1a1.RbdConfigSpec{
			RadosNamespace: cephBlockPoolRadosNamespacedName.Name,
		},
	}

<<<<<<< HEAD
	err := c.Get(ctx, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
=======
	err := clusterInfo.OwnerInfo.SetOwnerReference(csiOpClientProfile)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference for clientProfile CR %q for radosNamespace", csiOpClientProfile.Name)
	}

	err = c.Get(ctx, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
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

func CreateUpdateClientProfileSubVolumeGroup(ctx context.Context, c client.Client, clusterInfo *cephclient.ClusterInfo, cephFilesystemNamespacedName types.NamespacedName, clusterID, clusterName string) error {

	logger.Info("Creating ceph-csi clientProfile CR for subvolume group")

	csiOpClientProfile := generateProfileSubVolumeGroupSpec(clusterInfo, cephFilesystemNamespacedName, clusterID, clusterName)

<<<<<<< HEAD
	err := c.Get(ctx, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
=======
	err := clusterInfo.OwnerInfo.SetOwnerReference(csiOpClientProfile)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference for clientProfile CR %q for subVolGrp", csiOpClientProfile.Name)
	}

	err = c.Get(ctx, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
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

func generateProfileSubVolumeGroupSpec(clusterInfo *cephclient.ClusterInfo, cephFilesystemNamespacedName types.NamespacedName, clusterID, clusterName string) *csiopv1a1.ClientProfile {
	csiOpClientProfile := &csiopv1a1.ClientProfile{}
	csiOpClientProfile.Name = clusterID
<<<<<<< HEAD
	csiOpClientProfile.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
=======
	csiOpClientProfile.Namespace = cephFilesystemNamespacedName.Namespace
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	csiOpClientProfile.Spec = csiopv1a1.ClientProfileSpec{
		CephConnectionRef: v1.LocalObjectReference{
			Name: clusterName,
		},
		CephFs: &csiopv1a1.CephFsConfigSpec{
			SubVolumeGroup: cephFilesystemNamespacedName.Name,
		},
	}

<<<<<<< HEAD
	if !reflect.DeepEqual(clusterInfo.CSIDriverSpec.CephFS, cephv1.CSICephFSSpec{}) {
		if clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions != "" {
			kernelMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions, "=")
			csiOpClientProfile.Spec.CephFs.KernelMountOptions = map[string]string{kernelMountKeyVal[0]: kernelMountKeyVal[1]}
		} else if clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions != "" {
			fuseMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions, "=")
			csiOpClientProfile.Spec.CephFs.FuseMountOptions = map[string]string{fuseMountKeyVal[0]: fuseMountKeyVal[1]}
		}
=======
	kernelMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions, "=")
	fuseMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions, "=")

	if len(kernelMountKeyVal) == 2 {
		csiOpClientProfile.Spec.CephFs.KernelMountOptions = map[string]string{kernelMountKeyVal[0]: kernelMountKeyVal[1]}
	}

	if len(fuseMountKeyVal) == 2 {
		csiOpClientProfile.Spec.CephFs.FuseMountOptions = map[string]string{fuseMountKeyVal[0]: fuseMountKeyVal[1]}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	}

	return csiOpClientProfile
}

// CreateDefaultClientProfile creates a default client profile for csi-operator to connect driver
func CreateDefaultClientProfile(c client.Client, clusterInfo *cephclient.ClusterInfo, namespaced types.NamespacedName) error {
	logger.Info("Creating ceph-csi clientProfile default CR")
<<<<<<< HEAD
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic when creating the default client profile: %+v", r)
			logger.Errorf("Stack trace:")
			logger.Errorf(string(debug.Stack()))
		}
	}()

	csiOpClientProfile := &csiopv1a1.ClientProfile{}
	csiOpClientProfile.Name = clusterInfo.Namespace
	csiOpClientProfile.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
=======

	csiOpClientProfile := &csiopv1a1.ClientProfile{}
	csiOpClientProfile.Name = clusterInfo.Namespace
	csiOpClientProfile.Namespace = clusterInfo.Namespace
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	csiOpClientProfile.Spec = csiopv1a1.ClientProfileSpec{
		CephConnectionRef: v1.LocalObjectReference{
			Name: namespaced.Name,
		},
	}

<<<<<<< HEAD
	if !reflect.DeepEqual(clusterInfo.CSIDriverSpec.CephFS, cephv1.CSICephFSSpec{}) {
		if clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions != "" {
			kernelMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions, "=")
			if len(kernelMountKeyVal) >= 2 {
				csiOpClientProfile.Spec.CephFs = &csiopv1a1.CephFsConfigSpec{
					KernelMountOptions: map[string]string{kernelMountKeyVal[0]: kernelMountKeyVal[1]},
				}
			}
		} else if clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions != "" {
			fuseMountKeyVal := strings.Split(clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions, "=")
			if len(fuseMountKeyVal) >= 2 {
				csiOpClientProfile.Spec.CephFs = &csiopv1a1.CephFsConfigSpec{
					FuseMountOptions: map[string]string{fuseMountKeyVal[0]: fuseMountKeyVal[1]},
				}
			}
		}
	}

	err := c.Get(clusterInfo.Context, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
=======
	err := clusterInfo.OwnerInfo.SetOwnerReference(csiOpClientProfile)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference for default clientProfile CR %q", csiOpClientProfile.Name)
	}

	err = c.Get(clusterInfo.Context, types.NamespacedName{Name: csiOpClientProfile.Name, Namespace: csiOpClientProfile.Namespace}, csiOpClientProfile)
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
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
