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
{"entity":"osd.0"},
{"entity":"client.admin"},
{"entity":"client.bootstrap-mds"},
{"entity":"client.bootstrap-rbd-mirror"},
{"entity":"client.csi-rbd-node.10"},
{"entity":"client.csi-rbd-node.4"},
{"entity":"client.csi-rbd-node.2"},
{"entity":"client.csi-rbd-node.1"},
{"entity":"client.csi-cepfs-node"},
{"entity":"client.csi-rbd-node"},
{"entity":"client.csi-rbd-provisioner"},
{"entity":"client.rbd-mirror-peer"},
{"entity":"mgr.a"},
{"entity":"client.csi-cepfs-node"},
{"entity":"client.csi-cephfs-node.2"}
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
	clientset := k8sfake.NewClientset()

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
	err := createOrUpdateCSISecret(clusterInfo, csiSecretStore{}, k)
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
		args []string
		want []string
	}{
		{
			args: []string{"client.csi-rbd-node.10", "client.csi-rbd-node.1", "client.csi-rbd-node.4", "client.csi-rbd-node.2", "client.csi-rbd-node"},
			want: []string{
				"client.csi-rbd-node", "client.csi-rbd-node.1", "client.csi-rbd-node.2", "client.csi-rbd-node.4", "client.csi-rbd-node.10",
			},
		},
		{
			args: []string{"client.csi-rbd-node"},
			want: []string{"client.csi-rbd-node"},
		},
		{
			args: []string{},
			want: []string{},
		},
		{
			// JFYI, // We'll not have list of mixed client name as argument to `sortCsiClientName`
			args: []string{"client.csi-cephfs-node.1", "client.csi-cephfs-node.2", "client.csi-rbd-node.1", "client.csi-cephfs-node", "client.csi-rbd-node"},
			want: []string{"client.csi-cephfs-node", "client.csi-rbd-node", "client.csi-cephfs-node.1", "client.csi-rbd-node.1", "client.csi-cephfs-node.2"},
		},
	}

	for _, tt := range tests {
		t.Run("testCases", func(t *testing.T) {
			got := sortCSIClientName(tt.args)
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

	var keyDeleted []string
	var err error

	// deleting nothing when keep prior count more than current number of existing prior keys
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 5)
	assert.NoError(t, err)
	assert.Equal(t, []string{}, keyDeleted)

	// deleting nothing when keep prior count is same as current number of existing prior keys
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 4)
	assert.NoError(t, err)
	assert.Equal(t, []string{}, keyDeleted)

	// Delete 1 oldest key ("client.csi-rbd-node") is deleted to retain the prior 3 generations.
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 3)
	assert.NoError(t, err)
	assert.Equal(t, []string{"client.csi-rbd-node"}, keyDeleted)

	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 2)
	assert.NoError(t, err)
	assert.Equal(t, []string{"client.csi-rbd-node", "client.csi-rbd-node.1"}, keyDeleted)

	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 1)
	assert.NoError(t, err)
	assert.Equal(t, []string{"client.csi-rbd-node", "client.csi-rbd-node.1", "client.csi-rbd-node.2"}, keyDeleted)

	// No keys should be deleted when the input key list is empty.
	keys = []string{}
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 0)
	assert.NoError(t, err)
	assert.Equal(t, []string{}, keyDeleted)

	// When only one key is present and keepCount is 0, no keys should be deleted.
	keys = []string{"client.csi-rbd-node"}
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 0)
	assert.NoError(t, err)
	assert.Equal(t, []string{}, keyDeleted)

	// When only one key is present and keepCount is 1, no keys should be deleted.
	keys = []string{"client.csi-rbd-node"}
	keyDeleted, err = deleteOldKeyGen(ctx, clusterInfo, keys, 1)
	assert.NoError(t, err)
	assert.Equal(t, []string{}, keyDeleted)
}

func TestGetCsiKeyRotationInfo(t *testing.T) {
	ctx, clusterInfo, _ := loadTestClusterDetails()

	// Different mock auth responses for testing different currentMaxKeyGen values
	mockAuthLsWithMaxGen10 := `{"auth_dump":[
	{"entity":"osd.0"},{"entity":"client.admin"},{"entity":"client.bootstrap-mds"},{"entity":"client.bootstrap-rbd-mirror"},{"entity":"client.csi-rbd-node.10"},
	{"entity":"client.csi-rbd-node.4"},{"entity":"client.csi-rbd-node.2"},{"entity":"client.csi-rbd-node.1"},{"entity":"client.csi-cepfs-node"},
	{"entity":"client.csi-rbd-node"},{"entity":"client.csi-rbd-provisioner"},{"entity":"client.rbd-mirror-peer"},{"entity":"mgr.a"},
	{"entity":"client.csi-cepfs-node"},{"entity":"client.csi-cephfs-node.2"}
	]}`

	mockAuthLsWithMaxGen5 := `{"auth_dump":[
	{"entity":"osd.0"},{"entity":"client.admin"},{"entity":"client.bootstrap-mds"},{"entity":"client.bootstrap-rbd-mirror"},
	{"entity":"client.csi-rbd-node.5"},{"entity":"client.csi-rbd-node.3"},{"entity":"client.csi-rbd-node.1"},{"entity":"client.csi-cepfs-node"},
	{"entity":"client.csi-rbd-node"},{"entity":"client.csi-rbd-provisioner"},{"entity":"client.rbd-mirror-peer"},{"entity":"mgr.a"}
	]}`

	mockAuthLsWithMaxGen0 := `{"auth_dump":[
	{"entity":"osd.0"},{"entity":"client.admin"},{"entity":"client.bootstrap-mds"},{"entity":"client.bootstrap-rbd-mirror"},
	{"entity":"client.csi-cepfs-node"},{"entity":"client.csi-rbd-node"},{"entity":"client.csi-rbd-provisioner"},{"entity":"client.rbd-mirror-peer"},{"entity":"mgr.a"}
	]}`

	mockAuthLsEmpty := `{"auth_dump":[]}`

	mockAuthLsNoCSIKeys := `{"auth_dump":[
	{"entity":"osd.0"},{"entity":"client.admin"},{"entity":"client.bootstrap-mds"},{"entity":"client.bootstrap-rbd-mirror"},
	{"entity":"client.rbd-mirror-peer"},{"entity":"mgr.a"}
	]}`

	tests := []struct {
		name              string
		mockAuthResponse  string
		mockError         error
		expectedMaxKeyGen int
		expectedKeyList   []string
		expectError       bool
		errorType         string
	}{
		{
			name:              "currentMaxKeyGen 10 with multiple generations",
			mockAuthResponse:  mockAuthLsWithMaxGen10,
			mockError:         nil,
			expectedMaxKeyGen: 10,
			expectedKeyList:   []string{"client.csi-rbd-node.10", "client.csi-rbd-node.4", "client.csi-rbd-node.2", "client.csi-rbd-node.1", "client.csi-rbd-node"},
			expectError:       false,
		},
		{
			name:              "currentMaxKeyGen 5 with fewer generations",
			mockAuthResponse:  mockAuthLsWithMaxGen5,
			mockError:         nil,
			expectedMaxKeyGen: 5,
			expectedKeyList:   []string{"client.csi-rbd-node.5", "client.csi-rbd-node.3", "client.csi-rbd-node.1", "client.csi-rbd-node"},
			expectError:       false,
		},
		{
			name:              "currentMaxKeyGen 0 with only base name",
			mockAuthResponse:  mockAuthLsWithMaxGen0,
			mockError:         nil,
			expectedMaxKeyGen: 0,
			expectedKeyList:   []string{"client.csi-rbd-node"},
			expectError:       false,
		},
		{
			name:              "empty auth list returns no keys",
			mockAuthResponse:  mockAuthLsEmpty,
			mockError:         nil,
			expectedMaxKeyGen: 0,
			expectedKeyList:   []string{},
			expectError:       false,
		},
		{
			name:              "no CSI keys found returns empty list",
			mockAuthResponse:  mockAuthLsNoCSIKeys,
			mockError:         nil,
			expectedMaxKeyGen: 0,
			expectedKeyList:   []string{},
			expectError:       false,
		},
		{
			name:              "auth list command fails with timeout error",
			mockAuthResponse:  "",
			mockError:         errors.New("timeout: command execution exceeded time limit"),
			expectedMaxKeyGen: 0,
			expectedKeyList:   []string{},
			expectError:       true,
			errorType:         "timeout",
		},
		{
			name:              "invalid JSON response from auth list",
			mockAuthResponse:  `{"auth_dump":[invalid json}`,
			mockError:         nil,
			expectedMaxKeyGen: 0,
			expectedKeyList:   []string{},
			expectError:       true,
			errorType:         "invalid_json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &exectest.MockExecutor{}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "auth" && args[1] == "ls" {
					if tt.mockError != nil {
						return "", tt.mockError
					}
					return tt.mockAuthResponse, nil
				}
				return "", errors.New("unknown command")
			}
			ctx.Executor = executor

			currentMaxKeyGen, keysWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, csiKeyringRBDNodeUsername)

			if tt.expectError {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
				assert.Equal(t, tt.expectedMaxKeyGen, currentMaxKeyGen, "Max key generation should be 0 on error")
				assert.Equal(t, tt.expectedKeyList, keysWithBaseName, "Key list should be empty on error")

				if tt.errorType == "timeout" {
					assert.Contains(t, err.Error(), "timeout", "Error should contain timeout message")
				}
			} else {
				assert.NoError(t, err, "No error expected for test case: %s", tt.name)
				assert.Equal(t, tt.expectedMaxKeyGen, currentMaxKeyGen, "Max key generation mismatch for test case: %s", tt.name)
				assert.Equal(t, tt.expectedKeyList, keysWithBaseName, "Key list mismatch for test case: %s", tt.name)
			}
		})
	}
}

func TestGetCSIKeyInfoAndDeleteOldKey(t *testing.T) {
	ctx, clusterInfo, clusterSpec := loadTestClusterDetails()
	clusterSpec.Security.CephX.CSI.KeyGeneration = 18
	executor := &exectest.MockExecutor{}
	ctx.Executor = executor

	keyList := []string{"client.csi-rbd-node.10", "client.csi-rbd-node.4", "client.csi-rbd-node.2", "client.csi-rbd-node.1", "client.csi-rbd-node"}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "auth" && args[1] == "ls" {
			return mockAuthLs, nil
		}
		panic(fmt.Sprintf("unexpected command %s %v", command, args))
	}
	currentMaxKeyGen, keyWithBaseName, err := getCsiKeyRotationInfo(ctx, clusterInfo, csiKeyringRBDNodeUsername)
	assert.NoError(t, err)
	assert.Equal(t, 10, currentMaxKeyGen)
	assert.Equal(t, keyList, keyWithBaseName)

	tests := []struct {
		name        string
		keyList     []string
		keepCount   uint8
		wantDeleted []string
	}{
		{"test deletion", keyWithBaseName, 2, []string{"client.csi-rbd-node", "client.csi-rbd-node.1"}},
		{"keep count = current gen", keyWithBaseName, 10, []string{}},
		{"keep count > current gen", keyWithBaseName, 11, []string{}},
		{"keep count 0", keyWithBaseName, 0, []string{"client.csi-rbd-node", "client.csi-rbd-node.1", "client.csi-rbd-node.2", "client.csi-rbd-node.4"}},
		{"keep count 1", keyWithBaseName, 1, []string{"client.csi-rbd-node", "client.csi-rbd-node.1", "client.csi-rbd-node.2"}},
		{"keep count 2", keyWithBaseName, 2, []string{"client.csi-rbd-node", "client.csi-rbd-node.1"}},
		{"keep count 3", keyWithBaseName, 3, []string{"client.csi-rbd-node"}},
		{"keep count = current count - 1 = 4", keyWithBaseName, 4, []string{}},
		{"keep count = current count = 5", keyWithBaseName, 5, []string{}},
		{"keep count > current count", keyWithBaseName, 6, []string{}},
		{"key list empty, keep count 0", []string{}, 0, []string{}},
		{"key list empty, keep count 1", []string{}, 1, []string{}},
		{"key list empty, keep count 3", []string{}, 3, []string{}},
		{"key list with 1 item, keep count 0", []string{"client.csi-rbd-node"}, 0, []string{}},
		{"key list with 1 item, keep count 1", []string{"client.csi-rbd-node"}, 1, []string{}},
		{"key list with 1 item, keep count 3", []string{"client.csi-rbd-node"}, 3, []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			delKeyArgList := []string{}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "auth" && args[1] == "ls" {
					return mockAuthLs, nil
				} else if args[0] == "auth" && args[1] == "del" {
					delKeyArgList = append(delKeyArgList, args[2])
					return "", nil
				}
				panic(fmt.Sprintf("unexpected command %s %v", command, args))
			}
			keyDeleted, err := deleteOldKeyGen(ctx, clusterInfo, test.keyList, test.keepCount)
			assert.NoError(t, err)
			assert.Equal(t, test.wantDeleted, keyDeleted)
			assert.Equal(t, test.wantDeleted, delKeyArgList)
		})
	}
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

	clientset := k8sfake.NewClientset()

	ctx := &clusterd.Context{
		Clientset: clientset,
	}

	return ctx, clusterInfo, clusterSpec
}

func Test_getMatchingClient(t *testing.T) {
	clientBaseName := csiKeyringRBDNodeUsername
	want := []string{clientBaseName, clientBaseName + ".1", clientBaseName + ".2", clientBaseName + ".3"}

	t.Run("auth list is valid", func(t *testing.T) {
		authList := client.AuthListOutput{
			AuthDump: []client.AuthListEntry{
				{Entity: clientBaseName}, {Entity: clientBaseName + ".1"}, {Entity: clientBaseName + ".2"}, {Entity: clientBaseName + ".3"}, {Entity: csiKeyringRBDProvisionerUsername + ".4"},
			},
		}
		got, err := getMatchingClient(authList, clientBaseName)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	})

	// When fresh cluster is deployed `authList` will be empty
	t.Run("auth list empty", func(t *testing.T) {
		authList := client.AuthListOutput{
			AuthDump: []client.AuthListEntry{},
		}
		got, err := getMatchingClient(authList, clientBaseName)
		assert.NoError(t, err)
		assert.Equal(t, []string{}, got)
	})

	// When no matching key found in `authList` means key is created for first time
	t.Run("auth list doesn't contain match", func(t *testing.T) {
		authList := client.AuthListOutput{
			AuthDump: []client.AuthListEntry{
				{Entity: clientBaseName}, {Entity: clientBaseName + ".1"}, {Entity: clientBaseName + ".2"}, {Entity: clientBaseName + ".3"},
			},
		}
		got, err := getMatchingClient(authList, csiKeyringRBDProvisionerUsername)
		assert.NoError(t, err)
		assert.Equal(t, []string{}, got)
	})
}

func TestParseCsiClient(t *testing.T) {
	tests := []struct {
		args     string
		baseName string
		keyGen   int
		wantErr  bool
	}{
		{args: "client.csi-rbd-node", baseName: "client.csi-rbd-node", keyGen: 0, wantErr: false},
		{args: "client.csi-rbd-node.10", baseName: "client.csi-rbd-node", keyGen: 10, wantErr: false},
		{args: "client.csi-rbd-node.9", baseName: "client.csi-rbd-node", keyGen: 9, wantErr: false},
		{args: "client.csi-rbd-node.", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node.1a", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node.a1", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node. ", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node. 1", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node .1", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node.1 ", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node. a", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node .a", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node.a ", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node. a1", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node.a1 ", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node. a1", baseName: "", keyGen: 0, wantErr: true},
		{args: "      client.csi-rbd-node. ", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node. 1a", baseName: "", keyGen: 0, wantErr: true},
		{args: "client.csi-rbd-node .1a", baseName: "", keyGen: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
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

func TestGetPriorKeyCount(t *testing.T) {
	tests := []struct {
		name     string
		keyCount int
		want     int
	}{
		{"keyGen 0", 0, 0},
		{"keyGen 1", 1, 0},
		{"keyGen 2", 2, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPriorKeyCount(tt.keyCount)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_deleteCount(t *testing.T) {
	tests := []struct {
		numKeys        int
		keepPriorCount int
		want           int
	}{
		{2, 0, 1},
		{3, 0, 2},
		{4, 0, 3},
		{2, 1, 0},
		{3, 1, 1},
		{4, 1, 2},
		{2, 2, 0},
		{3, 2, 0},
		{4, 2, 1},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d,%d", tt.numKeys, tt.keepPriorCount), func(t *testing.T) {
			if got := deleteCount(tt.numKeys, tt.keepPriorCount); got != tt.want {
				t.Errorf("deleteCount() = %v, want %v", got, tt.want)
			}
		})
	}
}
