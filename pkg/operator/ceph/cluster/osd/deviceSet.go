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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Cluster) prepareStorageClassDeviceSets(errs *provisionErrors) []rookv1.VolumeSource {
	volumeSources := []rookv1.VolumeSource{}

	existingPVCs, uniqueOSDsPerDeviceSet, err := GetExistingPVCs(c.context, c.clusterInfo.Namespace)
	if err != nil {
		errs.addError("failed to detect existing OSD PVCs. %v", err)
		return volumeSources
	}

	// Iterate over deviceSet
	for _, deviceSet := range c.spec.Storage.StorageClassDeviceSets {
		if err := controller.CheckPodMemory(cephv1.ResourcesKeyPrepareOSD, deviceSet.Resources, cephOsdPodMinimumMemory); err != nil {
			errs.addError("failed to provision OSDs on PVC for storageClassDeviceSet %q. %v", deviceSet.Name, err)
			continue
		}
		// Check if the volume claim template is specified
		if len(deviceSet.VolumeClaimTemplates) == 0 {
			errs.addError("failed to provision OSDs on PVC for storageClassDeviceSet %q. no volumeClaimTemplate is specified. user must specify a volumeClaimTemplate", deviceSet.Name)
			continue
		}

		// Iterate through existing PVCs to ensure they are up-to-date, no metadata pvcs are missing, etc
		highestExistingID := -1
		countInDeviceSet := 0
		if existingIDs, ok := uniqueOSDsPerDeviceSet[deviceSet.Name]; ok {
			logger.Infof("verifying PVCs exist for %d OSDs in device set %q", existingIDs.Count(), deviceSet.Name)
			for existingID := range existingIDs.Iter() {
				pvcID, err := strconv.Atoi(existingID)
				if err != nil {
					errs.addError("invalid PVC index %q found for device set %q", existingID, deviceSet.Name)
					continue
				}
				// keep track of the max PVC index found so we know what index to start with for new OSDs
				if pvcID > highestExistingID {
					highestExistingID = pvcID
				}
				volumeSource := c.createDeviceSetPVCsForIndex(deviceSet, existingPVCs, pvcID, errs)
				volumeSources = append(volumeSources, volumeSource)
			}
			countInDeviceSet = existingIDs.Count()
		}
		// Create new PVCs if we are not yet at the expected count
		// No new PVCs will be created if we have too many
		pvcsToCreate := deviceSet.Count - countInDeviceSet
		if pvcsToCreate > 0 {
			logger.Infof("creating %d new PVCs for device set %q", pvcsToCreate, deviceSet.Name)
		}
		for i := 0; i < pvcsToCreate; i++ {
			pvcID := highestExistingID + i + 1
			volumeSource := c.createDeviceSetPVCsForIndex(deviceSet, existingPVCs, pvcID, errs)
			volumeSources = append(volumeSources, volumeSource)
			countInDeviceSet++
		}
	}

	return volumeSources
}

func (c *Cluster) createDeviceSetPVCsForIndex(deviceSet rookv1.StorageClassDeviceSet, existingPVCs map[string]*v1.PersistentVolumeClaim, setIndex int, errs *provisionErrors) rookv1.VolumeSource {
	// Create the PVC source for each of the data, metadata, and other types of templates if defined.
	pvcSources := map[string]v1.PersistentVolumeClaimVolumeSource{}

	var dataSize string
	var crushDeviceClass string
	for _, pvcTemplate := range deviceSet.VolumeClaimTemplates {
		if pvcTemplate.Name == "" {
			// For backward compatibility a blank name must be treated as a data volume
			pvcTemplate.Name = bluestorePVCData
		}

		pvc, err := c.createDeviceSetPVC(existingPVCs, deviceSet.Name, pvcTemplate, setIndex)
		if err != nil {
			errs.addError("failed to provision PVC for device set %q index %d. %v", deviceSet.Name, setIndex, err)
			continue
		}

		// The PVC type must be from a predefined set such as "data", "metadata", and "wal". These names must be enforced if the wal/db are specified
		// with a separate device, but if there is a single volume template we can assume it is always the data template.
		pvcType := pvcTemplate.Name
		if len(deviceSet.VolumeClaimTemplates) == 1 {
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

	return rookv1.VolumeSource{
		Name:                deviceSet.Name,
		Resources:           deviceSet.Resources,
		Placement:           deviceSet.Placement,
		PreparePlacement:    deviceSet.PreparePlacement,
		Config:              deviceSet.Config,
		Size:                dataSize,
		PVCSources:          pvcSources,
		Portable:            deviceSet.Portable,
		TuneSlowDeviceClass: deviceSet.TuneSlowDeviceClass,
		TuneFastDeviceClass: deviceSet.TuneFastDeviceClass,
		SchedulerName:       deviceSet.SchedulerName,
		CrushDeviceClass:    crushDeviceClass,
		Encrypted:           deviceSet.Encrypted,
	}
}

func (c *Cluster) createDeviceSetPVC(existingPVCs map[string]*v1.PersistentVolumeClaim, deviceSetName string, pvcTemplate v1.PersistentVolumeClaim, setIndex int) (*v1.PersistentVolumeClaim, error) {
	ctx := context.TODO()
	// old labels and PVC ID for backward compatibility
	pvcID := legacyDeviceSetPVCID(deviceSetName, setIndex)

	// check for the existence of the pvc
	existingPVC, ok := existingPVCs[pvcID]
	if !ok {
		// The old name of the PVC didn't exist, now try the new PVC name and label
		pvcID = deviceSetPVCID(deviceSetName, pvcTemplate.GetName(), setIndex)
		existingPVC = existingPVCs[pvcID]
	}
	pvc := makeDeviceSetPVC(deviceSetName, pvcID, setIndex, pvcTemplate, c.clusterInfo.Namespace)
	err := c.clusterInfo.OwnerInfo.SetControllerReference(pvc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to osd pvc %q", pvc.Name)
	}

	if existingPVC != nil {
		logger.Infof("OSD PVC %q already exists", existingPVC.Name)

		// Update the PVC in case the size changed
		c.updatePVCIfChanged(pvc, existingPVC)
		return existingPVC, nil
	}

	// No PVC found, creating a new one
	deployedPVC, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create PVC %q for device set %q", pvc.Name, deviceSetName)
	}
	logger.Infof("successfully provisioned PVC %q", deployedPVC.Name)

	return deployedPVC, nil
}

func (c *Cluster) updatePVCIfChanged(desiredPVC *v1.PersistentVolumeClaim, currentPVC *v1.PersistentVolumeClaim) {
	ctx := context.TODO()
	desiredSize, desiredOK := desiredPVC.Spec.Resources.Requests[v1.ResourceStorage]
	currentSize, currentOK := currentPVC.Spec.Resources.Requests[v1.ResourceStorage]
	if !desiredOK || !currentOK {
		logger.Debugf("desired or current size are not specified for PVC %q", currentPVC.Name)
		return
	}
	if desiredSize.Value() > currentSize.Value() {
		currentPVC.Spec.Resources.Requests[v1.ResourceStorage] = desiredSize
		logger.Infof("updating PVC %q size from %s to %s", currentPVC.Name, currentSize.String(), desiredSize.String())
		if _, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.clusterInfo.Namespace).Update(ctx, currentPVC, metav1.UpdateOptions{}); err != nil {
			// log the error, but don't fail the reconcile
			logger.Errorf("failed to update PVC size. %v", err)
			return
		}
		logger.Infof("successfully updated PVC %q size", currentPVC.Name)
	} else if desiredSize.Value() < currentSize.Value() {
		logger.Warningf("ignoring request to shrink osd PVC %q size from %s to %s, only expansion is allowed", currentPVC.Name, currentSize.String(), desiredSize.String())
	}
}

func makeDeviceSetPVC(deviceSetName, pvcID string, setIndex int, pvcTemplate v1.PersistentVolumeClaim, namespace string) *v1.PersistentVolumeClaim {
	pvcLabels := makeStorageClassDeviceSetPVCLabel(deviceSetName, pvcID, setIndex)

	// Add user provided labels to pvcTemplates
	for k, v := range pvcTemplate.GetLabels() {
		pvcLabels[k] = v
	}

	// pvc naming format rook-ceph-osd-<deviceSetName>-<SetNumber>-<PVCIndex>-<generatedSuffix>
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			// Use a generated name to avoid the possibility of two OSDs being created with the same ID.
			// If one is removed and a new one is created later with the same ID, the OSD would fail to start.
			GenerateName: pvcID,
			Namespace:    namespace,
			Labels:       pvcLabels,
			Annotations:  pvcTemplate.Annotations,
		},
		Spec: pvcTemplate.Spec,
	}
}

// GetExistingPVCs fetches the list of OSD PVCs
func GetExistingPVCs(clusterdContext *clusterd.Context, namespace string) (map[string]*v1.PersistentVolumeClaim, map[string]*util.Set, error) {
	ctx := context.TODO()
	selector := metav1.ListOptions{LabelSelector: CephDeviceSetPVCIDLabelKey}
	pvcs, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, selector)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to detect PVCs")
	}
	result := map[string]*v1.PersistentVolumeClaim{}
	uniqueOSDsPerDeviceSet := map[string]*util.Set{}
	for i, pvc := range pvcs.Items {
		// Populate the PVCs based on their unique name across all the device sets
		pvcID := pvc.Labels[CephDeviceSetPVCIDLabelKey]
		result[pvcID] = &pvcs.Items[i]

		// Create a map of the PVC IDs available in each device set based on PVC index
		deviceSet := pvc.Labels[CephDeviceSetLabelKey]
		pvcIndex := pvc.Labels[CephSetIndexLabelKey]
		if _, ok := uniqueOSDsPerDeviceSet[deviceSet]; !ok {
			uniqueOSDsPerDeviceSet[deviceSet] = util.NewSet()
		}
		uniqueOSDsPerDeviceSet[deviceSet].Add(pvcIndex)
	}

	return result, uniqueOSDsPerDeviceSet, nil
}

func legacyDeviceSetPVCID(deviceSetName string, setIndex int) string {
	return fmt.Sprintf("%s-%d", deviceSetName, setIndex)
}

// This is the new function that generates the labels
// It includes the pvcTemplateName in it
func deviceSetPVCID(deviceSetName, pvcTemplateName string, setIndex int) string {
	cleanName := strings.Replace(pvcTemplateName, " ", "-", -1)
	return fmt.Sprintf("%s-%s-%d", deviceSetName, cleanName, setIndex)
}
