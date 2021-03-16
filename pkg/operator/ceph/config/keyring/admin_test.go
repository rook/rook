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
	"fmt"
	"path"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAdminKeyringStore(t *testing.T) {
	ctxt := context.TODO()
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "test-ns"
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := clienttest.CreateTestClusterInfo(1)
	clusterInfo.Namespace = ns
	k := GetSecretStore(ctx, clusterInfo, ownerInfo)

	assertKeyringData := func(expectedKeyring string) {
		s, e := clientset.CoreV1().Secrets(ns).Get(ctxt, "rook-ceph-admin-keyring", metav1.GetOptions{})
		assert.NoError(t, e)
		assert.Equal(t, 1, len(s.StringData))
		assert.Equal(t, expectedKeyring, s.StringData["keyring"])
		assert.Equal(t, k8sutil.RookType, string(s.Type))
	}

	// create key
	clusterInfo.CephCred.Secret = "adminsecretkey"
	err := k.Admin().CreateOrUpdate(clusterInfo)
	assert.NoError(t, err)
	assertKeyringData(fmt.Sprintf(adminKeyringTemplate, "adminsecretkey"))

	// update key
	clusterInfo.CephCred.Secret = "differentsecretkey"
	err = k.Admin().CreateOrUpdate(clusterInfo)
	assert.NoError(t, err)
	assertKeyringData(fmt.Sprintf(adminKeyringTemplate, "differentsecretkey"))
}

func TestAdminVolumeAndMount(t *testing.T) {
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := clienttest.CreateTestClusterInfo(1)
	s := GetSecretStore(ctx, clusterInfo, ownerInfo)

	clusterInfo.CephCred.Secret = "adminsecretkey"
	err := s.Admin().CreateOrUpdate(clusterInfo)
	assert.NoError(t, err)

	v := Volume().Admin()
	m := VolumeMount().Admin()
	// Test that the secret will make it into containers with the appropriate filename at the
	// location where it is expected.
	assert.Equal(t, v.Name, m.Name)
	assert.Equal(t, "rook-ceph-admin-keyring", v.VolumeSource.Secret.SecretName)
	assert.Equal(t, VolumeMount().AdminKeyringFilePath(), path.Join(m.MountPath, keyringFileName))
}
