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

package keyring

import (
	"context"
	"path"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateKey(t *testing.T) {
	clientset := testop.New(t, 1)
	generateKey := ""
	failGenerateKey := false
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if failGenerateKey {
				return "", errors.New("test error")
			}
			return "{\"key\": \"" + generateKey + "\"}", nil
		},
	}
	ctx := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}
	ns := "rook-ceph"
	ownerInfo := k8sutil.OwnerInfo{}
	s := GetSecretStore(ctx, cephclient.AdminTestClusterInfo(ns), &ownerInfo)

	generateKey = "generatedsecretkey"
	failGenerateKey = false
	k, e := s.GenerateKey("testuser", []string{"test", "access"})
	assert.NoError(t, e)
	assert.Equal(t, "generatedsecretkey", k)

	generateKey = "differentsecretkey"
	failGenerateKey = false
	k, e = s.GenerateKey("testuser", []string{"test", "access"})
	assert.NoError(t, e)
	assert.Equal(t, "differentsecretkey", k)

	// make sure error on fail
	generateKey = "failgeneratekey"
	failGenerateKey = true
	_, e = s.GenerateKey("newuser", []string{"new", "access"})
	assert.Error(t, e)
}

func TestKeyringStore(t *testing.T) {
	ctxt := context.TODO()
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	ns := "rook-ceph"
	k := GetSecretStore(ctx, &cephclient.ClusterInfo{Namespace: ns}, ownerInfo)

	assertKeyringData := func(keyringName, expectedKeyring string) {
		s, e := clientset.CoreV1().Secrets(ns).Get(ctxt, keyringName, metav1.GetOptions{})
		assert.NoError(t, e)
		assert.Equal(t, 1, len(s.StringData))
		assert.Equal(t, expectedKeyring, s.StringData["keyring"])
		assert.Equal(t, k8sutil.RookType, string(s.Type))
	}

	assertDoesNotExist := func(keyringName string) {
		_, e := clientset.CoreV1().Secrets(ns).Get(ctxt, keyringName, metav1.GetOptions{})
		assert.True(t, kerrors.IsNotFound(e))
	}

	// create first key
	_, err := k.CreateOrUpdate("test-resource", "qwertyuiop")
	assert.NoError(t, err)
	assertKeyringData("test-resource-keyring", "qwertyuiop")

	// create second key
	_, err = k.CreateOrUpdate("second-resource", "asdfghjkl")
	assert.NoError(t, err)
	assertKeyringData("test-resource-keyring", "qwertyuiop")
	assertKeyringData("second-resource-keyring", "asdfghjkl")

	// update a key
	_, err = k.CreateOrUpdate("second-resource", "lkjhgfdsa")
	assert.NoError(t, err)
	assertKeyringData("test-resource-keyring", "qwertyuiop")
	assertKeyringData("second-resource-keyring", "lkjhgfdsa")

	// delete a key
	err = k.Delete("test-resource")
	assert.NoError(t, err)
	assertDoesNotExist("test-resource-keyring")
	assertKeyringData("second-resource-keyring", "lkjhgfdsa")
}

func TestResourceVolumeAndMount(t *testing.T) {
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	k := GetSecretStore(ctx, &cephclient.ClusterInfo{Namespace: "ns"}, ownerInfo)
	_, err := k.CreateOrUpdate("test-resource", "qwertyuiop")
	assert.NoError(t, err)
	_, err = k.CreateOrUpdate("second-resource", "asdfgyhujkl")
	assert.NoError(t, err)

	v := Volume().Resource("test-resource")
	m := VolumeMount().Resource("test-resource")
	// Test that the secret will make it into containers with the appropriate filename at the
	// location where it is expected.
	assert.Equal(t, v.Name, m.Name)
	assert.Equal(t, "test-resource-keyring", v.VolumeSource.Secret.SecretName)
	assert.Equal(t, VolumeMount().KeyringFilePath(), path.Join(m.MountPath, keyringFileName))
}
