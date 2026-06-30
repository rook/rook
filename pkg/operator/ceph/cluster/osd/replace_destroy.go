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
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// processOSDsDestroyForReplacement runs the OSD-replacement destroy flow, a long-running process spread across
// health ticks: it marks each replacement-marked OSD out, drains and destroys it, then annotates the
// fully-destroyed OSD as ready-for-swap to signal the user to swap the disk. Everything after the
// swap is handled by the prepare job.
//
// Called from OSD health goroutine. Returns owned OSD ids to ignore from normal OSD health goroutine flow.
func (m *OSDHealthMonitor) processOSDsDestroyForReplacement() (map[int]struct{}, error) {
	osdsUnderReplacement := map[int]struct{}{}

	deployments, err := m.cluster.getOSDDeployments()
	if err != nil {
		return osdsUnderReplacement, errors.Wrap(err, "failed to list OSD deployments for replacement processing")
	}

	// Collect the replacement-marked deployments. Every one (incl. ready-for-swap) goes into the
	// exclusion set, but only those with work left are processed below: a deployment without the
	// do-not-reconcile label is not owned by this function, and one already annotated ready-for-swap is done.
	var toProcess []*appsv1.Deployment
	for i := range deployments.Items {
		d := &deployments.Items[i]
		if d.Labels[cephv1.SkipReconcileLabelKey] != "true" {
			continue
		}

		osdID, err := GetOSDID(d)
		if err != nil {
			log.NamespacedWarning(m.clusterInfo.Namespace, logger,
				"skipping replacement processing for deployment %q: %v", d.Name, err)
			continue
		}

		osdsUnderReplacement[osdID] = struct{}{}
		if _, readyForSwap := d.Annotations[cephv1.ReadyForSwapOSDAnnotationKey]; readyForSwap {
			// already processed
			continue
		}
		toProcess = append(toProcess, d)
	}

	if len(toProcess) == 0 {
		return osdsUnderReplacement, nil
	}

	tree, err := cephclient.HostTree(m.context, m.clusterInfo)
	if err != nil {
		log.NamespacedWarning(m.clusterInfo.Namespace, logger,
			"failed to get osd tree for replacement processing; will retry next tick. %v", err)
		return osdsUnderReplacement, nil
	}
	osdDump, err := cephclient.GetOSDDump(m.context, m.clusterInfo)
	if err != nil {
		log.NamespacedWarning(m.clusterInfo.Namespace, logger,
			"failed to get osd dump for replacement processing; will retry next tick. %v", err)
		return osdsUnderReplacement, nil
	}

	for _, d := range toProcess {
		osdID, _ := GetOSDID(d) // already validated above
		if err := m.processOSDReplacementDestroy(d, osdID, &tree, osdDump); err != nil {
			log.NamespacedWarning(m.clusterInfo.Namespace, logger,
				"failed to advance replacement for osd.%d; will retry next tick. %v", osdID, err)
		}
	}

	return osdsUnderReplacement, nil
}

// processOSDReplacementDestroy advances one replacement-marked OSD's destroy flow from durable markers on every
// OSD heath tick
func (m *OSDHealthMonitor) processOSDReplacementDestroy(d *appsv1.Deployment, osdID int, osdTree *cephclient.OsdTree, osdDump *cephclient.OSDDump) error {
	_, isReplaceRequested := d.Annotations[cephv1.ReplaceOSDAnnotationKey]
	isDestroyed := isOSDDestroyedInTree(osdTree, osdID)
	isDownscaled := d.Spec.Replicas != nil && *d.Spec.Replicas == 0

	// Cancellation: the replace annotation was removed. Honored only before the slot is destroyed —
	// once destroyed, the Ceph state cannot be cheaply reversed, so the flow completes instead. Kept
	// ahead of every query so a cancellation can always mark the OSD back in.
	if !isReplaceRequested && !isDestroyed {
		return m.cancelReplaceOSD(d, osdID)
	}

	// Slot already destroyed: annotate ready-for-swap to signal the user.
	if isDestroyed {
		log.NamespacedInfo(m.clusterInfo.Namespace, logger,
			"osd.%d is destroyed; annotating deployment %q as ready for swap", osdID, d.Name)
		return m.annotateReadyForSwap(d, osdID)
	}

	if isDownscaled {
		gone, err := m.isOSDPodGone(osdID)
		if err != nil {
			return err
		}
		if !gone {
			log.NamespacedInfo(m.clusterInfo.Namespace, logger,
				"osd.%d is scaled down; waiting for the pod to terminate before destroying", osdID)
			return nil
		}
		// Pod is gone, run job to close crypto mappings on host for encrypted OSDs only.
		isEncrypted, err := m.isReplaceOSDEncrypted(d)
		if err != nil {
			return err
		}
		if isEncrypted {
			// run job or check status
			done, err := m.runCryptCloseJobForOSD(d, osdID)
			if err != nil {
				return err
			}
			if !done {
				return nil
			}
			// if job done. delete right away and proceed to osd-destroy.
			if err := m.cluster.deleteCryptCloseJob(osdID); err != nil {
				return errors.Wrapf(err, "failed to delete crypto-close job for osd.%d", osdID)
			}
		}
		// double check safe-to-destroy before destroy
		safe, err := cephclient.OsdSafeToDestroy(m.context, m.clusterInfo, osdID)
		if err != nil {
			log.NamespacedWarning(m.clusterInfo.Namespace, logger,
				"failed to check safe-to-destroy for osd.%d; will re-check next tick. %v", osdID, err)
			return nil
		}
		if !safe {
			log.NamespacedInfo(m.clusterInfo.Namespace, logger,
				"osd.%d is not yet safe-to-destroy; will re-check next tick", osdID)
			return nil
		}
		// force the mon view to down to dodge a heartbeat-lag EBUSY on destroy (idempotent), then destroy
		if err := cephclient.OSDDown(m.context, m.clusterInfo, osdID); err != nil {
			return err
		}
		if err := cephclient.OSDDestroy(m.context, m.clusterInfo, osdID); err != nil {
			return err
		}
		return nil
	}

	// OSD deployment is not yet scaled down: drive the drain.
	isOSDIn := true
	if _, in, err := osdDump.StatusByID(int64(osdID)); err != nil {
		// OSD absent from the dump but its slot is not destroyed. Treat it as in: marking out is
		// idempotent and the next tick re-reads once the dump reflects reality.
		log.NamespacedWarning(m.clusterInfo.Namespace, logger,
			"osd.%d not found in osd dump while not destroyed; treating it as in to begin drain. %v", osdID, err)
	} else {
		isOSDIn = in == inStatus
	}

	// Mark the OSD out to begin the drain. A just-out OSD cannot be safe-to-destroy yet, so return
	// and wait for the drain on the next ticks; never destroy in the same tick it went out.
	if isOSDIn {
		log.NamespacedInfo(m.clusterInfo.Namespace, logger, "marking osd.%d out to begin replacement drain", osdID)
		if err := cephclient.OSDOut(m.context, m.clusterInfo, osdID); err != nil {
			return err
		}
		return nil
	}

	// OSD is out: wait until it has drained and is safe to destroy, then scale the deployment to 0 so
	// the daemon releases the data/DB LVs. The pod-gone branch above takes over once the pod exits.
	safe, err := cephclient.OsdSafeToDestroy(m.context, m.clusterInfo, osdID)
	if err != nil {
		log.NamespacedWarning(m.clusterInfo.Namespace, logger,
			"failed to check safe-to-destroy for osd.%d; will re-check next tick. %v", osdID, err)
		return nil
	}
	if !safe {
		log.NamespacedInfo(m.clusterInfo.Namespace, logger,
			"osd.%d is draining; not yet safe-to-destroy, will re-check next tick", osdID)
		return nil
	}

	return m.scaleDownOSDDeployment(d, osdID)
}

// isOSDPodGone reports whether the OSD daemon pod has terminated. PodsRunningWithLabel counts pods
// whose phase is Running; a terminating pod stays Running until its containers exit, so this only
// reads true once the daemon has actually released the data/DB LVs.
func (m *OSDHealthMonitor) isOSDPodGone(osdID int) (bool, error) {
	label := fmt.Sprintf("%s=%d", OsdIdLabelKey, osdID)
	running, err := k8sutil.PodsRunningWithLabel(m.clusterInfo.Context, m.context.Clientset, m.clusterInfo.Namespace, label)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check for running pods of osd.%d", osdID)
	}
	if running > 0 {
		log.NamespacedInfo(m.clusterInfo.Namespace, logger,
			"osd.%d still has %d running pod(s) after scale-down; will re-check next tick", osdID, running)
		return false, nil
	}
	return true, nil
}

// scaleDownOSDDeployment sets the deployment replicas to 0 (only if not already). It is idempotent
// across ticks; pod-gone is checked separately by the caller.
func (m *OSDHealthMonitor) scaleDownOSDDeployment(d *appsv1.Deployment, osdID int) error {
	if d.Spec.Replicas != nil && *d.Spec.Replicas == 0 {
		return nil
	}
	log.NamespacedInfo(m.clusterInfo.Namespace, logger, "scaling osd.%d deployment %q to replicas=0", osdID, d.Name)
	zero := int32(0)
	d.Spec.Replicas = &zero
	updated, err := m.cluster.context.Clientset.AppsV1().Deployments(m.clusterInfo.Namespace).Update(m.clusterInfo.Context, d, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to scale osd.%d deployment %q to zero", osdID, d.Name)
	}
	// Keep the caller's copy in sync so a later update in the same flow does not clobber the
	// replicas change with a stale object.
	*d = *updated
	return nil
}

// runCryptCloseJobForOSD ensures the per-OSD crypto-close Job exists and reports whether it has
// succeeded. It (re)creates the Job when none exists or a previous one failed, and polls otherwise.
// Idempotent across ticks.
func (m *OSDHealthMonitor) runCryptCloseJobForOSD(d *appsv1.Deployment, osdID int) (bool, error) {
	status, err := m.cluster.cryptCloseJobStatusForOSD(osdID)
	if err != nil {
		return false, err
	}

	switch status {
	case cryptCloseJobSucceeded:
		return true, nil
	case cryptCloseJobRunning:
		log.NamespacedInfo(m.clusterInfo.Namespace, logger, "crypto-close job for osd.%d is still running", osdID)
		return false, nil
	default:
		// NotFound or Failed: (re)create the Job, pinned to the OSD's node.
		nodeName, err := m.replaceOSDNodeName(d, osdID)
		if err != nil {
			return false, err
		}
		// A previously-Failed Job means a genuinely stuck dm-crypt close (it already exhausted its
		// in-container BackoffLimit / ActiveDeadlineSeconds). The design has no timeout — the user
		// cancels by removing the annotation — so we keep recreating, but at Warning level so a wedged
		// replacement is visible rather than silently looping every tick.
		if status == cryptCloseJobFailed {
			log.NamespacedWarning(m.clusterInfo.Namespace, logger,
				"crypto-close job for osd.%d previously failed; recreating it on node %q", osdID, nodeName)
		} else {
			log.NamespacedInfo(m.clusterInfo.Namespace, logger,
				"creating crypto-close job for osd.%d on node %q", osdID, nodeName)
		}
		if err := m.cluster.startCryptCloseJob(osdID, nodeName); err != nil {
			return false, err
		}
		return false, nil
	}
}

// annotateReadyForSwap adds the ready-for-swap annotation to the deployment and persists it. It is
// idempotent: if the annotation is already present, it is a no-op.
func (m *OSDHealthMonitor) annotateReadyForSwap(d *appsv1.Deployment, osdID int) error {
	if _, ok := d.Annotations[cephv1.ReadyForSwapOSDAnnotationKey]; ok {
		return nil
	}
	if d.Annotations == nil {
		d.Annotations = map[string]string{}
	}
	d.Annotations[cephv1.ReadyForSwapOSDAnnotationKey] = "true"
	_, err := m.cluster.context.Clientset.AppsV1().Deployments(m.clusterInfo.Namespace).Update(m.clusterInfo.Context, d, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to annotate osd.%d deployment %q as ready for swap", osdID, d.Name)
	}
	return nil
}

// cancelReplaceOSD reverses a drain that was cancelled before the OSD was destroyed: mark the OSD
// back `in`, delete any in-flight crypto-close Job, and clear the do-not-reconcile label so the
// updater scales the deployment back to replicas=1. The goroutine only ever clears this label, never
// sets it. Idempotent across ticks.
func (m *OSDHealthMonitor) cancelReplaceOSD(d *appsv1.Deployment, osdID int) error {
	log.NamespacedInfo(m.clusterInfo.Namespace, logger,
		"replacement of osd.%d was cancelled before destroy; marking it back in and clearing the do-not-reconcile label", osdID)

	if err := cephclient.OSDIn(m.context, m.clusterInfo, osdID); err != nil {
		return errors.Wrapf(err, "failed to mark osd.%d back in on cancellation", osdID)
	}

	// Delete any in-flight crypto-close Job before clearing the fence label. If the encrypted OSD was
	// cancelled after its crypto-close Job was created, a lingering `cryptsetup close` would race the
	// daemon once it scales back up, so the Job must be gone before the label is cleared. The delete is
	// idempotent (no-op if absent); a failed delete is returned so the cancellation retries next tick.
	if err := m.cluster.deleteCryptCloseJob(osdID); err != nil {
		return errors.Wrapf(err, "failed to delete crypto-close job for osd.%d on cancellation", osdID)
	}

	if d.Labels[cephv1.SkipReconcileLabelKey] != "true" {
		return nil // already cleared on a previous tick
	}
	delete(d.Labels, cephv1.SkipReconcileLabelKey)
	_, err := m.cluster.context.Clientset.AppsV1().Deployments(m.clusterInfo.Namespace).Update(m.clusterInfo.Context, d, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to clear the %q label on osd.%d deployment %q", cephv1.SkipReconcileLabelKey, osdID, d.Name)
	}
	return nil
}

// isReplaceOSDEncrypted reports whether the OSD deployment is encrypted, using the same detection
// as getOSDInfo: the "encrypted" label, or a dmcrypt block path. The label is checked first so the
// common case needs no OSD-info parse.
func (m *OSDHealthMonitor) isReplaceOSDEncrypted(d *appsv1.Deployment) (bool, error) {
	if d.Labels[encrypted] == "true" {
		return true, nil
	}
	osdInfo, err := m.cluster.getOSDInfo(d)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read OSD info from deployment %q to determine encryption", d.Name)
	}
	return osdInfo.Encrypted, nil
}

// replaceOSDNodeName resolves the node the OSD runs on, so the crypto-close Job can be pinned to it.
func (m *OSDHealthMonitor) replaceOSDNodeName(d *appsv1.Deployment, osdID int) (string, error) {
	osdInfo, err := m.cluster.getOSDInfo(d)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read OSD info from deployment %q to resolve its node", d.Name)
	}
	if strings.TrimSpace(osdInfo.NodeName) == "" {
		return "", errors.Errorf("could not resolve the node name for osd.%d from deployment %q", osdID, d.Name)
	}
	return osdInfo.NodeName, nil
}

// isOSDDestroyedInTree reports whether the given OSD's slot is marked "destroyed" in the osd tree.
func isOSDDestroyedInTree(osdTree *cephclient.OsdTree, osdID int) bool {
	for _, node := range osdTree.Nodes {
		if node.Type == "osd" && node.ID == osdID {
			return node.Status == "destroyed"
		}
	}
	return false
}
