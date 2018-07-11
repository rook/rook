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

package cluster

import (
	"os"
	"sync"
	"time"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	opcluster "github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

const (
	removeAttachmentRetryInterval = 2 // seconds
	removeAttachmentMaxRetries    = 3
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "agent-cluster")
)

// ClusterController monitors cluster events and reacts to clean up any affected volume attachments
type ClusterController struct {
	context              *clusterd.Context
	scheme               *runtime.Scheme
	volumeAttachment     attachment.Attachment
	flexvolumeController flexvolume.VolumeController
}

// NewClusterController creates a new instance of a ClusterController
func NewClusterController(context *clusterd.Context, flexvolumeController flexvolume.VolumeController,
	volumeAttachment attachment.Attachment, manager flexvolume.VolumeManager) *ClusterController {

	return &ClusterController{
		context:              context,
		volumeAttachment:     volumeAttachment,
		flexvolumeController: flexvolumeController,
	}
}

// StartWatch will start the watching of cluster events by this controller
func (c *ClusterController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching cluster resources")
	watcher := opkit.NewWatcher(opcluster.ClusterResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1beta1().RESTClient())
	go watcher.Watch(&cephv1beta1.Cluster{}, stopCh)
	return nil
}

func (c *ClusterController) onDelete(obj interface{}) {
	cluster := obj.(*cephv1beta1.Cluster).DeepCopy()

	c.handleClusterDelete(cluster, removeAttachmentRetryInterval*time.Second)
}

func (c *ClusterController) handleClusterDelete(cluster *cephv1beta1.Cluster, retryInterval time.Duration) {
	node := os.Getenv(k8sutil.NodeNameEnvVar)
	agentNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	logger.Infof("cluster in namespace %s is being deleted, agent on node %s will attempt to clean up.", cluster.Namespace, node)

	// TODO: filter this List operation by node name and cluster namespace on the server side
	vols, err := c.volumeAttachment.List(agentNamespace)
	if err != nil {
		logger.Errorf("failed to get volume attachments for agent namespace %s: %+v", agentNamespace, err)
	}

	var waitGroup sync.WaitGroup
	var cleanupList []string

	// find volume attachments in the deleted cluster that are attached to this node
	for _, vol := range vols.Items {
		for _, a := range vol.Attachments {
			if a.Node == node && a.ClusterName == cluster.Namespace {
				logger.Infof("volume %s has an attachment belonging to deleted cluster %s, will clean it up now. mountDir: %s",
					vol.Name, cluster.Namespace, a.MountDir)

				// we will perform all the cleanup asynchronously later on.  Right now, just add this one
				// to the list and increment the wait group counter so we know up front the full list that
				// we need to wait on before any of them start executing.
				waitGroup.Add(1)
				cleanupList = append(cleanupList, a.MountDir)
			}
		}
	}

	for i := range cleanupList {
		// start a goroutine to perform the cleanup of this volume attachment asynchronously.
		// if one cleanup hangs, it will not affect the others.
		go func(mountDir string) {
			defer waitGroup.Done()
			if err := c.cleanupVolumeAttachment(mountDir, retryInterval); err != nil {
				logger.Errorf("failed to clean up attachment for mountDir %s: %+v", mountDir, err)
			} else {
				logger.Infof("cleaned up attachment for mountDir %s", mountDir)
			}
		}(cleanupList[i])
	}

	logger.Info("waiting for all volume cleanup goroutines to complete...")
	waitGroup.Wait()
	logger.Info("completed waiting for all volume cleanup")
}

func (c *ClusterController) cleanupVolumeAttachment(mountDir string, retryInterval time.Duration) error {
	// first get the attachment info
	attachInfo := flexvolume.AttachOptions{MountDir: mountDir}
	if err := c.flexvolumeController.GetAttachInfoFromMountDir(attachInfo.MountDir, &attachInfo); err != nil {
		return err
	}

	// forcefully detach the volume using the attachment info
	if err := c.flexvolumeController.DetachForce(attachInfo, nil); err != nil {
		return err
	}

	// remove this attachment from the CRD
	var safeToDelete bool
	retryCount := 0
	for {
		safeToDelete = false
		err := c.flexvolumeController.RemoveAttachmentObject(attachInfo, &safeToDelete)
		if err == nil {
			break
		}

		// the removal of the attachment object failed.  This can happen if another agent or goroutine
		// was trying to remove an attachment at the same time, due to consistency guarantees in the
		// Kubernetes API.  Let's wait a bit and retry again.
		retryCount++
		if retryCount > removeAttachmentMaxRetries {
			logger.Errorf("exceeded maximum retries for removing attachment object.")
			return err
		}

		logger.Infof("failed to remove the attachment object for mount dir %s, will retry again in %s",
			mountDir, retryInterval)
		<-time.After(retryInterval)
	}

	if safeToDelete {
		// its safe to delete the CRD entirely, do so now
		namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
		crdName := attachInfo.VolumeName
		if err := c.volumeAttachment.Delete(namespace, crdName); err != nil {
			return err
		}
	}

	return nil
}
