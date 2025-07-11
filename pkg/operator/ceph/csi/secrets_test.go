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
	"fmt"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const (
	mockAuthLs = `{"auth_dump":[
	{"entity":"osd.0"},{"entity":"client.admin"},{"entity":"client.bootstrap-mds"},{"entity":"client.bootstrap-rbd-mirror"},{"entity":"client.csi-rbd-node.10"},
	{"entity":"client.csi-rbd-node.4"},{"entity":"client.csi-rbd-node.2"},{"entity":"client.csi-rbd-node.1"},{"entity":"client.csi-cepfs-node"},
	{"entity":"client.csi-rbd-node"},{"entity":"client.csi-rbd-provisioner"},{"entity":"client.rbd-mirror-peer"},{"entity":"mgr.a"},
	{"entity":"client.csi-cepfs-node"},{"entity":"client.csi-cephfs-node.2"}
	]}`
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

func TestSortCsiClientName(t *testing.T) {
	tests := []struct {
		args    []string
		want    []string
		wantErr bool
	}{
		{
			args: []string{"client.csi-rbd-node.10", "client.csi-rbd-node.1", "client.csi-rbd-node.4", "client.csi-rbd-node.2", "client.csi-rbd-node"},
			want: []string{
				"client.csi-rbd-node", "client.csi-rbd-node.1", "client.csi-rbd-node.2", "client.csi-rbd-node.4", "client.csi-rbd-node.10",
			},
			wantErr: false,
		},
		{
			args:    []string{"client.csi-rbd-node"},
			want:    []string{"client.csi-rbd-node"},
			wantErr: false,
		},
		{
			args:    []string{},
			want:    []string{},
			wantErr: true,
		},
		{
			// JFYI, // We'll not have list of mixed client name as argument to `sortCsiClientName`
			args:    []string{"client.csi-cephfs-node.1", "client.csi-cephfs-node.2", "client.csi-rbd-node.1", "client.csi-cephfs-node", "client.csi-rbd-node"},
			want:    []string{"client.csi-cephfs-node", "client.csi-rbd-node", "client.csi-cephfs-node.1", "client.csi-rbd-node.1", "client.csi-cephfs-node.2"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run("testCases", func(t *testing.T) {
			got, err := sortCSIClientName(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("sortCsiClientName() error = %v, want eroor %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sortCsiClientName() go = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeleteOldKeyGen(t *testing.T) {
	ctx, clusterInfo, _ := loadTestClusterDetails()
	ctx.Executor = &exectest.MockExecutor{}

	keys := []string{
		"client.csi-rbd-node.10", "client.csi-rbd-node.1", "client.csi-rbd-node.4", "client.csi-rbd-node.2", "client.csi-rbd-node",
	}

	keyDeleted, err := deleteOldKeyGen(ctx, clusterInfo, keys, 3)
	assert.NoError(t, err)
	assert.Equal(t, []string{"client.csi-rbd-node", "client.csi-rbd-node.1"}, keyDeleted)

	keys = []string{"client.csi-rbd-node.10", "client.csi-rbd-node.1", "client.csi-rbd-node.4"}
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 1)
	assert.NoError(t, err)
	assert.Equal(t, []string{"client.csi-rbd-node.1", "client.csi-rbd-node.4"}, keyDeleted)

	keys = []string{}
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 0)
	assert.Error(t, err)
	assert.Equal(t, []string{}, keyDeleted)

	keys = []string{"client.csi-rbd-node"}
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 0)
	assert.NoError(t, err)
	assert.Equal(t, keys, keyDeleted)

	keys = []string{"client.csi-rbd-node"}
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 1)
	assert.NoError(t, err)
	assert.Equal(t, []string{}, keyDeleted)
}

func TestGetCsiKeyRotationInfo(t *testing.T) {
	ctx, clusterInfo, clusterSpec := loadTestClusterDetails()
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "auth" && args[1] == "ls" {
			return mockAuthLs, nil
		}
		return "", errors.New("unknown command")
	}
	ctx.Executor = executor

	expectedKeyList := []string{"client.csi-rbd-node.10", "client.csi-rbd-node.4", "client.csi-rbd-node.2", "client.csi-rbd-node.1", "client.csi-rbd-node"}

	t.Run("When keyGeneration is not set", func(t *testing.T) {
		cSpec := clusterSpec.DeepCopy()

		// When `keyGeneration` is not set `shouldRotate` should be `false`
		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		assert.False(t, shouldRotate)

		assert.Equal(t, expectedKeyList, keysWithBaseName)
		assert.NoError(t, err)
	})

	t.Run("keyGeneration is less than current maxCount", func(t *testing.T) {
		cSpec := clusterSpec.DeepCopy()
		cSpec.Security.CephX.CSI.KeyGeneration = 2

		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		// Since `keyGeneration` is lower than current maxCount `shouldRoate` should return `false`
		assert.False(t, shouldRotate)

		assert.Equal(t, expectedKeyList, keysWithBaseName)
		assert.NoError(t, err)
	})

	t.Run("keyGeneration one less than current maxCount", func(t *testing.T) {
		cSpec := clusterSpec.DeepCopy()
		cSpec.Security.CephX.CSI.KeyGeneration = 9

		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		// Since `keyGeneration` is one less than current maxCount `shouldRoate` should return `false`
		assert.False(t, shouldRotate)

		assert.Equal(t, expectedKeyList, keysWithBaseName)
		assert.NoError(t, err)
	})

	t.Run("keyGeneration equal current maxCount", func(t *testing.T) {
		cSpec := clusterSpec.DeepCopy()
		cSpec.Security.CephX.CSI.KeyGeneration = 10

		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		// Since `keyGeneration` is equal to the current maxCount `shouldRoate` should return `false`
		assert.False(t, shouldRotate)

		assert.Equal(t, expectedKeyList, keysWithBaseName)
		assert.NoError(t, err)
	})

	t.Run("keyGeneration greater than current maxCount", func(t *testing.T) {
		cSpec := clusterSpec.DeepCopy()
		cSpec.Security.CephX.CSI.KeyGeneration = 11

		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		assert.True(t, shouldRotate)

		assert.Equal(t, expectedKeyList, keysWithBaseName)
		assert.NoError(t, err)
	})

	t.Run("keyGeneration more greater than current maxCount", func(t *testing.T) {
		cSpec := clusterSpec.DeepCopy()
		cSpec.Security.CephX.CSI.KeyGeneration = 18

		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		assert.True(t, shouldRotate)

		assert.Equal(t, expectedKeyList, keysWithBaseName)
		assert.NoError(t, err)
	})

	t.Run("keepPriorKeyCountMax` is random value it should not affect the `shouldRotate`", func(t *testing.T) {
		cSpec := clusterSpec.DeepCopy()
		cSpec.Security.CephX.CSI.KeyGeneration = 1
		cSpec.Security.CephX.CSI.KeepPriorKeyCountMax = 0

		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		assert.False(t, shouldRotate)

		assert.Equal(t, expectedKeyList, keysWithBaseName)
		assert.NoError(t, err)
	})

	t.Run("keepPriorKeyCountMax is random value it should not affect the `shouldRotate`", func(t *testing.T) {
		cSpec := clusterSpec.DeepCopy()
		cSpec.Security.CephX.CSI.KeyGeneration = 18
		cSpec.Security.CephX.CSI.KeepPriorKeyCountMax = 2

		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		assert.True(t, shouldRotate)

		assert.Equal(t, expectedKeyList, keysWithBaseName)
		assert.NoError(t, err)
	})

	t.Run("When `authLsList return error`", func(t *testing.T) {
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "auth" && args[1] == "ls" {
				return "", errors.New("error")
			}
			return "", errors.New("unknown command")
		}
		ctx.Executor = executor

		cSpec := clusterSpec.DeepCopy()
		cSpec.Security.CephX.CSI.KeyGeneration = 18

		shouldRotate, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, cSpec, csiKeyringRBDNodeUsername)
		assert.False(t, shouldRotate)

		assert.Equal(t, []string{}, keysWithBaseName)
		assert.Error(t, err)
	})
}

func TestGetKeySuffixNumber(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    int
		wantErr bool
	}{
		{name: "test-1", args: "client.csi-rbd-node", want: 0, wantErr: false},
		{name: "test-2", args: "client.csi-rbd-node.10", want: 10, wantErr: false},
		{name: "test-3", args: "client.csi-rbd-node.9", want: 9, wantErr: false},
		{name: "test-4", args: "client.csi-rbd-node.", want: 0, wantErr: true},
		{name: "test-5", args: "client.csi-rbd-node.1a", want: 0, wantErr: true},
		{name: "test-6", args: "client.csi-rbd-node.a1", want: 0, wantErr: true},
		{name: "test-7", args: "client.csi-rbd-node. ", want: 0, wantErr: true},
		{name: "test-8", args: "client.csi-rbd-node. 1", want: 0, wantErr: true},
		{name: "test-9", args: "client.csi-rbd-node .1", want: 0, wantErr: true},
		{name: "test-10", args: "client.csi-rbd-node.1 ", want: 0, wantErr: true},
		{name: "test-11", args: "client.csi-rbd-node. a", want: 0, wantErr: true},
		{name: "test-12", args: "client.csi-rbd-node .a", want: 0, wantErr: true},
		{name: "test-13", args: "client.csi-rbd-node.a ", want: 0, wantErr: true},
		{name: "test-14", args: "client.csi-rbd-node. a1", want: 0, wantErr: true},
		{name: "test-15", args: "client.csi-rbd-node.a1 ", want: 0, wantErr: true},
		{name: "test-16", args: "client.csi-rbd-node. a1", want: 0, wantErr: true},
		{name: "test-17", args: "      client.csi-rbd-node. ", want: 0, wantErr: true},
		{name: "test-18", args: "client.csi-rbd-node. 1a", want: 0, wantErr: true},
		{name: "test-19", args: "client.csi-rbd-node .1a", want: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run("testCases", func(t *testing.T) {
			_, got, err := parseCsiClient(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s getKeySuffixNumber() error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("%s getKeySuffixNumber() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestGetCSIKeyInfoAndDeleteOldKey(t *testing.T) {
	ctx, clusterInfo, clusterSpec := loadTestClusterDetails()
	clusterSpec.Security.CephX.CSI.KeyGeneration = 18
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "auth" && args[1] == "ls" {
			return mockAuthLs, nil
		} else if args[0] == "auth" && args[1] == "del" {
			return "", nil
		}
		panic(fmt.Sprintf("unexpected command %s %v", command, args))
	}
	ctx.Executor = executor

	t.Run("Validate right keys are deleted", func(t *testing.T) {
		keyList := []string{"client.csi-rbd-node.10", "client.csi-rbd-node.4", "client.csi-rbd-node.2", "client.csi-rbd-node.1", "client.csi-rbd-node"}
		shouldRotate, keyWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, clusterSpec, csiKeyringRBDNodeUsername)
		assert.True(t, shouldRotate)
		assert.Equal(t, keyList, keyWithBaseName)
		assert.NoError(t, err)

		keyDeleted, err := deleteOldKeyGen(ctx, clusterInfo, keyWithBaseName, 3)
		assert.NoError(t, err)
		assert.Equal(t, []string{"client.csi-rbd-node", "client.csi-rbd-node.1"}, keyDeleted)

		mockDeleteAuthLs := `{"auth_dump":[
	{"entity":"osd.0"},{"entity":"client.admin"},{"entity":"client.bootstrap-mds"},{"entity":"client.bootstrap-rbd-mirror"},{"entity":"client.csi-rbd-node.10"},
	{"entity":"client.csi-rbd-node.4"},{"entity":"client.csi-cepfs-node"},{"entity":"client.csi-rbd-provisioner"},{"entity":"client.rbd-mirror-peer"},{"entity":"mgr.a"}
	]}`
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "auth" && args[1] == "ls" {
				return mockDeleteAuthLs, nil
			}
			panic(fmt.Sprintf("unexpected command %s %v", command, args))
		}
		ctx.Executor = executor

		keyList = []string{"client.csi-rbd-node.10", "client.csi-rbd-node.4"}
		shouldRotate, keyWithBaseName, err = getCsiKeyRotationInfo(ctx, clusterInfo, clusterSpec, csiKeyringRBDNodeUsername)
		assert.True(t, shouldRotate)
		assert.Equal(t, keyList, keyWithBaseName)
		assert.NoError(t, err)
	})
}

func loadTestClusterDetails() (*clusterd.Context, *client.ClusterInfo, *cephv1.ClusterSpec) {
	namespace := "rook-ceph"
	cephCluster := "my-ceph-cluster"

	clusterInfo := &client.ClusterInfo{
		OwnerInfo: k8sutil.NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{
			APIVersion: "ceph.rook.io/v1",
			Kind:       "CephCluster",
			Name:       cephCluster,
		}, namespace),
		Context:     context.TODO(),
		CephVersion: keyring.CephAuthRotateSupportedVersion,
	}
	clusterInfo.Namespace = namespace
	clusterInfo.SetName(cephCluster)

	clusterSpec := &cephv1.ClusterSpec{
		Security: cephv1.ClusterSecuritySpec{
			CephX: cephv1.ClusterCephxConfig{
				CSI: cephv1.CephXConfigWithPriorCount{
					CephxConfig: cephv1.CephxConfig{
						KeyRotationPolicy: "KeyGeneration",
					},
				},
			},
		},
	}

	clientset := k8sfake.NewSimpleClientset()

	ctx := &clusterd.Context{
		Clientset: clientset,
	}

	return ctx, clusterInfo, clusterSpec
}

func Test_getMatchingClient(t *testing.T) {
	clientBaseName := csiKeyringRBDNodeUsername
	want := []string{clientBaseName, clientBaseName + ".1", clientBaseName + ".2", clientBaseName + ".3"}

	t.Run("When `authLsList` is valid", func(t *testing.T) {
		authList := client.AuthList{
			AuthDump: []client.AuthListEntry{
				{Entity: clientBaseName}, {Entity: clientBaseName + ".1"}, {Entity: clientBaseName + ".2"}, {Entity: clientBaseName + ".3"}, {Entity: csiKeyringRBDProvisionerUsername + ".4"},
			},
		}
		got, err := getMatchingClient(authList, clientBaseName)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("When `authLsList` is empty", func(t *testing.T) {
		authList := client.AuthList{
			AuthDump: []client.AuthListEntry{},
		}
		got, err := getMatchingClient(authList, clientBaseName)
		assert.Error(t, err)
		assert.Equal(t, []string{}, got)
	})

	t.Run("When `authLsList` is non-empty but with non-matching user name", func(t *testing.T) {
		authList := client.AuthList{
			AuthDump: []client.AuthListEntry{
				{Entity: clientBaseName}, {Entity: clientBaseName + ".1"}, {Entity: clientBaseName + ".2"}, {Entity: clientBaseName + ".3"},
			},
		}
		got, err := getMatchingClient(authList, csiKeyringRBDProvisionerUsername)
		assert.Error(t, err)
		assert.Equal(t, []string{}, got)
	})
}

func Test_parseCsiClient(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		baseName string
		keyGen   int
		wantErr  bool
	}{
		{name: "test-1", args: "client.csi-rbd-node", baseName: "client.csi-rbd-node", keyGen: 0, wantErr: false},
		{name: "test-2", args: "client.csi-rbd-node.10", baseName: "client.csi-rbd-node", keyGen: 10, wantErr: false},
		{name: "test-3", args: "client.csi-rbd-node.9", baseName: "client.csi-rbd-node", keyGen: 9, wantErr: false},
		{name: "test-4", args: "client.csi-rbd-node.", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-5", args: "client.csi-rbd-node.1a", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-6", args: "client.csi-rbd-node.a1", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-7", args: "client.csi-rbd-node. ", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-8", args: "client.csi-rbd-node. 1", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-9", args: "client.csi-rbd-node .1", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-10", args: "client.csi-rbd-node.1 ", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-11", args: "client.csi-rbd-node. a", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-12", args: "client.csi-rbd-node .a", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-13", args: "client.csi-rbd-node.a ", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-14", args: "client.csi-rbd-node. a1", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-15", args: "client.csi-rbd-node.a1 ", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-16", args: "client.csi-rbd-node. a1", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-17", args: "      client.csi-rbd-node. ", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-18", args: "client.csi-rbd-node. 1a", baseName: "", keyGen: 0, wantErr: true},
		{name: "test-19", args: "client.csi-rbd-node .1a", baseName: "", keyGen: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := parseCsiClient(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCsiClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.baseName {
				t.Errorf("parseCsiClient() got = %v, want %v", got, tt.baseName)
			}
			if got1 != tt.keyGen {
				t.Errorf("parseCsiClient() got1 = %v, want %v", got1, tt.keyGen)
			}
		})
	}
}
