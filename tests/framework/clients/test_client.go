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

package clients

import (
	"fmt"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

// TestClient is a wrapper for test client, containing interfaces for all rook operations
type TestClient struct {
	BlockClient        *BlockOperation
	FSClient           *FilesystemOperation
	NFSClient          *NFSOperation
	ObjectClient       *ObjectOperation
	ObjectUserClient   *ObjectUserOperation
	PoolClient         *PoolOperation
	BucketClient       *BucketOperation
	UserClient         *ClientOperation
	RBDMirrorClient    *RBDMirrorOperation
	TopicClient        *TopicOperation
	NotificationClient *NotificationOperation
	k8sh               *utils.K8sHelper
}

// CreateTestClient creates new instance of test client for a platform
func CreateTestClient(k8sHelper *utils.K8sHelper, manifests installer.CephManifests) *TestClient {
	return &TestClient{
		CreateBlockOperation(k8sHelper, manifests),
		CreateFilesystemOperation(k8sHelper, manifests),
		CreateNFSOperation(k8sHelper, manifests),
		CreateObjectOperation(k8sHelper, manifests),
		CreateObjectUserOperation(k8sHelper, manifests),
		CreatePoolOperation(k8sHelper, manifests),
		CreateBucketOperation(k8sHelper, manifests),
		CreateClientOperation(k8sHelper, manifests),
		CreateRBDMirrorOperation(k8sHelper, manifests),
		CreateTopicOperation(k8sHelper, manifests),
		CreateNotificationOperation(k8sHelper, manifests),
		k8sHelper,
	}
}

// Status returns rook status details
func (c TestClient) Status(namespace string) (client.CephStatus, error) {
	context := c.k8sh.MakeContext()
	clusterInfo := client.AdminTestClusterInfo(namespace)
	status, err := client.Status(context, clusterInfo)
	if err != nil {
		return client.CephStatus{}, fmt.Errorf("failed to get status: %+v", err)
	}
	return status, nil
}
