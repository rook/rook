/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package controller

import (
	"context"
	"fmt"
	"os"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateClusterSecrets(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 1)
	configDir := "ns"
	err := os.MkdirAll(configDir, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(configDir)
	adminSecret := "AQDkLIBd9vLGJxAAnXsIKPrwvUXAmY+D1g0X1Q==" //nolint:gosec // This is just a var name, not a real secret
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("COMMAND: %s %v", command, args)
			if command == "ceph-authtool" && args[0] == "--create-keyring" {
				filename := args[1]
				assert.NoError(t, os.WriteFile(filename, []byte(fmt.Sprintf("key = %s", adminSecret)), 0600))
			}
			return "", nil
		},
	}
	context := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}
	cephClusterSpec := &cephv1.ClusterSpec{
		Network: cephv1.NetworkSpec{
			Provider: cephv1.NetworkProviderMultus},
	}
	namespace := "ns"
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	info, maxID, mapping, err := CreateOrLoadClusterInfo(context, ctx, namespace, ownerInfo, cephClusterSpec)
	assert.NoError(t, err)
	assert.Equal(t, -1, maxID)
	require.NotNil(t, info)
	assert.Equal(t, "client.admin", info.CephCred.Username)
	assert.Equal(t, adminSecret, info.CephCred.Secret)
	assert.NotEqual(t, "", info.FSID)
	assert.NotNil(t, mapping)

	// check for the cluster secret
	secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, "rook-ceph-mon", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, adminSecret, string(secret.Data["ceph-secret"]))

	// ensure secret owner info can be loaded and is useful
	// this is what the owner info looks like in a live cluster
	ownerController := true
	blockOwnerDel := true
	secret.OwnerReferences[0] = metav1.OwnerReference{
		APIVersion:         "ceph.rook.io/v1",
		Kind:               "CephCluster",
		Name:               "my-cluster",
		UID:                "e55604f2-710c-4353-9a3e-9d23ea2d6eb9", // random uuid
		Controller:         &ownerController,
		BlockOwnerDeletion: &blockOwnerDel,
	}
	_, err = clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	assert.NoError(t, err)
	info, _, _, err = CreateOrLoadClusterInfo(context, ctx, namespace, ownerInfo, cephClusterSpec)
	assert.NoError(t, err)
	// use the SetOwnerReference() method to ensure that the loaded OwnerInfo is usable and correct
	cm := corev1.ConfigMap{}
	cm.Name = "bob"
	cm.Namespace = namespace
	err = info.OwnerInfo.SetOwnerReference(&cm)
	assert.NoError(t, err)
	cmOwner := cm.OwnerReferences[0]
	assert.Equal(t, "ceph.rook.io/v1", cmOwner.APIVersion)
	assert.Equal(t, "CephCluster", cmOwner.Kind)
	assert.Equal(t, "my-cluster", cmOwner.Name)
	assert.Equal(t, "e55604f2-710c-4353-9a3e-9d23ea2d6eb9", string(cmOwner.UID))
	assert.True(t, *cmOwner.Controller)
	assert.True(t, *cmOwner.BlockOwnerDeletion)

	// For backward compatibility check that the admin secret can be loaded as previously specified
	// Update the secret as if created in an old cluster
	delete(secret.Data, CephUserSecretKey)
	delete(secret.Data, CephUsernameKey)
	secret.Data[adminSecretNameKey] = []byte(adminSecret)
	_, err = clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	assert.NoError(t, err)

	// Check that the cluster info can now be loaded
	info, _, _, err = CreateOrLoadClusterInfo(context, ctx, namespace, ownerInfo, cephClusterSpec)
	assert.NoError(t, err)
	assert.Equal(t, "client.admin", info.CephCred.Username)
	assert.Equal(t, adminSecret, info.CephCred.Secret)

	// Fail to load the external cluster if the admin placeholder is specified
	secret.Data[adminSecretNameKey] = []byte(adminSecretNameKey)
	_, err = clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	assert.NoError(t, err)
	_, _, _, err = CreateOrLoadClusterInfo(context, ctx, namespace, ownerInfo, cephClusterSpec)
	assert.Error(t, err)

	// Load the external cluster with the legacy external creds
	secret.Name = OperatorCreds
	secret.Data = map[string][]byte{
		"userID":  []byte("testid"),
		"userKey": []byte("testkey"),
	}
	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)
	info, _, _, err = CreateOrLoadClusterInfo(context, ctx, namespace, ownerInfo, cephClusterSpec)
	assert.NoError(t, err)
	assert.Equal(t, "testid", info.CephCred.Username)
	assert.Equal(t, "testkey", info.CephCred.Secret)
}
