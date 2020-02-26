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
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Cluster) prepareStorageClassDeviceSets(config *provisionConfig) []rookv1.VolumeSource {
	volumeSources := []rookv1.VolumeSource{}

	// Iterate over storageClassDeviceSet
	for _, storageClassDeviceSet := range c.DesiredStorage.StorageClassDeviceSets {
		if err := opspec.CheckPodMemory(storageClassDeviceSet.Resources, cephOsdPodMinimumMemory); err != nil {
			config.addError("cannot use storageClassDeviceSet %q for creating osds %v", storageClassDeviceSet.Name, err)
			continue
		}
		for i := 0; i < storageClassDeviceSet.Count; i++ {
			// Check if the volume claim template has PVCs
			if len(storageClassDeviceSet.VolumeClaimTemplates) == 0 {
				logger.Warningf("no PVC available for storageClassDeviceSet %q", storageClassDeviceSet.Name)
				continue
			}
			for _, pvcTemplate := range storageClassDeviceSet.VolumeClaimTemplates {
				pvc, err := c.createStorageClassDeviceSetPVC(storageClassDeviceSet.Name, pvcTemplate, i)
				if err != nil {
					config.addError("failed to create osd for storageClassDeviceSet %q for count %d. %v", storageClassDeviceSet.Name, i, err)
					continue
				}
				volumeSources = append(volumeSources, rookv1.VolumeSource{
					Name:      storageClassDeviceSet.Name,
					Resources: storageClassDeviceSet.Resources,
					Placement: storageClassDeviceSet.Placement,
					Config:    storageClassDeviceSet.Config,
					Type:      pvcTemplate.GetName(),
					PersistentVolumeClaimSource: v1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.GetName(),
						ReadOnly:  false,
					},
					Portable:            storageClassDeviceSet.Portable,
					TuneSlowDeviceClass: storageClassDeviceSet.TuneSlowDeviceClass,
					CrushDeviceClass:    pvcTemplate.Annotations["crushDeviceClass"],
				})
				logger.Infof("successfully provisioned pvc %q for VolumeClaimTemplates %q for storageClassDeviceSet %q of set %v", pvc.GetName(), pvcTemplate.GetName(), storageClassDeviceSet.Name, i)
			}
		}
	}

	return volumeSources
}

func (c *Cluster) createStorageClassDeviceSetPVC(storageClassDeviceSetName string, pvcTemplate v1.PersistentVolumeClaim, setIndex int) (*v1.PersistentVolumeClaim, error) {
	// old labels and PVC ID
	pvcStorageClassDeviceSetPVCId, pvcStorageClassDeviceSetPVCIdLabelSelector := makeStorageClassDeviceSetPVCID(storageClassDeviceSetName, setIndex)
	pvc := makeStorageClassDeviceSetPVC(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId, setIndex, pvcTemplate)
	oldPresentPVCs, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).List(metav1.ListOptions{LabelSelector: pvcStorageClassDeviceSetPVCIdLabelSelector})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create pvc %s for storageClassDeviceSet %s", pvc.GetGenerateName(), storageClassDeviceSetName)
	}

	// return old labelled pvc, if we find any
	if len(oldPresentPVCs.Items) == 1 {
		logger.Debugf("old labelled pvc %q found", oldPresentPVCs.Items[0].Name)
		return &oldPresentPVCs.Items[0], nil
	}

	// check again with the new label for the presence of updated pvc
	pvcStorageClassDeviceSetPVCId, pvcStorageClassDeviceSetPVCIdLabelSelector = makeStorageClassDeviceSetPVCIDNew(storageClassDeviceSetName, pvcTemplate.GetName(), setIndex)
	pvc = makeStorageClassDeviceSetPVC(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId, setIndex, pvcTemplate)
	presentPVCs, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).List(metav1.ListOptions{LabelSelector: pvcStorageClassDeviceSetPVCIdLabelSelector})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create pvc %s for storageClassDeviceSet %s", pvc.GetGenerateName(), storageClassDeviceSetName)
	}

	presentPVCsNum := len(presentPVCs.Items)
	// No PVC found, creating a new one
	if presentPVCsNum == 0 {
		deployedPVC, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Create(pvc)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create pvc %q for storageClassDeviceSet %q", pvc.GetGenerateName(), storageClassDeviceSetName)
		}
		logger.Debugf("just created pvc %q", deployedPVC.Name)
		return deployedPVC, nil
		// The PVC is already present
	} else if presentPVCsNum == 1 {
		logger.Debugf("already present pvc %q", presentPVCs.Items[0].Name)
		// Updating with the new label
		return &presentPVCs.Items[0], nil
	}
	// More than one PVC exists with same labelSelector
	return nil, errors.Errorf("more than one PVCs exists with label %q, pvcs %q", pvcStorageClassDeviceSetPVCIdLabelSelector, presentPVCs)
}

func makeStorageClassDeviceSetPVC(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId string, setIndex int, pvcTemplate v1.PersistentVolumeClaim) (pvcs *v1.PersistentVolumeClaim) {
	pvcLabels := makeStorageClassDeviceSetPVCLabel(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId, setIndex)

	// pvc naming format <storageClassDeviceSetName>-<SetNumber>-<PVCIndex>
	pvcGenerateName := pvcStorageClassDeviceSetPVCId + "-"

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
