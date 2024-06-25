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
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	addonsv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/apis/csiaddons/v1alpha1"
	pkgerror "github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// clientCluster struct contains a client to interact with Kubernetes object
// as well as the NamespacedName (used in requests)
type clientCluster struct {
	client    client.Client
	namespace string
	context   *clusterd.Context
}

var (
	nodesCheckedForReconcile = sets.New[string]()
	networkFenceLabel        = "cephClusterUID"
	errActiveClientNotFound  = stderrors.New("active client not found")
)

// drivers that supports fencing, used in naming networkFence object
const (
	rbdDriver = "rbd"
)

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

// onK8sNode is triggered when a node is added in the Kubernetes cluster
func (c *clientCluster) onK8sNode(ctx context.Context, object runtime.Object) bool {
	node, ok := object.(*corev1.Node)
	if !ok {
		return false
	}

	// Get CephCluster
	cluster := c.getCephCluster()

	// Continue reconcile in case of failure too since we don't want to block other node reconcile
	if err := c.handleNodeFailure(ctx, cluster, node); err != nil {
		logger.Errorf("failed to handle node failure. %v", err)
	}

	// skip reconcile if node is already checked in a previous reconcile
	if nodesCheckedForReconcile.Has(node.Name) {
		return false
	}

	if !k8sutil.GetNodeSchedulable(*node) {
		logger.Debugf("node watcher: skipping cluster update. added node %q is unschedulable", node.Labels[corev1.LabelHostname])
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
		hostname, ok := node.Labels[corev1.LabelHostname]
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
			logger.Infof("node watcher: adding node %q to cluster %q", node.Labels[corev1.LabelHostname], cluster.Namespace)
			return true
		}

		// This is Debug level because the node receives frequent updates and this will pollute the logs
		logger.Debugf("node watcher: node %q is already an OSD node with %q", nodeName, osds)
	}
	return false
}

func (c *clientCluster) handleNodeFailure(ctx context.Context, cluster *cephv1.CephCluster, node *corev1.Node) error {
	watchForNodeLoss, err := k8sutil.GetOperatorSetting(ctx, c.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_WATCH_FOR_NODE_FAILURE", "true")
	if err != nil {
		return pkgerror.Wrapf(err, "failed to get configmap value `ROOK_WATCH_FOR_NODE_FAILURE`.")
	}

	if strings.ToLower(watchForNodeLoss) != "true" {
		logger.Debugf("not watching for node failures since `ROOK_WATCH_FOR_NODE_FAILURE` is set to %q", watchForNodeLoss)
		return nil
	}

	disabledCSI, err := k8sutil.GetOperatorSetting(ctx, c.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_DISABLE_DRIVER", "false")
	if err != nil {
		return pkgerror.Wrapf(err, "failed to get configmap value `ROOK_CSI_DISABLE_DRIVER`.")
	}

	if strings.ToLower(disabledCSI) != "false" {
		logger.Debugf("not watching for node failures since `ROOK_CSI_DISABLE_DRIVER` is set to %q, skip creating networkFence", disabledCSI)
		return nil
	}

	_, err = c.context.ApiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, "networkfences.csiaddons.openshift.io", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debug("networkfences.csiaddons.openshift.io CRD not found, skip creating networkFence")
			return nil
		}
		return pkgerror.Wrapf(err, "failed to get networkfences.csiaddons.openshift.io CRD, skip creating networkFence")
	}

	nodeHasOutOfServiceTaint := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == corev1.TaintNodeOutOfService {
			nodeHasOutOfServiceTaint = true
			logger.Infof("Found taint: Key=%v, Value=%v on node %s\n", taint.Key, taint.Value, node.Name)
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

	err = c.unfenceAndDeleteNetworkFence(ctx, *node, cluster, rbdDriver)
	if err != nil {
		return pkgerror.Wrapf(err, "failed to delete rbd network fence for node %q.", node.Name)
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

	rbdVolumesInUse := getCephVolumesInUse(cluster, volumesInuse)
	if len(rbdVolumesInUse) == 0 {
		logger.Debugf("no rbd  volumes in use for out of service node %q", node.Name)
		return nil
	}

	listPVs, err := c.context.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return pkgerror.Wrapf(err, "failed to list PV")
	}

	if len(rbdVolumesInUse) != 0 {
		rbdPVList := listRBDPV(listPVs, cluster, rbdVolumesInUse)
		if len(rbdPVList) == 0 {
			logger.Debug("No rbd PVs found on the node")
		} else {
			logger.Infof("node %q require fencing, found rbd volumes in use", node.Name)
			clusterInfo, _, _, err := opcontroller.LoadClusterInfo(c.context, ctx, cluster.Namespace, &cluster.Spec)
			if err != nil {
				return pkgerror.Wrapf(err, "Failed to load cluster info.")
			}

			for i := range rbdPVList {
				err = c.fenceRbdImage(ctx, node, cluster, clusterInfo, rbdPVList[i])
				// We only need to create the network fence for any one of rbd pv.
				if err == nil {
					break
				}
				// continue to fence next rbd volume if active client not found
				if stderrors.Is(err, errActiveClientNotFound) {
					continue
				}

				if i == len(rbdPVList)-1 {
					return pkgerror.Wrapf(err, "failed to fence rbd volumes")
				}
				logger.Errorf("failed to fence rbd volumes %q, trying next rbd volume", rbdPVList[i].Name)
			}
		}
	}

	return nil
}

func getCephVolumesInUse(cluster *cephv1.CephCluster, volumesInUse []corev1.UniqueVolumeName) []string {
	var rbdVolumesInUse []string

	for _, volume := range volumesInUse {
		splitVolumeInUseBased := trimeVolumeInUse(volume)
		logger.Infof("volumeInUse after split based on '^' %v", splitVolumeInUseBased)

		if len(splitVolumeInUseBased) == 2 && splitVolumeInUseBased[0] == fmt.Sprintf("%s.rbd.csi.ceph.com", cluster.Namespace) {
			rbdVolumesInUse = append(rbdVolumesInUse, splitVolumeInUseBased[1])
		}
	}

	return rbdVolumesInUse
}

func trimeVolumeInUse(volume corev1.UniqueVolumeName) []string {
	volumesInuseRemoveK8sPrefix := strings.TrimPrefix(string(volume), "kubernetes.io/csi/")
	splitVolumeInUseBased := strings.Split(volumesInuseRemoveK8sPrefix, "^")
	return splitVolumeInUseBased
}

func listRBDPV(listPVs *corev1.PersistentVolumeList, cluster *cephv1.CephCluster, rbdVolumesInUse []string) []corev1.PersistentVolume {
	var listRbdPV []corev1.PersistentVolume

	for _, pv := range listPVs.Items {
		// Skip if pv is not provisioned by CSI
		if pv.Spec.CSI == nil {
			logger.Debugf("pv %q is not provisioned by CSI", pv.Name)
			continue
		}

		if pv.Spec.CSI.Driver == fmt.Sprintf("%s.rbd.csi.ceph.com", cluster.Namespace) {
			// Ignore PVs that support multinode access (RWX, ROX), since they can be mounted on multiple nodes.
			if pvSupportsMultiNodeAccess(pv.Spec.AccessModes) {
				continue
			}
			if pv.Spec.CSI.VolumeAttributes["staticVolume"] == "true" || pv.Spec.CSI.VolumeAttributes["pool"] == "" || pv.Spec.CSI.VolumeAttributes["imageName"] == "" {
				logger.Debugf("skipping, static pv %q", pv.Name)
				continue
			}

			for _, rbdVolume := range rbdVolumesInUse {
				if pv.Spec.CSI.VolumeHandle == rbdVolume {
					listRbdPV = append(listRbdPV, pv)
				}
			}
		}
	}
	return listRbdPV
}

// pvSupportsMultiNodeAccess returns true if the PV access modes contain ReadWriteMany or ReadOnlyMany.
func pvSupportsMultiNodeAccess(accessModes []corev1.PersistentVolumeAccessMode) bool {
	for _, accessMode := range accessModes {
		switch accessMode {
		case corev1.ReadOnlyMany, corev1.ReadWriteMany:
			return true
		}
	}

	return false
}

func (c *clientCluster) fenceRbdImage(
	ctx context.Context, node *corev1.Node, cluster *cephv1.CephCluster,
	clusterInfo *cephclient.ClusterInfo, rbdPV corev1.PersistentVolume) error {

	logger.Debugf("rbd PV NAME %v", rbdPV.Spec.CSI.VolumeAttributes)
	args := []string{"status", fmt.Sprintf("%s/%s", rbdPV.Spec.CSI.VolumeAttributes["pool"], rbdPV.Spec.CSI.VolumeAttributes["imageName"])}
	cmd := cephclient.NewRBDCommand(c.context, clusterInfo, args)
	cmd.JsonOutput = true

	buf, err := cmd.Run()
	if err != nil {
		return pkgerror.Wrapf(err, "failed to list watchers for pool/imageName %s/%s.", rbdPV.Spec.CSI.VolumeAttributes["pool"], rbdPV.Spec.CSI.VolumeAttributes["imageName"])
	}

	ips, err := rbdStatusUnMarshal(buf)
	if err != nil {
		return pkgerror.Wrapf(err, "failed to unmarshal rbd status output")
	}
	if len(ips) == 0 {
		logger.Infof("no active rbd clients found for rbd volume %q", rbdPV.Name)
		return errActiveClientNotFound
	}
	err = c.createNetworkFence(ctx, rbdPV, node, cluster, ips, rbdDriver)
	if err != nil {
		return pkgerror.Wrapf(err, "failed to create network fence for node %q", node.Name)
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
		watcherIP := concatenateWatcherIp(watcher.Address)
		watcherIPlist = append(watcherIPlist, watcherIP)
	}
	return watcherIPlist, nil
}

func concatenateWatcherIp(address string) string {
	// address is in format `10.63.0.5:0/1254753579` for rbd
	// split with separation ':0' to remove nounce and concatenating `/32` to define a network with only one IP address
	watcherIP := strings.Split(address, ":0")[0] + "/32"
	return watcherIP
}

func fenceResourceName(nodeName, driver, namespace string) string {
	return fmt.Sprintf("%s-%s-%s", nodeName, driver, namespace)
}

func (c *clientCluster) createNetworkFence(ctx context.Context, pv corev1.PersistentVolume, node *corev1.Node, cluster *cephv1.CephCluster, cidr []string, driver string) error {
	logger.Warningf("Blocking node IP %s", cidr)

	secretName := pv.Annotations["volume.kubernetes.io/provisioner-deletion-secret-name"]
	secretNameSpace := pv.Annotations["volume.kubernetes.io/provisioner-deletion-secret-namespace"]
	if secretName == "" || secretNameSpace == "" {
		storageClass, err := c.context.Clientset.StorageV1().StorageClasses().Get(ctx, pv.Spec.StorageClassName, metav1.GetOptions{})
		if err != nil {
			return pkgerror.Wrap(err, "failed to get storage class to fence volume")
		}
		secretName = storageClass.Parameters["csi.storage.k8s.io/provisioner-secret-name"]
		secretNameSpace = storageClass.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"]
	}

	networkFence := &addonsv1alpha1.NetworkFence{
		ObjectMeta: metav1.ObjectMeta{
			Name: fenceResourceName(node.Name, driver, cluster.Namespace),
			Labels: map[string]string{
				networkFenceLabel: string(cluster.GetUID()),
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

	logger.Infof("successfully created network fence CR for node %q", node.Name)

	return nil
}

func (c *clientCluster) unfenceAndDeleteNetworkFence(ctx context.Context, node corev1.Node, cluster *cephv1.CephCluster, driver string) error {
	networkFence := &addonsv1alpha1.NetworkFence{}
	err := c.client.Get(ctx, types.NamespacedName{Name: fenceResourceName(node.Name, driver, cluster.Namespace)}, networkFence)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		return nil
	}
	logger.Infof("node %s does not have taint %s, unfencing networkFence CR", node.Name, corev1.TaintNodeOutOfService)

	// Unfencing is required to unblock the node and then delete the network fence CR
	networkFence.Spec.FenceState = addonsv1alpha1.Unfenced
	err = c.client.Update(ctx, networkFence)
	if err != nil {
		logger.Errorf("failed to unFence network fence CR. %v", err)
		return err
	}

	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		err = c.client.Get(ctx, types.NamespacedName{Name: fenceResourceName(node.Name, driver, cluster.Namespace)}, networkFence)
		if err != nil && !errors.IsNotFound(err) {
			return false, err
		}

		if networkFence.Status.Message != addonsv1alpha1.UnFenceOperationSuccessfulMessage {
			logger.Infof("waiting for network fence CR %q status to get result %q", networkFence.Name, addonsv1alpha1.UnFenceOperationSuccessfulMessage)
			return false, err
		}

		logger.Infof("successfully unfenced %q network fence cr %q, proceeding with deletion", driver, networkFence.Name)

		err = c.client.Delete(ctx, networkFence)
		if err == nil || errors.IsNotFound(err) {
			logger.Infof("successfully deleted network fence CR %s", networkFence.Name)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return pkgerror.Wrapf(err, "timeout out deleting the %s network fence CR %s", driver, networkFence.Name)
	}

	return nil
}

// onDeviceCMUpdate is trigger when the hot plug config map is updated
func (c *clientCluster) onDeviceCMUpdate(oldObj, newObj runtime.Object) bool {
	oldCm, ok := oldObj.(*corev1.ConfigMap)
	if !ok {
		return false
	}
	logger.Debugf("hot-plug cm watcher: onDeviceCMUpdate old device cm: %+v", oldCm)

	newCm, ok := newObj.(*corev1.ConfigMap)
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
