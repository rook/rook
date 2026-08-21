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

package object

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// waitForDeploymentToStart is stubbed in unit tests, where fake deployments never progress
var waitForDeploymentToStart = k8sutil.WaitForDeploymentToStart

const (
	syncHealthCheckInterval = 1 * time.Minute

	// a sync status probe reaches out to the peer zone's gateways, which answer slowly while sync
	// is wedged; a budget above the default Ceph command timeout keeps a slow answer from being
	// thrown away as unknown
	syncStatusCommandTimeout = 60 * time.Second

	// a fresh zone legitimately passes through the same states a wedged zone is stuck in, so only
	// a signal that survives this many consecutive probes counts as a wedge
	wedgeProbeThreshold = 3

	maxRecoveriesPerSignal = 2
	recoveryBudgetWindow   = 30 * time.Minute

	// ceph reports "init" until a zone maps its shards for full sync; a zone that has started
	// syncing reports "sync" instead, even when it is idle and caught up
	syncStatusInit = "init"

	// changing this pod template annotation is what rolls the gateways
	multisiteSyncRecoveryAnnotation = "rook.io/multisite-sync-recovery"

	multisiteSyncRecoveryEvent        = "MultisiteSyncRecovery"
	multisiteSyncRecoverySkippedEvent = "MultisiteSyncRecoverySkipped"
)

// syncProbeResult is what a single sync status probe says about a zone's sync state machine
type syncProbeResult int

const (
	syncHealthy syncProbeResult = iota
	syncWedged
	// syncUnknown covers everything Rook cannot interpret: a command that outran its budget, an
	// unavailable command proxy, output that does not parse. it never changes a wedge streak
	syncUnknown
)

// syncSignal identifies one sync state machine of a zone: its metadata sync, or its data sync from
// one peer zone. streaks and recovery budgets are tracked per signal
type syncSignal struct {
	metadata bool
	peer     string
}

func (s syncSignal) key() string {
	if s.metadata {
		return "metadata"
	}
	return "data/" + s.peer
}

func (s syncSignal) String() string {
	if s.metadata {
		return "metadata sync"
	}
	return fmt.Sprintf("data sync from zone %q", s.peer)
}

type syncObservation struct {
	signal syncSignal
	result syncProbeResult
}

type multisiteSyncChecker struct {
	context        *clusterd.Context
	client         client.Client
	recorder       events.EventRecorder
	clusterInfo    *cephclient.ClusterInfo
	clusterSpec    *cephv1.ClusterSpec
	store          *cephv1.CephObjectStore
	namespacedName types.NamespacedName
	interval       time.Duration

	streaks    map[string]int
	recoveries map[string][]time.Time
	reported   map[string]bool
}

// shouldCheckMultisiteSync reports whether Rook should watch and recover the sync state of a store.
// The gateways of an external store are not Rook's to restart, and a store whose gateways serve no
// sync traffic has no sync state machine that can wedge.
func shouldCheckMultisiteSync(store *cephv1.CephObjectStore) bool {
	return !store.Spec.IsExternal() && store.Spec.IsMultisite() && !store.Spec.Gateway.DisableMultisiteSyncTraffic
}

func newMultisiteSyncChecker(
	context *clusterd.Context,
	client client.Client,
	recorder events.EventRecorder,
	clusterInfo *cephclient.ClusterInfo,
	clusterSpec *cephv1.ClusterSpec,
	store *cephv1.CephObjectStore,
	namespacedName types.NamespacedName,
) *multisiteSyncChecker {
	return &multisiteSyncChecker{
		context:        context,
		client:         client,
		recorder:       recorder,
		clusterInfo:    clusterInfo,
		clusterSpec:    clusterSpec,
		store:          store.DeepCopy(),
		namespacedName: namespacedName,
		interval:       syncHealthCheckInterval,
		streaks:        map[string]int{},
		recoveries:     map[string][]time.Time{},
		reported:       map[string]bool{},
	}
}

// checkSyncHealth periodically checks the multisite sync state of the object store
func (c *multisiteSyncChecker) checkSyncHealth(ctx context.Context) {
	c.checkSyncStatus(ctx)

	for {
		select {
		case <-ctx.Done():
			log.NamedInfo(c.namespacedName, logger, "stopping monitoring multisite sync status")
			return

		case <-time.After(c.interval):
			log.NamedDebug(c.namespacedName, logger, "checking multisite sync status")
			c.checkSyncStatus(ctx)
		}
	}
}

func (c *multisiteSyncChecker) checkSyncStatus(ctx context.Context) {
	objContext, err := NewMultisiteContext(c.context, c.clusterInfo, c.store)
	if err != nil {
		log.NamedDebug(c.namespacedName, logger, "failed to get object context to check multisite sync status. %v", err)
		return
	}

	isMaster, err := CheckZoneIsMaster(objContext)
	if err != nil {
		log.NamedDebug(c.namespacedName, logger, "failed to check whether zone %q is the master zone. %v", objContext.Zone, err)
		return
	}

	observations := []syncObservation{}
	// the master zone owns the metadata it would otherwise sync, so only a secondary can wedge
	if !isMaster {
		observations = append(observations, syncObservation{
			signal: syncSignal{metadata: true},
			result: probeMetadataSync(objContext),
		})
	}

	peers, err := peerZones(objContext)
	if err != nil {
		log.NamedDebug(c.namespacedName, logger, "failed to list the peer zones of zone %q. %v", objContext.Zone, err)
	}
	for _, peer := range peers {
		observations = append(observations, syncObservation{
			signal: syncSignal{peer: peer},
			result: probeDataSync(objContext, peer),
		})
	}

	c.applyObservations(ctx, objContext, observations)
}

func (c *multisiteSyncChecker) applyObservations(ctx context.Context, objContext *Context, observations []syncObservation) {
	healthy := len(observations) > 0
	firing := []syncSignal{}

	for _, observation := range observations {
		key := observation.signal.key()

		switch observation.result {
		case syncHealthy:
			delete(c.streaks, key)

		case syncUnknown:
			healthy = false

		case syncWedged:
			healthy = false
			c.streaks[key]++
			if c.streaks[key] < wedgeProbeThreshold {
				log.NamedInfo(c.namespacedName, logger, "%s of CephObjectStore %q is not initialized (%d/%d probes)",
					observation.signal, c.namespacedName.String(), c.streaks[key], wedgeProbeThreshold)
				continue
			}
			delete(c.streaks, key)
			firing = append(firing, observation.signal)
		}
	}

	if len(firing) > 0 {
		c.recoverFromWedges(ctx, objContext, firing)
	}

	if healthy {
		c.reportHealthy(ctx)
	}
}

// recoverFromWedges breaks sync state machines out of states they never leave on their own by
// pulling the period and restarting the store's gateways. One restart re-initializes every sync
// state machine of the store, so all signals that crossed the wedge threshold in the same probe
// cycle share a single recovery and are all charged for it.
//
// Rook deliberately runs no liveness probe on RGW because restarting a gateway that merely looks
// unhealthy usually costs more than it fixes (see noLivenessProbe). This is not that: it fires only
// on specific wedge signals that survived wedgeProbeThreshold consecutive probes, and it is capped
// at maxRecoveriesPerSignal per recoveryBudgetWindow, after which the store is reported as wedged
// rather than restarted again.
func (c *multisiteSyncChecker) recoverFromWedges(ctx context.Context, objContext *Context, signals []syncSignal) {
	withinBudget := false
	pullPeriod := false
	names := make([]string, 0, len(signals))
	for _, signal := range signals {
		if c.withinRecoveryBudget(signal.key()) {
			withinBudget = true
		}
		if signal.metadata {
			pullPeriod = true
		}
		names = append(names, signal.String())
	}
	if !withinBudget {
		for _, signal := range signals {
			c.reportWedged(ctx, signal)
		}
		return
	}

	now := time.Now()
	for _, signal := range signals {
		c.recoveries[signal.key()] = append(c.recoveries[signal.key()], now)
	}

	wedged := strings.Join(names, ", ")
	log.NamedWarning(c.namespacedName, logger, "%s of zone %q in CephObjectStore %q wedged for %d probes. recovering",
		wedged, objContext.Zone, c.namespacedName.String(), wedgeProbeThreshold)
	c.recorder.Eventf(c.store, nil, corev1.EventTypeNormal, multisiteSyncRecoveryEvent, multisiteSyncRecoveryEvent,
		"recovering wedged %s of zone %q", wedged, objContext.Zone)

	if pullPeriod {
		// the master pushes every new period to this zone and that push is rejected until the realm
		// system user resolves locally, so a zone that missed one never catches up. pulling the
		// period uses outbound auth, which works, and leaves the restart below with a current period
		if _, err := runAdminCommandWithTimeout(objContext, false, syncStatusCommandTimeout, "period", "pull"); err != nil {
			log.NamedWarning(c.namespacedName, logger, "failed to pull the period of realm %q while recovering %s. %v", objContext.Realm, wedged, err)
		}
	}

	if err := c.restartGateways(ctx); err != nil {
		log.NamedError(c.namespacedName, logger, "failed to restart the gateways of CephObjectStore %q to recover %s. %v",
			c.namespacedName.String(), wedged, err)
	}
}

func (c *multisiteSyncChecker) withinRecoveryBudget(key string) bool {
	cutoff := time.Now().Add(-recoveryBudgetWindow)

	recent := []time.Time{}
	for _, recovery := range c.recoveries[key] {
		if recovery.After(cutoff) {
			recent = append(recent, recovery)
		}
	}
	c.recoveries[key] = recent

	return len(recent) < maxRecoveriesPerSignal
}

func (c *multisiteSyncChecker) restartGateways(ctx context.Context) error {
	rgwsToSkipReconcile, err := opcontroller.GetDaemonsToSkipReconcile(ctx, c.context, c.namespacedName.Namespace, config.RgwType, AppName)
	if err != nil {
		return errors.Wrap(err, "failed to check for RGWs to skip reconcile")
	}
	if rgwsToSkipReconcile.Has(c.store.Name) {
		log.NamedWarning(c.namespacedName, logger, "skipping restart of the gateways of CephObjectStore %q labeled with %q",
			c.namespacedName.String(), cephv1.SkipReconcileLabelKey)
		c.recorder.Eventf(c.store, nil, corev1.EventTypeNormal, multisiteSyncRecoverySkippedEvent, multisiteSyncRecoverySkippedEvent,
			"not restarting the gateways labeled with %q to recover multisite sync", cephv1.SkipReconcileLabelKey)
		return nil
	}

	selector := labels.SelectorFromSet(getLabels(c.store.Name, c.namespacedName.Namespace, false)).String()
	deployments, err := k8sutil.GetDeployments(ctx, c.context.Clientset, c.namespacedName.Namespace, selector)
	if err != nil {
		return errors.Wrapf(err, "failed to get the gateway deployments of CephObjectStore %q", c.namespacedName.String())
	}
	if len(deployments.Items) == 0 {
		return errors.Errorf("no gateway deployment found for CephObjectStore %q", c.namespacedName.String())
	}

	// patch only the template annotation, the way kubectl rollout restart does: routing the
	// restart through the full deployment-update path would record the annotation in the
	// last-applied state, and the next reconcile's regenerated template (which does not carry
	// it) would then delete it and roll the gateways a second time for nothing. ok-to-stop is
	// not consulted because ceph implements no such check for rgw daemons.
	restartedAt := time.Now().UTC().Format(time.RFC3339)
	patch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		multisiteSyncRecoveryAnnotation, restartedAt))
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		if _, err := c.context.Clientset.AppsV1().Deployments(c.namespacedName.Namespace).Patch(
			ctx, deployment.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return errors.Wrapf(err, "failed to restart gateway deployment %q", deployment.Name)
		}
		if err := waitForDeploymentToStart(ctx, c.context, deployment); err != nil {
			return errors.Wrapf(err, "failed to wait for gateway deployment %q to restart", deployment.Name)
		}
		log.NamedInfo(c.namespacedName, logger, "restarted gateway deployment %q to recover multisite sync", deployment.Name)
	}

	return nil
}

func (c *multisiteSyncChecker) reportWedged(ctx context.Context, signal syncSignal) {
	key := signal.key()
	if c.reported[key] {
		return
	}
	c.reported[key] = true

	message := fmt.Sprintf("%s of CephObjectStore %q is still wedged after %d recoveries within %s. not restarting the gateways again",
		signal, c.namespacedName.String(), maxRecoveriesPerSignal, recoveryBudgetWindow)
	log.NamedError(c.namespacedName, logger, "%s", message)
	c.recorder.Eventf(c.store, nil, corev1.EventTypeWarning, string(cephv1.MultisiteSyncWedgedReason), string(cephv1.MultisiteSyncWedgedReason), "%s", message)

	c.setSyncHealthCondition(ctx, corev1.ConditionFalse, cephv1.MultisiteSyncWedgedReason, message)
}

func (c *multisiteSyncChecker) reportHealthy(ctx context.Context) {
	if len(c.reported) == 0 {
		return
	}
	c.reported = map[string]bool{}
	c.recoveries = map[string][]time.Time{}

	message := fmt.Sprintf("multisite sync of CephObjectStore %q is initialized", c.namespacedName.String())
	log.NamedInfo(c.namespacedName, logger, "%s", message)

	c.setSyncHealthCondition(ctx, corev1.ConditionTrue, cephv1.MultisiteSyncHealthyReason, message)
}

func (c *multisiteSyncChecker) setSyncHealthCondition(ctx context.Context, status corev1.ConditionStatus, reason cephv1.ConditionReason, message string) {
	condition := cephv1.Condition{
		Type:    cephv1.ConditionMultisiteSyncHealthy,
		Status:  status,
		Reason:  reason,
		Message: message,
	}

	err := reporting.UpdateStatusConditionsWithRetry(ctx, c.client, &cephv1.CephObjectStore{}, c.namespacedName, controllerTypeMeta.Kind, condition)
	if err != nil {
		log.NamedWarning(c.namespacedName, logger, "failed to set the %q condition of CephObjectStore %q to %q. %v",
			cephv1.ConditionMultisiteSyncHealthy, c.namespacedName.String(), status, err)
	}
}

func probeMetadataSync(objContext *Context) syncProbeResult {
	output, err := runAdminCommandWithTimeout(objContext, true, syncStatusCommandTimeout, "metadata", "sync", "status")
	return classifySyncStatus(output, err)
}

func probeDataSync(objContext *Context, peer string) syncProbeResult {
	output, err := runAdminCommandWithTimeout(objContext, true, syncStatusCommandTimeout, "data", "sync", "status", fmt.Sprintf("--source-zone=%s", peer))
	return classifySyncStatus(output, err)
}

// peerZones returns the zones of the zonegroup that this zone syncs from, skipping zones that
// advertise no endpoint to sync from
func peerZones(objContext *Context) ([]string, error) {
	output, err := runAdminCommandWithTimeout(objContext, true, syncStatusCommandTimeout, "zonegroup", "get")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get zonegroup %q", objContext.ZoneGroup)
	}
	zoneGroup, err := DecodeZoneGroupConfig(output)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode zonegroup %q", objContext.ZoneGroup)
	}

	peers := []string{}
	for _, zone := range zoneGroup.Zones {
		if zone.Name == objContext.Zone || len(zone.Endpoints) == 0 {
			continue
		}
		peers = append(peers, zone.Name)
	}

	return peers, nil
}

// syncStatusReport opts in to the only field Rook interprets, keeping the coupling to the
// radosgw-admin output as loose as possible
type syncStatusReport struct {
	SyncStatus struct {
		Info struct {
			Status string `json:"status"`
		} `json:"info"`
	} `json:"sync_status"`
}

func classifySyncStatus(output string, err error) syncProbeResult {
	if err != nil {
		if exec.IsTimeout(err) || kerrors.IsNotFound(err) {
			return syncUnknown
		}
		if syncStatusIsMissing(output) {
			return syncWedged
		}
		// "metadata sync status" reports a missing sync status object only through its exit
		// code, without a recognizable message
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			return syncWedged
		}
		return syncUnknown
	}

	var report syncStatusReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		return syncUnknown
	}
	if report.SyncStatus.Info.Status == syncStatusInit {
		return syncWedged
	}

	return syncHealthy
}

// syncStatusIsMissing reports whether radosgw-admin failed because the zone has no sync status
// object at all, which is how a sync that never initialized reports itself. "metadata sync
// status" phrases the ENOENT as read_sync_status() returning -2; the "sync status" summary
// phrases it as failing to read the sync status.
func syncStatusIsMissing(output string) bool {
	if strings.Contains(output, "sync.read_sync_status() returned ret=-2") {
		return true
	}
	if !strings.Contains(output, "failed to read sync status") {
		return false
	}
	return strings.Contains(output, "(2)") || strings.Contains(output, "No such file or directory")
}
