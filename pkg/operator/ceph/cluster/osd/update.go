/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// THE LIBRARY PROVIDED BY THIS FILE IS NOT THREAD SAFE

var (
	// allow unit tests to override these values
	defaultOSDMaxUpdatesInParallel       uint32 = 20
	updateMultipleDeploymentsAndWaitFunc        = k8sutil.UpdateMultipleDeploymentsAndWait
	deploymentOnNodeFunc                        = deploymentOnNode
	deploymentOnPVCFunc                         = deploymentOnPVC
	shouldCheckOkToStopFunc                     = cephclient.OSDUpdateShouldCheckOkToStop
)

type updateConfig struct {
	cluster             *Cluster
	provisionConfig     *provisionConfig
	queue               *updateQueue     // these OSDs need updated
	numUpdatesNeeded    int              // the number of OSDs that needed updating
	deployments         *existenceList   // these OSDs have existing deployments
	osdsToSkipReconcile sets.Set[string] // these OSDs should not be updated during reconcile
	osdDesiredState     map[int]*OSDInfo // the desired state of the OSDs determined during the reconcile
}

func (c *Cluster) newUpdateConfig(
	provisionConfig *provisionConfig,
	queue *updateQueue,
	deployments *existenceList,
	osdsToSkipReconcile sets.Set[string],
) *updateConfig {
	return &updateConfig{
		c,
		provisionConfig,
		queue,
		queue.Len(),
		deployments,
		osdsToSkipReconcile,
		map[int]*OSDInfo{},
	}
}

func (c *updateConfig) progress() (completed, initial int) {
	return (c.numUpdatesNeeded - c.queue.Len()), c.numUpdatesNeeded
}

func (c *updateConfig) doneUpdating() bool {
	return c.queue.Len() == 0
}

func (c *updateConfig) updateExistingOSDs(errs *provisionErrors) {
	if c.doneUpdating() {
		return // no more OSDs to update
	}
	if !c.cluster.spec.SkipUpgradeChecks && c.cluster.spec.UpgradeOSDRequiresHealthyPGs {
		pgHealthMsg, pgClean, err := cephclient.IsClusterClean(c.cluster.context, c.cluster.clusterInfo, c.cluster.spec.DisruptionManagement.PGHealthyRegex)
		if err != nil {
			log.NamespacedWarning(c.cluster.clusterInfo.Namespace, logger, "failed to check PGs status to update OSDs, will try updating it again later. %v", err)
			return
		}
		if !pgClean {
			log.NamespacedInfo(c.cluster.clusterInfo.Namespace, logger, "PGs are not healthy to update OSDs, will try updating it again later. PGs status: %q", pgHealthMsg)
			return
		}
		log.NamespacedInfo(c.cluster.clusterInfo.Namespace, logger, "PGs are healthy to proceed updating OSDs. %v", pgHealthMsg)
	}

	osdIDQuery, _ := c.queue.Pop()

	var osdIDs []int
	var err error
	if c.cluster.spec.SkipUpgradeChecks || !shouldCheckOkToStopFunc(c.cluster.context, c.cluster.clusterInfo) {
		// If we should not check ok-to-stop, then only process one OSD at a time. There are likely
		// less than 3 OSDs in the cluster or the cluster is on a single node. E.g., in CI :wink:.
		log.NamespacedInfo(c.cluster.clusterInfo.Namespace, logger, "skipping osd checks for ok-to-stop")
		osdIDs = []int{osdIDQuery}
	} else {
		osdMaxUpdatesInParallel := c.cluster.spec.Storage.OSDMaxUpdatesInParallel
		if osdMaxUpdatesInParallel == 0 {
			osdMaxUpdatesInParallel = defaultOSDMaxUpdatesInParallel
		}
		osdIDs, err = cephclient.OSDOkToStop(c.cluster.context, c.cluster.clusterInfo, osdIDQuery, osdMaxUpdatesInParallel)
		if err != nil {
			if c.cluster.spec.ContinueUpgradeAfterChecksEvenIfNotHealthy {
				log.NamespacedInfo(c.cluster.clusterInfo.Namespace, logger, "OSD %d is not ok-to-stop but 'continueUpgradeAfterChecksEvenIfNotHealthy' is true, so continuing to update it", osdIDQuery)
				osdIDs = []int{osdIDQuery} // make sure to update the queried OSD
			} else {
				log.NamespacedInfo(c.cluster.clusterInfo.Namespace, logger, "OSD %d is not ok-to-stop. will try updating it again later", osdIDQuery)
				c.queue.Push(osdIDQuery) // push back onto queue to make sure we retry it later
				return
			}
		}
	}

	log.NamespacedDebug(c.cluster.clusterInfo.Namespace, logger, "updating OSDs: %v", osdIDs)

	updatedDeployments := make([]*appsv1.Deployment, 0, len(osdIDs))
	listIDs := []string{} // use this to build the k8s api selector query
	for _, osdID := range osdIDs {
		if !c.deployments.Exists(osdID) {
			log.NamespacedDebug(c.cluster.clusterInfo.Namespace, logger, "not updating deployment for OSD %d that is newly created", osdID)
			continue
		}

		// osdIDQuery which has been popped off the queue but it does need to be updated
		if osdID != osdIDQuery && !c.queue.Exists(osdID) {
			log.NamespacedDebug(c.cluster.clusterInfo.Namespace, logger, "not updating deployment for OSD %d that is not in the update queue. the OSD has already been updated", osdID)
			continue
		}

		depName := deploymentName(osdID)
		dep, err := c.cluster.context.Clientset.AppsV1().Deployments(c.cluster.clusterInfo.Namespace).Get(c.cluster.clusterInfo.Context, depName, metav1.GetOptions{})
		if err != nil {
			errs.addError("failed to update OSD %d. failed to find existing deployment %q. %v", osdID, depName, err)
			continue
		}
		osdInfo, err := c.cluster.getOSDInfo(dep)
		if err != nil {
			errs.addError("failed to update OSD %d. failed to extract OSD info from existing deployment %q. %v", osdID, depName, err)
			continue
		}
		c.osdDesiredState[osdID] = &osdInfo

		if c.osdsToSkipReconcile.Has(strconv.Itoa(osdID)) {
			log.NamespacedWarning(c.cluster.clusterInfo.Namespace, logger, "Skipping update for OSD %d since labeled with %s", osdID, cephv1.SkipReconcileLabelKey)
			continue
		}

		// backward compatibility for old deployments
		// Checking DeviceClass with None too, because ceph-volume lvm list return crush device class as None
		// Tracker https://tracker.ceph.com/issues/53425
		if osdInfo.DeviceClass == "" || osdInfo.DeviceClass == "None" {
			deviceClassInfo, err := cephclient.OSDDeviceClasses(c.cluster.context, c.cluster.clusterInfo, []string{strconv.Itoa(osdID)})
			if err != nil {
				log.NamespacedError(c.cluster.clusterInfo.Namespace, logger, "failed to get device class for existing deployment %q. %v", depName, err)
			} else {
				osdInfo.DeviceClass = deviceClassInfo[0].DeviceClass
			}
		}

		nodeOrPVCName, err := getNodeOrPVCName(dep)
		if err != nil {
			errs.addError("%v", errors.Wrapf(err, "failed to update OSD %d", osdID))
			continue
		}

		cephxStatus, err := c.cluster.rotateCephxKey(osdInfo)
		if err != nil {
			// user-desired rotation failed, so report an error, but continue to try to update the OSD deployment
			errs.addError("%v", errors.Wrapf(err, "failed to rotate cephx key for OSD %d", osdID))
		}
		osdInfo.CephxStatus = cephxStatus // returned status is always correct

		var updatedDep *appsv1.Deployment

		if c.cluster.spec.Network.MultiClusterService.Enabled {
			osdInfo.ExportService = true
		}

		if osdIsOnPVC(dep) {
			log.NamespacedInfo(c.cluster.clusterInfo.Namespace, logger, "updating OSD %d on PVC %q", osdID, nodeOrPVCName)
			updatedDep, err = deploymentOnPVCFunc(c.cluster, &osdInfo, nodeOrPVCName, c.provisionConfig)
			message := fmt.Sprintf("Processing OSD %d on PVC %q", osdID, nodeOrPVCName)
			updateConditionFunc(c.cluster.clusterInfo.Context, c.cluster.context, c.cluster.clusterInfo.NamespacedName(), k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, message)
		} else {
			if !c.cluster.ValidStorage.NodeExists(nodeOrPVCName) {
				// node will not reconcile, so don't update the deployment
				log.NamespacedWarning(c.cluster.clusterInfo.Namespace, logger,
					"not updating OSD %d on node %q. node no longer exists in the storage spec. "+
						"if the user wishes to remove OSDs from the node, they must do so manually. "+
						"Rook will not remove OSDs from nodes that are removed from the storage spec in order to prevent accidental data loss",
					osdID, nodeOrPVCName)
				continue
			}

			log.NamespacedInfo(c.cluster.clusterInfo.Namespace, logger, "updating OSD %d on node %q", osdID, nodeOrPVCName)
			updatedDep, err = deploymentOnNodeFunc(c.cluster, &osdInfo, nodeOrPVCName, c.provisionConfig)
			message := fmt.Sprintf("Processing OSD %d on node %q", osdID, nodeOrPVCName)
			updateConditionFunc(c.cluster.clusterInfo.Context, c.cluster.context, c.cluster.clusterInfo.NamespacedName(), k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, message)
		}
		if err != nil {
			errs.addError("%v", errors.Wrapf(err, "failed to update OSD %d", osdID))
			continue
		}

		updatedDeployments = append(updatedDeployments, updatedDep)
		listIDs = append(listIDs, strconv.Itoa(osdID))
	}

	// when waiting on deployments to be updated, only list OSDs we intend to update specifically by ID
	listFunc := c.cluster.getFuncToListDeploymentsWithIDs(listIDs)

	failures := updateMultipleDeploymentsAndWaitFunc(c.cluster.clusterInfo.Context, c.cluster.context.Clientset, updatedDeployments, listFunc)
	for _, f := range failures {
		errs.addError("%v", errors.Wrapf(f.Error, "failed to update OSD deployment %q", f.ResourceName))
	}

	// If there were failures, don't retry them. If it's a transitory k8s/etcd issue, the next
	// reconcile should succeed. If it's a different issue, it will always error.
	c.queue.Remove(osdIDs)
}

// getOSDUpdateInfo returns an update queue of OSDs which need updated and an existence list of OSD
// Deployments which already exist.
func (c *Cluster) getOSDUpdateInfo(errs *provisionErrors) (*updateQueue, *existenceList, error) {
	namespace := c.clusterInfo.Namespace

	selector := fmt.Sprintf("%s=%s", k8sutil.AppAttr, AppName)
	listOpts := metav1.ListOptions{
		// list only rook-ceph-osd Deployments
		LabelSelector: selector,
	}
	deps, err := c.context.Clientset.AppsV1().Deployments(namespace).List(c.clusterInfo.Context, listOpts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to query existing OSD deployments to see if they need updated")
	}

	updateQueue := newUpdateQueueWithCapacity(len(deps.Items))
	existenceList := newExistenceListWithCapacity(len(deps.Items))
	for i := range deps.Items {
		id, err := GetOSDID(&deps.Items[i]) // avoid implicit memory aliasing by indexing
		if err != nil {
			// add a question to the user AFTER the error text to help them recover from user error
			errs.addError("%v. did a user create their own deployment with label %q?", selector, err)
			continue
		}
		// all OSD deployments should be marked as existing
		existenceList.Add(id)
		updateQueue.Push(id)
	}

	return updateQueue, existenceList, nil
}

// An updateQueue keeps track of OSDs which need updated.
type updateQueue struct {
	q []int // just a list of OSD IDs
}

// Create a new updateQueue with capacity reserved.
func newUpdateQueueWithCapacity(cap int) *updateQueue {
	return &updateQueue{
		q: make([]int, 0, cap),
	}
}

func newUpdateQueueWithIDs(ids ...int) *updateQueue {
	return &updateQueue{
		q: ids,
	}
}

// Len returns the length of the queue.
func (q *updateQueue) Len() int {
	return len(q.q)
}

// Push pushes an item onto the end of the queue.
func (q *updateQueue) Push(osdID int) {
	q.q = append(q.q, osdID)
}

// Pop pops an item off the beginning of the queue.
// Returns -1 and ok=false if the queue is empty. Otherwise, returns an OSD ID and ok=true.
func (q *updateQueue) Pop() (osdID int, ok bool) {
	if q.Len() == 0 {
		return -1, false
	}

	osdID = q.q[0]
	q.q = q.q[1:]
	return osdID, true
}

// Exists returns true if the item exists in the queue.
func (q *updateQueue) Exists(osdID int) bool {
	for _, id := range q.q {
		if id == osdID {
			return true
		}
	}
	return false
}

// Clear deletes all entries inside the queue
func (q *updateQueue) Clear() {
	q.q = q.q[:0]
}

// Remove removes the items from the queue if they exist.
func (q *updateQueue) Remove(osdIDs []int) {
	shouldRemove := func(rid int) bool {
		for _, id := range osdIDs {
			if id == rid {
				return true
			}
		}
		return false
	}

	lastIdx := 0
	for idx, osdID := range q.q {
		if !shouldRemove(osdID) {
			// do removal by shifting slice items that should be kept into the next good position in
			// the slice, and then reduce the slice capacity to match the number of kept items
			q.q[lastIdx] = q.q[idx]
			lastIdx++
		}
	}
	q.q = q.q[:lastIdx]
}

// An existenceList keeps track of which OSDs already have Deployments created for them that is
// queryable in O(1) time.
type existenceList struct {
	m map[int]bool
}

// Create a new existenceList with capacity reserved.
func newExistenceListWithCapacity(cap int) *existenceList {
	return &existenceList{
		m: make(map[int]bool, cap),
	}
}

func newExistenceListWithIDs(ids ...int) *existenceList {
	e := newExistenceListWithCapacity(len(ids))
	for _, id := range ids {
		e.Add(id)
	}
	return e
}

// Len returns the length of the existence list, the number of existing items.
func (e *existenceList) Len() int {
	return len(e.m)
}

// Add adds an item to the existenceList.
func (e *existenceList) Add(osdID int) {
	e.m[osdID] = true
}

// Exists returns true if an item is recorded in the existence list or false if it does not.
func (e *existenceList) Exists(osdID int) bool {
	_, ok := e.m[osdID]
	return ok
}

// return a function that will list only OSD deployments with the IDs given
func (c *Cluster) getFuncToListDeploymentsWithIDs(osdIDs []string) func() (*appsv1.DeploymentList, error) {
	selector := fmt.Sprintf("ceph-osd-id in (%s)", strings.Join(osdIDs, ", "))
	listOpts := metav1.ListOptions{
		LabelSelector: selector, // e.g. 'ceph-osd-id in (1, 3, 5, 7, 9)'
	}
	return func() (*appsv1.DeploymentList, error) {
		return c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).List(c.clusterInfo.Context, listOpts)
	}
}

// getOSDDeployments returns the list of existing OSD deployments
func (c *Cluster) getOSDDeployments() (*appsv1.DeploymentList, error) {
	namespace := c.clusterInfo.Namespace
	selector := fmt.Sprintf("%s=%s", k8sutil.AppAttr, AppName)
	listOpts := metav1.ListOptions{
		// list only rook-ceph-osd Deployments
		LabelSelector: selector,
	}
	deps, err := c.context.Clientset.AppsV1().Deployments(namespace).List(c.clusterInfo.Context, listOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query existing OSD deployments to check if they need to be updated")
	}
	return deps, nil
}

// if needed, rotate cephx key for the OSD
// always returns the cephx status that should be applied to the OSD annotation, even in error case
func (c *Cluster) rotateCephxKey(osdInfo OSDInfo) (cephv1.CephxStatus, error) {
	// TODO: for rotation WithCephVersionUpdate fix this to have the right runningCephVersion and desiredCephVersion
	runningCephVersion := c.clusterInfo.CephVersion
	desiredCephVersion := c.clusterInfo.CephVersion
	shouldRotate, err := keyring.ShouldRotateCephxKeys(c.spec.Security.CephX.Daemon,
		runningCephVersion, desiredCephVersion, osdInfo.CephxStatus)
	if err != nil {
		return osdInfo.CephxStatus, errors.Wrapf(err, "failed to determine if cephx key for OSD %d needs rotated", osdInfo.ID)
	}

	didRotateCephxStatus := keyring.UpdatedCephxStatus(true, c.spec.Security.CephX.Daemon,
		c.clusterInfo.CephVersion, osdInfo.CephxStatus)
	didNotRotateCephxStatus := keyring.UpdatedCephxStatus(false, c.spec.Security.CephX.Daemon,
		c.clusterInfo.CephVersion, osdInfo.CephxStatus)

	if !shouldRotate {
		return didNotRotateCephxStatus, nil
	}

	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "rotating cephx key of OSD %d for CephCluster in namespace %q", osdInfo.ID, c.clusterInfo.Namespace)
	user := fmt.Sprintf("osd.%d", osdInfo.ID)
	// Note: OSD key is not stored in k8s secret; rotated key is picked up by OSD init container
	_, err = cephclient.AuthRotate(c.context, c.clusterInfo, user)
	if err != nil {
		return didNotRotateCephxStatus, errors.Wrapf(err, "failed to rotate cephx key for OSD %d", osdInfo.ID)
	}

	// rotating the `client.osd-lockbox.$OSD_UUID` keys created for luks-encrypted OSDs by ceph-volume
	if osdInfo.Encrypted {
		osdLockBoxUser := fmt.Sprintf("client.osd-lockbox.%s", osdInfo.UUID)
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "rotating osd-lockbox cephx key of encrypted OSD %d for CephCluster in namespace %q", osdInfo.ID, c.clusterInfo.Namespace)
		_, err = cephclient.AuthRotate(c.context, c.clusterInfo, osdLockBoxUser)
		if err != nil {
			return didNotRotateCephxStatus, errors.Wrapf(err, "failed to rotate osd-lockbox cephx key for the encrypted OSD %d", osdInfo.ID)
		}
	}

	return didRotateCephxStatus, nil
}
