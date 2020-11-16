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
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Cluster) prepareStorageClassDeviceSets(config *provisionConfig) []rookv1.VolumeSource {
	volumeSources := []rookv1.VolumeSource{}

	existingPVCs, err := GetExistingOSDPVCs(c.context, c.clusterInfo.Namespace)
	if err != nil {
		config.addError("failed to detect existing OSD PVCs. %v", err)
		return volumeSources
	}

	// Iterate over storageClassDeviceSet
	for _, storageClassDeviceSet := range c.spec.Storage.StorageClassDeviceSets {
		if err := controller.CheckPodMemory(cephv1.ResourcesKeyPrepareOSD, storageClassDeviceSet.Resources, cephOsdPodMinimumMemory); err != nil {
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

				pvc, err := c.createStorageClassDeviceSetPVC(existingPVCs, storageClassDeviceSet.Name, pvcTemplate, i)
				if err != nil {
					config.addError("failed to create osd for storageClassDeviceSet %q for count %d. %v", storageClassDeviceSet.Name, i, err)
					continue
				}

				// The PVC type must be from a predefined set such as "data" and "metadata". These names must be enforced if the wal/db are specified
				// with a separate device, but if there is a single volume template we can assume it is always the data template.
				pvcType := pvcTemplate.Name
				if len(storageClassDeviceSet.VolumeClaimTemplates) == 1 {
					pvcType = bluestorePVCData
				}

				if pvcType == bluestorePVCData {
					pvcSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
					dataSize = pvcSize.String()
					crushDeviceClass = pvcTemplate.Annotations["crushDeviceClass"]
				}
				pvcSources[pvcType] = v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.GetName(),
					ReadOnly:  false,
				}
			}

			volumeSources = append(volumeSources, rookv1.VolumeSource{
				Name:                storageClassDeviceSet.Name,
				Resources:           storageClassDeviceSet.Resources,
				Placement:           storageClassDeviceSet.Placement,
				PreparePlacement:    storageClassDeviceSet.PreparePlacement,
				Config:              storageClassDeviceSet.Config,
				Size:                dataSize,
				PVCSources:          pvcSources,
				Portable:            storageClassDeviceSet.Portable,
				TuneSlowDeviceClass: storageClassDeviceSet.TuneSlowDeviceClass,
				TuneFastDeviceClass: storageClassDeviceSet.TuneFastDeviceClass,
				SchedulerName:       storageClassDeviceSet.SchedulerName,
				CrushDeviceClass:    crushDeviceClass,
				Encrypted:           storageClassDeviceSet.Encrypted,
			})
		}
	}

	return volumeSources
}

func (c *Cluster) createStorageClassDeviceSetPVC(existingPVCs map[string]*v1.PersistentVolumeClaim, storageClassDeviceSetName string, pvcTemplate v1.PersistentVolumeClaim, setIndex int) (*v1.PersistentVolumeClaim, error) {
	ctx := context.TODO()
	// old labels and PVC ID for backward compatibility
	pvcStorageClassDeviceSetPVCId := legacyDeviceSetPVCID(storageClassDeviceSetName, setIndex)

	// check for the existence of the pvc
	existingPVC, ok := existingPVCs[pvcStorageClassDeviceSetPVCId]
	if !ok {
		// The old name of the PVC didn't exist, now try the new PVC name and label
		pvcStorageClassDeviceSetPVCId = deviceSetPVCID(storageClassDeviceSetName, pvcTemplate.GetName(), setIndex)
		existingPVC = existingPVCs[pvcStorageClassDeviceSetPVCId]
	}
	pvc := makeStorageClassDeviceSetPVC(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId, setIndex, pvcTemplate)
	k8sutil.SetOwnerRef(&pvc.ObjectMeta, &c.clusterInfo.OwnerRef)

	if existingPVC != nil {
		logger.Infof("OSD PVC %q already exists", existingPVC.Name)

		// Update the PVC in case the size changed
		c.updatePVCIfChanged(pvc, existingPVC)
		return existingPVC, nil
	}

	// No PVC found, creating a new one
	deployedPVC, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create pvc %q for storageClassDeviceSet %q", pvc.GetGenerateName(), storageClassDeviceSetName)
	}
	logger.Infof("successfully provisioned PVC %q", deployedPVC.Name)
	return deployedPVC, nil
}

func (c *Cluster) updatePVCIfChanged(desiredPVC *v1.PersistentVolumeClaim, currentPVC *v1.PersistentVolumeClaim) {
	ctx := context.TODO()
	desiredSize, desiredOK := desiredPVC.Spec.Resources.Requests[v1.ResourceStorage]
	currentSize, currentOK := currentPVC.Spec.Resources.Requests[v1.ResourceStorage]
	if !desiredOK || !currentOK {
		logger.Debugf("desired or current size are not specified for pvc %q", currentPVC.Name)
		return
	}
	if desiredSize.Value() > currentSize.Value() {
		currentPVC.Spec.Resources.Requests[v1.ResourceStorage] = desiredSize
		logger.Infof("updating pvc %q size from %s to %s", currentPVC.Name, currentSize.String(), desiredSize.String())
		if _, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).Update(ctx, currentPVC, metav1.UpdateOptions{}); err != nil {
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

// GetExistingOSDPVCs fetches the list of OSD PVCs
func GetExistingOSDPVCs(clusterdContext *clusterd.Context, namespace string) (map[string]*v1.PersistentVolumeClaim, error) {
	ctx := context.TODO()
	selector := metav1.ListOptions{LabelSelector: CephDeviceSetPVCIDLabelKey}
	pvcs, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, selector)
	if err != nil {
		return nil, errors.Wrap(err, "failed to detect pvcs")
	}
	result := map[string]*v1.PersistentVolumeClaim{}
	for i, pvc := range pvcs.Items {
		pvcID := pvc.Labels[CephDeviceSetPVCIDLabelKey]
		result[pvcID] = &pvcs.Items[i]
	}

	return result, nil
}

func legacyDeviceSetPVCID(storageClassDeviceSetName string, setIndex int) string {
	return fmt.Sprintf("%s-%d", storageClassDeviceSetName, setIndex)
}

// This is the new function that generates the labels
// It includes the pvcTemplateName in it
func deviceSetPVCID(storageClassDeviceSetName, pvcTemplateName string, setIndex int) string {
	return fmt.Sprintf("%s-%s-%d", storageClassDeviceSetName, strings.Replace(pvcTemplateName, " ", "-", -1), setIndex)
}
