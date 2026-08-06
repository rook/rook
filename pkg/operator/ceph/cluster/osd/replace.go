/*
Copyright 2026 The Rook Authors. All rights reserved.

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

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// processOSDReplacements acts on OSD deployments involved in a replacement: it validates new requests and
// hands them to the OSD health monitor by labelling them, and it cleans up after a replacement that can no
// longer complete.
func (c *Cluster) processOSDReplacements() error {
	deployments, err := c.getOSDDeployments()
	if err != nil {
		return errors.Wrap(err, "failed to list OSD deployments to check for replacement requests")
	}

	var osdTree *cephclient.OsdTree
	var osdDump *cephclient.OSDDump
	var osdMetadata []cephclient.OSDMetadata
	for i := range deployments.Items {
		d := &deployments.Items[i]

		// A deployment waiting for its disk to be swapped needs no validation, but it does need cleaning
		// up if its OSD is gone from the osdmap
		if isWaitingForDiskSwap(d) {
			if osdDump == nil {
				dump, err := cephclient.GetOSDDump(c.context, c.clusterInfo)
				if err != nil {
					// Without the dump the OSD's existence cannot be established. Skip the check rather
					// than risk deleting the deployment of a replacement still in progress.
					log.NamespacedWarning(c.clusterInfo.Namespace, logger,
						"failed to get osd dump to check for aborted replacements; will retry on the next reconcile. %v", err)
					continue
				}
				osdDump = dump
			}
			missingFromOSDDump := c.isMissingFromOSDDump(d, osdDump)
			// deployment is "waiting for swap" AND osd is missing from OSD dump can mean only that replacement has failed and OSD id was deleted during
			// ceph volume rollback. Cleanup dangling downscaled OSD deployment in this case
			if missingFromOSDDump {
				err := k8sutil.DeleteDeployment(c.clusterInfo.Context, c.context.Clientset, d.Namespace, d.Name)
				if err != nil {
					log.NamespacedError(c.clusterInfo.Namespace, logger, "cannot cleanup dangling downscaled OSD deployment %q after failed osd-replacement: %v", d.Name, err)
					continue
				}
				log.NamespacedInfo(c.clusterInfo.Namespace, logger, "removed dangling downscaled OSD deployment %q after failed osd-replacement", d.Name)
			}
			continue
		}

		// Already marked: validated, and the goroutine owns it now.
		if d.Annotations[cephv1.ReplaceInProgressOSDAnnotationKey] == "true" {
			continue
		}

		replaceValue, requested := d.Annotations[cephv1.ReplaceOSDAnnotationKey]
		if !requested {
			// skip: dont have replace annotation
			continue
		}

		// Query Ceph only once a replacement needs validating: the tree carries the "destroyed" status
		// that `ceph osd dump` does not, and the metadata carries the physical devices behind each OSD.
		if osdTree == nil {
			tree, err := cephclient.HostTree(c.context, c.clusterInfo)
			if err != nil {
				// Without them the request cannot be validated. Skip it rather than reject a request
				// that may well be valid.
				log.NamespacedWarning(c.clusterInfo.Namespace, logger,
					"failed to get osd tree to validate replacement requests; will retry on the next reconcile. %v", err)
				continue
			}
			metadata, err := cephclient.GetOSDMetadata(c.context, c.clusterInfo)
			if err != nil {
				log.NamespacedWarning(c.clusterInfo.Namespace, logger,
					"failed to get osd metadata to validate replacement requests; will retry on the next reconcile. %v", err)
				continue
			}
			osdTree, osdMetadata = &tree, *metadata
		}

		if err := c.validateReplaceOSD(d, replaceValue, osdTree, osdMetadata); err != nil {
			// Skip the OSD; without the label it keeps reconciling normally.
			log.NamespacedWarning(c.clusterInfo.Namespace, logger,
				"skipping OSD replacement request on deployment %q: %v", d.Name, err)
			continue
		}

		// Hand the OSD over to the health goroutine: the skip-reconcile label fences the updater off the
		// deployment, and the in-progress annotation records that validation passed. Both are written in
		// the same update, so the goroutine never sees one without the other.
		k8sutil.AddLabelToDeployment(cephv1.SkipReconcileLabelKey, "true", d)
		k8sutil.AddAnnotationToDeployment(cephv1.ReplaceInProgressOSDAnnotationKey, "true", d)
		_, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Update(c.clusterInfo.Context, d, metav1.UpdateOptions{})
		if err != nil {
			log.NamespacedWarning(c.clusterInfo.Namespace, logger,
				"failed to set %q label and %q annotation on OSD deployment %q for replacement: %v",
				cephv1.SkipReconcileLabelKey, cephv1.ReplaceInProgressOSDAnnotationKey, d.Name, err)
			continue
		}

		log.NamespacedInfo(c.clusterInfo.Namespace, logger,
			"validated OSD replacement request on deployment %q and set the %q label and %q annotation; OSD health monitor will drive teardown",
			d.Name, cephv1.SkipReconcileLabelKey, cephv1.ReplaceInProgressOSDAnnotationKey)
	}

	return nil
}

// isWaitingForDiskSwap returns true if the deployment has the cephv1.ReadyForSwapOSDAnnotationKey annotation,
// meaning that the deployment represents destroyed OSD state awaiting physical disk swap to finish osd-replacement process.
func isWaitingForDiskSwap(d *appsv1.Deployment) bool {
	_, readyForSwap := d.Annotations[cephv1.ReadyForSwapOSDAnnotationKey]
	return readyForSwap
}

// isMissingFromOSDDump returns true only if osd dump does not contain osd ID from given Deployment.
// returns false in case of any error or empty OSD Dump
func (c *Cluster) isMissingFromOSDDump(d *appsv1.Deployment, osdDump *cephclient.OSDDump) bool {
	osdID, err := GetOSDID(d)
	if err != nil {
		log.NamespacedError(c.clusterInfo.Namespace, logger, "unable to get osd ID from Deployment: %v", err)
		return false
	}

	if osdDump == nil || len(osdDump.OSDs) == 0 {
		return false
	}
	return !osdDump.Exists(int64(osdID))
}

// validateReplaceOSD returns an error on the first failed validation check for a replacement request.
func (c *Cluster) validateReplaceOSD(d *appsv1.Deployment, replaceValue string, osdTree *cephclient.OsdTree, osdMetadata []cephclient.OSDMetadata) error {
	// The annotation value must match the deployment's own OSD id, guarding against a copy-paste typo.
	osdID, err := GetOSDID(d)
	if err != nil {
		return errors.Wrapf(err, "failed to read %q label", OsdIdLabelKey)
	}
	expected := fmt.Sprintf(cephv1.ReplaceOSDAnnotationValueFmt, osdID)
	if replaceValue != expected {
		return errors.Errorf("annotation %q value %q does not match the deployment's OSD id (expected %q)",
			cephv1.ReplaceOSDAnnotationKey, replaceValue, expected)
	}

	// Host-based only: PVC-backed OSDs are out of scope.
	if _, isPVC := d.Labels[OSDOverPVCLabelKey]; isPVC {
		return errors.Errorf("OSD %d is PVC-backed (label %q present); replacement supports host-based OSDs only",
			osdID, OSDOverPVCLabelKey)
	}

	// Target OSD must exist in the osd tree. An already-destroyed slot is accepted so the goroutine
	// can resume idempotently from its destroyed phase.
	found := false
	for _, node := range osdTree.Nodes {
		if node.Type != "osd" || node.ID != osdID {
			continue
		}
		found = true
		break // osd ids are unique in the tree
	}
	if !found {
		return errors.Errorf("OSD %d does not exist in the osd tree", osdID)
	}

	if err := validateSingleOSDPerDevice(osdID, osdMetadata); err != nil {
		return err
	}

	return nil
}

// validateSingleOSDPerDevice rejects the request when another OSD on the same host reports the same
// physical device set in `ceph osd metadata`: replacement pairs one destroyed OSD with one whole blank
// data device (see design/ceph/osd-replacement.md). The set is sorted and comma-separated, so string
// equality compares sets; a down OSD's metadata is stale, so a renamed device may reject spuriously.
func validateSingleOSDPerDevice(osdID int, osdMetadata []cephclient.OSDMetadata) error {
	var target *cephclient.OSDMetadata
	for i := range osdMetadata {
		if osdMetadata[i].Id == osdID {
			target = &osdMetadata[i]
			break
		}
	}
	// Metadata can be missing or carry no resolved devices/host; don't block the replacement on it.
	if target == nil || target.Devices == "" || target.HostName == "" {
		return nil
	}

	for i := range osdMetadata {
		other := &osdMetadata[i]
		if other.Id == osdID || other.HostName != target.HostName || other.Devices != target.Devices {
			continue
		}
		return errors.Errorf("OSD %d and OSD %d on host %q report the same physical device(s) %q; replacement requires an OSD that owns its whole data device (osdsPerDevice > 1 is not supported)",
			osdID, other.Id, target.HostName, target.Devices)
	}

	return nil
}

func (c *Cluster) replacementReadyForSwap(osdID int) (bool, error) {
	name := fmt.Sprintf(osdAppNameFmt, osdID)
	d, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Get(c.clusterInfo.Context, name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to get OSD deployment %q", name)
	}
	if _, readyForSwap := d.Annotations[cephv1.ReadyForSwapOSDAnnotationKey]; !readyForSwap {
		return false, nil
	}
	return true, nil
}
