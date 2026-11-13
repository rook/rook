/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package clusterdisruption

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// osdPDBAppName is the app label value for pdbs targeting osds. It is also the name of the default OSD pdb
	osdPDBAppName = "rook-ceph-osd"
	// osdPDBOsdIdLabel is the label on osd pods for pdbs targeting specific osd ids
	osdPDBOsdIdLabel = "osd"

	// Keys of the rook-ceph-pdbstatemap ConfigMap. They are only read and written by loadDrainState() and
	// drainState.save(). The format is documented for users and must stay stable.
	drainingFailureDomainKey         = "draining-failure-domain"
	drainingFailureDomainDurationKey = "draining-failure-domain-duration"
	setNoOut                         = "set-no-out"
	nooutLastSetAtKeySuffix          = "-noout-last-set-at"

	// DefaultMaintenanceTimeout is the period for which a drained failure domain will remain in noout
	DefaultMaintenanceTimeout = 30 * time.Minute
	nooutFlag                 = "noout"

	// cephStatusSettleTime is how long a "PGs are clean" status is not trusted after a node drain is detected.
	// The OSD pods of a drained node go down rapidly and Ceph needs some time to report the affected PGs.
	cephStatusSettleTime = 60 * time.Second
)

// drainState is the typed view of the rook-ceph-pdbstatemap ConfigMap.
type drainState struct {
	// domain is the only failure domain whose OSDs are currently allowed to go down.
	// Empty means there is no active drain and the default pdb is in place.
	domain string
	// noout records the intent to set the noout flag on the domain. It is only true for planned drains,
	// that is, when the OSDs are down on an unschedulable node.
	noout bool
	// lastDisruption is the last time OSDs were down while PGs were not clean. It also starts the
	// cephStatusSettleTime window when a node drain is seen while PGs are still reported clean.
	lastDisruption time.Time
	// nooutSetAt is when noout was first requested for the domain. The flag is removed from Ceph once the
	// maintenance timeout has passed since then.
	nooutSetAt time.Time
}

// observations is everything the controller observed about the cluster in one reconcile.
type observations struct {
	pgClean     bool
	pgHealthMsg string
	// allDomains are all the failure domains with OSDs
	allDomains []string
	// nodeDrainDomains are the failure domains with OSDs down on an unschedulable or missing node
	nodeDrainDomains []string
	// osdDownDomains are the failure domains with down OSDs, for any reason
	osdDownDomains []string
	// downOSDs are the IDs of the down OSDs
	downOSDs []int
}

// drainDecision is the result of the drain state machine.
type drainDecision struct {
	state drainState
	// excludeOSDs are down OSDs to exclude from the default pdb so that they do not block other drains.
	// It is only set when there is no draining failure domain.
	excludeOSDs []int
	// reason explains the decision for the logs
	reason string
}

func loadDrainState(namespace string, data map[string]string) drainState {
	state := drainState{
		domain: data[drainingFailureDomainKey],
		noout:  data[setNoOut] == "true",
	}
	state.lastDisruption = parseStateTime(namespace, data, drainingFailureDomainDurationKey)
	if state.domain != "" {
		state.nooutSetAt = parseStateTime(namespace, data, state.domain+nooutLastSetAtKeySuffix)
	}
	return state
}

// parseStateTime returns the zero time if the key is missing or invalid. The state machine initializes zero timestamps.
func parseStateTime(namespace string, data map[string]string, key string) time.Time {
	value := data[key]
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		log.NamespacedWarning(namespace, logger, "ignoring invalid timestamp %q for key %q in configmap %q. %v", value, key, pdbStateMapName, err)
		return time.Time{}
	}
	return t
}

// save writes the state to the ConfigMap data. Keys that are not owned by the state are left untouched.
func (s drainState) save(pdbStateMap *corev1.ConfigMap) {
	if pdbStateMap.Data == nil {
		pdbStateMap.Data = map[string]string{}
	}
	// only the draining failure domain can have a noout timestamp. This also removes timestamps that
	// were left behind for failure domains that no longer exist.
	for key := range pdbStateMap.Data {
		if strings.HasSuffix(key, nooutLastSetAtKeySuffix) {
			delete(pdbStateMap.Data, key)
		}
	}
	delete(pdbStateMap.Data, drainingFailureDomainDurationKey)

	pdbStateMap.Data[drainingFailureDomainKey] = s.domain
	pdbStateMap.Data[setNoOut] = ""
	if s.noout {
		pdbStateMap.Data[setNoOut] = "true"
	}
	if !s.lastDisruption.IsZero() {
		pdbStateMap.Data[drainingFailureDomainDurationKey] = s.lastDisruption.Format(time.RFC3339)
	}
	if s.domain != "" && s.noout && !s.nooutSetAt.IsZero() {
		pdbStateMap.Data[s.domain+nooutLastSetAtKeySuffix] = s.nooutSetAt.Format(time.RFC3339)
	}
}

func (r *ReconcileClusterDisruption) initializePDBState(request reconcile.Request) (*corev1.ConfigMap, error) {
	pdbStateMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbStateMapName,
			Namespace: request.Namespace,
		},
	}
	pdbStateMapRequest := types.NamespacedName{
		Name:      pdbStateMapName,
		Namespace: request.Namespace,
	}
	err := r.client.Get(r.context.OpManagerContext, pdbStateMapRequest, pdbStateMap)
	if apierrors.IsNotFound(err) {
		// create configmap to track the draining failure domain
		drainState{}.save(pdbStateMap)
		err := r.client.Create(r.context.OpManagerContext, pdbStateMap)
		if err != nil {
			return pdbStateMap, errors.Wrapf(err, "failed to create the PDB state map %q", pdbStateMapRequest)
		}
	} else if err != nil {
		return pdbStateMap, errors.Wrapf(err, "failed to get the pdbStateMap %s", pdbStateMapRequest)
	}
	return pdbStateMap, nil
}

// nextDrainState is the OSD drain state machine. It has no side effects and does not read the clock.
//
//	OSDs up   + PGs clean     -> steady state: no drain
//	OSDs up   + PGs not clean -> recovery: keep the previous decision until the PGs are clean
//	OSDs down + PGs not clean -> active disruption: exactly one failure domain is allowed to drain
//	OSDs down + PGs clean     -> down OSDs do not affect the data: exclude them from the default pdb,
//	                             unless a node drain was just detected or a drain is still active
func nextDrainState(prev drainState, obs observations, now time.Time) drainDecision {
	osdDown := len(obs.downOSDs) > 0
	var decision drainDecision

	switch {
	case !osdDown && obs.pgClean:
		decision = drainDecision{reason: "OSDs are up and PGs are clean"}

	case !osdDown && !obs.pgClean:
		// Do not allow another failure domain to drain until the PGs recovered from the previous drain
		decision = drainDecision{state: prev, reason: "OSDs are up but PGs are not clean"}

	case osdDown && !obs.pgClean:
		next := pickDrainingDomain(prev, obs)
		next.lastDisruption = now
		decision = drainDecision{state: next, reason: "OSDs are down and PGs are not clean"}

	case len(obs.nodeDrainDomains) == 0:
		// OSDs are down but not due to a node drain (for example a failed drive) and the PGs are clean
		decision = drainDecision{excludeOSDs: obs.downOSDs, reason: "OSDs are down but PGs are clean"}

	default:
		// OSDs are down on a draining node and the PGs are clean. Keep the previous decision.
		next := prev
		if next.lastDisruption.IsZero() {
			next.lastDisruption = now
		}
		decision = drainDecision{state: next}
		switch {
		case next.domain != "":
			decision.reason = "OSDs are down on a draining node and PGs are clean. Keeping the active drain"
		case now.Sub(next.lastDisruption) < cephStatusSettleTime:
			decision.reason = "node drain detected. Waiting for the PG status to settle"
		default:
			decision.excludeOSDs = obs.downOSDs
			decision.reason = "OSDs are down on a draining node but PGs are clean"
		}
	}

	// noout is only tracked for a draining failure domain
	state := &decision.state
	if state.domain == "" || !state.noout {
		state.noout = false
		state.nooutSetAt = time.Time{}
	} else if state.nooutSetAt.IsZero() {
		state.nooutSetAt = now
	}
	return decision
}

// pickDrainingDomain picks the single failure domain that is allowed to drain. The current draining
// failure domain is kept as long as it still has down OSDs. Failure domains with unschedulable nodes
// (that is, a node drain event) are given higher precedence than the failure domains where OSDs are
// down for some other reason. noout is requested only for node drains.
func pickDrainingDomain(prev drainState, obs observations) drainState {
	next := prev
	switch {
	case len(obs.nodeDrainDomains) > 0 && !slices.Contains(obs.nodeDrainDomains, prev.domain):
		next.domain = obs.nodeDrainDomains[0]
		next.noout = true
	case len(obs.osdDownDomains) > 0 && !slices.Contains(obs.osdDownDomains, prev.domain):
		next.domain = obs.osdDownDomains[0]
		next.noout = false
	}
	if next.domain != prev.domain {
		// the noout timeout starts again for a new draining failure domain
		next.nooutSetAt = time.Time{}
	}
	return next
}

// desiredOSDPDBs returns the specs of all the OSD pdbs that should exist, by name.
//   - no draining failure domain: only the default pdb (maxUnavailable=1) for all OSDs
//   - draining failure domain: a blocking pdb (maxUnavailable=0) for every other failure domain and no
//     default pdb, so that all the OSDs in the draining failure domain can go down
func desiredOSDPDBs(decision drainDecision, failureDomainType string, allFailureDomains []string) map[string]policyv1.PodDisruptionBudgetSpec {
	desired := map[string]policyv1.PodDisruptionBudgetSpec{}
	if decision.state.domain == "" {
		desired[osdPDBAppName] = defaultPDBSpec(decision.excludeOSDs)
		return desired
	}
	for _, failureDomainName := range allFailureDomains {
		if failureDomainName != decision.state.domain {
			desired[getPDBName(failureDomainType, failureDomainName)] = blockingPDBSpec(failureDomainType, failureDomainName)
		}
	}
	return desired
}

func defaultPDBSpec(excludeOSDs []int) policyv1.PodDisruptionBudgetSpec {
	matchExpressions := []metav1.LabelSelectorRequirement{
		// require the pod to be an OSD pod
		{
			Key:      k8sutil.AppAttr,
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{osdPDBAppName},
		},
	}
	if len(excludeOSDs) > 0 {
		excludeOSDsValues := make([]string, len(excludeOSDs))
		for i, excludeOSD := range excludeOSDs {
			excludeOSDsValues[i] = strconv.Itoa(excludeOSD)
		}
		matchExpressions = append(matchExpressions, metav1.LabelSelectorRequirement{
			// don't consider pods for excluded OSD IDs
			Key:      osdPDBOsdIdLabel,
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   excludeOSDsValues,
		})
	}
	return policyv1.PodDisruptionBudgetSpec{
		MaxUnavailable: &intstr.IntOrString{IntVal: 1},
		Selector:       &metav1.LabelSelector{MatchExpressions: matchExpressions},
	}
}

func blockingPDBSpec(failureDomainType, failureDomainName string) policyv1.PodDisruptionBudgetSpec {
	return policyv1.PodDisruptionBudgetSpec{
		MaxUnavailable: &intstr.IntOrString{IntVal: 0},
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{fmt.Sprintf(osd.TopologyLocationLabel, failureDomainType): failureDomainName},
		},
	}
}

func (r *ReconcileClusterDisruption) createPDB(pdb client.Object) error {
	err := r.client.Create(r.context.OpManagerContext, pdb)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create pdb %q", pdb.GetName())
	}
	return nil
}

func (r *ReconcileClusterDisruption) deletePDB(pdb client.Object) error {
	err := r.client.Delete(r.context.OpManagerContext, pdb)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete pdb %q", pdb.GetName())
	}
	return nil
}

// osdPDBNames returns the names of the default pdb and of the blocking pdbs of all the current failure domains
func osdPDBNames(failureDomainType string, allFailureDomains []string) sets.Set[string] {
	names := sets.New(osdPDBAppName)
	for _, failureDomainName := range allFailureDomains {
		names.Insert(getPDBName(failureDomainType, failureDomainName))
	}
	return names
}

// applyOSDPDBs makes the OSD pdbs in the cluster match the desired pdbs. Pdbs are created and updated
// before any pdb is deleted so that the OSDs are never left without a pdb in between.
// currentNames are the names of the OSD pdbs for the current failure domains, see osdPDBNames().
func (r *ReconcileClusterDisruption) applyOSDPDBs(namespace string, cephCluster *cephv1.CephCluster, desired map[string]policyv1.PodDisruptionBudgetSpec, currentNames sets.Set[string]) error {
	pdbList := &policyv1.PodDisruptionBudgetList{}
	if err := r.client.List(r.context.OpManagerContext, pdbList, client.InNamespace(namespace)); err != nil {
		return errors.Wrap(err, "failed to list pdbs")
	}
	existing := map[string]*policyv1.PodDisruptionBudget{}
	for i := range pdbList.Items {
		existing[pdbList.Items[i].Name] = &pdbList.Items[i]
	}

	names := make([]string, 0, len(desired))
	for name := range desired {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		spec := desired[name]
		if pdb, ok := existing[name]; ok {
			if apiequality.Semantic.DeepEqual(pdb.Spec, spec) {
				continue
			}
			log.NamespacedInfo(namespace, logger, "updating osd pdb %q. %s", name, describePDBSpec(spec))
			pdb.Spec = spec
			if err := r.client.Update(r.context.OpManagerContext, pdb); err != nil {
				return errors.Wrapf(err, "failed to update pdb %q", name)
			}
			continue
		}
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       spec,
		}
		if err := k8sutil.NewOwnerInfo(cephCluster, r.scheme).SetControllerReference(pdb); err != nil {
			return errors.Wrapf(err, "failed to set owner reference to pdb %q", name)
		}
		log.NamespacedInfo(namespace, logger, "creating osd pdb %q. %s", name, describePDBSpec(spec))
		if err := r.createPDB(pdb); err != nil {
			return err
		}
	}

	// Delete the OSD pdbs that are not desired:
	// - the pdbs of the current failure domains are deleted by name, whoever owns them. A default or
	//   blocking pdb that lost its owner reference would otherwise block the drain forever.
	// - any other OSD pdb owned by the cluster is a blocking pdb of a failure domain that no longer
	//   exists or changed its type.
	for name, pdb := range existing {
		if _, ok := desired[name]; ok {
			continue
		}
		if !currentNames.Has(name) && !isOwnedOSDPDB(pdb, cephCluster) {
			continue
		}
		log.NamespacedInfo(namespace, logger, "deleting osd pdb %q", name)
		if err := r.deletePDB(pdb); err != nil {
			return err
		}
	}
	return nil
}

// isOwnedOSDPDB returns true if the pdb is a default or blocking OSD pdb created by this controller
func isOwnedOSDPDB(pdb *policyv1.PodDisruptionBudget, cephCluster *cephv1.CephCluster) bool {
	if pdb.Name != osdPDBAppName && !strings.HasPrefix(pdb.Name, osdPDBAppName+"-") {
		return false
	}
	return metav1.IsControlledBy(pdb, cephCluster)
}

func describePDBSpec(spec policyv1.PodDisruptionBudgetSpec) string {
	return fmt.Sprintf("maxUnavailable=%s, selector=%q", spec.MaxUnavailable.String(), metav1.FormatLabelSelector(spec.Selector))
}

func (r *ReconcileClusterDisruption) reconcilePDBsForOSDs(
	clusterInfo *cephclient.ClusterInfo,
	request reconcile.Request,
	pdbStateMap *corev1.ConfigMap,
	failureDomainType string,
	allFailureDomains,
	osdDownFailureDomains,
	nodeDrainFailureDomains []string,
	downOSDs []int,
	pgHealthyRegex string,
) (reconcile.Result, error) {
	namespace := request.Namespace
	pgHealthMsg, pgClean, err := cephclient.IsClusterClean(r.context.ClusterdContext, clusterInfo, pgHealthyRegex)
	if err != nil {
		// If the error contains that message, this means the cluster is not up and running
		// No monitors are present and thus no ceph configuration has been created
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Debugf("ceph %q cluster not ready, cannot check status yet.", namespace)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		logger.Debugf("ceph %q cluster failed to check cluster health. %v", namespace, err)
		return opcontroller.WaitForRequeueIfCephClusterNotReady, nil
	}
	cephCluster, ok := r.clusterMap.GetCluster(namespace)
	if !ok {
		return reconcile.Result{}, errors.Errorf("failed to find the namespace %q in the clustermap", namespace)
	}

	// 1. observe
	obs := observations{
		pgClean:          pgClean,
		pgHealthMsg:      pgHealthMsg,
		allDomains:       allFailureDomains,
		nodeDrainDomains: nodeDrainFailureDomains,
		osdDownDomains:   osdDownFailureDomains,
		downOSDs:         downOSDs,
	}

	// 2. decide
	now := time.Now()
	prev := loadDrainState(namespace, pdbStateMap.Data)
	decision := nextDrainState(prev, obs, now)
	logDrainDecision(namespace, prev, decision, obs)

	// 3. act. The steps are idempotent. The state is saved even if a step failed so that the
	// timestamps of the decision are not lost, and the failed step is retried in the next reconcile.
	actErr := r.applyOSDPDBs(namespace, cephCluster, desiredOSDPDBs(decision, failureDomainType, allFailureDomains), osdPDBNames(failureDomainType, allFailureDomains))
	if actErr != nil {
		actErr = errors.Wrapf(actErr, "failed to reconcile osd pdbs in cluster %q", request)
	} else if err := r.updateNoout(clusterInfo, decision.state, allFailureDomains, now); err != nil {
		actErr = errors.Wrapf(err, "failed to update maintenance noout in cluster %q", request)
	}

	decision.state.save(pdbStateMap)
	err = r.client.Update(clusterInfo.Context, pdbStateMap)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to update configMap %q in cluster %q", pdbStateMapName, request)
	}
	if actErr != nil {
		return reconcile.Result{}, actErr
	}

	return r.requeuePDBController(request)
}

// logDrainDecision logs changes of the draining failure domain at info level and everything else at debug level
func logDrainDecision(namespace string, prev drainState, decision drainDecision, obs observations) {
	next := decision.state
	switch {
	case next.domain != "" && next.domain != prev.domain:
		log.NamespacedInfo(namespace, logger, "failure domain %q is draining (set noout: %t). %s. down OSDs: %v, node drains: %v, PG status: %q",
			next.domain, next.noout, decision.reason, obs.downOSDs, obs.nodeDrainDomains, obs.pgHealthMsg)
	case next.domain == "" && prev.domain != "":
		log.NamespacedInfo(namespace, logger, "failure domain %q is no longer draining. %s. down OSDs: %v, PG status: %q",
			prev.domain, decision.reason, obs.downOSDs, obs.pgHealthMsg)
	default:
		log.NamespacedDebug(namespace, logger, "%s. draining failure domain: %q, down OSDs: %v, excluded OSDs: %v, node drains: %v, PG status: %q",
			decision.reason, next.domain, obs.downOSDs, decision.excludeOSDs, obs.nodeDrainDomains, obs.pgHealthMsg)
	}
}

// updateNoout sets noout on the draining failure domain of a planned drain until the maintenance
// timeout expires, and makes sure that noout is not set on any other failure domain.
func (r *ReconcileClusterDisruption) updateNoout(clusterInfo *cephclient.ClusterInfo, state drainState, allFailureDomains []string, now time.Time) error {
	osdDump, err := cephclient.GetOSDDump(r.context.ClusterdContext, clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to get osddump for reconciling maintenance noout in namespace %s", clusterInfo.Namespace)
	}
	for _, failureDomainName := range allFailureDomains {
		wantNoout := failureDomainName == state.domain && state.noout && now.Sub(state.nooutSetAt) < r.maintenanceTimeout
		changed, err := osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, clusterInfo, wantNoout, failureDomainName, nooutFlag)
		if err != nil {
			return errors.Wrapf(err, "failed to update noout (set: %t) on failure domain %q", wantNoout, failureDomainName)
		}
		if !changed {
			continue
		}
		switch {
		case wantNoout:
			log.NamespacedInfo(clusterInfo.Namespace, logger, "set noout on draining failure domain %q for %s", failureDomainName, r.maintenanceTimeout)
		case failureDomainName == state.domain && state.noout:
			log.NamespacedInfo(clusterInfo.Namespace, logger, "unset noout on draining failure domain %q since the maintenance timeout of %s expired", failureDomainName, r.maintenanceTimeout)
		default:
			log.NamespacedInfo(clusterInfo.Namespace, logger, "unset noout on failure domain %q", failureDomainName)
		}
	}
	return nil
}

func (r *ReconcileClusterDisruption) getOSDFailureDomains(clusterInfo *cephclient.ClusterInfo, request reconcile.Request, poolFailureDomain string) ([]string, []string, []string, []int, error) {
	osdDeploymentList := &appsv1.DeploymentList{}
	namespaceListOpts := client.InNamespace(request.Namespace)
	topologyLocationLabel := fmt.Sprintf(osd.TopologyLocationLabel, poolFailureDomain)
	err := r.client.List(clusterInfo.Context, osdDeploymentList, client.MatchingLabels{k8sutil.AppAttr: osd.AppName}, namespaceListOpts)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to list osd deployments")
	}

	allFailureDomains := sets.New[string]()
	nodeDrainFailureDomains := sets.New[string]()
	osdDownFailureDomains := sets.New[string]()
	downOSDs := []int{}

	osdMetadata, err := cephclient.GetOSDMetadata(r.context.ClusterdContext, clusterInfo)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrapf(err, "failed to get OSD status")
	}
	osdNodeNames := make(map[int]string, len(*osdMetadata))
	for _, metadata := range *osdMetadata {
		osdNodeNames[metadata.Id] = metadata.HostName
	}

	for _, deployment := range osdDeploymentList.Items {
		labels := deployment.GetLabels()
		failureDomainName := labels[topologyLocationLabel]
		if failureDomainName == "" {
			return nil, nil, nil, nil, errors.Errorf("failed to get the topology location label %q in OSD deployment %q",
				topologyLocationLabel, deployment.Name)
		}
		allFailureDomains.Insert(failureDomainName)

		// Skip OSDs being replaced. The marker Deployment sits at replicas=0 during the replacement, so
		// counting it as down would create blocking PDBs on the other failure domains and freeze
		// cluster-wide maintenance for the whole (possibly multi-day) replacement.
		if shouldIgnoreOSD(&deployment) {
			logger.Debugf("skipping OSD deployment %q from down-detection because it is marked for replacement", deployment.Name)
			continue
		}

		if deployment.Status.ReadyReplicas >= 1 {
			continue
		}

		// the OSD is down
		osdDownFailureDomains.Insert(failureDomainName)
		osdID, err := osd.GetOSDID(&deployment)
		if err != nil {
			return nil, nil, nil, nil, errors.Wrapf(err, "failed to get ID for the OSD deployment %q", deployment.Name)
		}
		downOSDs = append(downOSDs, osdID)

		// Assume a node drain if the OSD is down on a node that is unschedulable or does not exist
		osdNodeName := osdNodeNames[osdID]
		if osdNodeName == "" {
			logger.Warningf("failed to get the node name for the OSD %d", osdID)
			continue
		}
		isDrained, err := hasOSDNodeDrained(clusterInfo.Context, r.client, osdNodeName)
		if err != nil {
			return nil, nil, nil, nil, errors.Wrapf(err, "failed to check if osd %q node is drained", deployment.Name)
		}
		if isDrained {
			logger.Infof("osd %q is down on node %q and a possible node drain is detected", deployment.Name, osdNodeName)
			nodeDrainFailureDomains.Insert(failureDomainName)
		} else if !strings.HasSuffix(deployment.Name, "-debug") {
			logger.Infof("osd %q is down on node %q but no node drain is detected", deployment.Name, osdNodeName)
		}
	}
	// keep the order stable so that the default pdb selector does not change between reconciles
	slices.Sort(downOSDs)
	return sets.List(allFailureDomains), sets.List(nodeDrainFailureDomains), sets.List(osdDownFailureDomains), downOSDs, nil
}

// shouldIgnoreOSD reports whether an OSD Deployment is part of an in-flight replacement. It keys
// solely on the in-progress annotation, which Rook sets once a replacement request passes validation
// and before the OSD is scaled to replicas=0, so it covers the entire window the replacement OSD is
// down. The two neighbouring markers are deliberately not consulted: the user's replace annotation
// lingers on a NOT-owned, still-running OSD when validation rejected it, and the do-not-reconcile
// fence label is also set by the kubectl-rook-ceph maintenance plugin and by admins fencing an OSD by
// hand. Keying on either would wrongly exempt a genuinely down OSD from PDB down-detection.
func shouldIgnoreOSD(deployment *appsv1.Deployment) bool {
	return deployment.GetAnnotations()[cephv1.ReplaceInProgressOSDAnnotationKey] == "true"
}

// hasOSDNodeDrained returns true if OSD pod is not assigned to any node or if the OSD node is not schedulable
func hasOSDNodeDrained(ctx context.Context, c client.Client, osdNodeName string) (bool, error) {
	node, err := getNode(ctx, c, osdNodeName)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, errors.Wrapf(err, "failed to get node %q", osdNodeName)
	}
	return node.Spec.Unschedulable, nil
}

func getNode(ctx context.Context, c client.Client, nodeName string) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := c.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get node %q", nodeName)
	}
	return node, nil
}

func getPDBName(failureDomainType, failureDomainName string) string {
	return k8sutil.TruncateNodeName(fmt.Sprintf("%s-%s-%s", osdPDBAppName, failureDomainType, "%s"), failureDomainName)
}

// requeuePDBController returns a requeue request with a timeout if:
// - default OSD PDB is not available, that is, a failure domain is draining.
// - allowedDisruption in the default PDB is 0, that is, one or more OSDs went down.
// - the default PDB excludes OSDs, that is, OSDs are down but PGs are clean.
func (r *ReconcileClusterDisruption) requeuePDBController(request reconcile.Request) (reconcile.Result, error) {
	defaultPDB := &policyv1.PodDisruptionBudget{}
	err := r.client.Get(r.context.OpManagerContext, types.NamespacedName{Name: osdPDBAppName, Namespace: request.Namespace}, defaultPDB)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.NamespacedDebug(request.Namespace, logger, "default osd pdb %q not found. requeuing the osd pdb controller", osdPDBAppName)
			return reconcile.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to get allowed disruptions count from default osd pdb %q.", osdPDBAppName)
	}

	if defaultPDB.Status.DisruptionsAllowed == 0 || pdbExcludesOSDs(defaultPDB) {
		log.NamespacedDebug(request.Namespace, logger, "default osd pdb %q does not allow disruptions or excludes OSDs. requeuing the osd pdb controller", osdPDBAppName)
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	log.NamespacedDebug(request.Namespace, logger, "successfully reconciled OSD PDB controller")
	return reconcile.Result{}, nil
}

func pdbExcludesOSDs(pdb *policyv1.PodDisruptionBudget) bool {
	if pdb == nil || pdb.Spec.Selector == nil {
		return false
	}
	for _, matchExpression := range pdb.Spec.Selector.MatchExpressions {
		if matchExpression.Key == osdPDBOsdIdLabel {
			return true
		}
	}
	return false
}
