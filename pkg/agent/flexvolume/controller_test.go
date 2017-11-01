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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package flexvolume to manage Kubernetes storage attach events.
package flexvolume

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/rook/rook/pkg/agent/flexvolume/crd"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/fake"
	fakerestclient "k8s.io/client-go/rest/fake"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/testapi"
)

func TestAttach(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset: clientset,
	}

	scheme := crd.RegisterFakeAPI()
	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)},
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch p, m := req.URL.Path, req.Method; {
			case p == "/namespaces/rook-system/volumeattachments/pvc-123" && m == "GET":
				return &http.Response{StatusCode: 404, Header: defaultHeader(), Body: ioutil.NopCloser(bytes.NewReader([]byte("")))}, nil
			case p == "/namespaces/rook-system/volumeattachments" && m == "POST":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(req.Body)}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}
		}),
	}

	devicePath := ""
	opts := AttachOptions{
		Image:        "image123",
		Pool:         "testpool",
		ClusterName:  "testCluster",
		StorageClass: "storageclass1",
		MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:   "pvc-123",
		Pod:          "myPod",
		PodNamespace: "Default",
		RW:           "rw",
	}

	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Attach(opts, &devicePath)
	assert.Nil(t, err)
	assert.Equal(t, "/image123/testpool/testCluster", devicePath)
}

func TestAttachAlreadyExist(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset: clientset,
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

	existingCRD := &crd.VolumeAttachment{
		Attachments: []crd.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "otherpod",
				MountDir:     "/tmt/test",
				ReadOnly:     false,
			},
		},
	}

	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: testapi.Default.NegotiatedSerializer(),
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(existingCRD)}, nil
		}),
	}

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

	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Attach(opts, devicePath)
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
		Clientset: clientset,
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

	existingCRD := &crd.VolumeAttachment{
		Attachments: []crd.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "otherpod",
				MountDir:     "/tmt/test",
				ReadOnly:     false,
			},
		},
	}

	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: testapi.Default.NegotiatedSerializer(),
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(existingCRD)}, nil
		}),
	}

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

	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Attach(opts, devicePath)
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
		Clientset: clientset,
	}

	existingCRD := &crd.VolumeAttachment{
		Attachments: []crd.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "otherpod",
				MountDir:     "/tmt/test",
				ReadOnly:     true,
			},
		},
	}

	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: testapi.Default.NegotiatedSerializer(),
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(existingCRD)}, nil
		}),
	}

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

	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Attach(opts, devicePath)
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
		Clientset: clientset,
	}

	existingCRD := &crd.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []crd.Attachment{
			{
				Node:         "otherNode",
				PodNamespace: "Default",
				PodName:      "myPod",
				MountDir:     "/tmt/test",
				ReadOnly:     true,
			},
		},
	}

	opts := AttachOptions{
		Image:        "image123",
		Pool:         "testpool",
		ClusterName:  "testCluster",
		MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:   "pvc-123",
		Pod:          "myPod",
		PodNamespace: "Default",
		RW:           "ro",
	}

	scheme := crd.RegisterFakeAPI()
	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)},
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch p, m := req.URL.Path, req.Method; {
			case p == "/namespaces/rook-system/volumeattachments/pvc-123" && m == "GET":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(existingCRD)}, nil
			case p == "/namespaces/rook-system/volumeattachments/pvc-123" && m == "PUT":
				o, _ := ioutil.ReadAll(req.Body)
				var volAtt crd.VolumeAttachment
				json.Unmarshal(o, &volAtt)

				assert.Equal(t, 2, len(volAtt.Attachments))

				assert.True(t, containsAttachment(
					crd.Attachment{
						PodNamespace: opts.PodNamespace,
						PodName:      opts.Pod,
						MountDir:     opts.MountDir,
						ReadOnly:     true,
						Node:         "node1",
					}, volAtt.Attachments,
				), "VolumeAttachment crd does not contain expected attachment")

				assert.True(t, containsAttachment(
					existingCRD.Attachments[0], volAtt.Attachments,
				), "VolumeAttachment crd does not contain expected attachment")

				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(req.Body)}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}

		}),
	}

	devicePath := ""
	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Attach(opts, &devicePath)
	assert.Nil(t, err)
}

func TestOrphanAttach(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset: clientset,
	}

	existingCRD := &crd.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []crd.Attachment{
			{
				Node:         "otherNode",
				PodNamespace: "Default",
				PodName:      "myPod",
				MountDir:     "/tmt/test",
				ReadOnly:     false,
			},
		},
	}

	opts := AttachOptions{
		Image:        "image123",
		Pool:         "testpool",
		ClusterName:  "testCluster",
		MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:   "pvc-123",
		Pod:          "myPod",
		PodNamespace: "Default",
		RW:           "rw",
	}

	scheme := crd.RegisterFakeAPI()
	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)},
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch p, m := req.URL.Path, req.Method; {
			case p == "/namespaces/rook-system/volumeattachments/pvc-123" && m == "GET":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(existingCRD)}, nil
			case p == "/namespaces/rook-system/volumeattachments/pvc-123" && m == "PUT":
				o, _ := ioutil.ReadAll(req.Body)
				var volAtt crd.VolumeAttachment
				json.Unmarshal(o, &volAtt)

				assert.Equal(t, 1, len(volAtt.Attachments))
				assert.True(t, containsAttachment(
					crd.Attachment{
						PodNamespace: opts.PodNamespace,
						PodName:      opts.Pod,
						MountDir:     opts.MountDir,
						ReadOnly:     false,
						Node:         "node1",
					}, volAtt.Attachments,
				), "VolumeAttachment crd does not contain expected attachment")

				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(req.Body)}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}

		}),
	}

	devicePath := ""
	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Attach(opts, &devicePath)
	assert.Nil(t, err)
}

// This tests the idempotency of the VolumeAttachment record.
// If the VolumeAttachment record was previously created for this pod
// and the attach flow should continue.
func TestVolumeAttachmentExistAttach(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset: clientset,
	}

	existingCRD := &crd.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-123",
			Namespace: "rook-system",
		},
		Attachments: []crd.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "myPod",
				MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
				ReadOnly:     false,
			},
		},
	}

	opts := AttachOptions{
		Image:        "image123",
		Pool:         "testpool",
		ClusterName:  "testCluster",
		MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
		VolumeName:   "pvc-123",
		Pod:          "myPod",
		PodNamespace: "Default",
		RW:           "rw",
	}

	scheme := crd.RegisterFakeAPI()
	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)},
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch p, m := req.URL.Path, req.Method; {
			case p == "/namespaces/rook-system/volumeattachments/pvc-123" && m == "GET":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(existingCRD)}, nil
			case p == "/namespaces/rook-system/volumeattachments/pvc-123" && m == "PUT":
				assert.Fail(t, "VolumeAttachment shoud not be updated")
				return &http.Response{StatusCode: 500, Header: defaultHeader(), Body: objBody(req.Body)}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}

		}),
	}

	devicePath := ""
	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Attach(opts, &devicePath)
	assert.Nil(t, err)
}

func TestDetach(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset: clientset,
	}

	existingCRD := &crd.VolumeAttachment{
		Attachments: []crd.Attachment{},
	}

	deleteCRDCalled := false
	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: testapi.Default.NegotiatedSerializer(),
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.Method == "DELETE" {
				deleteCRDCalled = true
				assert.Equal(t, req.URL.Path, "/namespaces/rook-system/volumeattachments/pvc-123")
			}
			return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(existingCRD)}, nil
		}),
	}

	opts := AttachOptions{
		VolumeName: "pvc-123",
		MountDir:   "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
	}

	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Detach(opts, nil)
	assert.Nil(t, err)
	assert.True(t, deleteCRDCalled)
}

func TestDetachWithAttachmentLeft(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset: clientset,
	}

	existingCRD := &crd.VolumeAttachment{
		Attachments: []crd.Attachment{
			{
				Node:         "node1",
				PodNamespace: "Default",
				PodName:      "myPod",
				MountDir:     "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
			},
		},
	}

	deleteCRDCalled := false
	fakeClient := &fakerestclient.RESTClient{
		NegotiatedSerializer: testapi.Default.NegotiatedSerializer(),
		APIRegistry:          api.Registry,
		Client: fakerestclient.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.Method == "DELETE" {
				deleteCRDCalled = true
			}
			return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(existingCRD)}, nil
		}),
	}

	opts := AttachOptions{
		VolumeName: "pvc-123",
		MountDir:   "/test/pods/pod123/volumes/rook.io~rook/pvc-123",
	}

	controller := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fakeClient),
		volumeManager:              &FakeVolumeManager{},
	}

	err := controller.Detach(opts, nil)
	assert.Nil(t, err)
	assert.Nil(t, err)
	assert.False(t, deleteCRDCalled)
}

func TestGetAttachInfoFromMountDir(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.NodeNameEnvVar, "node1")
	defer os.Unsetenv(k8sutil.NodeNameEnvVar)

	context := &clusterd.Context{
		Clientset: clientset,
	}

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvc-123",
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexVolumeSource{
					Driver:   "rook.io/rook",
					FSType:   "ext4",
					ReadOnly: false,
					Options: map[string]string{
						StorageClassKey: "storageClass1",
						PoolKey:         "pool123",
						ImageKey:        "pvc-123",
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
		Provisioner: "rook.io/rook",
		Parameters:  map[string]string{"pool": "testpool", "clusterName": "testCluster", "fsType": "ext3"},
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

	controller := &FlexvolumeController{
		context:       context,
		volumeManager: &FakeVolumeManager{},
	}

	err := controller.GetAttachInfoFromMountDir(opts.MountDir, &opts)
	assert.Nil(t, err)

	assert.Equal(t, "pod123", opts.PodID)
	assert.Equal(t, "pvc-123", opts.VolumeName)
	assert.Equal(t, "testnamespace", opts.PodNamespace)
	assert.Equal(t, "myPod", opts.Pod)
	assert.Equal(t, "pvc-123", opts.Image)
	assert.Equal(t, "pool123", opts.Pool)
	assert.Equal(t, "storageClass1", opts.StorageClass)
	assert.Equal(t, "testCluster", opts.ClusterName)
}

func TestParseClusterName(t *testing.T) {
	clientset := test.New(3)

	context := &clusterd.Context{
		Clientset: clientset,
	}
	sc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-storageclass",
		},
		Provisioner: "rook.io/rook",
		Parameters:  map[string]string{"pool": "testpool", "clusterName": "testCluster", "fsType": "ext3"},
	}
	clientset.StorageV1().StorageClasses().Create(&sc)
	fc := &FlexvolumeController{
		context:                    context,
		volumeAttachmentController: crd.New(fake.NewSimpleClientset().CoreV1().RESTClient()),
	}
	clusterName, _ := fc.parseClusterName("rook-storageclass")
	assert.Equal(t, "testCluster", clusterName)
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

type FakeVolumeManager struct{}

func (f *FakeVolumeManager) Init() error {
	return nil
}

func (f *FakeVolumeManager) Attach(image, pool, clusterName string) (string, error) {
	return fmt.Sprintf("/%s/%s/%s", image, pool, clusterName), nil
}

func (f *FakeVolumeManager) Detach(image, pool, clusterName string) error {
	return nil
}

func defaultHeader() http.Header {
	header := http.Header{}
	header.Set("Content-Type", runtime.ContentTypeJSON)
	return header
}

func objBody(object interface{}) io.ReadCloser {
	output, err := json.MarshalIndent(object, "", "")
	if err != nil {
		panic(err)
	}
	return ioutil.NopCloser(bytes.NewReader([]byte(output)))
}

func containsAttachment(attachment crd.Attachment, attachments []crd.Attachment) bool {
	for _, a := range attachments {
		if a.PodNamespace == attachment.PodNamespace && a.PodName == attachment.PodName && a.MountDir == attachment.MountDir && a.ReadOnly == attachment.ReadOnly && a.Node == attachment.Node {
			return true
		}
	}
	return false
}
