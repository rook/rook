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
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// THE LIBRARY PROVIDED BY THIS FILE IS NOT THREAD SAFE

var (
	// allow unit tests to override these values
	maxUpdatesInParallel                 = 20
	updateMultipleDeploymentsAndWaitFunc = k8sutil.UpdateMultipleDeploymentsAndWait
	deploymentOnNodeFunc                 = deploymentOnNode
	deploymentOnPVCFunc                  = deploymentOnPVC
	shouldCheckOkToStopFunc              = cephclient.OSDUpdateShouldCheckOkToStop
)

type updateConfig struct {
	cluster          *Cluster
	provisionConfig  *provisionConfig
	queue            *updateQueue   // these OSDs need updated
	numUpdatesNeeded int            // the number of OSDs that needed updating
	deployments      *existenceList // these OSDs have existing deployments
}

func (c *Cluster) newUpdateConfig(
	provisionConfig *provisionConfig,
	queue *updateQueue,
	deployments *existenceList,
) *updateConfig {
	return &updateConfig{
		c,
		provisionConfig,
		queue,
		queue.Len(),
		deployments,
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
	osdIDQuery, _ := c.queue.Pop()

	var osdIDs []int
	var err error
	if !shouldCheckOkToStopFunc(c.cluster.context, c.cluster.clusterInfo) {
		// If we should not check ok-to-stop, then only process one OSD at a time. There are likely
		// less than 3 OSDs in the cluster or the cluster is on a single node. E.g., in CI :wink:.
		osdIDs = []int{osdIDQuery}
	} else {
		osdIDs, err = cephclient.OSDOkToStop(c.cluster.context, c.cluster.clusterInfo, osdIDQuery, maxUpdatesInParallel)
		if err != nil {
			if c.cluster.spec.ContinueUpgradeAfterChecksEvenIfNotHealthy {
				logger.Infof("OSD %d is not ok-to-stop but 'continueUpgradeAfterChecksEvenIfNotHealthy' is true, so continuing to update it", osdIDQuery)
				osdIDs = []int{osdIDQuery} // make sure to update the queried OSD
			} else {
				logger.Infof("OSD %d is not ok-to-stop. will try updating it again later", osdIDQuery)
				c.queue.Push(osdIDQuery) // push back onto queue to make sure we retry it later
				return
			}
		}
	}

	logger.Debugf("updating OSDs: %v", osdIDs)

	updatedDeployments := make([]*appsv1.Deployment, 0, len(osdIDs))
	listIDs := []string{} // use this to build the k8s api selector query
	for _, osdID := range osdIDs {
		if !c.deployments.Exists(osdID) {
			logger.Debugf("not updating deployment for OSD %d that is newly created", osdID)
			continue
		}

		// osdIDQuery which has been popped off the queue but it does need to be updated
		if osdID != osdIDQuery && !c.queue.Exists(osdID) {
			logger.Debugf("not updating deployment for OSD %d that is not in the update queue. the OSD has already been updated", osdID)
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

		// backward compatibility for old deployments
		if osdInfo.DeviceClass == "" {
			deviceClassInfo, err := cephclient.OSDDeviceClasses(c.cluster.context, c.cluster.clusterInfo, []string{strconv.Itoa(osdID)})
			if err != nil {
				logger.Errorf("failed to get device class for existing deployment %q. %v", depName, err)
			} else {
				osdInfo.DeviceClass = deviceClassInfo[0].DeviceClass
			}
		}

		nodeOrPVCName, err := getNodeOrPVCName(dep)
		if err != nil {
			errs.addError("%v", errors.Wrapf(err, "failed to update OSD %d", osdID))
			continue
		}

		var updatedDep *appsv1.Deployment
		if osdIsOnPVC(dep) {
			logger.Infof("updating OSD %d on PVC %q", osdID, nodeOrPVCName)
			updatedDep, err = deploymentOnPVCFunc(c.cluster, osdInfo, nodeOrPVCName, c.provisionConfig)

			message := fmt.Sprintf("Processing OSD %d on PVC %q", osdID, nodeOrPVCName)
			updateConditionFunc(c.cluster.context, c.cluster.clusterInfo.NamespacedName(), cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, message)
		} else {
			if !c.cluster.ValidStorage.NodeExists(nodeOrPVCName) {
				// node will not reconcile, so don't update the deployment
				// allow the OSD health checker to remove the OSD
				logger.Warningf(
					"not updating OSD %d on node %q. node no longer exists in the storage spec. "+
						"if the user wishes to remove OSDs from the node, they must do so manually. "+
						"Rook will not remove OSDs from nodes that are removed from the storage spec in order to prevent accidental data loss",
					osdID, nodeOrPVCName)
				continue
			}

			logger.Infof("updating OSD %d on node %q", osdID, nodeOrPVCName)
			updatedDep, err = deploymentOnNodeFunc(c.cluster, osdInfo, nodeOrPVCName, c.provisionConfig)

			message := fmt.Sprintf("Processing OSD %d on node %q", osdID, nodeOrPVCName)
			updateConditionFunc(c.cluster.context, c.cluster.clusterInfo.NamespacedName(), cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, message)
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

	failures := updateMultipleDeploymentsAndWaitFunc(c.cluster.context.Clientset, updatedDeployments, listFunc)
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
		id, err := getOSDID(&deps.Items[i]) // avoid implicit memory aliasing by indexing
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
