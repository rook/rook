/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	"context"
	"fmt"
	"time"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClientOperation is a wrapper for k8s rook file operations
type ClientOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateClientOperation Constructor to create ClientOperation - client to perform rook file system operations on k8s
func CreateClientOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *ClientOperation {
	return &ClientOperation{k8sh, manifests}
}

// Create creates a client in Rook
func (c *ClientOperation) Create(name, namespace string, caps map[string]string) error {
	logger.Infof("creating the client via CRD")
	if err := c.k8sh.ResourceOperation("apply", c.manifests.GetClient(name, caps)); err != nil {
		return err
	}
	return nil
}

// Delete deletes a client in Rook
func (c *ClientOperation) Delete(name, namespace string) error {
	ctx := context.TODO()
	options := &metav1.DeleteOptions{}
	logger.Infof("Deleting filesystem %s in namespace %s", name, namespace)
	err := c.k8sh.RookClientset.CephV1().CephClients(namespace).Delete(ctx, name, *options)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	logger.Infof("Deleted client %s in namespace %s", name, namespace)
	return nil
}

// Get shows user created in Rook
func (c *ClientOperation) Get(clusterInfo *client.ClusterInfo, clientName string) (key string, error error) {
	context := c.k8sh.MakeContext()
	key, err := client.AuthGetKey(context, clusterInfo, clientName)
	if err != nil {
		return "", fmt.Errorf("failed to get client %s: %+v", clientName, err)
	}
	return key, nil
}

// Update updates provided user capabilities
func (c *ClientOperation) Update(clusterInfo *client.ClusterInfo, clientName string, caps map[string]string) (updatedcaps map[string]string, error error) {
	context := c.k8sh.MakeContext()
	logger.Infof("updating the client via CRD")
	if err := c.k8sh.ResourceOperation("apply", c.manifests.GetClient(clientName, caps)); err != nil {
		return nil, err
	}

	for i := 0; i < 30; i++ {
		updatedcaps, _ = client.AuthGetCaps(context, clusterInfo, "client."+clientName)
		if caps["mon"] == updatedcaps["mon"] {
			logger.Infof("Finished updating the client via CRD")
			return updatedcaps, nil
		}
		logger.Info("Waiting for client CRD to finish updating caps")
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("Unable to update client")
}
