/*
Copyright 2025 The Rook Authors. All rights reserved.

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

// validateAndStartOSDReplacement acts on annotated OSD deployments for replacements. Validates request and
// adds do-not-reconcile label to hand over the process to OSD health monitor.
func (c *Cluster) validateAndStartOSDReplacement() error {
	deployments, err := c.getOSDDeployments()
	if err != nil {
		return errors.Wrap(err, "failed to list OSD deployments to check for replacement requests")
	}

	var osdTree *cephclient.OsdTree
	for i := range deployments.Items {
		d := &deployments.Items[i]

		replaceValue, requested := d.Annotations[cephv1.ReplaceOSDAnnotationKey]
		if !requested {
			// skip: dont have replace annotation
			continue
		}

		// Already labeled: validated, and the goroutine owns it now.
		if d.Labels[cephv1.SkipReconcileLabelKey] == "true" {
			continue
		}

		// Fetch the osd tree only once a replacement needs validating; it carries the "destroyed"
		// status that `ceph osd dump` does not.
		if osdTree == nil {
			tree, err := cephclient.HostTree(c.context, c.clusterInfo)
			if err != nil {
				return errors.Wrap(err, "failed to get osd tree to validate replacement requests")
			}
			osdTree = &tree
		}

		if err := c.validateReplaceOSD(d, replaceValue, osdTree); err != nil {
			// Skip the OSD; without the label it keeps reconciling normally.
			log.NamespacedWarning(c.clusterInfo.Namespace, logger,
				"skipping OSD replacement request on deployment %q: %v", d.Name, err)
			continue
		}

		// Setting the skip-reconcile label here as a marker of successful validation.
		// The rest (osd drain and destroy) will be handled by OSD health goroutine.
		k8sutil.AddLabelToDeployment(cephv1.SkipReconcileLabelKey, "true", d)
		_, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Update(c.clusterInfo.Context, d, metav1.UpdateOptions{})
		if err != nil {
			log.NamespacedWarning(c.clusterInfo.Namespace, logger,
				"failed to set %q label on OSD deployment %q for replacement: %v",
				cephv1.SkipReconcileLabelKey, d.Name, err)
			continue
		}

		log.NamespacedInfo(c.clusterInfo.Namespace, logger,
			"validated OSD replacement request on deployment %q and set the %q label; OSD health monitor will drive teardown",
			d.Name, cephv1.SkipReconcileLabelKey)
	}

	return nil
}

// validateReplaceOSD returns an error on the first failed validation check for a replacement request.
func (c *Cluster) validateReplaceOSD(d *appsv1.Deployment, replaceValue string, osdTree *cephclient.OsdTree) error {
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

	return nil
}

func (c *Cluster) replacementReadyToRecreate(osdID int) (bool, error) {
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
	// A nil Replicas defaults to 1, so it is not scaled down and therefore not a marker awaiting recreate.
	if d.Spec.Replicas == nil || *d.Spec.Replicas != 0 {
		log.NamespacedWarning(c.clusterInfo.Namespace, logger,
			"OSD %d deployment %q carries the %q annotation but is not scaled to zero; not recreating it",
			osdID, name, cephv1.ReadyForSwapOSDAnnotationKey)
		return false, nil
	}
	return true, nil
}
