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

	"github.com/rook/rook/pkg/operator/ceph/disruption/nodedrain"

	cephClient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	// cephClient "github.com/rook/rook/pkg/daemon/ceph/client"
)

const (
	// PDBAppName is that app label value for pdbs targeting osds
	PDBAppName     = "rook-ceph-osd-pdb"
	disabledPDBKey = "disabled-pdb"
	// DefaultMaintenanceTimeout is the period for which a drained failure domain will remain in noout
	DefaultMaintenanceTimeout = 30 * time.Minute
	nooutFlag                 = "noout"
)

func (r *ReconcileClusterDisruption) createPDBForOSD(deployment appsv1.Deployment) error {
	deploymentLabels := deployment.ObjectMeta.GetLabels()
	deploymentName := deployment.ObjectMeta.Name
	namespace := deployment.ObjectMeta.Namespace
	osdIDLabel, ok := deploymentLabels[osd.OsdIdLabelKey]
	if !ok {
		return fmt.Errorf("could not find id label on osd %s/%s", namespace, deploymentName)
	}
	cephCluster, ok := r.clusterMap.GetCluster(namespace)
	if !ok {
		return fmt.Errorf("the namespace %s was not found in the clustermap", namespace)
	}

	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				k8sutil.AppAttr:   PDBAppName,
				osd.OsdIdLabelKey: osdIDLabel,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: cephCluster.APIVersion,
					Kind:       cephCluster.Kind,
					Name:       cephCluster.ObjectMeta.GetName(),
					UID:        cephCluster.GetUID(),
				},
			},
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{IntVal: 0},
			Selector:       deployment.Spec.Selector,
		},
	}

	err := r.client.Create(context.TODO(), pdb)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("could not create pdb for osd: %s in namespace %s: %+v", osdIDLabel, namespace, err)
	}
	return nil
}

func (r *ReconcileClusterDisruption) deletePDB(deployment appsv1.Deployment) error {
	deploymentLabels := deployment.ObjectMeta.GetLabels()
	deploymentName := deployment.ObjectMeta.Name
	namespace := deployment.ObjectMeta.Namespace

	osdIDLabel, ok := deploymentLabels[osd.OsdIdLabelKey]
	if !ok {
		return fmt.Errorf("could not find id label on osd %s/%s", namespace, deploymentName)
	}
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
	}
	err := r.client.Delete(context.TODO(), pdb)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("could not delete pdb for osd: %s in namespace %s: %+v", osdIDLabel, namespace, err)
	}
	return nil
}

func (r *ReconcileClusterDisruption) initializePDBState(request reconcile.Request, osdDataList []OsdData) (*corev1.ConfigMap, error) {
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

	if errors.IsNotFound(err) {
		// create configmap and PDBs for all nodes labeled by failuredomain
		logger.Infof("inititalizing pod disruption budgets for osds")
		// one pdb is created per OSD, but after initialization they are created/deleted in failuredomain groups
		for _, osdData := range osdDataList {
			err := r.createPDBForOSD(osdData.Deployment)
			if err != nil {
				return pdbStateMap, fmt.Errorf("failed to create pdb for osd deployment %s. %+v", osdData.Deployment.ObjectMeta.GetName(), err)
			}
		}
		pdbStateMap.Data = map[string]string{disabledPDBKey: ""}
		// create configmap
		err := r.client.Create(context.TODO(), pdbStateMap)
		if err != nil {
			return pdbStateMap, fmt.Errorf("could not create the PDB state map %s, %+v", pdbStateMapRequest, err)
		}
	} else if err != nil {
		return pdbStateMap, fmt.Errorf("could not get the pdbStateMap %s", pdbStateMapRequest)
	}
	return pdbStateMap, nil
}

func (r *ReconcileClusterDisruption) reconcilePDBsForOSDs(
	request reconcile.Request,
	pdbStateMap *corev1.ConfigMap,
	poolFailureDomain string,
	allFailureDomainsMap,
	drainingFailureDomainsMap map[string][]OsdData,
) error {
	drainingFailureDomains := getSortedOSDMapKeys(drainingFailureDomainsMap)

	pgHealthMsg, clean, err := cephClient.IsClusterClean(r.context.ClusterdContext, request.Namespace)
	if err != nil {
		// If the error contains that message, this means the cluster is not up and running
		// No monitors are present and thus no ceph configuration has been created
		if strings.Contains(err.Error(), "error calling conf_read_file") {
			logger.Debugf("Ceph %q cluster not ready, cannot check Ceph status yet.", request.Namespace)
			return nil
		}
		return fmt.Errorf("could not check cluster health: %+v", err)
	}
	_, ok := pdbStateMap.Data[disabledPDBKey]
	if !ok {
		pdbStateMap.Data[disabledPDBKey] = ""
	}
	if len(drainingFailureDomains) != 0 {
		logger.Infof("pg health: %s. detected drains on %ss: %v", pgHealthMsg, poolFailureDomain, drainingFailureDomains)
		// change only when clean
		if clean {
			pdbStateMap.Data[disabledPDBKey] = drainingFailureDomains[0]
		}
	} else if clean {
		pdbStateMap.Data[disabledPDBKey] = ""
	}

	err = r.updateNoout(pdbStateMap, allFailureDomainsMap)
	if err != nil {
		logger.Errorf("could not update maintenance noout in cluster %s with ceph image : %+v", request, err)
	}

	err = r.client.Update(context.TODO(), pdbStateMap)
	if err != nil {
		return fmt.Errorf("could not update %s in cluster %s: %+v", pdbStateMapName, request, err)
	}
	drainingFailureDomain, ok := pdbStateMap.Data[disabledPDBKey]
	if ok && clean && len(drainingFailureDomain) > 0 {

		canaryLabels := client.MatchingLabels{k8sutil.AppAttr: nodedrain.CanaryAppName, poolFailureDomain: drainingFailureDomain}

		// list and delete only if it's old
		drainingCanaryList := &appsv1.DeploymentList{}
		err := r.client.List(context.TODO(), drainingCanaryList, canaryLabels, client.InNamespace(r.context.OperatorNamespace))
		if err != nil {
			return fmt.Errorf("could not list canary pods by labels %q: %+v", canaryLabels, err)
		}
		// refresh old canaries in draining failure domain
		for _, drainingCanary := range drainingCanaryList.Items {
			if time.Since(drainingCanary.GetCreationTimestamp().Time) > time.Minute && drainingCanary.Status.ReadyReplicas < 1 {
				err := r.client.Delete(context.TODO(), &drainingCanary)
				if err != nil {
					logger.Warningf("could not delete canary deployment %q in namespace %q: %+v", drainingCanary.GetName(), drainingCanary.GetNamespace(), err)
				}
			}
		}
	}
	for failureDomain, osdDataList := range allFailureDomainsMap {
		for _, osdData := range osdDataList {
			var err error
			if failureDomain == pdbStateMap.Data[disabledPDBKey] {
				err = r.deletePDB(osdData.Deployment)
			} else {
				err = r.createPDBForOSD(osdData.Deployment)
			}
			if err != nil {
				return fmt.Errorf("failed to reconcile pdb for osd deployment %s. %+v", osdData.Deployment.ObjectMeta.GetName(), err)
			}
		}
	}

	return nil
}

func (r *ReconcileClusterDisruption) updateNoout(pdbStateMap *corev1.ConfigMap, allFailureDomainsMap map[string][]OsdData) error {
	disabledFailureDomain := pdbStateMap.Data[disabledPDBKey]
	namespace := pdbStateMap.ObjectMeta.Namespace
	osdDump, err := cephClient.GetOSDDump(r.context.ClusterdContext, namespace)
	if err != nil {
		return fmt.Errorf("could not get osddump for reconciling maintenance noout in namespace %s: %+v", namespace, err)
	}
	for failureDomain := range allFailureDomainsMap {
		disabledFailureDomainTimeStampKey := fmt.Sprintf("%s-noout-last-set-at", failureDomain)
		if disabledFailureDomain == failureDomain {

			// get the time stamp
			nooutSetTimeString, ok := pdbStateMap.Data[disabledFailureDomainTimeStampKey]
			if !ok || len(nooutSetTimeString) == 0 {
				// initialize it if it's not set
				pdbStateMap.Data[disabledFailureDomainTimeStampKey] = time.Now().Format(time.RFC3339)
			}
			// parse the timestamp
			nooutSetTime, err := time.Parse(time.RFC3339, pdbStateMap.Data[disabledFailureDomainTimeStampKey])
			if err != nil {
				return fmt.Errorf("could not parse timestamp %s for failureDomain %s", pdbStateMap.Data[disabledFailureDomainTimeStampKey], nooutSetTime)
			}
			if time.Since(nooutSetTime) >= r.maintenanceTimeout {
				// noout expired
				osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, false, namespace, failureDomain, nooutFlag)
			} else {
				// set noout
				osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, true, namespace, failureDomain, nooutFlag)
			}

		} else {
			// ensure noout unset
			osdDump.UpdateFlagOnCrushUnit(r.context.ClusterdContext, false, namespace, failureDomain, nooutFlag)
			// delete the timestamp
			delete(pdbStateMap.Data, disabledFailureDomainTimeStampKey)
		}
	}
	return nil
}
