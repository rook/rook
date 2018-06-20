/*
Copyright 2017 The Rook Authors. All rights reserved.

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

// Package flexvolume to manage Kubernetes storage attach events.
package flexvolume

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ClusterNamespaceKey   = "clusterNamespace"
	StorageClassKey       = "storageClass"
	PoolKey               = "pool"
	ImageKey              = "image"
	kubeletDefaultRootDir = "/var/lib/kubelet"
)

var driverLogger = capnslog.NewPackageLogger("github.com/rook/rook", "flexdriver")

// Controller handles all events from the Flexvolume driver
type Controller struct {
	context          *clusterd.Context
	volumeManager    VolumeManager
	volumeAttachment attachment.Attachment
}

type ClientAccessInfo struct {
	MonAddresses []string `json:"monAddresses"`
	UserName     string   `json:"userName"`
	SecretKey    string   `json:"secretKey"`
}

func NewController(context *clusterd.Context, volumeAttachment attachment.Attachment, manager VolumeManager) *Controller {

	return &Controller{
		context:          context,
		volumeAttachment: volumeAttachment,
		volumeManager:    manager,
	}
}

// Attach attaches rook volume to the node
func (c *Controller) Attach(attachOpts AttachOptions, devicePath *string) error {

	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	node := os.Getenv(k8sutil.NodeNameEnvVar)

	// Name of CRD is the PV name. This is done so that the CRD can be use for fencing
	crdName := attachOpts.VolumeName

	// Check if this volume has been attached
	volumeattachObj, err := c.volumeAttachment.Get(namespace, crdName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get volume CRD %s. %+v", crdName, err)
		}
		// No volumeattach CRD for this volume found. Create one
		volumeattachObj = rookalpha.NewVolume(crdName, namespace, node, attachOpts.PodNamespace, attachOpts.Pod,
			attachOpts.ClusterNamespace, attachOpts.MountDir, strings.ToLower(attachOpts.RW) == ReadOnly)
		logger.Infof("Creating Volume attach Resource %s/%s: %+v", volumeattachObj.Namespace, volumeattachObj.Name, attachOpts)
		err = c.volumeAttachment.Create(volumeattachObj)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create volume CRD %s. %+v", crdName, err)
			}
			// Some other attacher beat us in this race. Kubernetes will retry again.
			return fmt.Errorf("failed to attach volume %s for pod %s/%s. Volume is already attached by a different pod",
				crdName, attachOpts.PodNamespace, attachOpts.Pod)
		}
	} else {
		// Volume has already been attached.
		// find if the attachment object has been previously created.
		// This could be in the case of a multiple attachment for ROs or
		// it could be the the Volume record was created previously and
		// the attach operation failed and Kubernetes retried.
		found := false
		for _, a := range volumeattachObj.Attachments {
			if a.MountDir == attachOpts.MountDir {
				found = true
			}
		}

		if !found {
			// Check if there is already an attachment with RW.
			index := getPodRWAttachmentObject(volumeattachObj)
			if index != -1 {
				// check if the RW attachment is orphaned.
				attachment := &volumeattachObj.Attachments[index]

				logger.Infof("Volume attachment record %s/%s exists for pod: %s/%s", volumeattachObj.Namespace, volumeattachObj.Name, attachment.PodNamespace, attachment.PodName)
				// Note this could return the reference of the pod who is requesting the attach if this pod have the same name as the pod in the attachment record.
				pod, err := c.context.Clientset.CoreV1().Pods(attachment.PodNamespace).Get(attachment.PodName, metav1.GetOptions{})
				if err != nil || (attachment.PodNamespace == attachOpts.PodNamespace && attachment.PodName == attachOpts.Pod) {
					if err != nil && !errors.IsNotFound(err) {
						return fmt.Errorf("failed to get pod CRD %s/%s. %+v", attachment.PodNamespace, attachment.PodName, err)
					}

					logger.Infof("Volume attachment record %s/%s is orphaned. Updating record with new attachment information for pod %s/%s", volumeattachObj.Namespace, volumeattachObj.Name, attachOpts.PodNamespace, attachOpts.Pod)

					// Attachment is orphaned. Update attachment record and proceed with attaching
					attachment.Node = node
					attachment.MountDir = attachOpts.MountDir
					attachment.PodNamespace = attachOpts.PodNamespace
					attachment.PodName = attachOpts.Pod
					attachment.ClusterName = attachOpts.ClusterNamespace
					attachment.ReadOnly = attachOpts.RW == ReadOnly
					err = c.volumeAttachment.Update(volumeattachObj)
					if err != nil {
						return fmt.Errorf("failed to update volume CRD %s. %+v", crdName, err)
					}
				} else {
					// Attachment is not orphaned. Original pod still exists. Dont attach.
					return fmt.Errorf("failed to attach volume %s for pod %s/%s. Volume is already attached by pod %s/%s. Status %+v",
						crdName, attachOpts.PodNamespace, attachOpts.Pod, attachment.PodNamespace, attachment.PodName, pod.Status.Phase)
				}
			} else {
				// No RW attachment found. Check if this is a RW attachment request.
				// We only support RW once attachment. No mixing either with RO
				if attachOpts.RW == "rw" && len(volumeattachObj.Attachments) > 0 {
					return fmt.Errorf("failed to attach volume %s for pod %s/%s. Volume is already attached by one or more pods",
						crdName, attachOpts.PodNamespace, attachOpts.Pod)
				}

				// Create a new attachment record and proceed with attaching
				newAttach := rookalpha.Attachment{
					Node:         node,
					PodNamespace: attachOpts.PodNamespace,
					PodName:      attachOpts.Pod,
					ClusterName:  attachOpts.ClusterNamespace,
					MountDir:     attachOpts.MountDir,
					ReadOnly:     attachOpts.RW == ReadOnly,
				}
				volumeattachObj.Attachments = append(volumeattachObj.Attachments, newAttach)
				err = c.volumeAttachment.Update(volumeattachObj)
				if err != nil {
					return fmt.Errorf("failed to update volume CRD %s. %+v", crdName, err)
				}
			}
		}
	}
	*devicePath, err = c.volumeManager.Attach(attachOpts.Image, attachOpts.Pool, attachOpts.ClusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to attach volume %s/%s: %+v", attachOpts.Pool, attachOpts.Image, err)
	}
	return nil
}

// Detach detaches a rook volume to the node
func (c *Controller) Detach(detachOpts AttachOptions, _ *struct{} /* void reply */) error {
	return c.doDetach(detachOpts, false /* force */)
}

func (c *Controller) DetachForce(detachOpts AttachOptions, _ *struct{} /* void reply */) error {
	return c.doDetach(detachOpts, true /* force */)
}

func (c *Controller) doDetach(detachOpts AttachOptions, force bool) error {
	err := c.volumeManager.Detach(detachOpts.Image, detachOpts.Pool, detachOpts.ClusterNamespace, force)
	if err != nil {
		return fmt.Errorf("Failed to detach volume %s/%s: %+v", detachOpts.Pool, detachOpts.Image, err)
	}

	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	crdName := detachOpts.VolumeName
	volumeAttach, err := c.volumeAttachment.Get(namespace, crdName)
	if len(volumeAttach.Attachments) == 0 {
		logger.Infof("Deleting Volume CRD %s/%s", namespace, crdName)
		return c.volumeAttachment.Delete(namespace, crdName)
	}
	return nil
}

// RemoveAttachmentObject removes the attachment from the Volume CRD and returns whether the volume is safe to detach
func (c *Controller) RemoveAttachmentObject(detachOpts AttachOptions, safeToDetach *bool) error {
	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	crdName := detachOpts.VolumeName
	logger.Infof("Deleting attachment for mountDir %s from Volume attach CRD %s/%s", detachOpts.MountDir, namespace, crdName)
	volumeAttach, err := c.volumeAttachment.Get(namespace, crdName)
	if err != nil {
		return fmt.Errorf("failed to get Volume attach CRD %s/%s: %+v", namespace, crdName, err)
	}
	node := os.Getenv(k8sutil.NodeNameEnvVar)
	nodeAttachmentCount := 0
	needUpdate := false
	for i, v := range volumeAttach.Attachments {
		if v.Node == node {
			nodeAttachmentCount++
			if v.MountDir == detachOpts.MountDir {
				// Deleting slice
				volumeAttach.Attachments = append(volumeAttach.Attachments[:i], volumeAttach.Attachments[i+1:]...)
				needUpdate = true
			}
		}
	}

	if needUpdate {
		// only one attachment on this node, which is the one that got removed.
		if nodeAttachmentCount == 1 {
			*safeToDetach = true
		}
		return c.volumeAttachment.Update(volumeAttach)
	}
	return fmt.Errorf("Volume CRD %s found but attachment to the mountDir %s was not found", crdName, detachOpts.MountDir)
}

// Log logs messages from the driver
func (c *Controller) Log(message LogMessage, _ *struct{} /* void reply */) error {
	if message.IsError {
		driverLogger.Error(message.Message)
	} else {
		driverLogger.Info(message.Message)
	}
	return nil
}

func (c *Controller) parseClusterNamespace(storageClassName string) (string, error) {
	sc, err := c.context.Clientset.Storage().StorageClasses().Get(storageClassName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	clusterNamespace, ok := sc.Parameters["clusterNamespace"]
	if !ok {
		// Checks for older version of parameter i.e., clusterName if clusterNamespace not found
		logger.Infof("clusterNamespace not specified in the storage class %s. Checking for clusterName", storageClassName)
		clusterNamespace, ok := sc.Parameters["clusterName"]
		if !ok {
			// Defaults to rook if not found
			logger.Infof("clusterNamespace not specified in the storage class %s. Defaulting to '%s'", storageClassName, cluster.DefaultClusterName)
			return cluster.DefaultClusterName, nil
		}
		return clusterNamespace, nil
	}
	return clusterNamespace, nil
}

// GetAttachInfoFromMountDir obtain pod and volume information from the mountDir. K8s does not provide
// all necessary information to detach a volume (https://github.com/kubernetes/kubernetes/issues/52590).
// So we are hacking a bit and by parsing it from mountDir
func (c *Controller) GetAttachInfoFromMountDir(mountDir string, attachOptions *AttachOptions) error {

	if attachOptions.PodID == "" {
		podID, pvName, err := getPodAndPVNameFromMountDir(mountDir)
		if err != nil {
			return err
		}
		attachOptions.PodID = podID
		attachOptions.VolumeName = pvName
	}

	pv, err := c.context.Clientset.CoreV1().PersistentVolumes().Get(attachOptions.VolumeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get persistent volume %s: %+v", attachOptions.VolumeName, err)
	}

	if attachOptions.PodNamespace == "" {
		// pod namespace should be the same as the PVC namespace
		attachOptions.PodNamespace = pv.Spec.ClaimRef.Namespace
	}

	node := os.Getenv(k8sutil.NodeNameEnvVar)
	if attachOptions.Pod == "" {
		// Find all pods scheduled to this node
		opts := metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("spec.nodeName", node).String(),
		}
		pods, err := c.context.Clientset.CoreV1().Pods(attachOptions.PodNamespace).List(opts)
		if err != nil {
			return fmt.Errorf("failed to get pods in namespace %s: %+v", attachOptions.PodNamespace, err)
		}

		pod := findPodByID(pods, types.UID(attachOptions.PodID))
		if pod != nil {
			attachOptions.Pod = pod.GetName()
		}
	}

	if attachOptions.Image == "" {
		attachOptions.Image = pv.Spec.PersistentVolumeSource.FlexVolume.Options[ImageKey]
	}
	if attachOptions.Pool == "" {
		attachOptions.Pool = pv.Spec.PersistentVolumeSource.FlexVolume.Options[PoolKey]
	}
	if attachOptions.StorageClass == "" {
		attachOptions.StorageClass = pv.Spec.PersistentVolumeSource.FlexVolume.Options[StorageClassKey]
	}
	attachOptions.ClusterNamespace, err = c.parseClusterNamespace(attachOptions.StorageClass)
	if err != nil {
		return fmt.Errorf("Failed to parse clusterNamespace from storageClass %s: %+v", attachOptions.StorageClass, err)
	}
	return nil
}

// GetGlobalMountPath generate the global mount path where the device path is mounted.
// It is based on the kubelet root dir, which defaults to /var/lib/kubelet
func (c *Controller) GetGlobalMountPath(input GlobalMountPathInput, globalMountPath *string) error {
	vendor, driver, err := getFlexDriverInfo(input.DriverDir)
	if err != nil {
		return err
	}

	*globalMountPath = path.Join(c.getKubeletRootDir(), "plugins", vendor, driver, "mounts", input.VolumeName)
	return nil
}

// GetClientAccessInfo obtains the cluster monitor endpoints, username and secret
func (c *Controller) GetClientAccessInfo(clusterNamespace string, clientAccessInfo *ClientAccessInfo) error {
	clusterInfo, _, _, err := mon.LoadClusterInfo(c.context, clusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to load cluster information from clusters namespace %s: %+v", clusterNamespace, err)
	}

	monEndpoints := make([]string, 0, len(clusterInfo.Monitors))
	for _, monitor := range clusterInfo.Monitors {
		monEndpoints = append(monEndpoints, monitor.Endpoint)
	}

	clientAccessInfo.MonAddresses = monEndpoints
	clientAccessInfo.SecretKey = clusterInfo.AdminSecret
	clientAccessInfo.UserName = "admin"

	return nil
}

// GetKernelVersion returns the kernel version of the current node.
func (c *Controller) GetKernelVersion(_ *struct{} /* no inputs */, kernelVersion *string) error {
	nodeName := os.Getenv(k8sutil.NodeNameEnvVar)
	node, err := c.context.Clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get kernel version from node information for node %s: %+v", nodeName, err)
	}
	*kernelVersion = node.Status.NodeInfo.KernelVersion
	return nil
}

// getKubeletRootDir queries the kubelet configuration to find the kubelet root dir. Defaults to /var/lib/kubelet
func (c *Controller) getKubeletRootDir() string {
	nodeConfigURI, err := k8sutil.NodeConfigURI()
	if err != nil {
		logger.Warningf(err.Error())
		return kubeletDefaultRootDir
	}

	// determining where the path of the kubelet root dir and flexvolume dir on the node
	nodeConfig, err := c.context.Clientset.CoreV1().RESTClient().Get().RequestURI(nodeConfigURI).DoRaw()
	if err != nil {
		logger.Warningf("unable to query node configuration: %v", err)
		return kubeletDefaultRootDir
	}
	configKubelet := agent.NodeConfigKubelet{}
	if err := json.Unmarshal(nodeConfig, &configKubelet); err != nil {
		logger.Warningf("unable to parse node config from Kubelet: %+v", err)
		return kubeletDefaultRootDir
	}

	// in k8s 1.8 it does not appear possible to change the default root dir
	// see https://github.com/rook/rook/issues/1282
	return kubeletDefaultRootDir
}

// getPodAndPVNameFromMountDir parses pod information from the mountDir
func getPodAndPVNameFromMountDir(mountDir string) (string, string, error) {
	// mountDir is in the form of <rootDir>/pods/<podID>/volumes/rook.io~rook/<pv name>
	filepath.Clean(mountDir)
	token := strings.Split(mountDir, string(filepath.Separator))
	// token lenght should at least size 5
	length := len(token)
	if length < 5 {
		return "", "", fmt.Errorf("failed to parse mountDir %s for CRD name and podID", mountDir)
	}
	return token[length-4], token[length-1], nil
}

func findPodByID(pods *v1.PodList, podUID types.UID) *v1.Pod {
	for i := range pods.Items {
		if pods.Items[i].GetUID() == podUID {
			return &(pods.Items[i])
		}
	}
	return nil
}

// getPodRWAttachmentObject loops through the list of attachments of the Volume
// resource and returns the index of the first RW attachment object
func getPodRWAttachmentObject(volumeAttachmentObject *rookalpha.Volume) int {
	for i, a := range volumeAttachmentObject.Attachments {
		if !a.ReadOnly {
			return i
		}
	}
	return -1
}
