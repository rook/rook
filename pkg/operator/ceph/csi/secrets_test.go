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

package csi

import (
	"context"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestCephCSIKeyringRBDNodeCaps(t *testing.T) {
	caps := cephCSIKeyringRBDNodeCaps()
	assert.Equal(t, caps, []string{"mon", "profile rbd", "mgr", "allow rw", "osd", "profile rbd"})
}

func TestCephCSIKeyringRBDProvisionerCaps(t *testing.T) {
	caps := cephCSIKeyringRBDProvisionerCaps()
	assert.Equal(t, caps, []string{"mon", "profile rbd, allow command 'osd blocklist'", "mgr", "allow rw", "osd", "profile rbd"})
}

func TestCephCSIKeyringCephFSNodeCaps(t *testing.T) {
	caps := cephCSIKeyringCephFSNodeCaps()
	assert.Equal(t, caps, []string{"mon", "allow r", "mgr", "allow rw", "osd", "allow rwx tag cephfs metadata=*, allow rw tag cephfs data=*", "mds", "allow rw"})
}

func TestCephCSIKeyringCephFSProvisionerCaps(t *testing.T) {
	caps := cephCSIKeyringCephFSProvisionerCaps()
	assert.Equal(t, caps, []string{"mon", "allow r, allow command 'osd blocklist'", "mgr", "allow rw", "osd", "allow rw tag cephfs metadata=*", "mds", "allow *"})
}

func Test_deleteOwnedCSISecretsByCephCluster(t *testing.T) {
	const (
		namespace   = "rook-ceph"
		cephCluster = "my-ceph-cluster"
	)

	// Create fake client
	clientset := k8sfake.NewSimpleClientset()

	// Cluster context
	clusterContext := &clusterd.Context{
		Clientset: clientset,
	}

	clusterInfo := &client.ClusterInfo{
		OwnerInfo: k8sutil.NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{
			APIVersion: "ceph.rook.io/v1",
			Kind:       "CephCluster",
			Name:       cephCluster,
		}, namespace),
	}
	clusterInfo.Namespace = namespace
	clusterInfo.SetName(cephCluster)

	ctx := &clusterd.Context{
		Clientset: clientset,
	}

	k := keyring.GetSecretStore(ctx, clusterInfo, clusterInfo.OwnerInfo)

	// create csi secrets
	err := createOrUpdateCSISecret(clusterInfo, "", "", "", "", k)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Remove ownerRef from the CsiCephFSNodeSecret to ensure it won't get deleted
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), CsiCephFSProvisionerSecret, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret.OwnerReferences = nil
	secret.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "ceph.rook.io/v1",
			Kind:       "CephClient",
			Name:       "test-client",
		},
	})
	_, err = clientset.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = deleteOwnedCSISecretsByCephCluster(clusterContext, clusterInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify owned secret is deleted
	secrets := []string{CsiRBDNodeSecret, CsiRBDProvisionerSecret, CsiCephFSNodeSecret}
	for _, secretName := range secrets {
		_, err = clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err == nil {
			t.Errorf("expected owned secret %q to be deleted", secretName)
		}
	}
	// Verify unowned secret still exists
	_, err = clientset.CoreV1().Secrets(namespace).Get(context.TODO(), CsiCephFSProvisionerSecret, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		t.Errorf("expected unowned secret %q to still exist", CsiCephFSProvisionerSecret)
	}
}
