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
	// osdPDBAppName is that app label value for pdbs targeting osds
	osdPDBAppName = "rook-ceph-osd"
	// osdPDBOsdIdLabel is the label on osd pods for pdbs targeting specific osd ids
	osdPDBOsdIdLabel                 = "osd"
	drainingFailureDomainKey         = "draining-failure-domain"
	drainingFailureDomainDurationKey = "draining-failure-domain-duration"
	setNoOut                         = "set-no-out"
	// DefaultMaintenanceTimeout is the period for which a drained failure domain will remain in noout
	DefaultMaintenanceTimeout = 30 * time.Minute
	nooutFlag                 = "noout"
)

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

// createDefaultPDBforOSD creates a single PDB for all OSDs with maxUnavailable=1
// This allows all OSDs in a single failure domain to go down.
func (r *ReconcileClusterDisruption) createDefaultPDBforOSD(namespace string, excludeOSDs []int) error {
	cephCluster, ok := r.clusterMap.GetCluster(namespace)
	if !ok {
		return errors.Errorf("failed to find the namespace %q in the clustermap", namespace)
	}
	pdbRequest := types.NamespacedName{Name: osdPDBAppName, Namespace: namespace}
	objectMeta := metav1.ObjectMeta{
		Name:      osdPDBAppName,
		Namespace: namespace,
	}
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

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 1},
			Selector: &metav1.LabelSelector{
				MatchExpressions: matchExpressions,
			},
		},
	}
	ownerInfo := k8sutil.NewOwnerInfo(cephCluster, r.scheme)
	err := ownerInfo.SetControllerReference(pdb)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to pdb %v", pdb)
	}

	existingPDB := &policyv1.PodDisruptionBudget{}
	err = r.client.Get(r.context.OpManagerContext, pdbRequest, existingPDB)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("all PGs are active+clean. Restoring default OSD pdb settings")
			logger.Infof("creating the default pdb %q with maxUnavailable=1 for all osd", osdPDBAppName)
			return r.createPDB(pdb)
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
	}

	existingPDB.Spec = pdb.Spec
	err = r.client.Update(r.context.OpManagerContext, existingPDB)
	if err != nil {
		return errors.Wrapf(err, "failed to update existing pdb %q", existingPDB.Name)
	}
	return nil
}

func (r *ReconcileClusterDisruption) deleteDefaultPDBforOSD(namespace string) error {
	pdbRequest := types.NamespacedName{Name: osdPDBAppName, Namespace: namespace}
	objectMeta := metav1.ObjectMeta{
		Name:      osdPDBAppName,
		Namespace: namespace,
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
	}
	err := r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1.PodDisruptionBudget{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
	}
	logger.Infof("deleting the default pdb %q with maxUnavailable=1 for all osd", osdPDBAppName)
	return r.deletePDB(pdb)
}

// createBlockingPDBForOSD creates individual blocking PDBs (maxUnavailable=0) for all the OSDs in
// failure domains that are not draining
func (r *ReconcileClusterDisruption) createBlockingPDBForOSD(namespace, failureDomainType, failureDomainName string) error {
	cephCluster, ok := r.clusterMap.GetCluster(namespace)
	if !ok {
		return errors.Errorf("failed to find the namespace %q in the clustermap", namespace)
	}

	pdbName := getPDBName(failureDomainType, failureDomainName)
	pdbRequest := types.NamespacedName{Name: pdbName, Namespace: namespace}
	objectMeta := metav1.ObjectMeta{
		Name:      pdbName,
		Namespace: namespace,
	}
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{fmt.Sprintf(osd.TopologyLocationLabel, failureDomainType): failureDomainName},
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 0},
			Selector:       selector,
		},
	}
	ownerInfo := k8sutil.NewOwnerInfo(cephCluster, r.scheme)
	err := ownerInfo.SetControllerReference(pdb)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to pdb %v", pdb)
	}
	err = r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1.PodDisruptionBudget{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Infof("creating temporary blocking pdb %q with maxUnavailable=0 for %q failure domain %q", pdbName, failureDomainType, failureDomainName)
			return r.createPDB(pdb)
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
	}
	return nil
}

func (r *ReconcileClusterDisruption) deleteBlockingPDBForOSD(namespace, failureDomainType, failureDomainName string) error {
	pdbName := getPDBName(failureDomainType, failureDomainName)
	pdbRequest := types.NamespacedName{Name: pdbName, Namespace: namespace}
	objectMeta := metav1.ObjectMeta{
		Name:      pdbName,
		Namespace: namespace,
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
	}
	err := r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1.PodDisruptionBudget{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
	}
	logger.Infof("deleting temporary blocking pdb with %q with maxUnavailable=0 for %q failure domain %q", pdbName, failureDomainType, failureDomainName)
	return r.deletePDB(pdb)
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
		pdbStateMap.Data = map[string]string{drainingFailureDomainKey: "", setNoOut: ""}
		err := r.client.Create(r.context.OpManagerContext, pdbStateMap)
		if err != nil {
			return pdbStateMap, errors.Wrapf(err, "failed to create the PDB state map %q", pdbStateMapRequest)
		}
	} else if err != nil {
		return pdbStateMap, errors.Wrapf(err, "failed to get the pdbStateMap %s", pdbStateMapRequest)
	}
	return pdbStateMap, nil
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

	osdDown := len(downOSDs) > 0
	// OSDs which should be excluded from the default PDB. This is done when there are no active drains and all PGs are
	// active+clean, but there are some down OSDs. In that case we exclude the down OSDs from the PDB otherwise all drains
	// in the cluster would be blocked until the down OSDs came back.
	excludeOSDs := make([]int, 0)

	// switch block to update the PDB state config map based on the PG status and running OSDs
	switch {
	case !osdDown && pgClean:
		logger.Infof("OSDs are up and PGs are clean. PG status: %q", pgHealthMsg)
		resetPDBConfig(pdbStateMap)
	case osdDown && pgClean:
		logger.Infof("OSD(s) %v are down but PGs are clean. PG Status: %q", downOSDs, pgHealthMsg)
		// In case of a node drain event, the OSD pods can get drained rapidly and it would take some time for rook to fetch
		// the correct PG status. So wait for 60 seconds when OSD is down and node drain event is detected
		if len(nodeDrainFailureDomains) > 0 {
			lastNodeDrainTimeStamp, err := getLastNodeDrainTimeStamp(pdbStateMap, drainingFailureDomainDurationKey)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to get last node drain timestamp from the configmap %q", pdbStateMap.Name)
			}
			if time.Since(lastNodeDrainTimeStamp) < 60*time.Second {
				logger.Infof("node drain is detected. Requeue to ensure that correct PG status is read.")
			} else {
				excludeOSDs = slices.Clone(downOSDs)
			}
		} else {
			excludeOSDs = slices.Clone(downOSDs)
			resetPDBConfig(pdbStateMap)
		}
	case osdDown && !pgClean:
		setPDBConfig(pdbStateMap, osdDownFailureDomains, nodeDrainFailureDomains)
		logger.Infof("OSD(s) %v are down and PGs are not clean. PGs Status: %q", downOSDs, pgHealthMsg)

	// no-op. Wait for the PGs to become healthy from the previous node drain event
	case !osdDown && !pgClean && len(pdbStateMap.Data[drainingFailureDomainKey]) > 1:
		logger.Infof("OSDs are up but PGs are not clean from previous drain event. PGs Status: %q", pgHealthMsg)
	}

	// handle drains based on the PDB config map
	if pdbStateMap.Data[drainingFailureDomainKey] != "" {
		logger.Infof("OSD failure Domains : %q", allFailureDomains)
		logger.Infof("Draining Failure Domain: %q", pdbStateMap.Data[drainingFailureDomainKey])
		logger.Infof("Set noout on draining Failure Domain: %q", pdbStateMap.Data[setNoOut])
		// delete default OSD pdb and create blocking OSD pdbs
		err := r.handleActiveDrains(allFailureDomains, pdbStateMap.Data[drainingFailureDomainKey], failureDomainType, clusterInfo.Namespace)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to handle active drains")
		}
	} else if pdbStateMap.Data[drainingFailureDomainKey] == "" {
		logger.Infof("`maxUnavailable` for the main OSD PDB is set to 1")
		// delete all blocking OSD pdb and restore the default OSD pdb
		err = r.handleInactiveDrains(allFailureDomains, failureDomainType, clusterInfo.Namespace, excludeOSDs)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to handle inactive drains")
		}
	}

	err = r.updateNoout(clusterInfo, pdbStateMap, allFailureDomains)
	if err != nil {
		logger.Errorf("failed to update maintenance noout in cluster %q. %v", request, err)
	}

	// update PDB configmap
	err = r.client.Update(clusterInfo.Context, pdbStateMap)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to update configMap %q in cluster %q", pdbStateMapName, request)
	}

	return r.requeuePDBController(request)
}

func (r *ReconcileClusterDisruption) handleActiveDrains(allFailureDomains []string, drainingFailureDomain,
	failureDomainType, namespace string,
) error {
	for _, failureDomainName := range allFailureDomains {
		// create blocking PDB for failure domains not currently draining
		if failureDomainName != drainingFailureDomain {
			err := r.createBlockingPDBForOSD(namespace, failureDomainType, failureDomainName)
			if err != nil {
				return errors.Wrapf(err, "failed to create blocking pdb for %q failure domain %q", failureDomainType, failureDomainName)
			}
		} else {
			err := r.deleteBlockingPDBForOSD(namespace, failureDomainType, failureDomainName)
			if err != nil {
				return errors.Wrapf(err, "failed to delete blocking pdb for %q failure domain %q. %v", failureDomainType, failureDomainName, err)
			}
		}
	}

	// delete the default PDB for OSD
	// This will allow all OSDs in the currently drained failure domain to be removed.
	logger.Debug("deleting default pdb with maxUnavailable=1 for all osd")
	err := r.deleteDefaultPDBforOSD(namespace)
	if err != nil {
		return errors.Wrap(err, "failed to delete the default osd pdb")
	}
	return nil
}

func (r *ReconcileClusterDisruption) handleInactiveDrains(allFailureDomains []string, failureDomainType, namespace string, excludeOSDs []int) error {
	err := r.createDefaultPDBforOSD(namespace, excludeOSDs)
	if err != nil {
		return errors.Wrap(err, "failed to create default pdb")
	}
	for _, failureDomainName := range allFailureDomains {
		err := r.deleteBlockingPDBForOSD(namespace, failureDomainType, failureDomainName)
		if err != nil {
			return errors.Wrapf(err, "failed to delete pdb for %q failure domain %q. %v", failureDomainType, failureDomainName, err)
		}
		logger.Debugf("deleted temporary blocking pdb for %q failure domain %q.", failureDomainType, failureDomainName)
	}
	return nil
}

func (r *ReconcileClusterDisruption) updateNoout(clusterInfo *cephclient.ClusterInfo, pdbStateMap *corev1.ConfigMap, allFailureDomains []string) error {
	osdDump, err := cephclient.GetOSDDump(r.context.ClusterdContext, clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to get osddump for reconciling maintenance noout in namespace %s", clusterInfo.Namespace)
	}
	for _, failureDomainName := range allFailureDomains {
		drainingFailureDomainTimeStampKey := fmt.Sprintf("%s-noout-last-set-at", failureDomainName)
		if pdbStateMap.Data[drainingFailureDomainKey] == failureDomainName {
			if pdbStateMap.Data[setNoOut] == "true" {
				// get the time stamp
				nooutSetTimeString, ok := pdbStateMap.Data[drainingFailureDomainTimeStampKey]
				if !ok || len(nooutSetTimeString) == 0 {
					// initialize it if it's not set
					pdbStateMap.Data[drainingFailureDomainTimeStampKey] = time.Now().Format(time.RFC3339)
				}
				// parse the timestamp
				nooutSetTime, err := time.Parse(time.RFC3339, pdbStateMap.Data[drainingFailureDomainTimeStampKey])
				if err != nil {
					return errors.Wrapf(err, "failed to parse timestamp %s for failureDomain %s", pdbStateMap.Data[drainingFailureDomainTimeStampKey], nooutSetTime)
				}
				if time.Since(nooutSetTime) >= r.maintenanceTimeout {
					// noout expired
					if _, err := osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, clusterInfo, false, failureDomainName, nooutFlag); err != nil {
						return errors.Wrapf(err, "failed to update flag on crush unit when noout expired.")
					}
				} else {
					// set noout
					if _, err := osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, clusterInfo, true, failureDomainName, nooutFlag); err != nil {
						return errors.Wrapf(err, "failed to update flag on crush unit while setting noout.")
					}
				}
			} else {
				if _, err := osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, clusterInfo, false, failureDomainName, nooutFlag); err != nil {
					return errors.Wrapf(err, "failed to update flag on crush unit when ensuring noout is unset.")
				}
				// delete the timestamp
				delete(pdbStateMap.Data, drainingFailureDomainTimeStampKey)
			}
		} else {
			// ensure noout unset
			if _, err := osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, clusterInfo, false, failureDomainName, nooutFlag); err != nil {
				return errors.Wrapf(err, "failed to update flag on crush unit when ensuring noout is unset.")
			}
			// delete the timestamp
			delete(pdbStateMap.Data, drainingFailureDomainTimeStampKey)
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

	for _, deployment := range osdDeploymentList.Items {
		labels := deployment.GetLabels()
		failureDomainName := labels[topologyLocationLabel]
		if failureDomainName == "" {
			return nil, nil, nil, nil, errors.Errorf("failed to get the topology location label %q in OSD deployment %q",
				topologyLocationLabel, deployment.Name)
		}

		// Assume node drain if osd deployment ReadyReplicas count is 0 and OSD pod is not scheduled on a node
		if deployment.Status.ReadyReplicas < 1 {
			if !osdDownFailureDomains.Has(failureDomainName) {
				osdDownFailureDomains.Insert(failureDomainName)
			}

			osdID, err := osd.GetOSDID(&deployment)
			if err != nil {
				return nil, nil, nil, nil, errors.Wrapf(err, "failed to get ID for the OSD deployment %q", deployment.Name)
			}
			downOSDs = append(downOSDs, osdID)

			// check if OSD is down on unscheduleable node
			var osdNodeName string
			for _, metadata := range *osdMetadata {
				if metadata.Id == osdID {
					osdNodeName = metadata.HostName
				}
			}
			if osdNodeName != "" {
				isDrained, err := hasOSDNodeDrained(clusterInfo.Context, r.client, osdNodeName)
				if err != nil {
					return nil, nil, nil, nil, errors.Wrapf(err, "failed to check if osd %q node is drained", deployment.Name)
				}
				if isDrained {
					logger.Infof("osd %q is down on node %q and a possible node drain is detected", deployment.Name, osdNodeName)
					if !nodeDrainFailureDomains.Has(failureDomainName) {
						nodeDrainFailureDomains.Insert(failureDomainName)
					}
				} else {
					if !strings.HasSuffix(deployment.Name, "-debug") {
						logger.Infof("osd %q is down on node %q but no node drain is detected", deployment.Name, osdNodeName)
					}
				}
			} else {
				logger.Warningf("failed to get the node name for the OSD %d", osdID)
				continue
			}

		}

		if !allFailureDomains.Has(failureDomainName) {
			allFailureDomains.Insert(failureDomainName)
		}
	}
	return sets.List(allFailureDomains), sets.List(nodeDrainFailureDomains), sets.List(osdDownFailureDomains), downOSDs, nil
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

func resetPDBConfig(pdbStateMap *corev1.ConfigMap) {
	pdbStateMap.Data[drainingFailureDomainKey] = ""
	delete(pdbStateMap.Data, drainingFailureDomainDurationKey)
	// reset `set-no-out` flag on the configMap
	pdbStateMap.Data[setNoOut] = ""
}

// setPDBConfig updates the OSD PDB config map. If there are unschedulable nodes (that is, a node drain event)
// then those failureDomains are given higher precedence than the failureDomains where OSDs might be down
// due to some reason but node is schedulable. `Noout` is set only if nodes are unscheduleable.
func setPDBConfig(pdbStateMap *corev1.ConfigMap, osdDownFailureDomains, nodeDrainFailureDomains []string) {
	if len(pdbStateMap.Data[drainingFailureDomainKey]) == 0 {
		if len(nodeDrainFailureDomains) > 0 {
			pdbStateMap.Data[drainingFailureDomainKey] = nodeDrainFailureDomains[0]
			pdbStateMap.Data[setNoOut] = "true"
		} else if len(osdDownFailureDomains) > 0 {
			pdbStateMap.Data[drainingFailureDomainKey] = osdDownFailureDomains[0]
			pdbStateMap.Data[setNoOut] = ""
		}
		pdbStateMap.Data[drainingFailureDomainDurationKey] = time.Now().Format(time.RFC3339)
	}
}

// requeuePDBController returns requeue request with timeout if:
// - allowedDisruption in main PDB is 0, that is, One or more OSD went down.
// - MaxUnavailable in the main PDB is > 1, that is, OSDs are down but PGs might be clean.
// - default OSD PDB is not available.
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

func getLastNodeDrainTimeStamp(pdbStateMap *corev1.ConfigMap, key string) (time.Time, error) {
	var err error
	var lastDrainTimeStamp time.Time
	lastDrainTimeStampString, ok := pdbStateMap.Data[key]
	if !ok || len(lastDrainTimeStampString) == 0 {
		currentTimeStamp := time.Now()
		pdbStateMap.Data[key] = currentTimeStamp.Format(time.RFC3339)
		return currentTimeStamp, nil
	} else {
		lastDrainTimeStamp, err = time.Parse(time.RFC3339, pdbStateMap.Data[key])
		if err != nil {
			return time.Time{}, errors.Wrapf(err, "failed to parse timestamp %q", pdbStateMap.Data[key])
		}
	}
	return lastDrainTimeStamp, nil
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
