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

	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NFSOperation is a wrapper for k8s rook file operations
type NFSOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateNFSOperation Constructor to create NFSOperation - client to perform ceph nfs operations on k8s
func CreateNFSOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *NFSOperation {
	return &NFSOperation{k8sh, manifests}
}

// Create creates a filesystem in Rook
func (n *NFSOperation) Create(namespace, name string, daemonCount int) error {
	logger.Infof("creating the NFS pool")
	if err := n.k8sh.ResourceOperation("apply", n.manifests.GetNFSPool()); err != nil {
		return err
	}

	logger.Infof("creating the NFS daemons via CRD")
	if err := n.k8sh.ResourceOperation("apply", n.manifests.GetNFS(name, daemonCount)); err != nil {
		return err
	}

	logger.Infof("Make sure rook-ceph-nfs pod is running")
	err := n.k8sh.WaitForLabeledPodsToRun(fmt.Sprintf("ceph_nfs=%s", name), namespace)
	assert.Nil(n.k8sh.T(), err)

	assert.True(n.k8sh.T(), n.k8sh.CheckPodCountAndState("rook-ceph-nfs", namespace, daemonCount, "Running"),
		"Make sure all nfs daemon pods are in Running state")

	return nil
}

// Delete deletes a filesystem in Rook
func (n *NFSOperation) Delete(namespace, name string) error {
	ctx := context.TODO()
	options := &metav1.DeleteOptions{}
	logger.Infof("Deleting nfs %s in namespace %s", name, namespace)
	err := n.k8sh.RookClientset.CephV1().CephNFSes(namespace).Delete(ctx, name, *options)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	crdCheckerFunc := func() error {
		_, err := n.k8sh.RookClientset.CephV1().CephNFSes(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	}

	logger.Infof("Deleted nfs %s in namespace %s", name, namespace)
	err = n.k8sh.WaitForCustomResourceDeletion(namespace, name, crdCheckerFunc)
	if err != nil {
		return err
	}

	logger.Infof("Deleting .nfs pool in namespace %s", namespace)
	err = n.k8sh.RookClientset.CephV1().CephBlockPools(namespace).Delete(ctx, "dot-nfs", *options)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	crdCheckerFunc = func() error {
		_, err := n.k8sh.RookClientset.CephV1().CephBlockPools(namespace).Get(ctx, "dot-nfs", metav1.GetOptions{})
		return err
	}

	logger.Infof("Deleted .nfs pool %s in namespace %s", name, namespace)
	err = n.k8sh.WaitForCustomResourceDeletion(namespace, name, crdCheckerFunc)
	if err != nil {
		return err
	}

	logger.Infof("Verified Deletion of nfs %s in namespace %s", name, namespace)
	return nil
}

// CreateStorageClass creates a storage class for NFS clients
func (f *NFSOperation) CreateStorageClass(fsName, nfsClusterName, systemNamespace, namespace, storageClassName string) error {
	server := fmt.Sprintf("rook-ceph-nfs-%s-a.%s.svc.cluster.local", nfsClusterName, namespace)

	return f.k8sh.ResourceOperation("apply", f.manifests.GetNFSStorageClass(fsName, nfsClusterName, server, storageClassName))
}

// CreateSnapshotClass creates a Snapshot class for NFS clients
func (f *NFSOperation) CreateSnapshotClass(fsName, snapshotClassName string) error {
	return f.k8sh.ResourceOperation("apply", f.manifests.GetNFSSnapshotClass(fsName, snapshotClassName))
}

// DeleteSnapshotClass deletes a Snapshot class for NFS clients
func (f *NFSOperation) DeleteSnapshotClass(fsName, snapshotClassName string) error {
	return f.k8sh.ResourceOperation("delete", f.manifests.GetNFSSnapshotClass(fsName, snapshotClassName))
}
