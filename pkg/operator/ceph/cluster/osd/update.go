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
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/topology"
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
	crushTree           *crushTree       // CRUSH tree used to restrict updates to a failure domain, fetched when first needed
	osdsNotInCrushTree  sets.Set[int]    // these OSDs were missing from the CRUSH tree even after fetching it again
	failureDomain       *string          // failure domain to restrict updates to, determined when first needed
}

func (c *Cluster) newUpdateConfig(
	provisionConfig *provisionConfig,
	queue *updateQueue,
	deployments *existenceList,
	osdsToSkipReconcile sets.Set[string],
) *updateConfig {
	return &updateConfig{
		cluster:             c,
		provisionConfig:     provisionConfig,
		queue:               queue,
		numUpdatesNeeded:    queue.Len(),
		deployments:         deployments,
		osdsToSkipReconcile: osdsToSkipReconcile,
		osdDesiredState:     map[int]*OSDInfo{},
		osdsNotInCrushTree:  sets.New[int](),
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
		} else {
			osdIDs = c.restrictToFailureDomain(osdIDQuery, osdIDs)
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

		// osdIDQuery has been popped off the queue, but it does need to be updated
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
		// Checking DeviceClass with None too, because ceph-volume lvm list returns crush device class as None
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

	if len(updatedDeployments) > 0 {
		// when waiting on deployments to be updated, only list OSDs we intend to update specifically by ID
		listFunc := c.cluster.getFuncToListDeploymentsWithIDs(listIDs)

		failures := updateMultipleDeploymentsAndWaitFunc(c.cluster.clusterInfo.Context, c.cluster.context.Clientset, updatedDeployments, listFunc)
		for _, f := range failures {
			errs.addError("%v", errors.Wrapf(f.Error, "failed to update OSD deployment %q", f.ResourceName))
		}
	}

	// If there were failures, don't retry them. If it's a transitory k8s/etcd issue, the next
	// reconcile should succeed. If it's a different issue, it will always error.
	c.queue.Remove(osdIDs)
}

// hostFailureDomain is the CRUSH bucket type of the host that an OSD runs on.
const hostFailureDomain = "host"

// restrictToFailureDomain drops the OSDs in osdIDs that are not in the same CRUSH bucket as
// osdIDQuery, for the bucket type returned by getFailureDomain. 'osd ok-to-stop --max' fills its
// result from the queried OSD's failure domain first, but then moves up the CRUSH tree and adds
// OSDs from other buckets when the pools can tolerate it, which would make a batch of updates span
// more than one failure domain. If the failure domain of osdIDQuery cannot be determined, only
// osdIDQuery is returned. Updates then proceed one OSD at a time, which makes a wrong setting
// noticeable without blocking the reconcile. The exception is an OSD that is not in a single
// bucket of that type, because it has none above it or more than one, when the failure domain was
// derived from the pools rather than set explicitly, such as an OSD under another CRUSH root used
// by pools not defined by CRs: its batch is then restricted to its host instead.
func (c *updateConfig) restrictToFailureDomain(osdIDQuery int, osdIDs []int) []int {
	namespace := c.cluster.clusterInfo.Namespace
	if len(osdIDs) <= 1 {
		return osdIDs
	}

	failureDomain, explicit, err := c.getFailureDomain()
	if err != nil {
		log.NamespacedError(namespace, logger, "failed to determine the failure domain to restrict OSD updates to; updating only OSD %d. %v", osdIDQuery, err)
		return []int{osdIDQuery}
	}
	if failureDomain == "" {
		return osdIDs // there are no pools whose failure domain could be crossed
	}

	tree, err := c.getCrushTree(osdIDQuery)
	if err != nil {
		log.NamespacedError(namespace, logger, "failed to get CRUSH tree to restrict OSD updates to %q failure domain; updating only OSD %d. %v", failureDomain, osdIDQuery, err)
		return []int{osdIDQuery}
	}
	inDomain, err := tree.osdsInSameBucket(osdIDQuery, failureDomain)
	if err != nil && !explicit && isUnplaceable(err) && failureDomain != hostFailureDomain {
		// The failure domain was derived from the pools, so this OSD may be under another CRUSH
		// root used by other pools. The OSDs on its host share all its ancestors, so updating
		// them together cannot cross a failure domain of any pool.
		log.NamespacedInfo(namespace, logger, "OSD %d is not in a single %q bucket (%v); restricting its batch to its host instead", osdIDQuery, failureDomain, err)
		failureDomain = hostFailureDomain
		inDomain, err = tree.osdsInSameBucket(osdIDQuery, failureDomain)
	}
	if err != nil {
		var noBucket *noBucketOfTypeError
		if explicit && errors.As(err, &noBucket) {
			err = errors.Wrap(err, "check that storage.osdUpdateFailureDomain names a CRUSH bucket type that contains the OSDs")
		}
		log.NamespacedError(namespace, logger, "failed to determine the %q failure domain of OSD %d; updating only that OSD. %v", failureDomain, osdIDQuery, err)
		return []int{osdIDQuery}
	}

	restricted := make([]int, 0, len(osdIDs))
	dropped := []int{}
	for _, id := range osdIDs {
		if inDomain.Has(id) { // always true for osdIDQuery, which is in its own failure domain
			restricted = append(restricted, id)
		} else {
			dropped = append(dropped, id)
		}
	}
	if len(dropped) > 0 {
		log.NamespacedInfo(namespace, logger, "not updating OSDs %v together with OSD %d since they are outside its %s failure domain", dropped, osdIDQuery, failureDomain)
	}
	return restricted
}

// getFailureDomain returns the CRUSH bucket type to restrict each batch of OSD updates to. It is
// storage.osdUpdateFailureDomain if set, and otherwise the minimum failure domain of the pools
// defined by CRs, which the OSD PDBs also allow to be disrupted. The latter is determined the
// first time it is needed during this reconcile. It returns an empty string if no CRs define pools,
// and whether the failure domain was set explicitly.
func (c *updateConfig) getFailureDomain() (failureDomain string, explicit bool, err error) {
	if configured := c.cluster.spec.Storage.OSDUpdateFailureDomain; configured != "" {
		return configured, true, nil
	}
	if c.failureDomain != nil {
		return *c.failureDomain, false, nil
	}

	poolCRs, err := topology.ListPoolCRs(c.cluster.clusterInfo.Context, c.cluster.context.Client, c.cluster.clusterInfo.Namespace)
	if err != nil {
		return "", false, err
	}
	if poolSpecs := poolCRs.PoolSpecs(); len(poolSpecs) > 0 {
		failureDomain = topology.GetMinimumFailureDomain(poolSpecs)
		log.NamespacedInfo(c.cluster.clusterInfo.Namespace, logger, "restricting OSD updates to the %q failure domain, the minimum failure domain of the pools; set storage.osdUpdateFailureDomain to override it", failureDomain)
	}
	c.failureDomain = &failureDomain
	return failureDomain, false, nil
}

// getCrushTree returns the CRUSH tree, fetching it the first time it is needed during this
// reconcile so that it is not fetched again for every batch of OSD updates. The CRUSH tree is
// fetched again if osdID is not in it, since the tree may have changed while OSDs were updated,
// but only once per OSD, so that an OSD that is not in CRUSH at all (a stray OSD) does not cause
// the tree to be fetched for every batch.
func (c *updateConfig) getCrushTree(osdID int) (*crushTree, error) {
	if c.crushTree != nil && (c.crushTree.has(osdID) || c.osdsNotInCrushTree.Has(osdID)) {
		return c.crushTree, nil
	}
	tree, err := cephclient.HostTree(c.cluster.context, c.cluster.clusterInfo)
	if err != nil {
		return nil, err
	}
	c.crushTree = newCrushTree(tree)
	if !c.crushTree.has(osdID) {
		c.osdsNotInCrushTree.Insert(osdID)
	}
	return c.crushTree, nil
}

// noBucketOfTypeError is returned when an OSD has no bucket of the requested type above it.
type noBucketOfTypeError struct {
	osdID      int
	bucketType string
}

func (e *noBucketOfTypeError) Error() string {
	return fmt.Sprintf("OSD %d has no ancestor of CRUSH type %q", e.osdID, e.bucketType)
}

// ambiguousBucketError is returned when an item on the path from an OSD to a bucket of the
// requested type is linked under more than one bucket.
type ambiguousBucketError struct {
	osdID      int
	bucketType string
	itemID     int
	parents    []int
}

func (e *ambiguousBucketError) Error() string {
	return fmt.Sprintf("CRUSH item %d is linked under more than one bucket (%v), so the %q failure domain of OSD %d is ambiguous", e.itemID, e.parents, e.bucketType, e.osdID)
}

// isUnplaceable returns true if err means that an OSD is not in a single bucket of the requested
// type, because it has no such bucket above it or more than one.
func isUnplaceable(err error) bool {
	var noBucket *noBucketOfTypeError
	var ambiguous *ambiguousBucketError
	return errors.As(err, &noBucket) || errors.As(err, &ambiguous)
}

// crushTree indexes the output of 'ceph osd tree' to look up the failure domain of OSDs.
type crushTree struct {
	tree    cephclient.OsdTree
	nodes   map[int]int   // CRUSH ID -> index in tree.Nodes
	parents map[int][]int // CRUSH ID -> IDs of the buckets it is linked under
}

func newCrushTree(tree cephclient.OsdTree) *crushTree {
	t := &crushTree{tree: tree, nodes: map[int]int{}, parents: map[int][]int{}}
	for i, node := range tree.Nodes {
		if _, ok := t.nodes[node.ID]; ok {
			// 'ceph osd tree' dumps a bucket linked under more than one bucket, and the items below
			// it, once for each of them
			continue
		}
		t.nodes[node.ID] = i
		for _, child := range node.Children {
			t.parents[child] = append(t.parents[child], node.ID)
		}
	}
	return t
}

// has returns true if the CRUSH item with the given ID is in the tree.
func (t *crushTree) has(id int) bool {
	_, ok := t.nodes[id]
	return ok
}

// osdsInSameBucket returns the IDs of all OSDs under the CRUSH item of type bucketType that
// contains osdID, which may be the OSD itself if bucketType is "osd". It returns an error if osdID
// is not in the CRUSH tree, has no such item above it, or if the path to that item is ambiguous
// because an item on it is linked under more than one bucket.
func (t *crushTree) osdsInSameBucket(osdID int, bucketType string) (sets.Set[int], error) {
	bucketID := osdID
	for {
		i, ok := t.nodes[bucketID]
		if !ok {
			return nil, errors.Errorf("CRUSH item %d is not in the CRUSH tree", bucketID)
		}
		if t.tree.Nodes[i].Type == bucketType {
			break
		}
		switch p := t.parents[bucketID]; len(p) {
		case 0:
			return nil, &noBucketOfTypeError{osdID: osdID, bucketType: bucketType}
		case 1:
			bucketID = p[0]
		default:
			return nil, &ambiguousBucketError{osdID: osdID, bucketType: bucketType, itemID: bucketID, parents: p}
		}
	}

	osds := sets.New[int]()
	toVisit := []int{bucketID}
	for len(toVisit) > 0 {
		id := toVisit[len(toVisit)-1]
		toVisit = toVisit[:len(toVisit)-1]
		if id >= 0 {
			osds.Insert(id) // non-negative CRUSH IDs are OSDs
			continue
		}
		if i, ok := t.nodes[id]; ok {
			for _, child := range t.tree.Nodes[i].Children {
				// an item also linked under another bucket is in more than one failure domain
				if len(t.parents[child]) == 1 {
					toVisit = append(toVisit, child)
				}
			}
		}
	}
	return osds, nil
}

// getOSDUpdateInfo returns an update queue of OSDs which need to be updated and an existence list of OSD
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
		identity := osdIdentity{
			UUID: getOSDUUIDFromDeployment(&deps.Items[i]),
		}
		identity.NodeOrPVCName, _ = getNodeOrPVCName(&deps.Items[i])

		existenceList.AddWithDetail(id, identity)
		updateQueue.Push(id)
	}

	return updateQueue, existenceList, nil
}

// An updateQueue keeps track of OSDs which need to be updated.
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
	return slices.Contains(q.q, osdID)
}

// Remove removes the items from the queue if they exist.
func (q *updateQueue) Remove(osdIDs []int) {
	shouldRemove := func(rid int) bool {
		return slices.Contains(osdIDs, rid)
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

// osdIdentity stores identifying details about an existing OSD deployment.
type osdIdentity struct {
	UUID          string
	NodeOrPVCName string
}

// An existenceList keeps track of which OSDs already have Deployments created for them that is
// queryable in O(1) time.
type existenceList struct {
	m map[int]osdIdentity
}

// Create a new existenceList with capacity reserved.
func newExistenceListWithCapacity(cap int) *existenceList {
	return &existenceList{
		m: make(map[int]osdIdentity, cap),
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

// Add adds an item to the existenceList with an empty identity.
func (e *existenceList) Add(osdID int) {
	e.m[osdID] = osdIdentity{}
}

// AddWithDetail adds an item to the existenceList along with its OSD identity.
func (e *existenceList) AddWithDetail(osdID int, identity osdIdentity) {
	e.m[osdID] = identity
}

// Exists returns true if an item is recorded in the existence list or false if it is not.
func (e *existenceList) Exists(osdID int) bool {
	_, ok := e.m[osdID]
	return ok
}

// Get returns the identity for an existing OSD, or an empty identity and false if the OSD is not found.
func (e *existenceList) Get(osdID int) (osdIdentity, bool) {
	d, ok := e.m[osdID]
	return d, ok
}

// return a function that will list only OSD deployments with the IDs given
func (c *Cluster) getFuncToListDeploymentsWithIDs(osdIDs []string) func() (*appsv1.DeploymentList, error) {
	if len(osdIDs) == 0 {
		return func() (*appsv1.DeploymentList, error) {
			return &appsv1.DeploymentList{}, nil
		}
	}
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
		runningCephVersion, desiredCephVersion, osdInfo.CephxStatus, true, c.clusterInfo.Namespace) // daemon key type always takes the default from setDefaultCephxKeyType()
	if err != nil {
		return osdInfo.CephxStatus, errors.Wrapf(err, "failed to determine if cephx key for OSD %d needs rotated", osdInfo.ID)
	}

	keyType := cephv1.CephxKeyTypeUndefined // daemon key type always takes the default from setDefaultCephxKeyType()

	didRotateCephxStatus := keyring.UpdatedCephxStatus(true, c.spec.Security.CephX.Daemon,
		c.clusterInfo.CephVersion, osdInfo.CephxStatus, keyType)
	didNotRotateCephxStatus := keyring.UpdatedCephxStatus(false, c.spec.Security.CephX.Daemon,
		c.clusterInfo.CephVersion, osdInfo.CephxStatus, keyType)

	if !shouldRotate {
		return didNotRotateCephxStatus, nil
	}

	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "rotating cephx key of OSD %d for CephCluster in namespace %q", osdInfo.ID, c.clusterInfo.Namespace)
	user := fmt.Sprintf("osd.%d", osdInfo.ID)
	// Note: OSD key is not stored in k8s secret; rotated key is picked up by OSD init container

	_, err = cephclient.AuthRotate(c.context, c.clusterInfo, user, string(keyType))
	if err != nil {
		return didNotRotateCephxStatus, errors.Wrapf(err, "failed to rotate cephx key for OSD %d", osdInfo.ID)
	}

	// rotating the `client.osd-lockbox.$OSD_UUID` keys created for luks-encrypted OSDs by ceph-volume
	if osdInfo.Encrypted {
		osdLockBoxUser := fmt.Sprintf("client.osd-lockbox.%s", osdInfo.UUID)
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "rotating osd-lockbox cephx key of encrypted OSD %d for CephCluster in namespace %q", osdInfo.ID, c.clusterInfo.Namespace)
		_, err = cephclient.AuthRotate(c.context, c.clusterInfo, osdLockBoxUser, string(keyType))
		if err != nil {
			return didNotRotateCephxStatus, errors.Wrapf(err, "failed to rotate osd-lockbox cephx key for the encrypted OSD %d", osdInfo.ID)
		}
	}

	return didRotateCephxStatus, nil
}
