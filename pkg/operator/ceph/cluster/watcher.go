/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// clientCluster struct contains a client to interact with Kubernetes object
// as well as the NamespacedName (used in requests)
type clientCluster struct {
	client    client.Client
	namespace string
	context   *clusterd.Context
}

var nodesCheckedForReconcile = util.NewSet()

func newClientCluster(client client.Client, namespace string, context *clusterd.Context) *clientCluster {
	return &clientCluster{
		client:    client,
		namespace: namespace,
		context:   context,
	}
}

func checkStorageForNode(cluster *cephv1.CephCluster) bool {
	if !cluster.Spec.Storage.UseAllNodes && len(cluster.Spec.Storage.Nodes) == 0 && len(cluster.Spec.Storage.StorageClassDeviceSets) == 0 {
		logger.Debugf("node watcher: useAllNodes is set to false and no nodes storageClassDevicesets or volumeSources are specified in cluster %q, skipping", cluster.Namespace)
		return false
	}
	return true
}

// onK8sNodeAdd is triggered when a node is added in the Kubernetes cluster
func (c *clientCluster) onK8sNode(object runtime.Object) bool {
	node, ok := object.(*v1.Node)
	if !ok {
		return false
	}
	// skip reconcile if node is already checked in a previous reconcile
	if nodesCheckedForReconcile.Contains(node.Name) {
		return false
	}
	// Get CephCluster
	cluster := c.getCephCluster()

	if !k8sutil.GetNodeSchedulable(*node) {
		logger.Debugf("node watcher: skipping cluster update. added node %q is unschedulable", node.Labels[v1.LabelHostname])
		return false
	}

	if !k8sutil.NodeIsTolerable(*node, cephv1.GetOSDPlacement(cluster.Spec.Placement).Tolerations, false) {
		logger.Debugf("node watcher: node %q is not tolerable for cluster %q, skipping", node.Name, cluster.Namespace)
		return false
	}

	if !checkStorageForNode(cluster) {
		nodesCheckedForReconcile.Add(node.Name)
		return false
	}

	// Too strict? this replaces clusterInfo == nil
	if cluster.Status.Phase != cephv1.ConditionReady {
		logger.Debugf("node watcher: cluster %q is not ready. skipping orchestration", cluster.Namespace)
		return false
	}

	logger.Debugf("node %q is ready, checking if it can run OSDs", node.Name)
	nodesCheckedForReconcile.Add(node.Name)
	valid, _ := k8sutil.ValidNode(*node, cephv1.GetOSDPlacement(cluster.Spec.Placement))
	if valid {
		nodeName := node.Name
		hostname, ok := node.Labels[v1.LabelHostname]
		if ok && hostname != "" {
			nodeName = hostname
		}
		// Make sure we can call Ceph properly
		// Is the node in the CRUSH map already?
		// If so we don't need to reconcile, this is done to avoid double reconcile on operator restart
		// Assume the admin key since we are watching for node status to create OSDs
		clusterInfo := cephclient.AdminClusterInfo(cluster.Namespace)
		osds, err := cephclient.GetOSDOnHost(c.context, clusterInfo, nodeName)
		if err != nil {
			// If it fails, this might be due to the the operator just starting and catching an add event for that node
			logger.Debugf("failed to get osds on node %q, assume reconcile is necessary", nodeName)
			return true
		}

		// Reconcile if there are no OSDs in the CRUSH map and if the host does not exist in the CRUSH map.
		if osds == "" {
			logger.Infof("node watcher: adding node %q to cluster %q", node.Labels[v1.LabelHostname], cluster.Namespace)
			return true
		}

		// This is Debug level because the node receives frequent updates and this will pollute the logs
		logger.Debugf("node watcher: node %q is already an OSD node with %q", nodeName, osds)
	}
	return false
}

// onDeviceCMUpdate is trigger when the hot plug config map is updated
func (c *clientCluster) onDeviceCMUpdate(oldObj, newObj runtime.Object) bool {
	oldCm, ok := oldObj.(*v1.ConfigMap)
	if !ok {
		return false
	}
	logger.Debugf("hot-plug cm watcher: onDeviceCMUpdate old device cm: %+v", oldCm)

	newCm, ok := newObj.(*v1.ConfigMap)
	if !ok {
		return false
	}
	logger.Debugf("hot-plug cm watcher: onDeviceCMUpdate new device cm: %+v", newCm)

	oldDevStr, ok := oldCm.Data[discoverDaemon.LocalDiskCMData]
	if !ok {
		logger.Warning("hot-plug cm watcher: unexpected old configmap data")
		return false
	}

	newDevStr, ok := newCm.Data[discoverDaemon.LocalDiskCMData]
	if !ok {
		logger.Warning("hot-plug cm watcher: unexpected new configmap data")
		return false
	}

	devicesEqual, err := discoverDaemon.DeviceListsEqual(oldDevStr, newDevStr)
	if err != nil {
		logger.Warningf("hot-plug cm watcher: failed to compare device lists. %v", err)
		return false
	}

	if devicesEqual {
		logger.Debug("hot-plug cm watcher: device lists are equal. skipping orchestration")
		return false
	}

	// Get CephCluster
	cluster := c.getCephCluster()

	if cluster.Status.Phase != cephv1.ConditionReady {
		logger.Debugf("hot-plug cm watcher: cluster %q is not ready. skipping orchestration.", cluster.Namespace)
		return false
	}

	if len(cluster.Spec.Storage.StorageClassDeviceSets) > 0 {
		logger.Info("hot-plug cm watcher: skip orchestration on device config map update for OSDs on PVC")
		return false
	}

	logger.Infof("hot-plug cm watcher: running orchestration for namespace %q after device change", cluster.Namespace)
	return true
}

func (c *clientCluster) getCephCluster() *cephv1.CephCluster {
	clusterList := &cephv1.CephClusterList{}

	err := c.client.List(context.TODO(), clusterList, client.InNamespace(c.namespace))
	if err != nil {
		logger.Debugf("%q: failed to fetch CephCluster %v", controllerName, err)
		return &cephv1.CephCluster{}
	}
	if len(clusterList.Items) == 0 {
		logger.Debugf("%q: no CephCluster resource found in namespace %q", controllerName, c.namespace)
		return &cephv1.CephCluster{}
	}

	return &clusterList.Items[0]
}

// each OSD nearfull status is checked
// OSD capacity is increased if we get nearfull warning
func increaseOSDsCapacity(cephStatusChecker *cephStatusChecker) {
	osdUsage, err := cephclient.GetOSDUsage(cephStatusChecker.context, cephStatusChecker.clusterInfo)
	if err != nil {
		logger.Debugf("failed to get osd usage.%v", err)
		return
	}
	// by default nearfull_ratio is set to 85%
	osdNearfullRatio := 85

	var osdsNearfullList []int
	for _, osdStatus := range osdUsage.OSDNodes {
		osdId := osdStatus.ID
		storageUtilization, err := osdStatus.Utilization.Int64()
		if err == nil {
			if storageUtilization > int64(osdNearfullRatio) {
				osdsNearfullList = append(osdsNearfullList, osdId)
			}
		}
	}
	increaseDeviceSetCapacity(cephStatusChecker, osdsNearfullList)
}

// list the storageClassDeviceSets that are linked to OSDs, and increase there storageSize
func increaseDeviceSetCapacity(cephStatusChecker *cephStatusChecker, osdsNearfullList []int) {
	var storageClassDeviceSetsList = util.NewSet()
	for _, osdID := range osdsNearfullList {
		storageClassDeviceSetName, err := getDeviceSetName(cephStatusChecker, osdID)
		if err != nil {
			logger.Debugf("failed to fetch deviceSet name %v", err)
		}
		storageClassDeviceSetsList.Add(storageClassDeviceSetName)
	}

	clusterName := cephStatusChecker.clusterInfo.NamespacedName()
	cephCluster, err := cephStatusChecker.context.RookClientset.CephV1().CephClusters(clusterName.Namespace).Get(context.TODO(), clusterName.Name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve ceph cluster %q in namespace %q", clusterName.Name, clusterName.Namespace)
		return
	}
	for deviceSet := range storageClassDeviceSetsList.Iter() {
		for _, deviceSetProp := range cephCluster.Spec.Storage.StorageClassDeviceSets {
			if deviceSetProp.Name != deviceSet {
				continue
			}
			var defaultDeviceStorage int
			for _, volumes := range deviceSetProp.VolumeClaimTemplates {
				if volumes.Name == "data" || volumes.Name == "" {
					defaultDeviceStorage = volumes.Spec.Resources.Requests.Storage().Size()
					break
				}
			}
			growthRatePercent := deviceSetProp.GrowthPolicy.GrowthRatePercent
			maxSize := deviceSetProp.GrowthPolicy.MaxSize
			increaseStorageValue := getIncreaseStorageValue(defaultDeviceStorage, growthRatePercent, maxSize)
			logger.Debugf("update StorageClassDeviceSet %q size to %q", deviceSet, increaseStorageValue)
		}
	}
}

func getDeviceSetName(cephStatusChecker *cephStatusChecker, osdID int) (string, error) {
	deploymentName := fmt.Sprintf("rook-ceph-osd-%d", osdID)
	deployment, err := cephStatusChecker.context.Clientset.AppsV1().Deployments(cephStatusChecker.clusterInfo.Namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return "", err
	} else {
		if deviceSetName, ok := deployment.GetLabels()[osd.CephDeviceSetLabelKey]; ok {
			return deviceSetName, nil
		}
		return "", errors.Wrap(err, "failed to get deviceSetName")
	}
}

func getIncreaseStorageValue(defaultDeviceStorage int, growthRatePercent int, maxSize string) int {
	storageIncreaseCapacity := defaultDeviceStorage * (1 + (growthRatePercent)/100)
	storageMaxCapacity := resource.MustParse(maxSize)
	var increaseStorageValue int
	if storageMaxCapacity.Size() > storageIncreaseCapacity {
		increaseStorageValue = storageIncreaseCapacity
	} else {
		increaseStorageValue = storageMaxCapacity.Size()
	}
	return increaseStorageValue
}
