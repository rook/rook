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
	"fmt"
	"strings"

	"github.com/pkg/errors"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Cluster) prepareStorageClassDeviceSets(config *provisionConfig) []rookv1.VolumeSource {
	volumeSources := []rookv1.VolumeSource{}

	// Iterate over storageClassDeviceSet
	for _, storageClassDeviceSet := range c.spec.Storage.StorageClassDeviceSets {
		if err := controller.CheckPodMemory(storageClassDeviceSet.Resources, cephOsdPodMinimumMemory); err != nil {
			config.addError("cannot use storageClassDeviceSet %q for creating osds %v", storageClassDeviceSet.Name, err)
			continue
		}
		for i := 0; i < storageClassDeviceSet.Count; i++ {
			// Check if the volume claim template has PVCs
			if len(storageClassDeviceSet.VolumeClaimTemplates) == 0 {
				logger.Warningf("no PVC available for storageClassDeviceSet %q", storageClassDeviceSet.Name)
				continue
			}

			// Create the PVC source for each of the data, metadata, and other types of templates if defined.
			pvcSources := map[string]v1.PersistentVolumeClaimVolumeSource{}
			var dataSize string
			var crushDeviceClass string
			for _, pvcTemplate := range storageClassDeviceSet.VolumeClaimTemplates {
				if pvcTemplate.Name == "" {
					// For backward compatibility a blank name must be treated as a data volume
					pvcTemplate.Name = bluestorePVCData
				}

				pvc, err := c.createStorageClassDeviceSetPVC(storageClassDeviceSet.Name, pvcTemplate, i)
				if err != nil {
					config.addError("failed to create osd for storageClassDeviceSet %q for count %d. %v", storageClassDeviceSet.Name, i, err)
					continue
				}

				if pvcTemplate.Name == bluestorePVCData {
					pvcSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
					dataSize = pvcSize.String()
					crushDeviceClass = pvcTemplate.Annotations["crushDeviceClass"]
				}
				pvcSources[pvcTemplate.Name] = v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.GetName(),
					ReadOnly:  false,
				}
				logger.Infof("successfully provisioned pvc %q for VolumeClaimTemplates %q for storageClassDeviceSet %q of set %v", pvc.GetName(), pvcTemplate.GetName(), storageClassDeviceSet.Name, i)
			}

			volumeSources = append(volumeSources, rookv1.VolumeSource{
				Name:                storageClassDeviceSet.Name,
				Resources:           storageClassDeviceSet.Resources,
				Placement:           storageClassDeviceSet.Placement,
				Config:              storageClassDeviceSet.Config,
				Size:                dataSize,
				PVCSources:          pvcSources,
				Portable:            storageClassDeviceSet.Portable,
				TuneSlowDeviceClass: storageClassDeviceSet.TuneSlowDeviceClass,
				SchedulerName:       storageClassDeviceSet.SchedulerName,
				CrushDeviceClass:    crushDeviceClass,
				Encrypted:           storageClassDeviceSet.Encrypted,
			})
		}
	}

	return volumeSources
}

func (c *Cluster) createStorageClassDeviceSetPVC(storageClassDeviceSetName string, pvcTemplate v1.PersistentVolumeClaim, setIndex int) (*v1.PersistentVolumeClaim, error) {
	// old labels and PVC ID
	pvcStorageClassDeviceSetPVCId, pvcStorageClassDeviceSetPVCIdLabelSelector := makeStorageClassDeviceSetPVCID(storageClassDeviceSetName, setIndex)
	pvc := makeStorageClassDeviceSetPVC(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId, setIndex, pvcTemplate)
	oldPresentPVCs, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).List(metav1.ListOptions{LabelSelector: pvcStorageClassDeviceSetPVCIdLabelSelector})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pvc %s for storageClassDeviceSet %s", pvcStorageClassDeviceSetPVCIdLabelSelector, storageClassDeviceSetName)
	}

	// return old labeled pvc, if we find any
	if len(oldPresentPVCs.Items) == 1 {
		logger.Debugf("old labeled pvc %q found", oldPresentPVCs.Items[0].Name)
		c.updatePVCIfChanged(pvc, &oldPresentPVCs.Items[0])
		return &oldPresentPVCs.Items[0], nil
	}

	// check again with the new label for the presence of updated pvc
	pvcStorageClassDeviceSetPVCId, pvcStorageClassDeviceSetPVCIdLabelSelector = makeStorageClassDeviceSetPVCIDNew(storageClassDeviceSetName, pvcTemplate.GetName(), setIndex)
	pvc = makeStorageClassDeviceSetPVC(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId, setIndex, pvcTemplate)
	presentPVCs, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).List(metav1.ListOptions{LabelSelector: pvcStorageClassDeviceSetPVCIdLabelSelector})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pvc %s for storageClassDeviceSet %s", pvcStorageClassDeviceSetPVCIdLabelSelector, storageClassDeviceSetName)
	}

	presentPVCsNum := len(presentPVCs.Items)
	// No PVC found, creating a new one
	if presentPVCsNum == 0 {
		deployedPVC, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).Create(pvc)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create pvc %q for storageClassDeviceSet %q", pvc.GetGenerateName(), storageClassDeviceSetName)
		}
		logger.Debugf("just created pvc %q", deployedPVC.Name)
		return deployedPVC, nil
		// The PVC is already present
	} else if presentPVCsNum == 1 {
		logger.Debugf("already present pvc %q", presentPVCs.Items[0].Name)
		c.updatePVCIfChanged(pvc, &presentPVCs.Items[0])

		// Updating with the new label
		return &presentPVCs.Items[0], nil
	}
	// More than one PVC exists with same labelSelector
	return nil, errors.Errorf("more than one PVCs exists with label %q, pvcs %q", pvcStorageClassDeviceSetPVCIdLabelSelector, presentPVCs)
}

func (c *Cluster) updatePVCIfChanged(desiredPVC *v1.PersistentVolumeClaim, currentPVC *v1.PersistentVolumeClaim) {
	desiredSize, desiredOK := desiredPVC.Spec.Resources.Requests[v1.ResourceStorage]
	currentSize, currentOK := currentPVC.Spec.Resources.Requests[v1.ResourceStorage]
	if !desiredOK || !currentOK {
		logger.Debugf("desired or current size are not specified for pvc %q", currentPVC.Name)
		return
	}
	if desiredSize.Value() > currentSize.Value() {
		currentPVC.Spec.Resources.Requests[v1.ResourceStorage] = desiredSize
		logger.Infof("updating pvc %q size from %s to %s", currentPVC.Name, currentSize.String(), desiredSize.String())
		if _, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).Update(currentPVC); err != nil {
			// log the error, but don't fail the reconcile
			logger.Errorf("failed to update pvc size. %v", err)
			return
		}
		logger.Infof("successfully updated pvc %q size", currentPVC.Name)
	} else if desiredSize.Value() < currentSize.Value() {
		logger.Warningf("ignoring request to shrink osd pvc %q size from %s to %s, only expansion is allowed", currentPVC.Name, currentSize.String(), desiredSize.String())
	}
}

func makeStorageClassDeviceSetPVC(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId string, setIndex int, pvcTemplate v1.PersistentVolumeClaim) *v1.PersistentVolumeClaim {
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

func makeStorageClassDeviceSetPVCID(storageClassDeviceSetName string, setIndex int) (pvcID, pvcLabelSelector string) {
	pvcStorageClassDeviceSetPVCId := fmt.Sprintf("%s-%d", storageClassDeviceSetName, setIndex)
	return pvcStorageClassDeviceSetPVCId, fmt.Sprintf("%s=%s", CephDeviceSetPVCIDLabelKey, pvcStorageClassDeviceSetPVCId)
}

// This is the new function that generates the labels
// It includes the pvcTemplateName in it
func makeStorageClassDeviceSetPVCIDNew(storageClassDeviceSetName, pvcTemplateName string, setIndex int) (pvcID, pvcLabelSelector string) {
	pvcStorageClassDeviceSetPVCId := fmt.Sprintf("%s-%s-%d", storageClassDeviceSetName, strings.Replace(pvcTemplateName, " ", "-", -1), setIndex)
	return pvcStorageClassDeviceSetPVCId, fmt.Sprintf("%s=%s", CephDeviceSetPVCIDLabelKey, pvcStorageClassDeviceSetPVCId)
}
