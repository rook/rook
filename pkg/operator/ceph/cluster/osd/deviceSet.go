/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package osd

import (
	"github.com/pkg/errors"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Cluster) prepareStorageClassDeviceSets(config *provisionConfig) []rookalpha.VolumeSource {
	volumeSources := []rookalpha.VolumeSource{}
	for _, storageClassDeviceSet := range c.DesiredStorage.StorageClassDeviceSets {
		if err := opspec.CheckPodMemory(storageClassDeviceSet.Resources, cephOsdPodMinimumMemory); err != nil {
			config.addError("cannot use storageClassDeviceSet %s for creating osds %v", storageClassDeviceSet.Name, err)
			continue
		}
		for i := 0; i < storageClassDeviceSet.Count; i++ {
			pvc, err := c.createStorageClassDeviceSetPVC(storageClassDeviceSet, i)
			if err != nil {
				config.addError("%v", err)
				config.addError("OSD creation for storageClassDeviceSet %v failed for count %v", storageClassDeviceSet.Name, i)
				continue
			}
			volumeSources = append(volumeSources, rookalpha.VolumeSource{
				Name:      storageClassDeviceSet.Name,
				Resources: storageClassDeviceSet.Resources,
				Placement: storageClassDeviceSet.Placement,
				Config:    storageClassDeviceSet.Config,
				PersistentVolumeClaimSource: v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.GetName(),
					ReadOnly:  false,
				},
				Portable: storageClassDeviceSet.Portable,
			})
			logger.Infof("successfully provisioned osd for storageClassDeviceSet %s of set %v", storageClassDeviceSet.Name, i)
		}
	}
	return volumeSources
}

func (c *Cluster) createStorageClassDeviceSetPVC(storageClassDeviceSet rookalpha.StorageClassDeviceSet, setIndex int) (*v1.PersistentVolumeClaim, error) {
	if len(storageClassDeviceSet.VolumeClaimTemplates) == 0 {
		return nil, errors.Errorf("no PVC available for storageClassDeviceSet %s", storageClassDeviceSet.Name)
	}
	pvcStorageClassDeviceSetPVCId, pvcStorageClassDeviceSetPVCIdLabelSelector := makeStorageClassDeviceSetPVCID(storageClassDeviceSet.Name, setIndex, 0)

	pvc := makeStorageClassDeviceSetPVC(storageClassDeviceSet.Name, pvcStorageClassDeviceSetPVCId, 0, setIndex, storageClassDeviceSet.VolumeClaimTemplates[0])
	// Check if a PVC already exists with same StorageClassDeviceSet label
	presentPVCs, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).List(metav1.ListOptions{LabelSelector: pvcStorageClassDeviceSetPVCIdLabelSelector})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create pvc %s for storageClassDeviceSet %s", pvc.GetGenerateName(), storageClassDeviceSet.Name)
	}
	if len(presentPVCs.Items) == 0 { // No PVC found, creating a new one
		deployedPVC, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Create(pvc)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create pvc %s for storageClassDeviceSet %s", pvc.GetGenerateName(), storageClassDeviceSet.Name)
		}
		return deployedPVC, nil
	} else if len(presentPVCs.Items) == 1 { // The PVC is already present.
		return &presentPVCs.Items[0], nil
	}
	// More than one PVC exists with same labelSelector
	return nil, errors.Errorf("more than one PVCs exists with label %s, pvcs %s", pvcStorageClassDeviceSetPVCIdLabelSelector, presentPVCs)
}

func makeStorageClassDeviceSetPVC(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId string, pvcIndex, setIndex int, pvcTemplate v1.PersistentVolumeClaim) (pvcs *v1.PersistentVolumeClaim) {
	pvcLabels := makeStorageClassDeviceSetPVCLabel(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId, pvcIndex, setIndex)

	// pvc naming format <storageClassDeviceSetName>-<SetNumber>-<PVCIndex>
	pvcGenerateName := pvcStorageClassDeviceSetPVCId + "-"

	// Add pvcTemplate name to pvc name. i.e <storageClassDeviceSetName>-<SetNumber>-<PVCIndex>-<pvcTemplateName>
	if len(pvcTemplate.GetName()) != 0 {
		pvcGenerateName = pvcGenerateName + pvcTemplate.GetName() + "-"
	}

	// Add user provided labels to pvcTemplates
	for k, v := range pvcTemplate.GetLabels() {
		pvcLabels[k] = v
	}

	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: pvcGenerateName,
			Labels:       pvcLabels,
			Annotations:  pvcTemplate.Annotations,
		},
		Spec: pvcTemplate.Spec,
	}
}
