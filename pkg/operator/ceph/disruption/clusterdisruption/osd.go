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
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// osdPDBAppName is that app label value for pdbs targeting osds
	osdPDBAppName                    = "rook-ceph-osd"
	drainingFailureDomainKey         = "draining-failure-domain"
	drainingFailureDomainDurationKey = "draining-failure-domain-duration"
	setNoOut                         = "set-no-out"
	pgHealthCheckDurationKey         = "pg-health-check-duration"
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
func (r *ReconcileClusterDisruption) createDefaultPDBforOSD(namespace string) error {
	cephCluster, ok := r.clusterMap.GetCluster(namespace)
	if !ok {
		return errors.Errorf("failed to find the namespace %q in the clustermap", namespace)
	}
	pdbRequest := types.NamespacedName{Name: osdPDBAppName, Namespace: namespace}
	objectMeta := metav1.ObjectMeta{
		Name:      osdPDBAppName,
		Namespace: namespace,
	}
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{k8sutil.AppAttr: osdPDBAppName},
	}
	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(r.context.ClusterdContext.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to fetch pdb version")
	}
	if usePDBV1Beta1 {
		pdb := &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				MaxUnavailable: &intstr.IntOrString{IntVal: 1},
				Selector:       selector,
			},
		}
		ownerInfo := k8sutil.NewOwnerInfo(cephCluster, r.scheme)
		err := ownerInfo.SetControllerReference(pdb)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to pdb %v", pdb)
		}

		err = r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1beta1.PodDisruptionBudget{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("all PGs are active+clean. Restoring default OSD pdb settings")
				logger.Infof("creating the default pdb %q with maxUnavailable=1 for all osd", osdPDBAppName)
				return r.createPDB(pdb)
			}
			return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
		}
		return nil
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 1},
			Selector:       selector,
		},
	}
	ownerInfo := k8sutil.NewOwnerInfo(cephCluster, r.scheme)
	err = ownerInfo.SetControllerReference(pdb)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to pdb %v", pdb)
	}

	err = r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1.PodDisruptionBudget{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("all PGs are active+clean. Restoring default OSD pdb settings")
			logger.Infof("creating the default pdb %q with maxUnavailable=1 for all osd", osdPDBAppName)
			return r.createPDB(pdb)
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
	}
	return nil
}

func (r *ReconcileClusterDisruption) deleteDefaultPDBforOSD(namespace string) error {
	pdbRequest := types.NamespacedName{Name: osdPDBAppName, Namespace: namespace}
	objectMeta := metav1.ObjectMeta{
		Name:      osdPDBAppName,
		Namespace: namespace,
	}
	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(r.context.ClusterdContext.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to fetch pdb version")
	}
	if usePDBV1Beta1 {
		pdb := &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
		}
		err := r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1beta1.PodDisruptionBudget{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
		}
		logger.Infof("deleting the default pdb %q with maxUnavailable=1 for all osd", osdPDBAppName)
		return r.deletePDB(pdb)
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
	}
	err = r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1.PodDisruptionBudget{})
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
	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(r.context.ClusterdContext.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to fetch pdb version")
	}
	if usePDBV1Beta1 {
		pdb := &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				MaxUnavailable: &intstr.IntOrString{IntVal: 0},
				Selector:       selector,
			},
		}
		ownerInfo := k8sutil.NewOwnerInfo(cephCluster, r.scheme)
		err := ownerInfo.SetControllerReference(pdb)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to pdb %v", pdb)
		}
		err = r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1beta1.PodDisruptionBudget{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Infof("creating temporary blocking pdb %q with maxUnavailable=0 for %q failure domain %q", pdbName, failureDomainType, failureDomainName)
				return r.createPDB(pdb)
			}
			return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
		}
		return nil
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 0},
			Selector:       selector,
		},
	}
	ownerInfo := k8sutil.NewOwnerInfo(cephCluster, r.scheme)
	err = ownerInfo.SetControllerReference(pdb)
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
	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(r.context.ClusterdContext.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to fetch pdb version")
	}
	if usePDBV1Beta1 {
		pdb := &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
		}
		err := r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1beta1.PodDisruptionBudget{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
		}
		logger.Infof("deleting temporary blocking pdb with %q with maxUnavailable=0 for %q failure domain %q", pdbName, failureDomainType, failureDomainName)
		return r.deletePDB(pdb)
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
	}
	err = r.client.Get(r.context.OpManagerContext, pdbRequest, &policyv1.PodDisruptionBudget{})
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
	osdDownFailureDomains []string,
	activeNodeDrains bool,
	pgHealhtyRegex string,
) (reconcile.Result, error) {
	var osdDown bool
	var drainingFailureDomain string
	if len(osdDownFailureDomains) > 0 {
		osdDown = true
		drainingFailureDomain = osdDownFailureDomains[0]
	}

	pgHealthMsg, pgClean, err := cephclient.IsClusterClean(r.context.ClusterdContext, clusterInfo, pgHealhtyRegex)
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

	switch {
	// osd is down but pgs are active+clean
	case osdDown && pgClean:
		lastDrainTimeStamp, err := getLastDrainTimeStamp(pdbStateMap, drainingFailureDomainDurationKey)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to get last drain timestamp from the configmap %q", pdbStateMap.Name)
		}
		timeSinceOSDDown := time.Since(lastDrainTimeStamp)
		if timeSinceOSDDown > 30*time.Second {
			logger.Infof("osd is down in failure domain %q is down for the last %.2f minutes, but pgs are active+clean", drainingFailureDomain, timeSinceOSDDown.Minutes())
			resetPDBConfig(pdbStateMap)
		} else {
			logger.Infof("osd is down in the failure domain %q, but pgs are active+clean. Requeuing in case pg status is not updated yet...", drainingFailureDomain)
			return reconcile.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
		}

	// osd is down and pgs are not healthy
	case osdDown && !pgClean:
		logger.Infof("osd is down in failure domain %q and pgs are not active+clean. pg health: %q", drainingFailureDomain, pgHealthMsg)
		currentlyDrainingFD, ok := pdbStateMap.Data[drainingFailureDomainKey]
		if !ok || drainingFailureDomain != currentlyDrainingFD {
			pdbStateMap.Data[drainingFailureDomainKey] = drainingFailureDomain
			pdbStateMap.Data[drainingFailureDomainDurationKey] = time.Now().Format(time.RFC3339)
		}
		if activeNodeDrains {
			pdbStateMap.Data[setNoOut] = "true"
		}

	// osd is back up and either pgs have become healthy or pg healthy check timeout has elapsed
	case !osdDown && (pgClean || r.hasPGHealthCheckTimedout(pdbStateMap)):
		// reset the configMap if cluster is clean or if the timeout for PGs to become active+clean has exceeded
		logger.Debugf("no OSD is down in the %q failure domains: %v. pg health: %q", failureDomainType, allFailureDomains, pgHealthMsg)
		resetPDBConfig(pdbStateMap)

	default:
		logger.Infof("all %q failure domains: %v. osd is down in failure domain: %q. active node drains: %t. pg health: %q", failureDomainType,
			allFailureDomains, drainingFailureDomain, activeNodeDrains, pgHealthMsg)
	}

	if pdbStateMap.Data[setNoOut] == "true" {
		err = r.updateNoout(clusterInfo, pdbStateMap, allFailureDomains)
		if err != nil {
			logger.Errorf("failed to update maintenance noout in cluster %q. %v", request, err)
		}
	}

	if pdbStateMap.Data[drainingFailureDomainKey] != "" && !pgClean {
		// delete default OSD pdb and create blocking OSD pdbs
		err := r.handleActiveDrains(allFailureDomains, pdbStateMap.Data[drainingFailureDomainKey], failureDomainType, clusterInfo.Namespace, pgClean)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to handle active drains")
		}
	} else if pdbStateMap.Data[drainingFailureDomainKey] == "" {
		// delete all blocking OSD pdb and restore the default OSD pdb
		err := r.handleInactiveDrains(allFailureDomains, failureDomainType, clusterInfo.Namespace)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to handle inactive drains")
		}
		// reset `set-no-out` flag on the configMap
		pdbStateMap.Data[setNoOut] = ""
	}

	err = r.client.Update(clusterInfo.Context, pdbStateMap)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to update configMap %q in cluster %q", pdbStateMapName, request)
	}

	// requeue if drain is still in progress
	if len(pdbStateMap.Data[drainingFailureDomainKey]) > 0 {
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// requeue if allowed disruptions in the default PDB is 0
	allowedDisruptions, err := r.getAllowedDisruptions(osdPDBAppName, request.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debugf("default osd pdb %q not found. Skipping reconcile", osdPDBAppName)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to get allowed disruptions count from default osd pdb %q.", osdPDBAppName)
	}

	if allowedDisruptions == 0 {
		logger.Info("reconciling osd pdb reconciler as the allowed disruptions in default pdb is 0")
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDisruption) handleActiveDrains(allFailureDomains []string, drainingFailureDomain,
	failureDomainType, namespace string, isClean bool) error {

	for _, failureDomainName := range allFailureDomains {
		// create blocking PDB for failure domains not currently draining
		if failureDomainName != drainingFailureDomain {
			err := r.createBlockingPDBForOSD(namespace, failureDomainType, failureDomainName)
			if err != nil {
				return errors.Wrapf(err, "failed to create blocking pdb for %q failure domain %q", failureDomainType, failureDomainName)
			}
		} else {
			if isClean {
				err := r.deleteBlockingPDBForOSD(namespace, failureDomainType, failureDomainName)
				if err != nil {
					return errors.Wrapf(err, "failed to delete pdb for %q failure domain %q. %v", failureDomainType, failureDomainName, err)
				}
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

func (r *ReconcileClusterDisruption) handleInactiveDrains(allFailureDomains []string, failureDomainType, namespace string) error {
	err := r.createDefaultPDBforOSD(namespace)
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
	drainingFailureDomain := pdbStateMap.Data[drainingFailureDomainKey]
	osdDump, err := cephclient.GetOSDDump(r.context.ClusterdContext, clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to get osddump for reconciling maintenance noout in namespace %s", clusterInfo.Namespace)
	}
	for _, failureDomainName := range allFailureDomains {
		drainingFailureDomainTimeStampKey := fmt.Sprintf("%s-noout-last-set-at", failureDomainName)
		if drainingFailureDomain == failureDomainName {

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

func (r *ReconcileClusterDisruption) getOSDFailureDomains(clusterInfo *cephclient.ClusterInfo, request reconcile.Request, poolFailureDomain string) ([]string, []string, []string, error) {
	osdDeploymentList := &appsv1.DeploymentList{}
	namespaceListOpts := client.InNamespace(request.Namespace)
	topologyLocationLabel := fmt.Sprintf(osd.TopologyLocationLabel, poolFailureDomain)
	err := r.client.List(clusterInfo.Context, osdDeploymentList, client.MatchingLabels{k8sutil.AppAttr: osd.AppName}, namespaceListOpts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to list osd deployments")
	}

	allFailureDomains := sets.New[string]()
	nodeDrainFailureDomains := sets.New[string]()
	osdDownFailureDomains := sets.New[string]()

	for _, deployment := range osdDeploymentList.Items {
		labels := deployment.Spec.Template.ObjectMeta.GetLabels()
		failureDomainName := labels[topologyLocationLabel]
		if failureDomainName == "" {
			return nil, nil, nil, errors.Errorf("failed to get the topology location label %q in OSD deployment %q",
				topologyLocationLabel, deployment.Name)
		}

		// Assume node drain if osd deployment ReadyReplicas count is 0 and OSD pod is not scheduled on a node
		if deployment.Status.ReadyReplicas < 1 {
			if !osdDownFailureDomains.Has(failureDomainName) {
				osdDownFailureDomains.Insert(failureDomainName)
			}
			isDrained, err := hasOSDNodeDrained(clusterInfo.Context, r.client, request.Namespace, labels[osd.OsdIdLabelKey])
			if err != nil {
				return nil, nil, nil, errors.Wrapf(err, "failed to check if osd %q node is drained", deployment.Name)
			}
			if isDrained {
				logger.Infof("osd %q is down and a possible node drain is detected", deployment.Name)
				if !nodeDrainFailureDomains.Has(failureDomainName) {
					nodeDrainFailureDomains.Insert(failureDomainName)
				}
			} else {
				if !strings.HasSuffix(deployment.Name, "-debug") {
					logger.Infof("osd %q is down but no node drain is detected", deployment.Name)
				}
			}
		}

		if !allFailureDomains.Has(failureDomainName) {
			allFailureDomains.Insert(failureDomainName)
		}
	}
	return sets.List(allFailureDomains), sets.List(nodeDrainFailureDomains), sets.List(osdDownFailureDomains), nil
}

func (r *ReconcileClusterDisruption) hasPGHealthCheckTimedout(pdbStateMap *corev1.ConfigMap) bool {
	if r.pgHealthCheckTimeout == 0 {
		logger.Debug("pg health check timeout is not set in the cluster. waiting for PGs to get active+clean")
		return false
	}

	timeString, ok := pdbStateMap.Data[pgHealthCheckDurationKey]
	if !ok || len(timeString) == 0 {
		pdbStateMap.Data[pgHealthCheckDurationKey] = time.Now().Format(time.RFC3339)
	} else {
		pgHealthCheckDuration, err := time.Parse(time.RFC3339, timeString)
		if err != nil {
			logger.Errorf("failed to parse timestamp %v. %v", pgHealthCheckDuration, err)
			pdbStateMap.Data[pgHealthCheckDurationKey] = time.Now().Format(time.RFC3339)
			return false
		}
		timeElapsed := time.Since(pgHealthCheckDuration)
		if timeElapsed >= r.pgHealthCheckTimeout {
			logger.Info("timed out waiting for the PGs to become active+clean")
			return true
		}
		timeleft := r.pgHealthCheckTimeout - timeElapsed
		logger.Infof("waiting for %d minute(s) for PGs to become active+clean", int(timeleft.Minutes()))
	}
	return false
}

// hasNodeDrained returns true if OSD pod is not assigned to any node or if the OSD node is not schedulable
func hasOSDNodeDrained(ctx context.Context, c client.Client, namespace, osdID string) (bool, error) {
	osdNodeName, err := getOSDNodeName(ctx, c, namespace, osdID)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get node name assigned to OSD %q POD", osdID)
	}

	if osdNodeName == "" {
		logger.Debugf("osd %q POD is not assigned to any node. assuming node drain", osdID)
		return true, nil
	}

	node, err := getNode(ctx, c, osdNodeName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get node assigned to OSD %q POD", osdID)
	}
	return node.Spec.Unschedulable, nil
}

func getOSDNodeName(ctx context.Context, c client.Client, namespace, osdID string) (string, error) {
	pods := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{osd.OsdIdLabelKey: osdID},
	}

	err := c.List(ctx, pods, listOpts...)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list pods for osd %q", osdID)
	}

	if len(pods.Items) > 0 {
		return pods.Items[0].Spec.NodeName, nil
	}
	return "", nil
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

func getLastDrainTimeStamp(pdbStateMap *corev1.ConfigMap, key string) (time.Time, error) {
	var err error
	var lastDrainTimeStamp time.Time
	lastDrainTimeStampString, ok := pdbStateMap.Data[key]
	if !ok || len(lastDrainTimeStampString) == 0 {
		return time.Now(), nil
	} else {
		lastDrainTimeStamp, err = time.Parse(time.RFC3339, pdbStateMap.Data[key])
		if err != nil {
			return time.Time{}, errors.Wrapf(err, "failed to parse timestamp %q", pdbStateMap.Data[key])
		}
	}

	return lastDrainTimeStamp, nil
}

func (r *ReconcileClusterDisruption) getAllowedDisruptions(pdbName, namespace string) (int32, error) {
	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(r.context.ClusterdContext.Clientset)
	if err != nil {
		return -1, errors.Wrap(err, "failed to fetch pdb version")
	}
	if usePDBV1Beta1 {
		pdb := &policyv1beta1.PodDisruptionBudget{}
		err = r.client.Get(r.context.OpManagerContext, types.NamespacedName{Name: pdbName, Namespace: namespace}, pdb)
		if err != nil {
			return -1, err
		}

		return pdb.Status.DisruptionsAllowed, nil
	}

	pdb := &policyv1.PodDisruptionBudget{}
	err = r.client.Get(r.context.OpManagerContext, types.NamespacedName{Name: pdbName, Namespace: namespace}, pdb)
	if err != nil {
		return -1, err
	}

	return pdb.Status.DisruptionsAllowed, nil
}

func resetPDBConfig(pdbStateMap *corev1.ConfigMap) {
	pdbStateMap.Data[drainingFailureDomainKey] = ""
	delete(pdbStateMap.Data, drainingFailureDomainDurationKey)
	delete(pdbStateMap.Data, pgHealthCheckDurationKey)
}
