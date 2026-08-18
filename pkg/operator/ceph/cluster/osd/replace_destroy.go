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
	return m.destroySweep(replaceFlow{m: m})
}

// replaceFlow is the OSD-replacement flow over the shared destroy machine: drain fully
// (safe-to-destroy as both stop gate and terminal gate), destroy slot-preservingly, and leave the
// deployment as the ready-for-swap marker. Design: design/ceph/osd-replacement.md.
type replaceFlow struct {
	m *OSDHealthMonitor
}

func (f replaceFlow) name() string { return "replacement" }

func (f replaceFlow) ownsDeployment(d *appsv1.Deployment) bool {
	return d.Annotations[cephv1.ReplaceInProgressOSDAnnotationKey] == "true"
}

// isComplete: a ready-for-swap deployment is done (checked by presence, deliberately not by value).
func (f replaceFlow) isComplete(d *appsv1.Deployment) bool {
	_, readyForSwap := d.Annotations[cephv1.ReadyForSwapOSDAnnotationKey]
	return readyForSwap
}

// skipSweep: OSD replacement is host-based only; on a PVC-backed cluster there is nothing to
// process, so skip the per-tick listing of all OSD deployments.
func (f replaceFlow) skipSweep() bool {
	return len(f.m.cluster.spec.Storage.StorageClassDeviceSets) > 0
}

// disengageRequested: the replace annotation was removed. Honored only before the slot is destroyed —
// once destroyed, the Ceph state cannot be cheaply reversed, so the flow completes instead.
func (f replaceFlow) disengageRequested(d *appsv1.Deployment, st destroyState) bool {
	_, isReplaceRequested := d.Annotations[cephv1.ReplaceOSDAnnotationKey]
	return !isReplaceRequested && !st.destroyed
}

func (f replaceFlow) disengage(d *appsv1.Deployment, osdID int) error {
	return f.m.cancelReplaceOSD(d, osdID)
}

func (f replaceFlow) terminalReached(st destroyState) bool { return st.destroyed }

func (f replaceFlow) finalize(d *appsv1.Deployment, osdID int) error {
	log.NamespacedInfo(f.m.clusterInfo.Namespace, logger,
		"osd.%d is destroyed; annotating deployment %q as ready for swap", osdID, d.Name)
	return f.m.annotateReadyForSwap(d, osdID)
}

// startDrain marks the OSD out. A just-out OSD cannot be safe-to-destroy yet, so acting ends the
// tick; never destroy in the same tick it went out.
func (f replaceFlow) startDrain(d *appsv1.Deployment, osdID int, st destroyState) (bool, error) {
	if !st.in {
		return false, nil
	}
	log.NamespacedInfo(f.m.clusterInfo.Namespace, logger, "marking osd.%d out to begin replacement drain", osdID)
	return true, cephclient.OSDOut(f.m.context, f.m.clusterInfo, osdID)
}

func (f replaceFlow) stopGate(osdID int) (bool, error) {
	return cephclient.OsdSafeToDestroy(f.m.context, f.m.clusterInfo, osdID)
}

func (f replaceFlow) terminalGate(osdID int) (bool, error) {
	return cephclient.OsdSafeToDestroy(f.m.context, f.m.clusterInfo, osdID)
}

// terminal forces the mon view to down to dodge a heartbeat-lag EBUSY on destroy (idempotent), then
// destroys the slot. The deployment stays as the ready-for-swap marker; finalize annotates it on a
// later tick once the tree reports the slot destroyed.
func (f replaceFlow) terminal(d *appsv1.Deployment, osdID int) error {
	if err := cephclient.OSDDown(f.m.context, f.m.clusterInfo, osdID); err != nil {
		return err
	}
	return cephclient.OSDDestroy(f.m.context, f.m.clusterInfo, osdID)
}

// processOSDReplacementDestroy advances one replacement-marked OSD's destroy flow from durable
// markers on every OSD health tick.
func (m *OSDHealthMonitor) processOSDReplacementDestroy(d *appsv1.Deployment, osdID int, osdTree *cephclient.OsdTree, osdDump *cephclient.OSDDump) error {
	return m.advanceDestroy(replaceFlow{m: m}, d, osdID, osdTree, osdDump)
}

// annotateReadyForSwap adds the ready-for-swap annotation to the deployment and persists it. It is
// idempotent: if the annotation is already present, it is a no-op.
func (m *OSDHealthMonitor) annotateReadyForSwap(d *appsv1.Deployment, osdID int) error {
	if _, ok := d.Annotations[cephv1.ReadyForSwapOSDAnnotationKey]; ok {
		return nil
	}
	k8sutil.AddAnnotationToDeployment(cephv1.ReadyForSwapOSDAnnotationKey, "true", d)
	_, err := m.cluster.context.Clientset.AppsV1().Deployments(m.clusterInfo.Namespace).Update(m.clusterInfo.Context, d, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to annotate osd.%d deployment %q as ready for swap", osdID, d.Name)
	}
	return nil
}

// cancelReplaceOSD reverses a drain that was cancelled before the OSD was destroyed: mark the OSD
// back `in`, delete any in-flight crypto-close Job, scale the deployment back up, and clear the
// in-progress annotation and the do-not-reconcile label. The goroutine only ever clears this label,
// never sets it. Idempotent across ticks.
func (m *OSDHealthMonitor) cancelReplaceOSD(d *appsv1.Deployment, osdID int) error {
	log.NamespacedInfo(m.clusterInfo.Namespace, logger,
		"replacement of osd.%d was cancelled before destroy; marking it back in and clearing the replacement markers", osdID)

	if err := cephclient.OSDIn(m.context, m.clusterInfo, osdID); err != nil {
		return errors.Wrapf(err, "failed to mark osd.%d back in on cancellation", osdID)
	}

	// The crypto-close Job must be fully gone before the fence is cleared, so a lingering `cryptsetup
	// close` can never race the returning daemon. The delete is foreground, so the Job outlives its
	// pods: while it is still there the cancellation is deferred and completes on a later tick.
	if err := m.cluster.deleteCryptCloseJob(osdID); err != nil {
		return errors.Wrapf(err, "failed to delete crypto-close job for osd.%d on cancellation", osdID)
	}
	status, err := m.cluster.cryptCloseJobStatusForOSD(osdID)
	if err != nil {
		return errors.Wrapf(err, "failed to check the crypto-close job of osd.%d on cancellation", osdID)
	}
	if status != cryptCloseJobNotFound {
		log.NamespacedInfo(m.clusterInfo.Namespace, logger,
			"cancellation of osd.%d is waiting for its crypto-close job to be deleted; will re-check next tick", osdID)
		return nil
	}

	// Scale back up and drop both markers in one update, the reverse of how the controller set them, so
	// the OSD is never left owned-but-unfenced or fenced-but-unowned. Removing the annotation fires no
	// reconcile, so nothing else would scale the deployment back up.
	d.Spec.Replicas = new(int32(1))
	delete(d.Annotations, cephv1.ReplaceInProgressOSDAnnotationKey)
	delete(d.Labels, cephv1.SkipReconcileLabelKey)
	_, err = m.cluster.context.Clientset.AppsV1().Deployments(m.clusterInfo.Namespace).Update(m.clusterInfo.Context, d, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to clear the replacement markers on osd.%d deployment %q", osdID, d.Name)
	}
	return nil
}
