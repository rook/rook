/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package cluster

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_genKeyring(t *testing.T) {
	twoCapsKeyring := `[client.them]
	key = themkey==
	caps any = "thing"
`
	adminRotatorKeyring := `[client.admin-rotator]
	key = adminrotatorkey==
	caps mds = "allow *"
	caps mon = "allow *"
	caps osd = "allow *"
	caps mgr = "allow *"
`

	tests := []struct {
		name       string
		clientName string
		clientKey  string
		clientCaps []string
		wantErr    bool
		want       string
	}{
		{"no caps", "client.me", "mekey==", []string{}, false, "[client.me]\n	key = mekey==\n"},
		{"no caps, no key", "client.me", "", []string{}, true, ""},
		{"one cap", "client.you", "youkey==", []string{"bad"}, true, ""},
		{"two caps", "client.them", "themkey==", []string{"any", "thing"}, false, twoCapsKeyring},
		{"three caps", "client.batman", "batmankey==", []string{"some", "thing", "else"}, true, ""},
		{"admin rotator caps", "client.admin-rotator", "adminrotatorkey==", adminKeyAccessCaps, false, adminRotatorKeyring},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := genKeyring(tt.clientName, tt.clientKey, tt.clientCaps)
			if (err != nil) != tt.wantErr {
				t.Errorf("genKeyring() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("genKeyring() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_admin_key_rotation(t *testing.T) {
	logger.SetLevel(capnslog.DEBUG) // TESTING TESTING TESTING

	reloadManagerCalled := false
	reloadManagerFunc = func() {
		reloadManagerCalled = true
	}

	// this feature requires commands to execute in a specific order and often with specific arg(s)
	// it also requires commands to not be skipped
	// set up an executor that allows tests to expect commands in a certain order
	type expect struct {
		cargs      []string
		withArgs   []string
		returnFunc func(cmd string, args ...string) (string, error)
	}
	numCalls := 0
	expectList := []expect{}

	mockExecutor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(cmd string, args ...string) (string, error) {
			cargs := append([]string{cmd}, args...) // cmd+args in one slice for easy comparison
			t.Logf("mock command: %v", cargs)

			// get current command expectations
			if len(expectList) <= numCalls {
				e := fmt.Errorf("execution iteration=%d: no expectation set for called %v", numCalls, cargs)
				t.Error(e)
				return "", e
			}
			currentExpect := expectList[numCalls]
			numCalls++

			// assert current command matches expectations
			if !isCommand(cargs, currentExpect.cargs...) {
				e := fmt.Errorf("execution iteration=%d: expected command %v does not match called %v", numCalls, currentExpect.cargs, cargs)
				t.Error(e)
				return "", e
			}
			if len(currentExpect.withArgs) > 0 {
				for _, arg := range currentExpect.withArgs {
					if !hasArg(cargs, arg) {
						e := fmt.Errorf("execution iteration=%d: expected arg %q missing in called %v", numCalls, arg, cargs)
						t.Error(e)
						return "", e
					}
				}
			}

			// expectations met, call return func with args
			return currentExpect.returnFunc(cmd, args...)
		},
	}

	tmpCfgDir, err := os.MkdirTemp("", "")
	t.Logf("temp dir: %q", tmpCfgDir)
	assert.NoError(t, err)
	defer func() { os.RemoveAll(tmpCfgDir) }()

	ns := "ns"
	clusterName := "my-cluster"

	newTest := func() (*clusterd.Context, *client.ClusterInfo, *k8sutil.OwnerInfo, *cephv1.CephCluster) {
		reloadManagerCalled = false
		numCalls = 0
		expectList = []expect{} // clear expect list

		clusterInfo := &client.ClusterInfo{
			Context:       context.TODO(),
			FSID:          "00000000-0000-0000-0000-000000000000",
			Namespace:     ns,
			CephVersion:   version.CephVersion{Major: 20, Minor: 3, Extra: 0},
			MonitorSecret: "MONSECRET/SHOULDNOTCHANGE=",
			CephCred: client.CephCred{
				Username: "client.admin",
				Secret:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			},
			OwnerInfo: &k8sutil.OwnerInfo{},
		}
		clusterInfo.SetName(clusterName)

		cephCluster := &cephv1.CephCluster{ // create via controller-runtime client
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      clusterName,
			},
			Spec: cephv1.ClusterSpec{
				Security: cephv1.ClusterSecuritySpec{
					CephX: cephv1.ClusterCephxConfig{
						Daemon: cephv1.CephxConfig{
							KeyRotationPolicy: "KeyGeneration",
							KeyGeneration:     2,
						},
					},
				},
			},
			Status: cephv1.ClusterStatus{
				Cephx: cephv1.ClusterCephxStatus{
					Admin: cephv1.CephxStatus{
						KeyGeneration:  1,
						KeyCephVersion: "19.2.3-0",
					},
				},
			},
		}

		monSecret := &corev1.Secret{ // create via clientset
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "rook-ceph-mon",
			},
			Data: map[string][]byte{
				"ceph-secret":   []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="), // before rotation key
				"ceph-username": []byte("client.admin"),
				"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
				"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			},
		}

		clientset := testop.New(t, 3)
		_, err := clientset.CoreV1().Secrets(ns).Create(context.TODO(), monSecret, metav1.CreateOptions{})
		if err != nil {
			panic(err) // test setup failed
		}

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(cephCluster).Build()

		clusterdCtx := &clusterd.Context{
			Clientset: clientset,
			Client:    cl,
			Executor:  mockExecutor,
			ConfigDir: tmpCfgDir,
		}

		return clusterdCtx, clusterInfo, clusterInfo.OwnerInfo, cephCluster
	}

	// args that the default clusterInfo should use
	defaultClientArgs := []string{"--name=client.admin", "--keyring=" + tmpCfgDir + "/ns/client.admin.keyring"}

	// args that the temporary admin-rotator user should use
	tmpRotatorClientArgs := []string{"--name=client.admin-rotator", "--keyring=" + tmpCfgDir + "/ns/admin-rotate/client.admin-rotator.keyring"}

	// args that the new admin client should use temporarily to verify it works before final storage
	tmpAdminClientArgs := []string{"--name=client.admin", "--keyring=" + tmpCfgDir + "/ns/admin-rotate/client.admin.keyring"}

	t.Run("happy path rotation", func(t *testing.T) {
		clusterdCtx, clusterInfo, ownerInfo, cephCluster := newTest()

		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "get-or-create-key", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authCreateAdminRotatorOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authRotateAdminOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpAdminClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotatorDeletion, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "del", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", nil
			},
		})

		err := rotateAdminCephxKey(clusterdCtx, clusterInfo, ownerInfo, cephCluster)
		assert.ErrorIs(t, err, errSuccessfulAdminKeyRotation)
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.True(t, reloadManagerCalled)

		cluster := cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), cluster.Status.Cephx.Admin.KeyGeneration)   // updated
		assert.Equal(t, "20.3.0-0", cluster.Status.Cephx.Admin.KeyCephVersion) // updated

		monSec, err := clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData := map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="), // changed
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.ErrorContains(t, err, "not found") // rotator keyring not persisted

		// after successful rotation, recovery should not occur
		err = recoverPriorAdminCephxKeyRotation(clusterdCtx, clusterInfo, ownerInfo, ns)
		assert.NoError(t, err)
		assert.Equal(t, len(expectList), numCalls) // same number calls from before

		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), cluster.Status.Cephx.Admin.KeyGeneration)   // unchanged
		assert.Equal(t, "20.3.0-0", cluster.Status.Cephx.Admin.KeyCephVersion) // unchanged

		monSec, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, wantMonSecretData, monSec.Data) // unchanged

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.ErrorContains(t, err, "not found") // rotator keyring not persisted
	})

	t.Run("fail create admin-rotator", func(t *testing.T) {
		clusterdCtx, clusterInfo, ownerInfo, cephCluster := newTest()

		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "get-or-create-key", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", fmt.Errorf("induced error")
			},
		})
		expectList = append(expectList, expect{
			// auth generation tries to modify caps if auth get-or-create-key fails
			cargs:    []string{"ceph", "auth", "caps", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", fmt.Errorf("induced error")
			},
		})

		err := rotateAdminCephxKey(clusterdCtx, clusterInfo, ownerInfo, cephCluster)
		assert.ErrorContains(t, err, "induced error")
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.False(t, reloadManagerCalled)

		cluster := cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), cluster.Status.Cephx.Admin.KeyGeneration)   // unchanged
		assert.Equal(t, "19.2.3-0", cluster.Status.Cephx.Admin.KeyCephVersion) // unchanged

		monSec, err := clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData := map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="), // unchanged
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		// nothing to recover before admin-rotator created and stored in secret
		err = recoverPriorAdminCephxKeyRotation(clusterdCtx, clusterInfo, ownerInfo, ns)
		assert.NoError(t, err)
		assert.Equal(t, len(expectList), numCalls) // same number calls from before

		monSec, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, wantMonSecretData, monSec.Data) // unchanged

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.ErrorContains(t, err, "not found") // rotator keyring not persisted
	})

	t.Run("fail ls as admin rotator", func(t *testing.T) {
		clusterdCtx, clusterInfo, ownerInfo, cephCluster := newTest()

		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "get-or-create-key", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authCreateAdminRotatorOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", fmt.Errorf("induced error")
			},
		})

		err := rotateAdminCephxKey(clusterdCtx, clusterInfo, ownerInfo, cephCluster)
		assert.ErrorContains(t, err, "induced error")
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.False(t, reloadManagerCalled)

		cluster := cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), cluster.Status.Cephx.Admin.KeyGeneration)   // unchanged
		assert.Equal(t, "19.2.3-0", cluster.Status.Cephx.Admin.KeyCephVersion) // unchanged

		monSec, err := clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData := map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="), // unchanged
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.NoError(t, err) // rotator secret now persisted

		// expect recovery to proceed
		numCalls = 0
		expectList = []expect{} // clear expect list
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				// client.admin can still ls, and admin-rotator is present
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authRotateAdminOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpAdminClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotatorDeletion, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "del", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", nil
			},
		})

		err = recoverPriorAdminCephxKeyRotation(clusterdCtx, clusterInfo, ownerInfo, ns)
		assert.ErrorIs(t, err, errSuccessfulAdminKeyRotation)
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.True(t, reloadManagerCalled)

		cluster = cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), cluster.Status.Cephx.Admin.KeyGeneration)   // updated
		assert.Equal(t, "20.3.0-0", cluster.Status.Cephx.Admin.KeyCephVersion) // updated

		monSec, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData = map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="), // changed
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.ErrorContains(t, err, "not found") // rotator keyring not persisted

		t.Run("recovery doesn't reoccur after successful recovery", func(t *testing.T) {
			// only need to test this in one of the unit tests
			err = recoverPriorAdminCephxKeyRotation(clusterdCtx, clusterInfo, ownerInfo, ns)
			assert.NoError(t, err)
			assert.Equal(t, len(expectList), numCalls) // same number calls from before

			err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
			assert.NoError(t, err)
			assert.Equal(t, uint32(2), cluster.Status.Cephx.Admin.KeyGeneration)   // unchanged
			assert.Equal(t, "20.3.0-0", cluster.Status.Cephx.Admin.KeyCephVersion) // unchanged

			monSec, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Equal(t, wantMonSecretData, monSec.Data) // unchanged

			_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
			assert.ErrorContains(t, err, "not found") // rotator keyring not persisted
		})
	})

	t.Run("fail auth rotate", func(t *testing.T) {
		clusterdCtx, clusterInfo, ownerInfo, cephCluster := newTest()

		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "get-or-create-key", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authCreateAdminRotatorOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", fmt.Errorf("induced error")
			},
		})

		err := rotateAdminCephxKey(clusterdCtx, clusterInfo, ownerInfo, cephCluster)
		assert.ErrorContains(t, err, "induced error")
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.False(t, reloadManagerCalled)

		cluster := cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), cluster.Status.Cephx.Admin.KeyGeneration)   // unchanged
		assert.Equal(t, "19.2.3-0", cluster.Status.Cephx.Admin.KeyCephVersion) // unchanged

		monSec, err := clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData := map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="), // unchanged
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.NoError(t, err) // rotator secret now persisted

		// expect recovery to proceed
		numCalls = 0
		expectList = []expect{} // clear expect list
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				// client.admin can still ls, and admin-rotator is present
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authRotateAdminOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpAdminClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotatorDeletion, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "del", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", nil
			},
		})

		err = recoverPriorAdminCephxKeyRotation(clusterdCtx, clusterInfo, ownerInfo, ns)
		assert.ErrorIs(t, err, errSuccessfulAdminKeyRotation)
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.True(t, reloadManagerCalled)

		cluster = cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), cluster.Status.Cephx.Admin.KeyGeneration)   // updated
		assert.Equal(t, "20.3.0-0", cluster.Status.Cephx.Admin.KeyCephVersion) // updated

		monSec, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData = map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="), // changed
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.ErrorContains(t, err, "not found") // rotator keyring not persisted
	})

	t.Run("fail auth ls as temp rotated client.admin", func(t *testing.T) {
		clusterdCtx, clusterInfo, ownerInfo, cephCluster := newTest()

		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "get-or-create-key", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authCreateAdminRotatorOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authRotateAdminOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpAdminClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", fmt.Errorf("induced error")
			},
		})

		err := rotateAdminCephxKey(clusterdCtx, clusterInfo, ownerInfo, cephCluster)
		assert.ErrorContains(t, err, "induced error")
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.False(t, reloadManagerCalled)

		cluster := cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), cluster.Status.Cephx.Admin.KeyGeneration)   // unchanged
		assert.Equal(t, "19.2.3-0", cluster.Status.Cephx.Admin.KeyCephVersion) // unchanged

		monSec, err := clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData := map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="), // unchanged
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.NoError(t, err) // rotator secret now persisted

		// expect recovery to proceed
		numCalls = 0
		expectList = []expect{} // clear expect list
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", fmt.Errorf("client.admin key stored rook-ceph-mon secret now doesn't work")
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authRotateAdminOutput, nil // auth rotate happens again
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpAdminClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotatorDeletion, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "del", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", nil
			},
		})

		err = recoverPriorAdminCephxKeyRotation(clusterdCtx, clusterInfo, ownerInfo, ns)
		assert.ErrorIs(t, err, errSuccessfulAdminKeyRotation)
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.True(t, reloadManagerCalled)

		cluster = cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), cluster.Status.Cephx.Admin.KeyGeneration)   // updated
		assert.Equal(t, "20.3.0-0", cluster.Status.Cephx.Admin.KeyCephVersion) // updated

		monSec, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData = map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="), // changed
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.ErrorContains(t, err, "not found") // rotator keyring not persisted
	})

	t.Run("fail auth del admin-rotator", func(t *testing.T) {
		clusterdCtx, clusterInfo, ownerInfo, cephCluster := newTest()

		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "get-or-create-key", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authCreateAdminRotatorOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authRotateAdminOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpAdminClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotatorDeletion, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "del", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", fmt.Errorf("induced error")
			},
		})

		err := rotateAdminCephxKey(clusterdCtx, clusterInfo, ownerInfo, cephCluster)
		assert.ErrorContains(t, err, "induced error")
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.False(t, reloadManagerCalled)

		cluster := cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), cluster.Status.Cephx.Admin.KeyGeneration)   // unchanged
		assert.Equal(t, "19.2.3-0", cluster.Status.Cephx.Admin.KeyCephVersion) // unchanged

		monSec, err := clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData := map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="), // IS UPDATED!
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.NoError(t, err) // rotator secret still persisted

		// expect recovery to proceed
		numCalls = 0
		expectList = []expect{} // clear expect list
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				// client.admin key stored rook-ceph-mon secret works, but Rook can't know if that
				// is because it is un-rotated or because auth del failed
				return authLsOutputAfterRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authRotateAdminOutput, nil // auth rotate happens again
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpAdminClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotatorDeletion, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "del", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", nil
			},
		})

		err = recoverPriorAdminCephxKeyRotation(clusterdCtx, clusterInfo, ownerInfo, ns)
		assert.ErrorIs(t, err, errSuccessfulAdminKeyRotation)
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.True(t, reloadManagerCalled)

		cluster = cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), cluster.Status.Cephx.Admin.KeyGeneration)   // updated
		assert.Equal(t, "20.3.0-0", cluster.Status.Cephx.Admin.KeyCephVersion) // updated

		monSec, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData = map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="), // rotated value
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.ErrorContains(t, err, "not found") // rotator keyring not persisted
	})

	t.Run("recover cleanup", func(t *testing.T) {
		clusterdCtx, clusterInfo, ownerInfo, cephCluster := newTest()

		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "get-or-create-key", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authCreateAdminRotatorOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputBeforeRotation, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "rotate", "client.admin"},
			withArgs: tmpRotatorClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authRotateAdminOutput, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: tmpAdminClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return authLsOutputAfterRotatorDeletion, nil
			},
		})
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "del", "client.admin-rotator"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				return "", fmt.Errorf("induced error")
			},
		})

		err := rotateAdminCephxKey(clusterdCtx, clusterInfo, ownerInfo, cephCluster)
		assert.ErrorContains(t, err, "induced error")
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.False(t, reloadManagerCalled)

		cluster := cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), cluster.Status.Cephx.Admin.KeyGeneration)   // unchanged
		assert.Equal(t, "19.2.3-0", cluster.Status.Cephx.Admin.KeyCephVersion) // unchanged

		monSec, err := clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData := map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="), // IS UPDATED!
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.NoError(t, err) // rotator secret still persisted

		// expect recovery to proceed
		numCalls = 0
		expectList = []expect{} // clear expect list
		expectList = append(expectList, expect{
			cargs:    []string{"ceph", "auth", "ls"},
			withArgs: defaultClientArgs,
			returnFunc: func(cmd string, args ...string) (string, error) {
				// pretend that auth del successfully deleted auth-rotator
				return authLsOutputAfterRotatorDeletion, nil
			},
		})
		// don't expect any more commands, but just check that secrets are correct

		err = recoverPriorAdminCephxKeyRotation(clusterdCtx, clusterInfo, ownerInfo, ns)
		assert.ErrorIs(t, err, errSuccessfulAdminKeyRotation)
		assert.Equal(t, len(expectList), numCalls) // any expected calls not called?
		assert.True(t, reloadManagerCalled)

		cluster = cephv1.CephCluster{}
		err = clusterdCtx.Client.Get(context.TODO(), clusterInfo.NamespacedName(), &cluster)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), cluster.Status.Cephx.Admin.KeyGeneration)   // updated
		assert.Equal(t, "20.3.0-0", cluster.Status.Cephx.Admin.KeyCephVersion) // updated

		monSec, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-mon", metav1.GetOptions{})
		assert.NoError(t, err)
		wantMonSecretData = map[string][]byte{
			"mon-secret":    []byte("MONSECRET/SHOULDNOTCHANGE="),
			"fsid":          []byte("00000000-0000-0000-0000-000000000000"),
			"ceph-username": []byte("client.admin"),
			"ceph-secret":   []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="), // unchanged
		}
		assert.Equal(t, wantMonSecretData, monSec.Data)

		_, err = clusterdCtx.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "rook-ceph-admin-rotator-keyring", metav1.GetOptions{})
		assert.ErrorContains(t, err, "not found") // rotator keyring not persisted
	})
}

// super simple helper to see if the command matches
func isCommand(cargs []string, lookFor ...string) bool {
	for i, want := range lookFor {
		if cargs[i] != want {
			return false
		}
	}
	return true
}

func hasArg(args []string, lookFor string) bool {
	for _, arg := range args {
		if arg == lookFor {
			return true
		}
	}
	return false
}

const (
	// `ceph auth get-or-create-key client.admin-rotator` output
	authCreateAdminRotatorOutput = `{
  "entity": "client.admin-rotator",
  "key": "ADMINROTATORADMINROTATORADMINROTATORADMINROTATORADMINROTATO=",
  "caps": {
    "mds": "allow *",
    "mgr": "allow *",
    "mon": "allow *",
    "osd": "allow *"
  }
}`

	// `ceph auth ls` output from before client.admin rotation (after admin-rotator user created)
	// output pared down to a few entries to keep CI test more readable without being too trivial
	authLsOutputBeforeRotation = `{
  "auth_dump": [
    {
      "entity": "client.admin",
      "key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      "caps": {
        "mds": "allow *",
        "mgr": "allow *",
        "mon": "allow *",
        "osd": "allow *"
      }
    },
    {
      "entity": "client.admin-rotator",
      "key": "ADMINROTATORADMINROTATORADMINROTATORADMINROTATORADMINROTATO=",
      "caps": {
        "mds": "allow *",
        "mgr": "allow *",
        "mon": "allow *",
        "osd": "allow *"
      }
    },
    {
      "entity": "client.csi-rbd-node.1",
      "key": "AgASB6ZohDE6MCAANiC1Nt0tE6IjQi5nPeiFKvaNZLcZcIJ1bgYfRQ6hlPE=",
      "caps": {
        "mgr": "allow rw",
        "mon": "profile rbd",
        "osd": "profile rbd"
      }
    },
    {
      "entity": "client.csi-rbd-provisioner.1",
      "key": "AgASB6ZoEIZ1IyAANmYrvZmHsseRtTb2iftoD0Jp3Un3Ob+6wa+HDW1Qh5I=",
      "caps": {
        "mgr": "allow rw",
        "mon": "profile rbd, allow command 'osd blocklist'",
        "osd": "profile rbd"
      }
    }
  ]
}`

	// `ceph auth rotate client.admin` output
	authRotateAdminOutput = `[{
  "entity": "client.admin",
  "key": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
  "caps": {
    "mds": "allow *",
    "mgr": "allow *",
    "mon": "allow *",
    "osd": "allow *"
  }
}]`

	authLsOutputAfterRotation = `{
  "auth_dump": [
    {
      "entity": "client.admin",
      "key": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
      "caps": {
        "mds": "allow *",
        "mgr": "allow *",
        "mon": "allow *",
        "osd": "allow *"
      }
    },
    {
      "entity": "client.admin-rotator",
      "key": "ADMINROTATORADMINROTATORADMINROTATORADMINROTATORADMINROTATO=",
      "caps": {
        "mds": "allow *",
        "mgr": "allow *",
        "mon": "allow *",
        "osd": "allow *"
      }
    },
    {
      "entity": "client.csi-rbd-node.1",
      "key": "AgASB6ZohDE6MCAANiC1Nt0tE6IjQi5nPeiFKvaNZLcZcIJ1bgYfRQ6hlPE=",
      "caps": {
        "mgr": "allow rw",
        "mon": "profile rbd",
        "osd": "profile rbd"
      }
    },
    {
      "entity": "client.csi-rbd-provisioner.1",
      "key": "AgASB6ZoEIZ1IyAANmYrvZmHsseRtTb2iftoD0Jp3Un3Ob+6wa+HDW1Qh5I=",
      "caps": {
        "mgr": "allow rw",
        "mon": "profile rbd, allow command 'osd blocklist'",
        "osd": "profile rbd"
      }
    }
  ]
}`

	// `ceph auth ls` output from after client.admin-rotator deleted
	// output pared down to a few entries to keep CI test more readable without being too trivial
	authLsOutputAfterRotatorDeletion = `{
  "auth_dump": [
    {
      "entity": "client.admin",
      "key": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
      "caps": {
        "mds": "allow *",
        "mgr": "allow *",
        "mon": "allow *",
        "osd": "allow *"
      }
    },
    {
      "entity": "client.csi-rbd-node.1",
      "key": "AgASB6ZohDE6MCAANiC1Nt0tE6IjQi5nPeiFKvaNZLcZcIJ1bgYfRQ6hlPE=",
      "caps": {
        "mgr": "allow rw",
        "mon": "profile rbd",
        "osd": "profile rbd"
      }
    },
    {
      "entity": "client.csi-rbd-provisioner.1",
      "key": "AgASB6ZoEIZ1IyAANmYrvZmHsseRtTb2iftoD0Jp3Un3Ob+6wa+HDW1Qh5I=",
      "caps": {
        "mgr": "allow rw",
        "mon": "profile rbd, allow command 'osd blocklist'",
        "osd": "profile rbd"
      }
    }
  ]
}`
)

func Test_adminRotationLock(t *testing.T) {
	var err error

	err = claimAdminRotationLock("ns")
	assert.NoError(t, err)

	err = claimAdminRotationLock("ns")
	assert.Error(t, err)

	err = claimAdminRotationLock("other")
	assert.NoError(t, err)

	releaseAdminRotationLock("ns")

	err = claimAdminRotationLock("ns")
	assert.NoError(t, err)

	assert.Equal(t, map[string]struct{}{
		"ns":    {},
		"other": {},
	}, adminRotationInProgress)

	releaseAdminRotationLock("ns")
	releaseAdminRotationLock("other")

	assert.Equal(t, map[string]struct{}{}, adminRotationInProgress)
}
