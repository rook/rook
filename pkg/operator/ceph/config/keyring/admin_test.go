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
	"fmt"
	"path"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAdminKeyringStore(t *testing.T) {
	clientset := testop.New(1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "rook-ceph"
	owner := metav1.OwnerReference{}
	clusterInfo := testop.CreateConfigDir(1)
	k := GetSecretStore(ctx, ns, &owner)

	assertKeyringData := func(expectedKeyring string) {
		s, e := clientset.CoreV1().Secrets(ns).Get("rook-ceph-admin-keyring", metav1.GetOptions{})
		assert.NoError(t, e)
		assert.Equal(t, 1, len(s.StringData))
		assert.Equal(t, expectedKeyring, s.StringData["keyring"])
		assert.Equal(t, k8sutil.RookType, string(s.Type))
	}

	// create key
	clusterInfo.AdminSecret = "adminsecretkey"
	k.Admin().CreateOrUpdate(clusterInfo)
	assertKeyringData(fmt.Sprintf(adminKeyringTemplate, "adminsecretkey"))

	// update key
	clusterInfo.AdminSecret = "differentsecretkey"
	k.Admin().CreateOrUpdate(clusterInfo)
	assertKeyringData(fmt.Sprintf(adminKeyringTemplate, "differentsecretkey"))
}

func TestAdminVolumeAndMount(t *testing.T) {
	clientset := testop.New(1)
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "rook-ceph"
	owner := metav1.OwnerReference{}
	clusterInfo := testop.CreateConfigDir(1)
	s := GetSecretStore(ctx, ns, &owner)

	clusterInfo.AdminSecret = "adminsecretkey"
	s.Admin().CreateOrUpdate(clusterInfo)

	v := Volume().Admin()
	m := VolumeMount().Admin()
	// Test that the secret will make it into containers with the appropriate filename at the
	// location where it is expected.
	assert.Equal(t, v.Name, m.Name)
	assert.Equal(t, "rook-ceph-admin-keyring", v.VolumeSource.Secret.SecretName)
	assert.Equal(t, VolumeMount().AdminKeyringFilePath(), path.Join(m.MountPath, keyringFileName))
}
