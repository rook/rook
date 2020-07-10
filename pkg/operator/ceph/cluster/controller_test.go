/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterDeleteFlexEnabled(t *testing.T) {
	nodeName := "node841"
	clusterName := "cluster684"
	pvName := "pvc-540"
	rookSystemNamespace := "rook-system-6413"

	os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "true")
	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	context := &clusterd.Context{
		Clientset: testop.New(t, 3),
	}
	listCount := 0
	volumeAttachmentController := &attachment.MockAttachment{
		MockList: func(namespace string) (*rookalpha.VolumeList, error) {
			listCount++
			if listCount == 1 {
				// first listing returns an existing volume attachment, so the controller should wait
				return &rookalpha.VolumeList{
					Items: []rookalpha.Volume{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      pvName,
								Namespace: rookSystemNamespace,
							},
							Attachments: []rookalpha.Attachment{
								{
									Node:        nodeName,
									ClusterName: clusterName,
								},
							},
						},
					},
				}, nil
			}

			// subsequent listings should return no volume attachments, meaning that they have all
			// been cleaned up and the controller can move on.
			return &rookalpha.VolumeList{Items: []rookalpha.Volume{}}, nil

		},
	}
	operatorConfigCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			return nil
		},
	}
	addCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			return nil
		},
	}
	// create the cluster controller and tell it that the cluster has been deleted
	controller := NewClusterController(context, "", volumeAttachmentController, operatorConfigCallbacks, addCallbacks)
	clusterToDelete := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}

	// The test returns a volume on the first call
	assert.Error(t, controller.checkIfVolumesExist(clusterToDelete))

	// The test does not return volumes on the second call
	assert.NoError(t, controller.checkIfVolumesExist(clusterToDelete))
}

func TestClusterDeleteFlexDisabled(t *testing.T) {
	clusterName := "cluster684"
	rookSystemNamespace := "rook-system-6413"

	os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "false")
	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	context := &clusterd.Context{
		Clientset: testop.New(t, 3),
	}
	listCount := 0
	volumeAttachmentController := &attachment.MockAttachment{
		MockList: func(namespace string) (*rookalpha.VolumeList, error) {
			listCount++
			return &rookalpha.VolumeList{Items: []rookalpha.Volume{}}, nil

		},
	}
	operatorConfigCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			return nil
		},
	}
	addCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "true")
			os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
			defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
			return nil
		},
	}
	// create the cluster controller and tell it that the cluster has been deleted
	controller := NewClusterController(context, "", volumeAttachmentController, operatorConfigCallbacks, addCallbacks)
	clusterToDelete := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	assert.NoError(t, controller.checkIfVolumesExist(clusterToDelete))

	// Ensure that the listing of volume attachments was never called.
	assert.Equal(t, 0, listCount)
}
