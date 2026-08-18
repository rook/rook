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
	"strings"

	"github.com/pkg/errors"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// destroyState is the per-OSD state the destroy machine derives fresh on every tick from Ceph and
// Kubernetes; nothing here is persisted (design/ceph/osd-device-exclusion.md (design PR #18125),
// "Shared destroy state machine").
type destroyState struct {
	// destroyed: the OSD's slot is marked "destroyed" in the osd tree.
	destroyed bool
	// downscaled: the OSD deployment is at zero replicas.
	downscaled bool
	// in: the OSD is "in" per the osd dump. An OSD absent from the dump while its slot is not
	// destroyed is treated as in: marking out is idempotent and the next tick re-reads once the dump
	// reflects reality.
	in bool
}

func deriveDestroyState(namespace string, d *appsv1.Deployment, osdID int, osdTree *cephclient.OsdTree, osdDump *cephclient.OSDDump) destroyState {
	st := destroyState{
		destroyed:  isOSDDestroyedInTree(osdTree, osdID),
		downscaled: d.Spec.Replicas != nil && *d.Spec.Replicas == 0,
		in:         true,
	}
	if _, in, err := osdDump.StatusByID(int64(osdID)); err != nil {
		if !st.destroyed {
			log.NamespacedWarning(namespace, logger,
				"osd.%d not found in osd dump while not destroyed; treating it as in to begin drain. %v", osdID, err)
		}
	} else {
		st.in = in == inStatus
	}
	return st
}

// destroyFlow is one flow over the shared destroy state machine (the replacement flow today; the
// exclusion removal modes later — see
// design/ceph/osd-device-exclusion.md (design PR #18125), "Shared destroy state machine" for the
// parameter table this interface encodes). Methods are the flow-specific parameters; the spine
// (advanceDestroy) and sweep (destroySweep) own the ordering and the shared primitives (pod-gone,
// crypt-close, scale-down).
type destroyFlow interface {
	// name is the flow's log prefix.
	name() string
	// ownsDeployment reports whether the flow's ownership marker is on the deployment.
	ownsDeployment(d *appsv1.Deployment) bool
	// isComplete reports whether the flow is done with the deployment: it stays in the owned set
	// (exempt from normal health handling) but is not advanced.
	isComplete(d *appsv1.Deployment) bool
	// skipSweep reports whether the whole sweep is a no-op for this cluster.
	skipSweep() bool
	// disengageRequested reports whether the request was withdrawn; the spine honors it only while
	// the flow's commit point has not passed, which the flow encodes in this predicate.
	disengageRequested(d *appsv1.Deployment, st destroyState) bool
	// disengage reverses the flow's own markers and restores the OSD. It is one opaque, idempotent
	// step: internal ordering (e.g. crypt-close teardown before unfencing) belongs to the flow.
	disengage(d *appsv1.Deployment, osdID int) error
	// terminalReached reports whether the flow's Ceph-side terminal state already holds.
	terminalReached(st destroyState) bool
	// finalize runs the flow's post-terminal bookkeeping. Idempotent.
	finalize(d *appsv1.Deployment, osdID int) error
	// startDrain moves the OSD toward the stop gate and reports whether it acted; an action ends
	// this OSD's tick (a just-started drain can never pass the stop gate in the same tick).
	startDrain(d *appsv1.Deployment, osdID int, st destroyState) (bool, error)
	// stopGate certifies the OSD's pod may be stopped now.
	stopGate(osdID int) (bool, error)
	// terminalGate re-certifies immediately before the terminal action.
	terminalGate(osdID int) (bool, error)
	// terminal performs the flow's Ceph-side teardown at the commit point.
	terminal(d *appsv1.Deployment, osdID int) error
}

// advanceDestroy advances one flow-owned OSD one step per health tick. State is derived fresh; every
// error is warn-and-retry-next-tick at the caller.
func (m *OSDHealthMonitor) advanceDestroy(f destroyFlow, d *appsv1.Deployment, osdID int, osdTree *cephclient.OsdTree, osdDump *cephclient.OSDDump) error {
	st := deriveDestroyState(m.clusterInfo.Namespace, d, osdID, osdTree, osdDump)

	// Disengage is checked ahead of every fallible query and side effect, so a withdrawal can always reverse the flow.
	if f.disengageRequested(d, st) {
		return f.disengage(d, osdID)
	}

	if f.terminalReached(st) {
		return f.finalize(d, osdID)
	}

	if st.downscaled {
		gone, err := m.isOSDPodGone(osdID)
		if err != nil {
			return err
		}
		if !gone {
			log.NamespacedInfo(m.clusterInfo.Namespace, logger,
				"osd.%d is scaled down; waiting for the pod to terminate before %s teardown", osdID, f.name())
			return nil
		}
		isEncrypted, err := m.isReplaceOSDEncrypted(d)
		if err != nil {
			return err
		}
		if isEncrypted {
			done, err := m.runCryptCloseJobForOSD(d, osdID)
			if err != nil {
				return err
			}
			if !done {
				return nil
			}
			if err := m.cluster.deleteCryptCloseJob(osdID); err != nil {
				return errors.Wrapf(err, "failed to delete crypto-close job for osd.%d", osdID)
			}
		}
		ok, err := f.terminalGate(osdID)
		if err != nil {
			log.NamespacedWarning(m.clusterInfo.Namespace, logger,
				"failed the %s terminal gate for osd.%d; will re-check next tick. %v", f.name(), osdID, err)
			return nil
		}
		if !ok {
			log.NamespacedInfo(m.clusterInfo.Namespace, logger,
				"osd.%d has not passed the %s terminal gate; will re-check next tick", osdID, f.name())
			return nil
		}
		return f.terminal(d, osdID)
	}

	acted, err := f.startDrain(d, osdID, st)
	if err != nil || acted {
		return err
	}

	ok, err := f.stopGate(osdID)
	if err != nil {
		log.NamespacedWarning(m.clusterInfo.Namespace, logger,
			"failed the %s stop gate for osd.%d; will re-check next tick. %v", f.name(), osdID, err)
		return nil
	}
	if !ok {
		log.NamespacedInfo(m.clusterInfo.Namespace, logger,
			"osd.%d has not passed the %s stop gate; will re-check next tick", osdID, f.name())
		return nil
	}

	return m.scaleDownOSDDeployment(d, osdID)
}

// destroySweep runs one flow's per-tick sweep: collect the flow-owned deployments, and advance each
// one that still has work. Returns the owned OSD ids — including completed members — for the caller
// to exempt from normal health handling. Fetch failures return the owned set with no advancement:
// the set is built from deployment markers before any Ceph query.
func (m *OSDHealthMonitor) destroySweep(f destroyFlow) (map[int]struct{}, error) {
	owned := map[int]struct{}{}

	if f.skipSweep() {
		return owned, nil
	}

	deployments, err := m.cluster.getOSDDeployments()
	if err != nil {
		return owned, errors.Wrapf(err, "failed to list OSD deployments for %s processing", f.name())
	}

	var toProcess []*appsv1.Deployment
	for i := range deployments.Items {
		d := &deployments.Items[i]
		if !f.ownsDeployment(d) {
			continue
		}
		osdID, err := GetOSDID(d)
		if err != nil {
			log.NamespacedWarning(m.clusterInfo.Namespace, logger,
				"skipping %s processing for deployment %q: %v", f.name(), d.Name, err)
			continue
		}
		owned[osdID] = struct{}{}
		if f.isComplete(d) {
			continue
		}
		toProcess = append(toProcess, d)
	}

	if len(toProcess) == 0 {
		return owned, nil
	}

	tree, err := cephclient.HostTree(m.context, m.clusterInfo)
	if err != nil {
		log.NamespacedWarning(m.clusterInfo.Namespace, logger,
			"failed to get osd tree for %s processing; will retry next tick. %v", f.name(), err)
		return owned, nil
	}
	osdDump, err := cephclient.GetOSDDump(m.context, m.clusterInfo)
	if err != nil {
		log.NamespacedWarning(m.clusterInfo.Namespace, logger,
			"failed to get osd dump for %s processing; will retry next tick. %v", f.name(), err)
		return owned, nil
	}

	for _, d := range toProcess {
		osdID, _ := GetOSDID(d) // already validated above
		if err := m.advanceDestroy(f, d, osdID, &tree, osdDump); err != nil {
			log.NamespacedWarning(m.clusterInfo.Namespace, logger,
				"failed to advance %s for osd.%d; will retry next tick. %v", f.name(), osdID, err)
		}
	}

	return owned, nil
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
	d.Spec.Replicas = new(int32(0))
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
