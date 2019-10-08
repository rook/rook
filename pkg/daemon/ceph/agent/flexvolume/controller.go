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
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rook/rook/pkg/util/display"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// ClusterNamespaceKey key for cluster namespace option.
	ClusterNamespaceKey = "clusterNamespace"
	// ClusterNameKey key for cluster name option (deprecated).
	ClusterNameKey = "clusterName"
	// StorageClassKey key for storage class name option.
	StorageClassKey = "storageClass"
	// PoolKey key for pool name option.
	PoolKey = "pool"
	// BlockPoolKey key for blockPool name option.
	BlockPoolKey = "blockPool"
	// PoolKey key for image name option.
	ImageKey = "image"
	// PoolKey key for data pool name option.
	DataBlockPoolKey      = "dataBlockPool"
	kubeletDefaultRootDir = "/var/lib/kubelet"
)

var driverLogger = capnslog.NewPackageLogger("github.com/rook/rook", "flexdriver")

// Controller handles all events from the Flexvolume driver
type Controller struct {
	context           *clusterd.Context
	volumeManager     VolumeManager
	volumeAttachment  attachment.Attachment
	mountSecurityMode string
}

// ClientAccessInfo hols info for Ceph access
type ClientAccessInfo struct {
	MonAddresses []string `json:"monAddresses"`
	UserName     string   `json:"userName"`
	SecretKey    string   `json:"secretKey"`
}

// NewController create a new controller to handle events from the flexvolume driver
func NewController(context *clusterd.Context, volumeAttachment attachment.Attachment, manager VolumeManager, mountSecurityMode string) *Controller {
	return &Controller{
		context:           context,
		volumeAttachment:  volumeAttachment,
		volumeManager:     manager,
		mountSecurityMode: mountSecurityMode,
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
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get volume CRD %s", crdName)
		}
		// No volumeattach CRD for this volume found. Create one
		volumeattachObj = rookalpha.NewVolume(
			crdName,
			namespace,
			node,
			attachOpts.PodNamespace,
			attachOpts.Pod,
			attachOpts.ClusterNamespace,
			attachOpts.MountDir,
			strings.ToLower(attachOpts.RW) == ReadOnly,
		)
		logger.Infof("creating Volume attach Resource %s/%s: %+v", volumeattachObj.Namespace, volumeattachObj.Name, attachOpts)
		err = c.volumeAttachment.Create(volumeattachObj)
		if err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create volume CRD %s", crdName)
			}
			// Some other attacher beat us in this race. Kubernetes will retry again.
			return errors.Errorf("failed to attach volume %s for pod %s/%s. Volume is already attached by a different pod",
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

				logger.Infof("volume attachment record %s/%s exists for pod: %s/%s", volumeattachObj.Namespace, volumeattachObj.Name, attachment.PodNamespace, attachment.PodName)
				// Note this could return the reference of the pod who is requesting the attach if this pod have the same name as the pod in the attachment record.
				allowAttach := false
				pod, err := c.context.Clientset.CoreV1().Pods(attachment.PodNamespace).Get(attachment.PodName, metav1.GetOptions{})
				if err != nil {
					if !kerrors.IsNotFound(err) {
						return errors.Wrapf(err, "failed to get pod CRD %s/%s", attachment.PodNamespace, attachment.PodName)
					}
					allowAttach = true
					logger.Infof("volume attachment record %s/%s is orphaned. Updating record with new attachment information for pod %s/%s", volumeattachObj.Namespace, volumeattachObj.Name, attachOpts.PodNamespace, attachOpts.Pod)
				}
				if err == nil && (attachment.PodNamespace == attachOpts.PodNamespace && attachment.PodName == attachOpts.Pod && attachment.Node == node) {
					allowAttach = true
					logger.Infof("volume attachment record %s/%s is starting on the same node. Updating record with new attachment information for pod %s/%s", volumeattachObj.Namespace, volumeattachObj.Name, attachOpts.PodNamespace, attachOpts.Pod)
				}
				if allowAttach {
					// Update attachment record and proceed with attaching
					attachment.Node = node
					attachment.MountDir = attachOpts.MountDir
					attachment.PodNamespace = attachOpts.PodNamespace
					attachment.PodName = attachOpts.Pod
					attachment.ClusterName = attachOpts.ClusterNamespace
					attachment.ReadOnly = attachOpts.RW == ReadOnly
					err = c.volumeAttachment.Update(volumeattachObj)
					if err != nil {
						return errors.Wrapf(err, "failed to update volume CRD %s", crdName)
					}
				} else {
					// Attachment is not orphaned. Original pod still exists. Don't attach.
					return errors.Errorf("failed to attach volume %s for pod %s/%s. Volume is already attached by pod %s/%s. Status %+v",
						crdName, attachOpts.PodNamespace, attachOpts.Pod, attachment.PodNamespace, attachment.PodName, pod.Status.Phase)
				}
			} else {
				// No RW attachment found. Check if this is a RW attachment request.
				// We only support RW once attachment. No mixing either with RO
				if attachOpts.RW == "rw" && len(volumeattachObj.Attachments) > 0 {
					return errors.Errorf("failed to attach volume %s for pod %s/%s. Volume is already attached by one or more pods",
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
					return errors.Wrapf(err, "failed to update volume CRD %s", crdName)
				}
			}
		}
	}
	*devicePath, err = c.volumeManager.Attach(attachOpts.Image, attachOpts.BlockPool, attachOpts.MountUser, attachOpts.MountSecret, attachOpts.ClusterNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to attach volume %s/%s", attachOpts.BlockPool, attachOpts.Image)
	}
	return nil
}

// Expand RBD image
func (c *Controller) Expand(expandArgs ExpandArgs, _ *struct{}) error {
	expandOpts := expandArgs.ExpandOptions
	sizeInMb := display.BToMb(expandArgs.Size)
	err := c.volumeManager.Expand(expandOpts.Image, expandOpts.Pool, expandOpts.ClusterNamespace, sizeInMb)
	if err != nil {
		return errors.Wrapf(err, "failed to resize volume %s/%s", expandOpts.Pool, expandOpts.Image)
	}
	return nil
}

// Detach detaches a rook volume to the node
func (c *Controller) Detach(detachOpts AttachOptions, _ *struct{} /* void reply */) error {
	return c.doDetach(detachOpts, false /* force */)
}

// DetachForce forces a detach on a rook volume to the node
func (c *Controller) DetachForce(detachOpts AttachOptions, _ *struct{} /* void reply */) error {
	return c.doDetach(detachOpts, true /* force */)
}

func (c *Controller) doDetach(detachOpts AttachOptions, force bool) error {
	if err := c.volumeManager.Detach(
		detachOpts.Image,
		detachOpts.BlockPool,
		detachOpts.MountUser,
		detachOpts.MountSecret,
		detachOpts.ClusterNamespace,
		force,
	); err != nil {
		return errors.Wrapf(err, "failed to detach volume %s/%s", detachOpts.BlockPool, detachOpts.Image)
	}

	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	crdName := detachOpts.VolumeName
	volumeAttach, err := c.volumeAttachment.Get(namespace, crdName)
	if err != nil {
		return errors.Wrapf(err, "failed to get VolumeAttachment for %s in namespace %s", crdName, namespace)
	}
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
	logger.Infof("deleting attachment for mountDir %s from Volume attach CRD %s/%s", detachOpts.MountDir, namespace, crdName)
	volumeAttach, err := c.volumeAttachment.Get(namespace, crdName)
	if err != nil {
		return errors.Wrapf(err, "failed to get Volume attach CRD %s/%s", namespace, crdName)
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
	return errors.Errorf("volume CRD %s found but attachment to the mountDir %s was not found", crdName, detachOpts.MountDir)
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
	sc, err := c.context.Clientset.StorageV1().StorageClasses().Get(storageClassName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	clusterNamespace, ok := sc.Parameters["clusterNamespace"]
	if !ok {
		// Checks for older version of parameter i.e., clusterName if clusterNamespace not found
		logger.Infof("clusterNamespace not specified in the storage class %s. Checking for clusterName", storageClassName)
		clusterNamespace, ok = sc.Parameters["clusterName"]
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
		return errors.Wrapf(err, "failed to get persistent volume %s", attachOptions.VolumeName)
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
			return errors.Wrapf(err, "failed to get pods in namespace %s", attachOptions.PodNamespace)
		}

		pod := findPodByID(pods, types.UID(attachOptions.PodID))
		if pod != nil {
			attachOptions.Pod = pod.GetName()
		}
	}

	if attachOptions.Image == "" {
		attachOptions.Image = pv.Spec.PersistentVolumeSource.FlexVolume.Options[ImageKey]
	}
	if attachOptions.BlockPool == "" {
		attachOptions.BlockPool = pv.Spec.PersistentVolumeSource.FlexVolume.Options[BlockPoolKey]
		if attachOptions.BlockPool == "" {
			// fall back to the "pool" if the "blockPool" is not set
			attachOptions.BlockPool = pv.Spec.PersistentVolumeSource.FlexVolume.Options[PoolKey]
		}
	}
	if attachOptions.StorageClass == "" {
		attachOptions.StorageClass = pv.Spec.PersistentVolumeSource.FlexVolume.Options[StorageClassKey]
	}
	if attachOptions.MountUser == "" {
		attachOptions.MountUser = "admin"
	}
	attachOptions.ClusterNamespace, err = c.parseClusterNamespace(attachOptions.StorageClass)
	if err != nil {
		return errors.Wrapf(err, "failed to parse clusterNamespace from storageClass %s", attachOptions.StorageClass)
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
func (c *Controller) GetClientAccessInfo(args []string, clientAccessInfo *ClientAccessInfo) error {
	// args: 0 ClusterNamespace, 1 PodNamespace, 2 MountUser, 3 MountSecret
	clusterNamespace := args[0]
	clusterInfo, _, _, err := mon.LoadClusterInfo(c.context, clusterNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to load cluster information from clusters namespace %s", clusterNamespace)
	}

	monEndpoints := make([]string, 0, len(clusterInfo.Monitors))
	for _, monitor := range clusterInfo.Monitors {
		monEndpoints = append(monEndpoints, monitor.Endpoint)
	}

	clientAccessInfo.MonAddresses = monEndpoints

	podNamespace := args[1]
	clientAccessInfo.UserName = args[2]
	clientAccessInfo.SecretKey = args[3]

	if c.mountSecurityMode == agent.MountSecurityModeRestricted && (clientAccessInfo.UserName == "" || clientAccessInfo.SecretKey == "") {
		return errors.New("no mount user and/or mount secret given")
	}

	if c.mountSecurityMode == agent.MountSecurityModeAny && clientAccessInfo.UserName == "" {
		clientAccessInfo.UserName = "admin"
	}

	if clientAccessInfo.SecretKey != "" {
		secret, err := c.context.Clientset.CoreV1().Secrets(podNamespace).Get(clientAccessInfo.SecretKey, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "unable to get mount secret %s from pod namespace %s", clientAccessInfo.SecretKey, podNamespace)
		}
		if len(secret.Data) == 0 || len(secret.Data) > 1 {
			return errors.Errorf("no data or more than one data (length %d) in mount secret %s in namespace %s", len(secret.Data), clientAccessInfo.SecretKey, podNamespace)
		}
		var secretValue string
		for _, value := range secret.Data {
			secretValue = string(value[:])
			break
		}
		clientAccessInfo.SecretKey = secretValue
	} else if c.mountSecurityMode == agent.MountSecurityModeAny && clientAccessInfo.SecretKey == "" {
		clientAccessInfo.SecretKey = clusterInfo.AdminSecret
	}

	return nil
}

// GetKernelVersion returns the kernel version of the current node.
func (c *Controller) GetKernelVersion(_ *struct{} /* no inputs */, kernelVersion *string) error {
	nodeName := os.Getenv(k8sutil.NodeNameEnvVar)
	node, err := c.context.Clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get kernel version from node information for node %s", nodeName)
	}
	*kernelVersion = node.Status.NodeInfo.KernelVersion
	return nil
}

// getKubeletRootDir queries the kubelet configuration to find the kubelet root dir. Defaults to /var/lib/kubelet
func (c *Controller) getKubeletRootDir() string {
	// in k8s 1.8 it does not appear possible to change the default root dir
	// see https://github.com/rook/rook/issues/1282
	return kubeletDefaultRootDir
}

// getPodAndPVNameFromMountDir parses pod information from the mountDir
func getPodAndPVNameFromMountDir(mountDir string) (string, string, error) {
	// mountDir is in the form of <rootDir>/pods/<podID>/volumes/rook.io~rook/<pv name>
	filepath.Clean(mountDir)
	token := strings.Split(mountDir, string(filepath.Separator))
	// token length should at least size 5
	length := len(token)
	if length < 5 {
		return "", "", errors.Errorf("failed to parse mountDir %s for CRD name and podID", mountDir)
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
