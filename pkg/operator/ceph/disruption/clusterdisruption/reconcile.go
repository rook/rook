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
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/coreos/pkg/capnslog"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

const (
	osdDisruptionAppName = "rook-ceph-osd-disruption"
	controllerName       = "clusterdisruption-controller"
	// pdbStateMapName for the clusterdisruption pdb state map
	pdbStateMapName    = "rook-ceph-pdbstatemap"
	maxNamelessRetries = 20
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
	context             *controllerconfig.Context
	clusterMap          *ClusterMap
	osdCrushLocationMap *OSDCrushLocationMap
	maintenanceTimeout  time.Duration
	namelessRetries     int
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
	if len(request.Namespace) == 0 {
		return reconcile.Result{}, errors.Errorf("request did not have namespace: %q", request.NamespacedName)
	}

	// ensure that the cluster name is populated
	if len(request.Name) == 0 {
		clusterName, found := r.clusterMap.GetClusterName(request.Namespace)
		if !found && r.namelessRetries < maxNamelessRetries {
			// ensure that a deleted cluster doesn't result in infinite retries
			r.namelessRetries++
			logger.Infof("clusterName is not known yet for namespace %q", request.Namespace)
			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, errors.New("clusterName for this namespace not yet known")
		}
		request.Name = clusterName
		logger.Debugf("discovered NamespacedName: %q", request.NamespacedName)
	}
	r.namelessRetries = 0
	logger.Debugf("reconciling %q", request.NamespacedName)

	// get the ceph cluster
	cephCluster := &cephv1.CephCluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephCluster)
	if kerrors.IsNotFound(err) {
		logger.Errorf("cephcluster %q seems to be deleted, not requeuing until triggered again", request)
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "could not get the ceph cluster %q", request.NamespacedName)
	}

	// update the clustermap with the cluster's name so that
	// events on resources associated with the cluster can trigger reconciliation by namespace
	r.clusterMap.UpdateClusterMap(request.Namespace, cephCluster)

	if !cephCluster.Spec.DisruptionManagement.ManagePodBudgets {
		// feature disabled for this cluster. not requeueing
		return reconcile.Result{Requeue: false}, nil
	}
	//signal to the nodedrain controller to start
	r.context.ReconcileCanaries.Update(true)
	r.maintenanceTimeout = cephCluster.Spec.DisruptionManagement.OSDMaintenanceTimeout
	if r.maintenanceTimeout == 0 {
		r.maintenanceTimeout = DefaultMaintenanceTimeout
		logger.Debugf("Using default maintenance timeout: %v", r.maintenanceTimeout)
	}

	//  reconcile the pools and get the failure domain
	cephObjectStoreList, cephFilesystemList, poolFailureDomain, poolCount, err := r.processPools(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	// reconcile the static mon PDB
	err = r.reconcileMonPDB(cephCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	// reconcile the pdbs for objectstores
	err = r.reconcileCephObjectStore(cephObjectStoreList)
	if err != nil {
		return reconcile.Result{}, err
	}

	// reconcile the pdbs for filesystems
	err = r.reconcileCephFilesystem(cephFilesystemList)
	if err != nil {
		return reconcile.Result{}, err
	}

	// no pools, no need to reconcile OSD PDB
	if poolCount < 1 {
		return reconcile.Result{}, nil
	}

	// get the osds with crush data populated
	osdDataList, err := r.getOsdDataList(request, poolFailureDomain)
	if err != nil {
		return reconcile.Result{}, err
	}

	// get the list of nodes with ongoing drains
	drainingNodes, err := r.getOngoingDrains(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	drainingOSDs, err := getOSDsForNodes(osdDataList, drainingNodes, poolFailureDomain)
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

	err = r.reconcilePDBsForOSDs(request, pdbStateMap, poolFailureDomain, allFailureDomainsMap, drainingFailureDomainsMap)
	if err != nil {
		return reconcile.Result{}, err
	}
	disabledPDB, ok := pdbStateMap.Data[disabledPDBKey]
	if ok && len(disabledPDB) > 0 {
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}
	return reconcile.Result{}, nil
}

// ClusterMap maintains the association between namespace and clusername
type ClusterMap struct {
	clusterMap map[string]*cephv1.CephCluster
	mux        sync.Mutex
}

// UpdateClusterMap to populate the clusterName for the namespace
func (c *ClusterMap) UpdateClusterMap(namespace string, cluster *cephv1.CephCluster) {
	defer c.mux.Unlock()
	c.mux.Lock()
	if len(c.clusterMap) == 0 {
		c.clusterMap = make(map[string]*cephv1.CephCluster)
	}
	c.clusterMap[namespace] = cluster

}

// GetClusterName returns vars clusterName, found. clusterName is the cluster name associated
// with that namespace and found is the boolean indicating whether a cluster was
// populated for that namespace or not.
func (c *ClusterMap) GetClusterName(namespace string) (string, bool) {
	defer c.mux.Unlock()
	c.mux.Lock()

	if len(c.clusterMap) == 0 {
		c.clusterMap = make(map[string]*cephv1.CephCluster)
	}

	cluster, ok := c.clusterMap[namespace]
	if !ok {
		return "", false
	}

	return cluster.ObjectMeta.GetName(), true
}

// GetCluster returns vars cluster, found. cluster is the cephcluster associated
// with that namespace and found is the boolean indicating whether a cluster was
// populated for that namespace or not.
func (c *ClusterMap) GetCluster(namespace string) (*cephv1.CephCluster, bool) {
	defer c.mux.Unlock()
	c.mux.Lock()

	if len(c.clusterMap) == 0 {
		c.clusterMap = make(map[string]*cephv1.CephCluster)
	}

	cluster, ok := c.clusterMap[namespace]
	if !ok {
		return nil, false
	}

	return cluster, true
}

// GetClusterMap returns the internal clustermap for iteration purporses
func (c *ClusterMap) GetClusterMap() map[string]*cephv1.CephCluster {
	defer c.mux.Unlock()
	c.mux.Lock()
	return c.clusterMap
}
