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

package nfs

import (
	"fmt"

	"github.com/pkg/errors"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type nfsProvisioner struct {
	client     kubernetes.Interface
	rookClient rookclient.Interface
}

var _ controller.Provisioner = &nfsProvisioner{}

// NewNFSProvisioner returns an instance of nfsProvisioner
func NewNFSProvisioner(clientset kubernetes.Interface, rookClientset rookclient.Interface) *nfsProvisioner {
	return &nfsProvisioner{clientset, rookClientset}
}

func (p *nfsProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
	logger.Infof("nfs provisioner: VolumeOptions %v", options)

	// Get the storage class for this volume.
	storageClass, err := p.getClassForPVC(options.PVC)
	if err != nil {
		return nil, err
	}

	if options.PVC.Spec.Selector != nil {
		return nil, fmt.Errorf("claim Selector is not supported")
	}

	nfsServerName, present := storageClass.Parameters["nfsServerName"]
	if !present {
		return nil, errors.Errorf("NFS share Path not found in the storageclass: %v", storageClass.GetName())
	}

	nfsNamespace, present := storageClass.Parameters["nfsServerNamespace"]
	if !present {
		return nil, errors.Errorf("NFS share Path not found in the storageclass: %v", storageClass.GetName())
	}

	exportName, present := storageClass.Parameters["exportName"]
	if !present {
		return nil, errors.Errorf("NFS share Path not found in the storageclass: %v", storageClass.GetName())
	}

	nfsVolumeSource, err := p.getNFSVolumeSource(nfsServerName, nfsNamespace, exportName)
	if err != nil {
		return nil, err
	}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			MountOptions:                  options.MountOptions,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				NFS: nfsVolumeSource,
			},
		},
	}
	return pv, nil
}

func (p *nfsProvisioner) Delete(volume *v1.PersistentVolume) error {
	return nil
}

// getClassForPV returns StorageClass
func (p *nfsProvisioner) getClassForPV(pv *v1.PersistentVolume) (*storage.StorageClass, error) {
	if p.client == nil {
		return nil, fmt.Errorf("Cannot get kube client")
	}
	className := helper.GetPersistentVolumeClass(pv)
	if className == "" {
		return nil, fmt.Errorf("Volume has no storage class")
	}
	class, err := p.client.StorageV1().StorageClasses().Get(className, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return class, nil
}

// getClassForPVC returns StorageClass
func (p *nfsProvisioner) getClassForPVC(pvc *v1.PersistentVolumeClaim) (*storage.StorageClass, error) {
	if p.client == nil {
		return nil, fmt.Errorf("Cannot get kube client")
	}
	className := helper.GetPersistentVolumeClaimClass(pvc)
	if className == "" {
		return nil, fmt.Errorf("Volume has no storage class")
	}
	class, err := p.client.StorageV1().StorageClasses().Get(className, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return class, nil
}

// getNFSVolumeSource returns the nfs source configuration for the pv
func (p *nfsProvisioner) getNFSVolumeSource(nfsServerName, namespace, exportName string) (*v1.NFSVolumeSource, error) {
	nfsServer, err := p.rookClient.NfsV1alpha1().NFSServers(namespace).Get(nfsServerName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	nfsService, err := p.client.CoreV1().Services(namespace).Get(nfsServerName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	for _, export := range nfsServer.Spec.Exports {
		if export.Name == exportName {
			return &v1.NFSVolumeSource{
				ReadOnly: false,
				Server:   nfsService.Spec.ClusterIP,
				Path:     "/" + export.PersistentVolumeClaim.ClaimName,
			}, nil
		}
	}
	return nil, errors.Errorf("exportName not Found")
}
