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
	cephClient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	// cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
)

const (
	// osdPDBAppName is that app label value for pdbs targeting osds
	osdPDBAppName                    = "rook-ceph-osd"
	drainingFailureDomainKey         = "draining-failure-domain"
	drainingFailureDomainDurationKey = "draining-failure-domain-duration"
	pgHealthCheckDurationKey         = "pg-health-check-duration"
	// DefaultMaintenanceTimeout is the period for which a drained failure domain will remain in noout
	DefaultMaintenanceTimeout = 30 * time.Minute
	nooutFlag                 = "noout"
)

func (r *ReconcileClusterDisruption) createPDB(pdb *policyv1beta1.PodDisruptionBudget) error {
	err := r.client.Create(context.TODO(), pdb)
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create pdb %q", pdb.Name)
	}
	return nil
}

func (r *ReconcileClusterDisruption) deletePDB(pdb *policyv1beta1.PodDisruptionBudget) error {
	err := r.client.Delete(context.TODO(), pdb)
	if err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete pdb %q", pdb.Name)
	}
	return nil
}

// createOverallPDBforOSD creates a single PDB for all OSDs with maxUnavailable=1
// This allows all OSDs in a single failure domain to go down.
func (r *ReconcileClusterDisruption) createOverallPDBforOSD(namespace string) error {
	cephCluster, ok := r.clusterMap.GetCluster(namespace)
	if !ok {
		return errors.Errorf("failed to find the namespace %q in the clustermap", namespace)
	}
	pdbRequest := types.NamespacedName{Name: osdPDBAppName, Namespace: namespace}
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      osdPDBAppName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				opcontroller.ClusterOwnerRef(cephCluster.GetName(), string(cephCluster.GetUID())),
			},
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 1},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{k8sutil.AppAttr: osdPDBAppName},
			},
		},
	}

	err := r.client.Get(context.TODO(), pdbRequest, &policyv1beta1.PodDisruptionBudget{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("all osds are up. pg health is active+clean")
			logger.Infof("creating the main pdb %q with maxUnavailable=1 for all osd", osdPDBAppName)
			return r.createPDB(pdb)
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
	}
	return nil
}

func (r *ReconcileClusterDisruption) deleteOverallPDBforOSD(namespace string) error {
	pdbRequest := types.NamespacedName{Name: osdPDBAppName, Namespace: namespace}
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      osdPDBAppName,
			Namespace: namespace,
		},
	}
	err := r.client.Get(context.TODO(), pdbRequest, &policyv1beta1.PodDisruptionBudget{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
	}
	logger.Infof("deleting the main pdb %q with maxUnavailable=1 for all osd", osdPDBAppName)
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
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				opcontroller.ClusterOwnerRef(cephCluster.GetName(), string(cephCluster.GetUID())),
			},
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 0},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{fmt.Sprintf(osd.TopologyLocationLabel, failureDomainType): failureDomainName},
			},
		},
	}
	err := r.client.Get(context.TODO(), pdbRequest, &policyv1beta1.PodDisruptionBudget{})
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
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: namespace,
		},
	}
	err := r.client.Get(context.TODO(), pdbRequest, &policyv1beta1.PodDisruptionBudget{})
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
	err := r.client.Get(context.TODO(), pdbStateMapRequest, pdbStateMap)

	if kerrors.IsNotFound(err) {
		// create configmap to track the draining failure domain
		pdbStateMap.Data = map[string]string{drainingFailureDomainKey: ""}
		err := r.client.Create(context.TODO(), pdbStateMap)
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
	drainingFailureDomains []string,
) error {

	pgHealthMsg, clean, err := cephclient.IsClusterClean(r.context.ClusterdContext, clusterInfo)
	if err != nil {
		// If the error contains that message, this means the cluster is not up and running
		// No monitors are present and thus no ceph configuration has been created
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Debugf("Ceph %q cluster not ready, cannot check Ceph status yet.", request.Namespace)
			return nil
		}
		return errors.Wrapf(err, "failed to check cluster health")
	}

	if pdbStateMap.Data == nil {
		pdbStateMap.Data = make(map[string]string)
	}
	_, ok := pdbStateMap.Data[drainingFailureDomainKey]
	if !ok {
		pdbStateMap.Data[drainingFailureDomainKey] = ""
	}

	var drainingFailureDomain string
	activeDrains := len(drainingFailureDomains) > 0

	// create blocking PDBs for all the OSDs that are not being drained right now
	if activeDrains {
		drainingFailureDomain = drainingFailureDomains[0]
		logger.Infof("all %q failure domains: %v. currently draining: %q. pg health: %q", failureDomainType,
			allFailureDomains, drainingFailureDomain, pgHealthMsg)
		err := r.handleActiveDrains(allFailureDomains, drainingFailureDomain, failureDomainType, clusterInfo.Namespace, clean)
		if err != nil {
			return errors.Wrap(err, "failed to handle active drains")
		}
		currentlyDrainingFD, ok := pdbStateMap.Data[drainingFailureDomainKey]
		if !ok || drainingFailureDomain != currentlyDrainingFD {
			pdbStateMap.Data[drainingFailureDomainKey] = drainingFailureDomain
			pdbStateMap.Data[drainingFailureDomainDurationKey] = time.Now().Format(time.RFC3339)
		}
	} else if clean || r.hasPGHealthCheckTimedout(pdbStateMap) {
		// reset the configMap if cluster is clean or if the timeout for PGs to become active+clean has exceeded
		logger.Debugf("no drains detected in %q failure domains: %v. pg health: %q", failureDomainType, allFailureDomains, pgHealthMsg)
		pdbStateMap.Data[drainingFailureDomainKey] = ""
		delete(pdbStateMap.Data, drainingFailureDomainDurationKey)
		delete(pdbStateMap.Data, pgHealthCheckDurationKey)
	} else {
		logger.Infof("all osds are up. last drained failure domain: %q.  pg health: %q. waiting for PGs to be active+clean or PG health check to timeout",
			pdbStateMap.Data[drainingFailureDomainKey], pgHealthMsg)
	}

	err = r.updateNoout(clusterInfo, pdbStateMap, allFailureDomains)
	if err != nil {
		logger.Errorf("failed to update maintenance noout in cluster %q. %v", request, err)
	}

	err = r.client.Update(context.TODO(), pdbStateMap)
	if err != nil {
		return errors.Wrapf(err, "failed to update configMap %q in cluster %q", pdbStateMapName, request)
	}

	// handle inactive drains by deleting all blocking pdb and restoring the main pdb
	if pdbStateMap.Data[drainingFailureDomainKey] == "" {
		err := r.handleInactiveDrains(allFailureDomains, clusterInfo.Namespace, failureDomainType)
		if err != nil {
			return errors.Wrap(err, "failed to handle inactive drains")
		}
	}
	return nil
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

	// delete the main PDB for OSD
	// This will allow all OSDs in the currently drained failure domain to be removed.
	logger.Debug("deleting main pdb with maxUnavailable=1 for all osd")
	err := r.deleteOverallPDBforOSD(namespace)
	if err != nil {
		return errors.Wrap(err, "failed to delete the main osd pdb")
	}
	return nil
}

func (r *ReconcileClusterDisruption) handleInactiveDrains(allFailureDomains []string, namespace, failureDomainType string) error {
	err := r.createOverallPDBforOSD(namespace)
	if err != nil {
		return errors.Wrap(err, "failed to create main pdb")
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

func (r *ReconcileClusterDisruption) getOSDFailureDomains(clusterInfo *cephClient.ClusterInfo, request reconcile.Request, poolFailureDomain string) ([]string, []string, error) {
	osdDeploymentList := &appsv1.DeploymentList{}
	namespaceListOpts := client.InNamespace(request.Namespace)
	topologyLocationLabel := fmt.Sprintf(osd.TopologyLocationLabel, poolFailureDomain)
	err := r.client.List(context.TODO(), osdDeploymentList, client.MatchingLabels{k8sutil.AppAttr: osd.AppName}, namespaceListOpts)
	if err != nil {
		return []string{}, []string{}, errors.Wrap(err, "failed to list osd deployments")
	}

	allFailureDomains := sets.NewString()
	drainingFailureDomains := sets.NewString()

	for _, deployment := range osdDeploymentList.Items {
		labels := deployment.Spec.Template.ObjectMeta.GetLabels()
		failureDomainName := labels[topologyLocationLabel]
		if failureDomainName == "" {
			return []string{}, []string{}, errors.Errorf("failed to get the topology location label %q in OSD deployment %q",
				topologyLocationLabel, deployment.Name)
		}

		if deployment.Status.ReadyReplicas < 1 {
			if !drainingFailureDomains.Has(failureDomainName) {
				drainingFailureDomains.Insert(failureDomainName)
			}
		} else {
			if !allFailureDomains.Has(failureDomainName) {
				allFailureDomains.Insert(failureDomainName)
			}
		}
	}
	return allFailureDomains.List(), drainingFailureDomains.List(), nil
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

func getPDBName(failureDomainType, failureDomainName string) string {
	return k8sutil.TruncateNodeName(fmt.Sprintf("%s-%s-%s", osdPDBAppName, failureDomainType, "%s"), failureDomainName)
}
