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
	"testing"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/manager"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAttach(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	devicePath := ""
	opts := AttachOptions{
		Image:            "image123",
		Pool:             "testpool",
		ClusterNamespace: "testCluster",
		StorageClass:     "storageclass1",
		MountDir:         "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:       "pvc-123",
		Pod:              "myPod",
		PodNamespace:     "Default",
		RW:               "rw",
	}
	att, err := attachment.New(context)
	assert.Nil(t, err)

	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Attach(opts, &devicePath)
	assert.Nil(t, err)
	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Get("pvc-123", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)
	assert.Equal(t, 1, len(volumeAttachment.Attachments))

	a := volumeAttachment.Attachments[0]
	assert.Equal(t, "node1", a.Node)
	assert.Equal(t, "Default", a.PodNamespace)
	assert.Equal(t, "myPod", a.PodName)
	assert.Equal(t, "testCluster", a.ClusterName)
	assert.Equal(t, "/test/pods/pod123/volumes/rook.io~rook/pvc-123", a.MountDir)
	assert.False(t, a.ReadOnly)
}

func TestAttachAlreadyExist(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "otherpod",
			Namespace: "Default",
		},
		Status: v1.PodStatus{
			Phase: "running",
		},
	}
	clientset.CoreV1().Pods("Default").Create(&pod)

	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "otherpod",
				MountDir:     "/tmt/test",
				ReadOnly:     false,
			},
		},
	}

	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	var devicePath *string
	opts := AttachOptions{
		Image:        "image123",
		Pool:         "testpool",
		StorageClass: "storageclass1",
		MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:   "pvc-123",
		Pod:          "myPod",
		PodNamespace: "Default",
		RW:           "rw",
	}

	att, err := attachment.New(context)
	assert.Nil(t, err)

	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Attach(opts, devicePath)
	assert.NotNil(t, err)
	assert.Equal(t, "failed to attach volume pvc-123 for pod Default/myPod. Volume is already attached by pod Default/otherpod. Status running", err.Error())
}

func TestAttachReadOnlyButRWAlreadyExist(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "otherpod",
			Namespace: "Default",
		},
		Status: v1.PodStatus{
			Phase: "running",
		},
	}
	clientset.CoreV1().Pods("Default").Create(&pod)

	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "otherpod",
				MountDir:     "/tmt/test",
				ReadOnly:     false,
			},
		},
	}
	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	var devicePath *string
	opts := AttachOptions{
		Image:        "image123",
		Pool:         "testpool",
		StorageClass: "storageclass1",
		MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:   "pvc-123",
		Pod:          "myPod",
		PodNamespace: "Default",
		RW:           "ro",
	}

	att, err := attachment.New(context)
	assert.Nil(t, err)

	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Attach(opts, devicePath)
	assert.NotNil(t, err)
	assert.Equal(t, "failed to attach volume pvc-123 for pod Default/myPod. Volume is already attached by pod Default/otherpod. Status running", err.Error())
}

func TestAttachRWButROAlreadyExist(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "otherpod",
				MountDir:     "/tmt/test",
				ReadOnly:     true,
			},
		},
	}
	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	var devicePath *string
	opts := AttachOptions{
		Image:        "image123",
		Pool:         "testpool",
		StorageClass: "storageclass1",
		MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:   "pvc-123",
		Pod:          "myPod",
		PodNamespace: "Default",
		RW:           "rw",
	}
	att, err := attachment.New(context)
	assert.Nil(t, err)

	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Attach(opts, devicePath)
	assert.NotNil(t, err)
	assert.Equal(t, "failed to attach volume pvc-123 for pod Default/myPod. Volume is already attached by one or more pods", err.Error())
}

func TestMultipleAttachReadOnly(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	opts := AttachOptions{
		Image:            "image123",
		Pool:             "testpool",
		ClusterNamespace: "testCluster",
		MountDir:         "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:       "pvc-123",
		Pod:              "myPod",
		PodNamespace:     "Default",
		RW:               "ro",
	}
	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{
			{
				Node:         "otherNode",
				PodNamespace: "Default",
				PodName:      "myPod",
				MountDir:     "/tmt/test",
				ReadOnly:     true,
			},
		},
	}
	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	att, err := attachment.New(context)
	assert.Nil(t, err)

	devicePath := ""
	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Attach(opts, &devicePath)
	assert.Nil(t, err)

	volAtt, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Get("pvc-123", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)
	assert.Equal(t, 2, len(volAtt.Attachments))

	assert.True(t, containsAttachment(
		rookalpha.Attachment{
			PodNamespace: opts.PodNamespace,
			PodName:      opts.Pod,
			MountDir:     opts.MountDir,
			ReadOnly:     true,
			Node:         "node1",
		}, volAtt.Attachments,
	), "Volume crd does not contain expected attachment")

	assert.True(t, containsAttachment(
		existingCRD.Attachments[0], volAtt.Attachments,
	), "Volume crd does not contain expected attachment")
}

func TestOrphanAttachOriginalPodDoesntExist(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	opts := AttachOptions{
		Image:            "image123",
		Pool:             "testpool",
		ClusterNamespace: "testCluster",
		MountDir:         "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:       "pvc-123",
		Pod:              "newPod",
		PodNamespace:     "Default",
		RW:               "rw",
	}
	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{
			{
				Node:         "otherNode",
				PodNamespace: "Default",
				PodName:      "oldPod",
				MountDir:     "/tmt/test",
				ReadOnly:     false,
			},
		},
	}
	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	att, err := attachment.New(context)
	assert.Nil(t, err)

	devicePath := ""
	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Attach(opts, &devicePath)
	assert.Nil(t, err)

	volAtt, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Get("pvc-123", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, volAtt)
	assert.Equal(t, 1, len(volAtt.Attachments))
	assert.True(t, containsAttachment(
		rookalpha.Attachment{
			PodNamespace: opts.PodNamespace,
			PodName:      opts.Pod,
			MountDir:     opts.MountDir,
			ReadOnly:     false,
			Node:         "node1",
		}, volAtt.Attachments,
	), "Volume crd does not contain expected attachment")
}

func TestOrphanAttachOriginalPodNameSame(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	// Setting up the pod to ensure that it is exists
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myPod",
			Namespace: "Default",
			UID:       "pod456",
		},
		Spec: v1.PodSpec{
			NodeName: "node1",
		},
	}
	clientset.CoreV1().Pods("Default").Create(&pod)

	// existing record of old attachment. Pod namespace and name must much with the new attachment input to simulate that the new attachment is for the same pod
	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{
			{
				Node:         "otherNode",
				PodNamespace: "Default",
				PodName:      "myPod",
				MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
				ReadOnly:     false,
			},
		},
	}

	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	// attachment input. The ID of the pod must be different than the original record to simulate that
	// the pod resource is a different one but for the same pod metadata. This is reflected in the MountDir.
	// The namespace and name, however, must match.
	opts := AttachOptions{
		Image:            "image123",
		Pool:             "testpool",
		ClusterNamespace: "testCluster",
		MountDir:         "/test/pods/pod456/volumes/rook.io~rook/pvc-123",
		VolumeName:       "pvc-123",
		Pod:              "myPod",
		PodNamespace:     "Default",
		RW:               "rw",
	}

	att, err := attachment.New(context)
	assert.Nil(t, err)

	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	// Attach should fail because the pod is on a different node
	devicePath := ""
	err = controller.Attach(opts, &devicePath)
	assert.Error(t, err)

	// Attach should succeed and the stale volumeattachment record should be updated to reflect the new pod information
	// since the pod is restarting on the same node
	os.Setenv(k8sutil.NodeNameEnvVar, "otherNode")
	err = controller.Attach(opts, &devicePath)
	assert.NoError(t, err)

	volAtt, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Get("pvc-123", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, volAtt)
	assert.Equal(t, 1, len(volAtt.Attachments))
	assert.True(t, containsAttachment(
		rookalpha.Attachment{
			PodNamespace: opts.PodNamespace,
			PodName:      opts.Pod,
			MountDir:     opts.MountDir,
			ReadOnly:     false,
			Node:         "otherNode",
		}, volAtt.Attachments,
	), "Volume crd does not contain expected attachment")
}

// This tests the idempotency of the Volume record.
// If the Volume record was previously created for this pod
// and the attach flow should continue.
func TestVolumeExistAttach(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	opts := AttachOptions{
		Image:            "image123",
		Pool:             "testpool",
		ClusterNamespace: "testCluster",
		MountDir:         "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:       "pvc-123",
		Pod:              "myPod",
		PodNamespace:     "Default",
		RW:               "rw",
	}

	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "myPod",
				MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
				ReadOnly:     false,
			},
		},
	}
	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	att, err := attachment.New(context)
	assert.Nil(t, err)

	devicePath := ""
	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Attach(opts, &devicePath)
	assert.Nil(t, err)

	newAttach, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Get("pvc-123", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, newAttach)
	// TODO: Check that the volume attach was not updated (can't use ResourceVersion in the fake testing)
}

func TestDetach(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{},
	}
	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	opts := AttachOptions{
		VolumeName: "pvc-123",
		MountDir:   "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
	}

	att, err := attachment.New(context)
	assert.Nil(t, err)

	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Detach(opts, nil)
	assert.Nil(t, err)

	_, err = context.RookClientset.RookV1alpha2().Volumes("rook-system").Get("pvc-123", metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestDetachWithAttachmentLeft(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	existingCRD := &rookalpha.Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []rookalpha.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "myPod",
				MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
			},
		},
	}
	volumeAttachment, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Create(existingCRD)
	assert.Nil(t, err)
	assert.NotNil(t, volumeAttachment)

	opts := AttachOptions{
		VolumeName: "pvc-123",
		MountDir:   "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
	}

	att, err := attachment.New(context)
	assert.Nil(t, err)

	controller := &Controller{
		context:          context,
		volumeAttachment: att,
		volumeManager:    &manager.FakeVolumeManager{},
	}

	err = controller.Detach(opts, nil)
	assert.Nil(t, err)

	volAttach, err := context.RookClientset.RookV1alpha2().Volumes("rook-system").Get("pvc-123", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, volAttach)
}

func TestGetAttachInfoFromMountDir(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvc-123",
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexPersistentVolumeSource{
					Driver:   "ceph.rook.io/rook",
					FSType:   "ext4",
					ReadOnly: false,
					Options: map[string]string{
						StorageClassKey:  "storageClass1",
						PoolKey:          "pool123",
						ImageKey:         "pvc-123",
						DataBlockPoolKey: "",
					},
				},
			},
			ClaimRef: &v1.ObjectReference{
				Namespace: "testnamespace",
			},
		},
	}
	clientset.CoreV1().PersistentVolumes().Create(pv)

	sc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "storageClass1",
		},
		Provisioner: "ceph.rook.io/rook",
		Parameters:  map[string]string{"pool": "testpool", "clusterNamespace": "testCluster", "fsType": "ext3"},
	}
	clientset.StorageV1().StorageClasses().Create(&sc)

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myPod",
			Namespace: "testnamespace",
			UID:       "pod123",
		},
		Spec: v1.PodSpec{
			NodeName: "node1",
		},
	}
	clientset.CoreV1().Pods("testnamespace").Create(&pod)

	opts := AttachOptions{
		VolumeName: "pvc-123",
		MountDir:   "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
	}

	controller := &Controller{
		context:       context,
		volumeManager: &manager.FakeVolumeManager{},
	}

	err := controller.GetAttachInfoFromMountDir(opts.MountDir, &opts)
	assert.Nil(t, err)

	assert.Equal(t, "pod123", opts.PodID)
	assert.Equal(t, "pvc-123", opts.VolumeName)
	assert.Equal(t, "testnamespace", opts.PodNamespace)
	assert.Equal(t, "myPod", opts.Pod)
	assert.Equal(t, "pvc-123", opts.Image)
	assert.Equal(t, "pool123", opts.BlockPool)
	assert.Equal(t, "storageClass1", opts.StorageClass)
	assert.Equal(t, "testCluster", opts.ClusterNamespace)
}

func TestParseClusterNamespace(t *testing.T) {
	testParseClusterNamespace(t, "clusterNamespace")
}

func TestParseClusterName(t *testing.T) {
	testParseClusterNamespace(t, "clusterName")
}

func testParseClusterNamespace(t *testing.T, namespaceParameter string) {
	clientset := test.New(3)

	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
	}
	sc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-storageclass",
		},
		Provisioner: "ceph.rook.io/rook",
		Parameters:  map[string]string{"pool": "testpool", namespaceParameter: "testCluster", "fsType": "ext3"},
	}
	clientset.StorageV1().StorageClasses().Create(&sc)
	volumeAttachment, err := attachment.New(context)
	assert.Nil(t, err)

	fc := &Controller{
		context:          context,
		volumeAttachment: volumeAttachment,
	}
	clusterNamespace, _ := fc.parseClusterNamespace("rook-storageclass")
	assert.Equal(t, "testCluster", clusterNamespace)
}

func TestGetPodAndPVNameFromMountDir(t *testing.T) {
	mountDir := "/var/lib/kubelet/pods/b8b7c55f-99ea-11e7-8994-0800277c89a7/volumes/rook.io~rook/pvc-b8aea7f4-99ea-11e7-8994-0800277c89a7"
	pod, pv, err := getPodAndPVNameFromMountDir(mountDir)
	assert.Nil(t, err)
	assert.Equal(t, "b8b7c55f-99ea-11e7-8994-0800277c89a7", pod)
	assert.Equal(t, "pvc-b8aea7f4-99ea-11e7-8994-0800277c89a7", pv)
}

func TestGetCRDNameFromMountDirInvalid(t *testing.T) {
	mountDir := "volumes/rook.io~rook/pvc-b8aea7f4-99ea-11e7-8994-0800277c89a7"
	_, _, err := getPodAndPVNameFromMountDir(mountDir)
	assert.NotNil(t, err)
}

func containsAttachment(attachment rookalpha.Attachment, attachments []rookalpha.Attachment) bool {
	for _, a := range attachments {
		if a.PodNamespace == attachment.PodNamespace && a.PodName == attachment.PodName && a.MountDir == attachment.MountDir && a.ReadOnly == attachment.ReadOnly && a.Node == attachment.Node {
			return true
		}
	}
	return false
}
