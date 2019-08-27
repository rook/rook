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
	"time"

	"github.com/coreos/pkg/capnslog"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rook/rook/pkg/operator/ceph/controllers/controllerconfig"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	osdDisruptionAppName = "rook-ceph-osd-disruption"
	controllerName       = "clusterdisruption-controller"
	// pdbStateMapName for the clusterdisruption pdb state map
	pdbStateMapName = "rook-ceph-pdbstatemap"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

	// Implement reconcile.Reconciler so the controller can reconcile objects
	_ reconcile.Reconciler = &ReconcileClusterDisruption{}
)

// ReconcileClusterDisruption reconciles ReplicaSets
type ReconcileClusterDisruption struct {
	// client can be used to retrieve objects from the APIServer.
	scheme              *runtime.Scheme
	client              client.Client
	options             *controllerconfig.Options
	clusterMap          *ClusterMap
	osdCrushLocationMap *OSDCrushLocationMap
}

// Reconcile reconciles a node and ensures that it has a drain-detection deployment
// attached to it.
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileClusterDisruption) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// wrapping reconcile because the rook logging mechanism is not compatible with the controller-runtime logging interface
	result, err := r.reconcile(request)
	if err != nil {
		logger.Error(err)
	}
	return result, err
}

func (r *ReconcileClusterDisruption) reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger.Infof("reconciling %v", request.NamespacedName)
	if len(request.Namespace) == 0 {
		return reconcile.Result{}, fmt.Errorf("request did not have namespace: %s", request.NamespacedName)
	}

	// ensure that the cluster name is populated
	if len(request.Name) == 0 {
		clusterName, found := r.clusterMap.GetClusterName(request.Namespace)
		if !found {
			logger.Infof("requeueing because clusterName is not known yet for namespace %s", request.Namespace)
			return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
		}
		request.Name = clusterName
		logger.Debugf("discovered NamespacedName: %s", request.NamespacedName)
	} else {
		// update the clustermap with the cluster's name so that
		// events on resources associated with the cluster can trigger reconciliation by namespace
		r.clusterMap.UpdateClusterMap(request.Namespace, request.Name)
	}

	// get the ceph cluster
	logger.Debugf("getting the cephcluster %s", request.NamespacedName)
	cephCluster := &cephv1.CephCluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephCluster)
	if err != nil {
		return emptyResultAndErrorf("could not get the ceph cluster %s: %+v", request.NamespacedName, err)
	}
	if !cephCluster.Spec.ManagedDisruptionBudgets {
		// feature disabled for this cluster. not requeueing
		return reconcile.Result{Requeue: false}, nil
	}

	//  determine failure domain
	logger.Debugf("getting the failure domain")
	poolFailureDomain, poolCount, err := r.getFailureDomain(request)
	if err != nil {
		return reconcile.Result{}, err
	}
	// no pools, no need to reconcile
	if poolCount < 1 {
		return reconcile.Result{}, nil
	}

	// get the osds with crush data populated
	osdDataList, err := r.getOsdDataList(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	// get the list of nodes with ongoing drains
	drainingNodes, err := r.getOngoingDrains(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	drainingOSDs, err := r.getOSDsForNodes(osdDataList, drainingNodes)
	if err != nil {
		return reconcile.Result{}, err
	}

	allFailureDomainsMap, err := getFailureDomainMapForOsds(osdDataList, poolFailureDomain)
	if err != nil {
		logger.Error(err)
	}
	drainingFailureDomainsMap, err := getFailureDomainMapForOsds(drainingOSDs, poolFailureDomain)
	if err != nil {
		logger.Error(err)
	}

	// get the map that stores which PDBs are intentionally down
	pdbStateMap, err := r.initializePDBState(request, osdDataList)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.reconcilePDB(request, pdbStateMap, poolFailureDomain, allFailureDomainsMap, drainingFailureDomainsMap)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: time.Minute}, nil
}

func emptyResultAndErrorf(format string, a ...interface{}) (reconcile.Result, error) {
	return reconcile.Result{}, fmt.Errorf(format, a...)
}
