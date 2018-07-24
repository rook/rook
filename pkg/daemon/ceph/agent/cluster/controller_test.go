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
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/manager"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterDeleteSingleAttachment(t *testing.T) {

	nodeName := "node09234"
	clusterName := "cluster4628"
	podName := "pod7620"
	pvName := "pvc-1427"
	rookSystemNamespace := "rook-system-03931"

	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	os.Setenv(k8sutil.NodeNameEnvVar, nodeName)
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	clientset := test.New(3)
	context := &clusterd.Context{
		Clientset: clientset,
	}

	// set up an existing volume attachment CRD that belongs to this node and the cluster we will delete later
	existingVolAttachList := &rookv1alpha2.VolumeList{
		Items: []rookv1alpha2.Volume{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvName,
					Namespace: rookSystemNamespace,
				},
				Attachments: []rookv1alpha2.Attachment{
					{
						Node:        nodeName,
						MountDir:    getMockMountDir(podName, pvName),
						ClusterName: clusterName,
					},
				},
			},
		},
	}

	detachCalled := false
	deleteAttachmentCalled := false
	removeAttachmentCalled := false

	flexvolumeManager := &manager.FakeVolumeManager{}
	volumeAttachmentController := &attachment.MockAttachment{
		MockList: func(namespace string) (*rookv1alpha2.VolumeList, error) {
			return existingVolAttachList, nil
		},
		MockDelete: func(namespace, name string) error {
			assert.Equal(t, rookSystemNamespace, namespace)
			assert.Equal(t, pvName, name)
			deleteAttachmentCalled = true
			return nil
		},
	}
	flexvolumeController := &flexvolume.MockFlexvolumeController{
		MockGetAttachInfoFromMountDir: func(mountDir string, attachOptions *flexvolume.AttachOptions) error {
			assert.Equal(t, getMockMountDir(podName, pvName), mountDir)
			attachOptions.VolumeName = pvName
			return nil
		},
		MockDetachForce: func(detachOpts flexvolume.AttachOptions, _ *struct{} /* void reply */) error {
			<-time.After(10 * time.Millisecond) // simulate the detach taking some time (even though it's a small amount)
			detachCalled = true
			return nil
		},
		MockRemoveAttachmentObject: func(detachOpts flexvolume.AttachOptions, safeToDetach *bool) error {
			removeAttachmentCalled = true
			*safeToDetach = true
			return nil
		},
	}

	controller := NewClusterController(context, flexvolumeController, volumeAttachmentController, flexvolumeManager)

	// tell the cluster controller that a cluster has been deleted.  the controller will perform the cleanup
	// async, but block and wait for it all to complete before returning to us, so there should be no races
	// with the asserts later on.
	clusterToDelete := &cephv1beta1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	controller.handleClusterDelete(clusterToDelete, time.Millisecond)

	// detaching, removing the attachment from the CRD, and deleting the CRD should have been called
	assert.True(t, detachCalled)
	assert.True(t, removeAttachmentCalled)
	assert.True(t, deleteAttachmentCalled)
}

func TestClusterDeleteAttachedToOtherNode(t *testing.T) {

	nodeName := "node314"
	clusterName := "cluster6841"
	podName := "pod9134"
	pvName := "pvc-1489"
	rookSystemNamespace := "rook-system-0084"

	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	os.Setenv(k8sutil.NodeNameEnvVar, nodeName)
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	clientset := test.New(3)
	context := &clusterd.Context{
		Clientset: clientset,
	}

	// set up an existing volume attachment CRD that belongs to another node
	existingVolAttachList := &rookv1alpha2.VolumeList{
		Items: []rookv1alpha2.Volume{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvName,
					Namespace: rookSystemNamespace,
				},
				Attachments: []rookv1alpha2.Attachment{
					{
						Node:        "some other node",
						MountDir:    getMockMountDir(podName, pvName),
						ClusterName: clusterName,
					},
				},
			},
		},
	}

	getAttachInfoCalled := false

	flexvolumeManager := &manager.FakeVolumeManager{}
	volumeAttachmentController := &attachment.MockAttachment{
		MockList: func(namespace string) (*rookv1alpha2.VolumeList, error) {
			return existingVolAttachList, nil
		},
	}
	flexvolumeController := &flexvolume.MockFlexvolumeController{
		MockGetAttachInfoFromMountDir: func(mountDir string, attachOptions *flexvolume.AttachOptions) error {
			getAttachInfoCalled = true // this should not get called since it belongs to another node
			return nil
		},
	}

	controller := NewClusterController(context, flexvolumeController, volumeAttachmentController, flexvolumeManager)

	// delete the cluster, nothing should happen
	clusterToDelete := &cephv1beta1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	controller.handleClusterDelete(clusterToDelete, time.Millisecond)

	// since the volume attachment was on a different node, nothing should have been called
	assert.False(t, getAttachInfoCalled)
}

func TestClusterDeleteMultiAttachmentRace(t *testing.T) {

	nodeName := "node09234"
	clusterName := "cluster4628"
	podName1 := "pod7620"
	podName2 := "pod216"
	pvName := "pvc-1427"
	rookSystemNamespace := "rook-system-03931"

	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	os.Setenv(k8sutil.NodeNameEnvVar, nodeName)
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	clientset := test.New(3)
	context := &clusterd.Context{
		Clientset: clientset,
	}

	// set up an existing volume attachment CRD that has two pods using the same underlying volume.
	existingVolAttachList := &rookv1alpha2.VolumeList{
		Items: []rookv1alpha2.Volume{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvName,
					Namespace: rookSystemNamespace,
				},
				Attachments: []rookv1alpha2.Attachment{
					{
						Node:        nodeName,
						MountDir:    getMockMountDir(podName1, pvName),
						ClusterName: clusterName,
					},
					{
						Node:        nodeName,
						MountDir:    getMockMountDir(podName2, pvName),
						ClusterName: clusterName,
					},
				},
			},
		},
	}

	flexvolumeManager := &manager.FakeVolumeManager{}

	var lock sync.Mutex

	deleteCount := 0
	volumeAttachmentController := &attachment.MockAttachment{
		MockList: func(namespace string) (*rookv1alpha2.VolumeList, error) {
			return existingVolAttachList, nil
		},
		MockDelete: func(namespace, name string) error {
			lock.Lock()
			defer lock.Unlock()
			deleteCount++
			return nil
		},
	}

	removeCount := 0
	flexvolumeController := &flexvolume.MockFlexvolumeController{
		MockGetAttachInfoFromMountDir: func(mountDir string, attachOptions *flexvolume.AttachOptions) error {
			attachOptions.VolumeName = pvName
			return nil
		},
		MockDetachForce: func(detachOpts flexvolume.AttachOptions, _ *struct{} /* void reply */) error {
			<-time.After(10 * time.Millisecond) // simulate the detach taking some time (even though it's a small amount)
			return nil
		},
		MockRemoveAttachmentObject: func(detachOpts flexvolume.AttachOptions, safeToDetach *bool) error {
			// Removing the attachment object is interesting from a concurrency perspective.  If two callers
			// are attempting to remove from an attachment from the CRD at the same time, it could fail.
			// Let's simulate that outcome in this function.
			lock.Lock()
			defer lock.Unlock()

			removeCount++
			*safeToDetach = true

			if removeCount%2 == 0 {
				// every other time, simulate a failure to remove the attachment, e.g., someone else
				// updated it before we could.
				return fmt.Errorf("mock error for failing to remove the volume attachment")
			}

			return nil
		},
	}

	// kick off the cluster deletion process
	controller := NewClusterController(context, flexvolumeController, volumeAttachmentController, flexvolumeManager)
	clusterToDelete := &cephv1beta1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	controller.handleClusterDelete(clusterToDelete, time.Millisecond)

	// both attachments should have made it all the way through the clean up process, meaing that Delete
	// (which is idempotent) should have been called twice.
	assert.Equal(t, 2, deleteCount)
}

func getMockMountDir(podName, pvName string) string {
	return fmt.Sprintf("/test/pods/%s/volumes/rook.io~rook/%s", podName, pvName)
}
