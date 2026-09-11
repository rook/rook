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
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
	// osdPDBAppName is the app label value for pdbs targeting osds
	osdPDBAppName = "rook-ceph-osd"
	// osdPDBOsdIdLabel is the label on osd pods for pdbs targeting specific osd ids
	osdPDBOsdIdLabel = "osd"
	// DefaultMaintenanceTimeout is the period for which a drained failure domain will remain in noout
	DefaultMaintenanceTimeout = 30 * time.Minute
	nooutFlag                 = "noout"

	// CephStatusSettleTime is how long to wait for Ceph to update PG status after detecting a node drain.
	// When a node is drained, OSDs go down rapidly but Ceph's PG health might take several seconds
	// to reflect the impact. During this window, Ceph may report "PGs clean" even though PGs are
	// about to become unhealthy. We wait up to 60 seconds before trusting the "PGs clean" status.
	CephStatusSettleTime = 60 * time.Second
)

// DrainState represents the current drain state of the cluster.
type DrainState struct {
	// ActiveDrains maps failure domain name → drain details
	// Empty map = no active drains
	ActiveDrains map[string]DomainDrainInfo `json:"activeDrains"`

	// When this state was last updated
	LastUpdate time.Time `json:"lastUpdate"`
}

// DomainDrainInfo contains details about a draining failure domain
type DomainDrainInfo struct {
	// Which failure domain is draining (e.g., "zone-a")
	Domain string `json:"domain"`

	// When the drain started
	StartTime time.Time `json:"startTime"`

	// Whether we're still waiting for Ceph PG status to settle (60s wait)
	WaitingForCephStatus bool `json:"waitingForCephStatus"`

	// Whether to set noout on this domain
	SetNoOut bool `json:"setNoOut"`

	// When noout was set (for timeout tracking)
	NoOutSetAt time.Time `json:"noOutSetAt,omitempty"`
}

// ToConfigMapData serializes DrainState to ConfigMap data
func (s DrainState) ToConfigMapData() (map[string]string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return map[string]string{"drainState": string(data)}, nil
}

// drainStateFromConfigMap deserializes DrainState from ConfigMap
func drainStateFromConfigMap(data map[string]string) (DrainState, error) {
	stateJSON := data["drainState"]
	if stateJSON == "" {
		// No state yet, return empty
		return DrainState{
			ActiveDrains: make(map[string]DomainDrainInfo),
			LastUpdate:   time.Now(),
		}, nil
	}

	var state DrainState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return DrainState{}, err
	}
	return state, nil
}

// drainingDomainNames returns list of draining domain names for logging
func (s DrainState) drainingDomainNames() []string {
	names := make([]string, 0, len(s.ActiveDrains))
	for name := range s.ActiveDrains {
		names = append(names, name)
	}
	return names
}

// DesiredPDBSet represents what PDBs should exist
type DesiredPDBSet struct {
	// Default PDB spec (nil = should not exist)
	DefaultPDB *policyv1.PodDisruptionBudgetSpec

	// Blocking PDBs keyed by PDB name
	BlockingPDBs map[string]policyv1.PodDisruptionBudgetSpec

	// Which OSDs to exclude from default PDB
	ExcludeOSDs []int
}

// ClusterObservations packages all cluster state we observe
type ClusterObservations struct {
	// PG health status
	PGClean     bool
	PGHealthMsg string

	// Failure domain information
	AllDomains       []string // All failure domains in cluster
	NodeDrainDomains []string // Domains with nodes being drained
	OSDDownDomains   []string // Domains with down OSDs
	DownOSDs         []int    // Individual OSD IDs that are down
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
		// Create configmap with empty drain state
		emptyState := DrainState{
			ActiveDrains: make(map[string]DomainDrainInfo),
			LastUpdate:   time.Now(),
		}
		data, err := emptyState.ToConfigMapData()
		if err != nil {
			return pdbStateMap, errors.Wrap(err, "failed to serialize empty drain state")
		}
		pdbStateMap.Data = data
		err = r.client.Create(r.context.OpManagerContext, pdbStateMap)
		if err != nil {
			return pdbStateMap, errors.Wrapf(err, "failed to create the PDB state map %q", pdbStateMapRequest)
		}
	} else if err != nil {
		return pdbStateMap, errors.Wrapf(err, "failed to get the pdbStateMap %s", pdbStateMapRequest)
	}
	return pdbStateMap, nil
}

// detectDrainState determines the new drain state based on observations.
// This is a PURE function with no side effects - easy to test.
func detectDrainState(
	currentState DrainState,
	observations ClusterObservations,
) DrainState {
	// If everything is healthy, clear all drain state
	if observations.PGClean &&
		len(observations.NodeDrainDomains) == 0 &&
		len(observations.OSDDownDomains) == 0 {
		return DrainState{
			ActiveDrains: make(map[string]DomainDrainInfo),
			LastUpdate:   time.Now(),
		}
	}

	newState := DrainState{
		ActiveDrains: make(map[string]DomainDrainInfo),
		LastUpdate:   time.Now(),
	}

	// Process node drain domains (these are actual maintenance events)
	for _, domain := range observations.NodeDrainDomains {
		existingDrain, alreadyDraining := currentState.ActiveDrains[domain]

		if !alreadyDraining {
			// NEW DRAIN: Start waiting for Ceph status to settle
			newState.ActiveDrains[domain] = DomainDrainInfo{
				Domain:               domain,
				StartTime:            time.Now(),
				WaitingForCephStatus: true, // Enter 60-second wait
				SetNoOut:             true, // Node drain → set noout
				NoOutSetAt:           time.Now(),
			}
			continue
		}

		// EXISTING DRAIN: Check wait period
		if existingDrain.WaitingForCephStatus {
			elapsed := time.Since(existingDrain.StartTime)
			if elapsed < CephStatusSettleTime {
				// Still in wait period - keep waiting
				newState.ActiveDrains[domain] = existingDrain
			} else {
				// Wait period over - exit waiting state
				existingDrain.WaitingForCephStatus = false
				newState.ActiveDrains[domain] = existingDrain
			}
			continue
		}

		// Past wait period: keep drain active if PGs not clean
		if !observations.PGClean {
			newState.ActiveDrains[domain] = existingDrain
		}
		// If PGs clean after wait: drain recovered, don't add to newState
	}

	// Process OSD-down domains (not node drains - e.g., disk failures)
	for _, domain := range observations.OSDDownDomains {
		if _, exists := newState.ActiveDrains[domain]; !exists {
			// OSD down in this domain but no node drain
			if !observations.PGClean {
				// PGs not clean - treat as drain
				newState.ActiveDrains[domain] = DomainDrainInfo{
					Domain:    domain,
					StartTime: time.Now(),
					SetNoOut:  false, // Not a node drain, don't set noout
				}
			}
			// If PGs clean: drive failure scenario, will be handled by excluding OSDs
		}
	}

	return newState
}

// calculateDesiredPDBs determines what PDBs should exist given the drain state.
func calculateDesiredPDBs(
	drainState DrainState,
	observations ClusterObservations,
	failureDomainType string,
) DesiredPDBSet {
	result := DesiredPDBSet{
		BlockingPDBs: make(map[string]policyv1.PodDisruptionBudgetSpec),
		ExcludeOSDs:  []int{},
	}

	// CASE 1: No active drains
	if len(drainState.ActiveDrains) == 0 {
		// Just default PDB, possibly excluding down OSDs
		if observations.PGClean && len(observations.DownOSDs) > 0 {
			// Drive failure scenario: exclude down OSDs from default PDB
			result.ExcludeOSDs = observations.DownOSDs
		}
		result.DefaultPDB = makeDefaultPDBSpec(result.ExcludeOSDs)
		return result
	}

	// CASE 2: Active drains
	// Determine if we should exclude OSDs from default PDB
	// (only if past wait period and PGs still clean)
	for _, drainInfo := range drainState.ActiveDrains {
		if !drainInfo.WaitingForCephStatus && observations.PGClean {
			// Past wait period, PGs clean - this is a genuine drive failure
			result.ExcludeOSDs = observations.DownOSDs
			break
		}
	}

	// Create blocking PDBs for all non-draining domains
	for _, domain := range observations.AllDomains {
		if _, isDraining := drainState.ActiveDrains[domain]; !isDraining {
			pdbName := getPDBName(failureDomainType, domain)
			result.BlockingPDBs[pdbName] = makeBlockingPDBSpec(failureDomainType, domain)
		}
	}

	return result
}

// makeDefaultPDBSpec constructs the default PDB spec
func makeDefaultPDBSpec(excludeOSDs []int) *policyv1.PodDisruptionBudgetSpec {
	matchExpressions := []metav1.LabelSelectorRequirement{
		{
			Key:      k8sutil.AppAttr,
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{osdPDBAppName},
		},
	}

	// Exclude specific OSDs if needed
	if len(excludeOSDs) > 0 {
		excludeValues := make([]string, len(excludeOSDs))
		for i, osd := range excludeOSDs {
			excludeValues[i] = strconv.Itoa(osd)
		}
		matchExpressions = append(matchExpressions, metav1.LabelSelectorRequirement{
			Key:      osdPDBOsdIdLabel,
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   excludeValues,
		})
	}

	return &policyv1.PodDisruptionBudgetSpec{
		MaxUnavailable: &intstr.IntOrString{IntVal: 1},
		Selector: &metav1.LabelSelector{
			MatchExpressions: matchExpressions,
		},
	}
}

// makeBlockingPDBSpec constructs a blocking PDB spec for a failure domain
func makeBlockingPDBSpec(failureDomainType, domainName string) policyv1.PodDisruptionBudgetSpec {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			fmt.Sprintf(osd.TopologyLocationLabel, failureDomainType): domainName,
		},
	}

	return policyv1.PodDisruptionBudgetSpec{
		MaxUnavailable: &intstr.IntOrString{IntVal: 0},
		Selector:       selector,
	}
}

// reconcilePDBs makes the actual PDB state match the desired state.
func (r *ReconcileClusterDisruption) reconcilePDBs(
	ctx context.Context,
	namespace string,
	desired DesiredPDBSet,
	cephCluster *cephv1.CephCluster,
) error {
	// Get all existing OSD PDBs
	existingPDBs, err := r.listOSDPDBs(ctx, namespace)
	if err != nil {
		return errors.Wrap(err, "failed to list existing PDBs")
	}

	trackedPDBs := make(map[string]bool)

	// Reconcile DEFAULT PDB
	if desired.DefaultPDB != nil {
		// Default PDB should exist
		if err := r.reconcileSinglePDB(ctx, namespace, osdPDBAppName, *desired.DefaultPDB, existingPDBs, cephCluster); err != nil {
			return errors.Wrap(err, "failed to reconcile default PDB")
		}
		trackedPDBs[osdPDBAppName] = true
	} else {
		// Default PDB should NOT exist (active drain case)
		if pdb, exists := existingPDBs[osdPDBAppName]; exists {
			logger.Infof("Deleting default PDB %q during active drain", osdPDBAppName)
			if err := r.client.Delete(ctx, pdb); err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete default PDB %q", osdPDBAppName)
			}
		}
		trackedPDBs[osdPDBAppName] = true
	}

	// Reconcile BLOCKING PDBs
	for pdbName, pdbSpec := range desired.BlockingPDBs {
		if err := r.reconcileSinglePDB(ctx, namespace, pdbName, pdbSpec, existingPDBs, cephCluster); err != nil {
			return errors.Wrapf(err, "failed to reconcile blocking PDB %q", pdbName)
		}
		trackedPDBs[pdbName] = true
	}

	// Delete UNEXPECTED PDBs
	for name, pdb := range existingPDBs {
		if !trackedPDBs[name] {
			logger.Infof("Deleting unexpected PDB %q", name)
			if err := r.client.Delete(ctx, pdb); err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete PDB %q", name)
			}
		}
	}

	return nil
}

// reconcileSinglePDB creates or updates a single PDB
func (r *ReconcileClusterDisruption) reconcileSinglePDB(
	ctx context.Context,
	namespace string,
	pdbName string,
	desiredSpec policyv1.PodDisruptionBudgetSpec,
	existingPDBs map[string]*policyv1.PodDisruptionBudget,
	cephCluster *cephv1.CephCluster,
) error {
	if currentPDB, exists := existingPDBs[pdbName]; exists {
		// PDB exists - update if spec changed
		if !reflect.DeepEqual(currentPDB.Spec, desiredSpec) {
			logger.Infof("Updating PDB %q", pdbName)
			currentPDB.Spec = desiredSpec
			if err := r.client.Update(ctx, currentPDB); err != nil {
				return errors.Wrapf(err, "failed to update PDB %q", pdbName)
			}
		}
	} else {
		// PDB doesn't exist - create it
		logger.Infof("Creating PDB %q", pdbName)
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pdbName,
				Namespace: namespace,
			},
			Spec: desiredSpec,
		}

		// Set owner reference
		ownerInfo := k8sutil.NewOwnerInfo(cephCluster, r.scheme)
		if err := ownerInfo.SetControllerReference(pdb); err != nil {
			return errors.Wrap(err, "failed to set owner reference")
		}

		if err := r.client.Create(ctx, pdb); err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create PDB %q", pdbName)
		}
	}

	return nil
}

// listOSDPDBs lists all OSD-related PDBs in the namespace
func (r *ReconcileClusterDisruption) listOSDPDBs(
	ctx context.Context,
	namespace string,
) (map[string]*policyv1.PodDisruptionBudget, error) {
	pdbList := &policyv1.PodDisruptionBudgetList{}
	if err := r.client.List(ctx, pdbList, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	result := make(map[string]*policyv1.PodDisruptionBudget)
	for i := range pdbList.Items {
		pdb := &pdbList.Items[i]
		// Only track OSD-related PDBs
		if strings.HasPrefix(pdb.Name, osdPDBAppName) {
			result[pdb.Name] = pdb
		}
	}

	return result, nil
}

// updateNooutFromState updates noout flags based on drain state
func (r *ReconcileClusterDisruption) updateNooutFromState(
	clusterInfo *cephclient.ClusterInfo,
	drainState DrainState,
	allFailureDomains []string,
) error {
	osdDump, err := cephclient.GetOSDDump(r.context.ClusterdContext, clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to get osddump for reconciling noout")
	}

	for _, failureDomainName := range allFailureDomains {
		drainInfo, isDraining := drainState.ActiveDrains[failureDomainName]

		if isDraining && drainInfo.SetNoOut {
			// Should have noout set
			elapsed := time.Since(drainInfo.NoOutSetAt)
			if elapsed >= r.maintenanceTimeout {
				// Noout expired - unset it
				logger.Infof("Noout timeout expired for %q", failureDomainName)
				if _, err := osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, clusterInfo, false, failureDomainName, nooutFlag); err != nil {
					return errors.Wrapf(err, "failed to unset noout on %q", failureDomainName)
				}
			} else {
				// Set noout
				if _, err := osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, clusterInfo, true, failureDomainName, nooutFlag); err != nil {
					return errors.Wrapf(err, "failed to set noout on %q", failureDomainName)
				}
			}
		} else {
			// Should NOT have noout set
			if _, err := osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, clusterInfo, false, failureDomainName, nooutFlag); err != nil {
				return errors.Wrapf(err, "failed to ensure noout unset on %q", failureDomainName)
			}
		}
	}

	return nil
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
	// Check Ceph cluster health
	pgHealthMsg, pgClean, err := cephclient.IsClusterClean(r.context.ClusterdContext, clusterInfo, pgHealthyRegex)
	if err != nil {
		// If the error contains that message, this means the cluster is not up and running
		// No monitors are present and thus no ceph configuration has been created
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Debugf("ceph %q cluster not ready, cannot check status yet.", request.Namespace)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		logger.Debugf("ceph %q cluster failed to check cluster health. %v", request.Namespace, err)
		return opcontroller.WaitForRequeueIfCephClusterNotReady, nil
	}

	// STEP 2: Package observations
	observations := ClusterObservations{
		PGClean:          pgClean,
		PGHealthMsg:      pgHealthMsg,
		AllDomains:       allFailureDomains,
		NodeDrainDomains: nodeDrainFailureDomains,
		OSDDownDomains:   osdDownFailureDomains,
		DownOSDs:         downOSDs,
	}

	logger.Infof("Cluster observations: PGs=%q, NodeDrains=%v, OSDDownDomains=%v, DownOSDs=%v",
		pgHealthMsg, nodeDrainFailureDomains, osdDownFailureDomains, downOSDs)

	// Load current drain state
	currentState, err := drainStateFromConfigMap(pdbStateMap.Data)
	if err != nil {
		logger.Warningf("Failed to parse drain state from configmap, starting fresh: %v", err)
		currentState = DrainState{
			ActiveDrains: make(map[string]DomainDrainInfo),
			LastUpdate:   time.Now(),
		}
	}

	// Detect new drain state
	newState := detectDrainState(currentState, observations)

	logger.Infof("Drain state: %d active drains", len(newState.ActiveDrains))
	for domain, info := range newState.ActiveDrains {
		logger.Infof("  - Domain %q: draining (waiting=%v, noout=%v)",
			domain, info.WaitingForCephStatus, info.SetNoOut)
	}

	// Calculate desired PDBs
	desiredPDBs := calculateDesiredPDBs(newState, observations, failureDomainType)

	if desiredPDBs.DefaultPDB != nil {
		logger.Infof("Desired state: default PDB (exclude %d OSDs), %d blocking PDBs",
			len(desiredPDBs.ExcludeOSDs), len(desiredPDBs.BlockingPDBs))
	} else {
		logger.Infof("Desired state: no default PDB (active drain), %d blocking PDBs",
			len(desiredPDBs.BlockingPDBs))
	}

	// Reconcile PDBs to match desired state
	cephCluster, ok := r.clusterMap.GetCluster(request.Namespace)
	if !ok {
		return reconcile.Result{}, errors.Errorf("failed to find cluster in namespace %q", request.Namespace)
	}

	if err := r.reconcilePDBs(clusterInfo.Context, clusterInfo.Namespace, desiredPDBs, cephCluster); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile PDBs")
	}

	// Update noout flags
	if err := r.updateNooutFromState(clusterInfo, newState, allFailureDomains); err != nil {
		logger.Errorf("failed to update noout in cluster %q: %v", request.Namespace, err)
		return reconcile.Result{}, err
	}

	// Save new drain state to ConfigMap
	newConfigMapData, err := newState.ToConfigMapData()
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to serialize drain state")
	}
	pdbStateMap.Data = newConfigMapData

	if err := r.client.Update(clusterInfo.Context, pdbStateMap); err != nil {
		if errors.Is(err, context.Canceled) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to update configMap %q", pdbStateMapName)
	}

	// Requeue if needed
	return requeueIfNeeded(newState, observations), nil
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

	// Build OSD ID → node name lookup map once (O(1) instead of O(m) per OSD)
	osdNodeMap := make(map[int]string, len(*osdMetadata))
	for _, metadata := range *osdMetadata {
		osdNodeMap[metadata.Id] = metadata.HostName
	}

	for _, deployment := range osdDeploymentList.Items {
		labels := deployment.GetLabels()
		failureDomainName := labels[topologyLocationLabel]
		if failureDomainName == "" {
			return nil, nil, nil, nil, errors.Errorf("failed to get the topology location label %q in OSD deployment %q",
				topologyLocationLabel, deployment.Name)
		}

		// Insert is idempotent - no need to check Has() first
		allFailureDomains.Insert(failureDomainName)

		// Skip OSDs being replaced. The marker Deployment sits at replicas=0 during the replacement, so
		// counting it as down would create blocking PDBs on the other failure domains and freeze
		// cluster-wide maintenance for the whole (possibly multi-day) replacement.
		if shouldIgnoreOSD(&deployment) {
			logger.Debugf("skipping OSD deployment %q from down-detection because it is marked for replacement", deployment.Name)
			continue
		}

		// Assume node drain if osd deployment ReadyReplicas count is 0 and OSD pod is not scheduled on a node
		if deployment.Status.ReadyReplicas < 1 {
			osdDownFailureDomains.Insert(failureDomainName)

			osdID, err := osd.GetOSDID(&deployment)
			if err != nil {
				return nil, nil, nil, nil, errors.Wrapf(err, "failed to get ID for the OSD deployment %q", deployment.Name)
			}
			downOSDs = append(downOSDs, osdID)

			// O(1) lookup instead of O(m) loop
			osdNodeName, ok := osdNodeMap[osdID]
			if !ok || osdNodeName == "" {
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
	}
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

// requeueIfNeeded determines if reconcile should requeue based on actual cluster state.
// Returns a requeue result if the cluster state requires continued monitoring.
func requeueIfNeeded(
	drainState DrainState,
	observations ClusterObservations,
) reconcile.Result {
	// Monitor active drains - need to watch for recovery
	if len(drainState.ActiveDrains) > 0 {
		logger.Infof("Active drains: %v. Requeuing in 30s to monitor recovery",
			drainState.drainingDomainNames())
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}
	}

	// Monitor drive failures - down OSDs with clean PGs
	if len(observations.DownOSDs) > 0 {
		logger.Infof("Down OSDs: %v. Requeuing in 30s to monitor recovery",
			observations.DownOSDs)
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}
	}

	// Everything healthy - wait for external events (pod changes, etc.)
	logger.Info("successfully reconciled OSD PDB controller")
	return reconcile.Result{}
}

// requeuePDBController is the legacy requeue logic based on PDB status.
// DEPRECATED: Kept for reference but no longer used. Use requeueIfNeeded() instead.
func (r *ReconcileClusterDisruption) requeuePDBController(request reconcile.Request) (reconcile.Result, error) {
	defaultPDB := &policyv1.PodDisruptionBudget{}
	err := r.client.Get(r.context.OpManagerContext, types.NamespacedName{Name: osdPDBAppName, Namespace: request.Namespace}, defaultPDB)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debugf("default osd pdb %q not found", osdPDBAppName)
			logger.Info("reconciling osd pdb controller")
			return reconcile.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
		} else {
			return reconcile.Result{}, errors.Wrapf(err, "failed to get allowed disruptions count from default osd pdb %q.", osdPDBAppName)
		}
	}

	if defaultPDB.Status.DisruptionsAllowed == 0 || pdbExcludesOSDs(defaultPDB) {
		logger.Info("reconciling osd pdb controller")
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	logger.Info("successfully reconciled OSD PDB controller")
	return reconcile.Result{}, nil
}

func pdbExcludesOSDs(pdb *policyv1.PodDisruptionBudget) bool {
	if pdb == nil {
		return false
	}
	if pdb.Spec.Selector == nil {
		return false
	}
	if pdb.Spec.Selector.MatchExpressions == nil {
		return false
	}
	for _, matchExpression := range pdb.Spec.Selector.MatchExpressions {
		if matchExpression.Key == osdPDBOsdIdLabel {
			return true
		}
	}
	return false
}
