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
	"encoding/json"
	"fmt"
	"strings"

	addonsv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/apis/csiaddons/v1alpha1"
	pkgerror "github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// clientCluster struct contains a client to interact with Kubernetes object
// as well as the NamespacedName (used in requests)
type clientCluster struct {
	client    client.Client
	namespace string
	context   *clusterd.Context
}

var nodesCheckedForReconcile = sets.New[string]()

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
func (c *clientCluster) onK8sNode(ctx context.Context, object runtime.Object) bool {
	node, ok := object.(*v1.Node)
	if !ok {
		return false
	}

	// Get CephCluster
	cluster := c.getCephCluster()

	if err := c.handleNodeFailure(ctx, cluster, node); err != nil {
		logger.Errorf("failed to handle node failure. %v", err)
	}

	// skip reconcile if node is already checked in a previous reconcile
	if nodesCheckedForReconcile.Has(node.Name) {
		return false
	}

	if !k8sutil.GetNodeSchedulable(*node) {
		logger.Debugf("node watcher: skipping cluster update. added node %q is unschedulable", node.Labels[v1.LabelHostname])
		return false
	}

	if !k8sutil.NodeIsTolerable(*node, cephv1.GetOSDPlacement(cluster.Spec.Placement).Tolerations, false) {
		logger.Debugf("node watcher: node %q is not tolerable for cluster %q, skipping", node.Name, cluster.Namespace)
		return false
	}

	if !checkStorageForNode(cluster) {
		nodesCheckedForReconcile.Insert(node.Name)
		return false
	}

	// Too strict? this replaces clusterInfo == nil
	if cluster.Status.Phase != cephv1.ConditionReady {
		logger.Debugf("node watcher: cluster %q is not ready. skipping orchestration", cluster.Namespace)
		return false
	}

	logger.Debugf("node %q is ready, checking if it can run OSDs", node.Name)
	nodesCheckedForReconcile.Insert(node.Name)
	err := k8sutil.ValidNode(*node, cephv1.GetOSDPlacement(cluster.Spec.Placement))
	if err == nil {
		nodeName := node.Name
		hostname, ok := node.Labels[v1.LabelHostname]
		if ok && hostname != "" {
			nodeName = hostname
		}
		// Make sure we can call Ceph properly
		// Is the node in the CRUSH map already?
		// If so we don't need to reconcile, this is done to avoid double reconcile on operator restart
		// Assume the admin key since we are watching for node status to create OSDs
		clusterInfo := cephclient.AdminClusterInfo(ctx, cluster.Namespace, cluster.Name)
		osds, err := cephclient.GetOSDOnHost(c.context, clusterInfo, nodeName)
		if err != nil {
			if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
				logger.Debug(opcontroller.OperatorNotInitializedMessage)
				return false
			}
			// If it fails, this might be due to the operator just starting and catching an add event for that node
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

func (c *clientCluster) handleNodeFailure(ctx context.Context, cluster *cephv1.CephCluster, node *v1.Node) error {
	watchForNodeLoss, err := k8sutil.GetOperatorSetting(ctx, c.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_WATCH_FOR_NODE_FAILURE", "true")
	if err != nil {
		return pkgerror.Wrapf(err, "failed to get configmap value `ROOK_WATCH_FOR_NODE_FAILURE`.")
	}

	_, err = c.context.ApiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, "networkfences.csiaddons.openshift.io", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("networkfences.csiaddons.openshift.io CRD not found, skip creating networkFence")
			return nil
		}
		return pkgerror.Wrapf(err, "failed to get networkfences.csiaddons.openshift.io CRD, skip creating networkFence")
	}

	if strings.ToLower(watchForNodeLoss) != "true" {
		logger.Debugf("not watching for node failures `ROOK_WATCH_FOR_NODE_FAILURE` is set to %q", watchForNodeLoss)
		return nil
	}

	nodeHasOutOfServiceTaint := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == v1.TaintNodeOutOfService {
			nodeHasOutOfServiceTaint = true
			logger.Debugf("Found taint: Key=%v, Value=%v on node %s\n", taint.Key, taint.Value, node.Name)
			break
		}

	}

	if nodeHasOutOfServiceTaint {
		err := c.fenceNode(ctx, node, cluster)
		if err != nil {
			return pkgerror.Wrapf(err, "failed to create network fence for node %q.", node.Name)
		}
		return nil
	}

	logger.Infof("node %s does not have taint %s, trying to remove networkFence CR if exists", node.Name, v1.TaintNodeOutOfService)
	err = c.unfenceAndDeleteNetworkFence(ctx, *node, cluster)
	if err != nil {
		return pkgerror.Wrapf(err, "failed to delete network fence for node %q.", node.Name)
	}

	return nil
}

func (c *clientCluster) fenceNode(ctx context.Context, node *corev1.Node, cluster *cephv1.CephCluster) error {
	volumesInuse := node.Status.VolumesInUse
	if len(volumesInuse) == 0 {
		logger.Debugf("no volumes in use for node %q", node.Name)
		return nil
	}
	logger.Debugf("volumesInuse %s", volumesInuse)

	rbdVolumesInUse, _ := getCephVolumesInUse(cluster, volumesInuse)
	if len(rbdVolumesInUse) == 0 {
		logger.Infof("no rbd `volumesInUse` for node %q", node.Name)
		return nil
	}

	listPVs, err := c.context.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return pkgerror.Wrapf(err, "failed to list PV")
	}

	rbdPVList, _ := listRBDAndCephFSPV(listPVs, cluster, rbdVolumesInUse)
	if len(rbdPVList) == 0 {
		logger.Info("No rbd PVs found on the node")
		return nil
	}

	clusterInfo := cephclient.AdminClusterInfo(ctx, cluster.Namespace, cluster.Name)

	// We only need to create the network fence for any one of rbd/cephFS pv.
	err = c.fenceRbdImage(ctx, node, cluster, clusterInfo, rbdPVList[0])
	if err != nil {
		return pkgerror.Wrapf(err, "failed to fence rbd volumes")
	}

	// else {
	// 	err = c.fennceCephFSVolumes(ctx, node, cluster, clusterInfo, listCephFSPV[0])
	// 	if err != nil {
	// 		return fmt.Errorf("failed to fence cephfs volumes. %v", err)
	// 	}
	// }

	return nil
}

func getCephVolumesInUse(cluster *cephv1.CephCluster, volumesInUse []v1.UniqueVolumeName) ([]string, []string) {
	var rbdVolumesInUse, cephFSVolumeInUse []string

	for _, volume := range volumesInUse {
		volumesInuseRemoveK8sPrefix := strings.TrimPrefix(string(volume), "kubernetes.io/csi/")
		splitVolumeInUseBased := strings.Split(volumesInuseRemoveK8sPrefix, "^")
		logger.Infof("volumeInUse after split based on '^' %v", splitVolumeInUseBased)

		if len(splitVolumeInUseBased) == 2 && splitVolumeInUseBased[0] == fmt.Sprintf("%s.rbd.csi.ceph.com", cluster.Namespace) {
			rbdVolumesInUse = append(rbdVolumesInUse, splitVolumeInUseBased[1])

		}
		// else if len(splitVolumeInUseBased) == 2 && splitVolumeInUseBased[0] == fmt.Sprintf("%s.cephfs.csi.ceph.com", cluster.Namespace) {
		// 	cephFSVolumeInUse = append(cephFSVolumeInUse, splitVolumeInUseBased[1])
		// }
	}
	return rbdVolumesInUse, cephFSVolumeInUse
}

func listRBDAndCephFSPV(listPVs *corev1.PersistentVolumeList, cluster *cephv1.CephCluster, rbdVolumesInUse []string) ([]corev1.PersistentVolume, []corev1.PersistentVolume) {
	var listRbdPV, listCephFSPV []corev1.PersistentVolume

	for _, pv := range listPVs.Items {
		if pv.Spec.CSI.Driver == fmt.Sprintf("%s.rbd.csi.ceph.com", cluster.Namespace) {
			for _, rbdVolume := range rbdVolumesInUse {
				if pv.Spec.CSI.VolumeHandle == rbdVolume {
					listRbdPV = append(listRbdPV, pv)
				}
			}
		}
		// else if pv.Spec.CSI.Driver == fmt.Sprintf("%s.cephfs.csi.ceph.com", cluster.Namespace) {
		// 	for _, cephFSVolume := range cephFSVolumeInUse {
		// 		if pv.Spec.CSI.VolumeHandle == cephFSVolume {
		// 			listCephFSPV = append(listCephFSPV, pv)
		// 		}
		// 	}
		// }
	}
	return listRbdPV, listCephFSPV
}

func (c *clientCluster) fenceRbdImage(
	ctx context.Context, node *corev1.Node, cluster *cephv1.CephCluster,
	clusterInfo *cephclient.ClusterInfo, rbdPV corev1.PersistentVolume) error {

	logger.Infof("rbd PV NAME %v", rbdPV.Spec.CSI.VolumeAttributes)
	args := []string{"status", fmt.Sprintf("%s/%s", rbdPV.Spec.CSI.VolumeAttributes["pool"], rbdPV.Spec.CSI.VolumeAttributes["imageName"])}
	cmd := cephclient.NewRBDCommand(c.context, clusterInfo, args)
	cmd.JsonOutput = true

	buf, err := cmd.Run()
	if err != nil {
		return pkgerror.Wrapf(err, "failed to list watchers for pool/imageName %s/%s.", rbdPV.Spec.CSI.VolumeAttributes["pool"], rbdPV.Spec.CSI.VolumeAttributes["imageName"])
	}

	logger.Infof("rbd status to get the client ips %v", string(buf))
	ips, err := rbdStatusUnMarshal(buf)
	if err != nil {
		return pkgerror.Wrapf(err, "failed to unmarshal rbd status output")
	}
	if len(ips) != 0 {
		err = c.createNetworkFence(ctx, rbdPV, node, cluster, ips)
		if err != nil {
			return pkgerror.Wrapf(err, "failed to create network fence for node %q", node.Name)
		}
	}

	return nil
}

func rbdStatusUnMarshal(output []byte) ([]string, error) {
	type rbdStatus struct {
		Watchers []struct {
			Address string `json:"address"`
		} `json:"watchers"`
	}

	var rbdStatusObj rbdStatus
	err := json.Unmarshal([]byte(output), &rbdStatusObj)
	if err != nil {
		return []string{}, pkgerror.Wrapf(err, "failed to unmarshal rbd status output")
	}

	watcherIPlist := []string{}
	for _, watcher := range rbdStatusObj.Watchers {
		// split with separation '/' to remove nounce and concatenating `/32` to define a network with only one IP address
		address := strings.Split(watcher.Address, "/")[0] + "/32"
		watcherIPlist = append(watcherIPlist, address)
	}
	return watcherIPlist, nil
}

// func (c *clientCluster) fennceCephFSVolumes(
// 	ctx context.Context, node corev1.Node, cluster *cephv1.CephCluster,
// 	clusterInfo *cephclient.ClusterInfo, cephFSPV corev1.PersistentVolume) error {

// 	logger.Infof("cephfs PV NAME %v", cephFSPV.Spec.CSI.VolumeAttributes)

// 	status, err := cephclient.StatusWithUser(c.context, clusterInfo)
// 	if err != nil {
// 		return fmt.Errorf("failed to get ceph status for check active mds. %v", err)
// 	}

// 	var activeMDS string
// 	for _, fsRank := range status.Fsmap.ByRank {
// 		if fsRank.Status == "up:active" {
// 			activeMDS = fsRank.Name
// 		}
// 	}

// 	args := []string{"tell", fmt.Sprintf("mds.%s", activeMDS), "client", "ls", "--format", "json"}
// 	cmd := cephclient.NewCephCommand(c.context, clusterInfo, args)
// 	cmd.JsonOutput = true

// 	buf, err := cmd.Run()
// 	if err != nil {
// 		return fmt.Errorf("failed to list watchers for cephfs pool/subvoumeName %s/%s. %v", cephFSPV.Spec.CSI.VolumeAttributes["pool"], cephFSPV.Spec.CSI.VolumeAttributes["subvolumeName"], err)
// 	}
// 	ips, err := cephFSMDSClientMarshal(buf, cephFSPV)
// 	if err != nil || ips == nil {
// 		return fmt.Errorf("failed to unmarshal cephfs mds  output. %v", err)
// 	}

// 	err = c.createNetworkFence(ctx, cephFSPV, node, cluster, ips)
// 	if err != nil {
// 		return fmt.Errorf("failed to create network fence for node %q. %v", node.Name, err)
// 	}

// 	return nil
// }

// func cephFSMDSClientMarshal(output []byte, cephFSPV corev1.PersistentVolume) ([]string, error) {
// 	type entity struct {
// 		Addr struct {
// 			Addr  string `json:"addr"`
// 			Nonce int    `json:"nonce"`
// 		} `json:"addr"`
// 	}

// 	type clientMetadata struct {
// 		Root string `json:"root"`
// 	}

// 	type cephFSData struct {
// 		Entity         entity         `json:"entity"`
// 		ClientMetadata clientMetadata `json:"client_metadata"`
// 	}

// 	var data []cephFSData
// 	err := json.Unmarshal(output, &data)
// 	if err != nil {
// 		return []string{}, err
// 	}

// 	for _, d := range data {
// 		if cephFSPV.Spec.CSI.VolumeAttributes["subvolumePath"] == d.ClientMetadata.Root {
// 			logger.Infof("cephfs mds client ips to fence %v", d.Entity.Addr)
// 			return []string{d.Entity.Addr.Addr + "/32"}, nil
// 		}
// 	}

// 	return []string{}, nil
// }

func (c *clientCluster) createNetworkFence(ctx context.Context, pv corev1.PersistentVolume, node *corev1.Node, cluster *cephv1.CephCluster, cidr []string) error {
	logger.Warningf("Blocking node IP %s", cidr)

	secretName := pv.Annotations["volume.kubernetes.io/provisioner-deletion-secret-name"]
	secretNameSpace := pv.Annotations["volume.kubernetes.io/provisioner-deletion-secret-namespace"]
	if secretName == "" || secretNameSpace == "" {
		storageClass, err := c.context.Clientset.StorageV1().StorageClasses().Get(ctx, pv.Spec.StorageClassName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		secretName = storageClass.Parameters["csi.storage.k8s.io/provisioner-secret-name"]
		secretNameSpace = storageClass.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"]
	}

	networkFence := &addonsv1alpha1.NetworkFence{
		ObjectMeta: metav1.ObjectMeta{
			Name:      node.Name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cluster, cephv1.SchemeGroupVersion.WithKind("CephCluster")),
			},
		},
		Spec: addonsv1alpha1.NetworkFenceSpec{
			Driver:     pv.Spec.CSI.Driver,
			FenceState: addonsv1alpha1.Fenced,
			Secret: addonsv1alpha1.SecretSpec{
				Name:      secretName,
				Namespace: secretNameSpace,
			},
			Cidrs: cidr,
			Parameters: map[string]string{
				"clusterID": pv.Spec.CSI.VolumeAttributes["clusterID"],
			},
		},
	}

	err := c.client.Create(ctx, networkFence)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	logger.Infof("successfully created network fence CR %s", node.Name)

	return nil
}

func (c *clientCluster) unfenceAndDeleteNetworkFence(ctx context.Context, node corev1.Node, cluster *cephv1.CephCluster) error {
	logger.Infof("unfencing node %q", node.Name)
	networkFence := &addonsv1alpha1.NetworkFence{}
	err := c.client.Get(ctx, types.NamespacedName{Name: node.Name, Namespace: cluster.Namespace}, networkFence)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		return nil
	}

	// Unfencing is required to unblock the node and then delete the network fence CR
	networkFence.Spec.FenceState = addonsv1alpha1.Unfenced
	err = c.client.Update(ctx, networkFence)
	if err != nil {
		logger.Errorf("failed to unFence network fence CR. %v", err)
		return err
	}

	err = c.client.Delete(ctx, networkFence)
	if err != nil {
		logger.Errorf("failed to delete network fence CR. %v", err)
		return err
	}

	return nil
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
