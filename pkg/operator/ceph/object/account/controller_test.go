/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package account

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephobject "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	accountCreateJSON = `{
	"id": "RGW12345678901234567",
	"name": "my-account"
}`
	accountModifyJSON = `{
	"id": "RGW12345678901234567",
	"name": "updated-account"
}`
)

var (
	name      = "my-account"
	namespace = "rook-ceph"
	store     = "my-store"
)

func TestCephObjectStoreAccountController(t *testing.T) {
	ctx := context.TODO()
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)

	objectAccount := &cephv1.CephObjectStoreAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{"cephobjectstoreaccount.ceph.rook.io"},
		},
		Spec: cephv1.ObjectStoreAccountSpec{
			Store: store,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectStoreAccount",
		},
	}
	cephCluster := &cephv1.CephCluster{}

	object := []runtime.Object{
		objectAccount,
		cephCluster,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			return "", nil
		},
	}
	clientset := test.New(t, 3)
	c := &clusterd.Context{
		Executor:      executor,
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     clientset,
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion,
		&cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{},
		&cephv1.CephCluster{}, &cephv1.CephClusterList{},
	)

	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	r := &ReconcileObjectStoreAccount{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: events.NewFakeRecorder(50)}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	t.Run("failure because no CephCluster", func(t *testing.T) {
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("failure CephCluster not ready", func(t *testing.T) {
		cephCluster = &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespace,
				Namespace: namespace,
			},
			Status: cephv1.ClusterStatus{
				Phase: "",
				CephStatus: &cephv1.CephStatus{
					Health: "",
				},
			},
		}

		object = append(object, cephCluster)
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		r = &ReconcileObjectStoreAccount{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: events.NewFakeRecorder(50)}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("failure CephCluster ready but no RGW object", func(t *testing.T) {
		secrets := map[string][]byte{
			"fsid":         []byte(name),
			"mon-secret":   []byte("monsecret"),
			"admin-secret": []byte("adminsecret"),
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rook-ceph-mon",
				Namespace: namespace,
			},
			Data: secrets,
			Type: k8sutil.RookType,
		}
		_, err := c.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)

		cephCluster.Status.Phase = k8sutil.ReadyStatus
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"

		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "status" {
					return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
				}
				return "", nil
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				return "", nil
			},
		}
		c.Executor = executor

		r = &ReconcileObjectStoreAccount{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: events.NewFakeRecorder(50)}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("failure RGW object exists but no pods running", func(t *testing.T) {
		cephObjectStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      store,
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "CephObjectStore",
			},
			Spec: cephv1.ObjectStoreSpec{
				Gateway: cephv1.GatewaySpec{
					Port: 80,
				},
			},
			Status: &cephv1.ObjectStoreStatus{
				Info: map[string]string{"endpoint": "http://rook-ceph-rgw-my-store.rook-ceph:80"},
			},
		}
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStore{}, &cephv1.CephObjectStoreList{})
		object = append(object, cephObjectStore)

		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		r = &ReconcileObjectStoreAccount{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: events.NewFakeRecorder(50)}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("success RGW object exists and pods running", func(t *testing.T) {
		// Build a fresh client with all required objects to avoid state issues from prior subtests
		freshAccount := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  namespace,
				UID:        "c1a2b3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
				Finalizers: []string{"cephobjectstoreaccount.ceph.rook.io"},
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "CephObjectStoreAccount",
			},
			Status: &cephv1.ObjectStoreAccountStatus{},
		}
		deterministicID, err := generateDeterministicAccountID("c1a2b3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d")
		assert.NoError(t, err)
		readyCluster := &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespace,
				Namespace: namespace,
			},
			Status: cephv1.ClusterStatus{
				Phase: k8sutil.ReadyStatus,
				CephStatus: &cephv1.CephStatus{
					Health: "HEALTH_OK",
				},
			},
		}
		cephObjectStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      store,
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "CephObjectStore",
			},
			Spec: cephv1.ObjectStoreSpec{
				Gateway: cephv1.GatewaySpec{
					Port: 80,
				},
			},
			Status: &cephv1.ObjectStoreStatus{
				Info: map[string]string{"endpoint": "http://rook-ceph-rgw-my-store.rook-ceph:80"},
			},
		}
		rgwPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-rgw-my-store-a-5fd6fb4489-xv65v",
			Namespace: namespace,
			Labels:    map[string]string{k8sutil.AppAttr: cephobject.AppName, "rgw": store},
		}}

		freshCl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(freshAccount, readyCluster, cephObjectStore, rgwPod).Build()

		freshExecutor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "status" {
					return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
				}
				return "", nil
			},
		}
		freshClientset := test.New(t, 3)
		freshC := &clusterd.Context{
			Executor:      freshExecutor,
			RookClientset: rookclient.NewSimpleClientset(),
			Clientset:     freshClientset,
		}

		// Create the mon secret required by LoadClusterInfo
		secrets := map[string][]byte{
			"fsid":         []byte(name),
			"mon-secret":   []byte("monsecret"),
			"admin-secret": []byte("adminsecret"),
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rook-ceph-mon",
				Namespace: namespace,
			},
			Data: secrets,
			Type: k8sutil.RookType,
		}
		_, err = freshC.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)

		freshR := &ReconcileObjectStoreAccount{client: freshCl, scheme: s, context: freshC, opManagerContext: ctx, recorder: events.NewFakeRecorder(50)}

		newAccountJSON := fmt.Sprintf(`{"id": "%s", "name": "my-account"}`, deterministicID)
		newMultisiteAdminOpsCtxFunc = func(objContext *cephobject.Context, spec *cephv1.ObjectStoreSpec) (*cephobject.AdminOpsContext, error) {
			mockClient := &cephobject.MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					// Handle account retrieval: 404 on first reconcile (account not yet created)
					if req.Method == http.MethodGet {
						return &http.Response{
							StatusCode: 404,
							Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchKey","RequestId":"tx000","HostId":""}`))),
						}, nil
					}
					// Handle account creation with deterministic ID
					if req.Method == http.MethodPost {
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewReader([]byte(newAccountJSON))),
						}, nil
					}
					return nil, fmt.Errorf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, req.URL.Path)
				},
			}

			msContext, err := cephobject.NewMultisiteContext(freshR.context, freshR.clusterInfo, cephObjectStore)
			if err != nil {
				return nil, err
			}
			adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient)
			if err != nil {
				return nil, err
			}

			return &cephobject.AdminOpsContext{
				Context:               *msContext,
				AdminOpsUserAccessKey: "53S6B9S809NUP19IJ2K3",
				AdminOpsUserSecretKey: "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR",
				AdminOpsClient:        adminClient,
			}, nil
		}
		defer func() {
			newMultisiteAdminOpsCtxFunc = cephobject.NewMultisiteAdminOpsContext
		}()

		res, err := freshR.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		err = freshR.client.Get(ctx, req.NamespacedName, freshAccount)
		assert.NoError(t, err)
		assert.Equal(t, k8sutil.ReadyStatus, freshAccount.Status.Phase)
		assert.Equal(t, deterministicID, freshAccount.Status.AccountID)
	})
}

func TestGetAccountName(t *testing.T) {
	t.Run("returns spec name when set", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "cr-name"},
			Spec:       cephv1.ObjectStoreAccountSpec{Name: "spec-name"},
		}
		assert.Equal(t, "spec-name", getAccountName(account))
	})

	t.Run("falls back to CR name when spec name is empty", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "cr-name"},
			Spec:       cephv1.ObjectStoreAccountSpec{},
		}
		assert.Equal(t, "cr-name", getAccountName(account))
	})
}

func TestGenerateDeterministicAccountID(t *testing.T) {
	t.Run("produces valid RGW account ID format", func(t *testing.T) {
		id, err := generateDeterministicAccountID("a675dfa7-a785-40a3-b690-eb1e33c1dbdb")
		assert.NoError(t, err)
		assert.Equal(t, "RGW", id[:3])
		assert.Len(t, id, 20) // "RGW" + 17 digits
		for _, c := range id[3:] {
			assert.True(t, c >= '0' && c <= '9', "expected digit, got %c", c)
		}
	})

	t.Run("is deterministic for same UID", func(t *testing.T) {
		id1, err := generateDeterministicAccountID("a675dfa7-a785-40a3-b690-eb1e33c1dbdb")
		assert.NoError(t, err)
		id2, err := generateDeterministicAccountID("a675dfa7-a785-40a3-b690-eb1e33c1dbdb")
		assert.NoError(t, err)
		assert.Equal(t, id1, id2)
	})

	t.Run("produces different IDs for different UIDs", func(t *testing.T) {
		id1, err := generateDeterministicAccountID("a675dfa7-a785-40a3-b690-eb1e33c1dbdb")
		assert.NoError(t, err)
		id2, err := generateDeterministicAccountID("f47ac10b-58cc-4372-a567-0e02b2c3d479")
		assert.NoError(t, err)
		assert.NotEqual(t, id1, id2)
	})

	t.Run("returns error for UID too short", func(t *testing.T) {
		_, err := generateDeterministicAccountID("short")
		assert.Error(t, err)
	})

	t.Run("returns error for non-hex UID", func(t *testing.T) {
		_, err := generateDeterministicAccountID("zzzzzzzzzzzzzz")
		assert.Error(t, err)
	})
}

func TestGetAccountID(t *testing.T) {
	t.Run("returns spec account ID when set", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{AccountID: "RGW11111111111111111"},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW22222222222222222",
			},
		}
		assert.Equal(t, "RGW11111111111111111", getAccountID(account))
	})

	t.Run("returns status account ID when spec is empty", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW22222222222222222",
			},
		}
		assert.Equal(t, "RGW22222222222222222", getAccountID(account))
	})

	t.Run("returns annotation ID when spec and status are empty", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				UID:         "d4e5f6a7-b8c9-4d0e-a1f2-3b4c5d6e7f80",
				Annotations: map[string]string{accountIDAnnotation: "RGW33333333333333333"},
			},
			Spec: cephv1.ObjectStoreAccountSpec{},
		}
		assert.Equal(t, "RGW33333333333333333", getAccountID(account))
	})

	t.Run("status ID takes priority over annotation ID", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{accountIDAnnotation: "RGW33333333333333333"},
			},
			Spec: cephv1.ObjectStoreAccountSpec{},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW22222222222222222",
			},
		}
		assert.Equal(t, "RGW22222222222222222", getAccountID(account))
	})

	t.Run("does not generate ID from UID", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{UID: "d4e5f6a7-b8c9-4d0e-a1f2-3b4c5d6e7f80"},
			Spec:       cephv1.ObjectStoreAccountSpec{},
		}
		assert.Equal(t, "", getAccountID(account))
	})

	t.Run("returns empty when no spec, status, or annotation", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{},
		}
		assert.Equal(t, "", getAccountID(account))
	})

	t.Run("returns empty when status is nil", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec:   cephv1.ObjectStoreAccountSpec{},
			Status: nil,
		}
		assert.Equal(t, "", getAccountID(account))
	})
}

func TestGetOrGenerateAccountID(t *testing.T) {
	t.Run("returns existing ID when present", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{AccountID: "RGW11111111111111111"},
		}
		assert.Equal(t, "RGW11111111111111111", getOrGenerateAccountID(account))
	})

	t.Run("generates deterministic ID from UID when no existing ID", func(t *testing.T) {
		uid := types.UID("d4e5f6a7-b8c9-4d0e-a1f2-3b4c5d6e7f80")
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{UID: uid},
			Spec:       cephv1.ObjectStoreAccountSpec{},
		}
		expected, err := generateDeterministicAccountID(uid)
		assert.NoError(t, err)
		assert.Equal(t, expected, getOrGenerateAccountID(account))
	})

	t.Run("spec ID takes priority over annotation and UID-derived ID", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				UID:         "d4e5f6a7-b8c9-4d0e-a1f2-3b4c5d6e7f80",
				Annotations: map[string]string{accountIDAnnotation: "RGW33333333333333333"},
			},
			Spec: cephv1.ObjectStoreAccountSpec{AccountID: "RGW11111111111111111"},
		}
		assert.Equal(t, "RGW11111111111111111", getOrGenerateAccountID(account))
	})

	t.Run("returns empty when no spec, status, annotation, or UID", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{},
		}
		assert.Equal(t, "", getOrGenerateAccountID(account))
	})
}

func TestIsAccountInSync(t *testing.T) {
	t.Run("in sync when names match", func(t *testing.T) {
		desired := admin.Account{ID: "RGW12345678901234567", Name: "my-account"}
		live := admin.Account{ID: "RGW12345678901234567", Name: "my-account"}
		assert.True(t, isAccountInSync(desired, live))
	})

	t.Run("out of sync when names differ", func(t *testing.T) {
		desired := admin.Account{ID: "RGW12345678901234567", Name: "new-name"}
		live := admin.Account{ID: "RGW12345678901234567", Name: "old-name"}
		assert.False(t, isAccountInSync(desired, live))
	})

	t.Run("out of sync with zero-value live account", func(t *testing.T) {
		desired := admin.Account{ID: "RGW12345678901234567", Name: "my-account"}
		live := admin.Account{}
		assert.False(t, isAccountInSync(desired, live))
	})
}

func TestReconcileAccount(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	ctx := context.TODO()

	t.Run("create account with UID-derived ID", func(t *testing.T) {
		uid := types.UID("a675dfa7-a785-40a3-b690-eb1e33c1dbdb")
		expectedID, err := generateDeterministicAccountID(uid)
		assert.NoError(t, err)
		accountJSON := fmt.Sprintf(`{"id": "%s", "name": "my-account"}`, expectedID)

		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchKey","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				if req.Method == http.MethodPost {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountJSON))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       uid,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
			},
		}

		s := scheme.Scheme
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()
		r := &ReconcileObjectStoreAccount{
			client: cl,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, expectedID, accountID)
	})

	t.Run("create account with explicit ID", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				// First call: GetAccount (should return not found)
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchKey","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				// Second call: CreateAccount
				if req.Method == http.MethodPost {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountCreateJSON))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store:     store,
				AccountID: "RGW12345678901234567",
			},
		}

		s := scheme.Scheme
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()
		r := &ReconcileObjectStoreAccount{
			client: cl,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW12345678901234567", accountID)
		// Verify annotation was persisted
		assert.Equal(t, "RGW12345678901234567", account.Annotations[accountIDAnnotation])
	})

	t.Run("account already exists with annotation confirming ownership", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountCreateJSON))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		r := &ReconcileObjectStoreAccount{
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		// Annotation matches the account ID, confirming this CR created the account.
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: map[string]string{accountIDAnnotation: "RGW12345678901234567"},
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store:     store,
				AccountID: "RGW12345678901234567",
			},
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW12345678901234567", accountID)
	})

	t.Run("account exists with different name triggers update when ownership confirmed", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"RGW12345678901234567","name":"old-name"}`))),
					}, nil
				}
				if req.Method == http.MethodPut {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountModifyJSON))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		r := &ReconcileObjectStoreAccount{
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		// status.AccountID is set, confirming ownership from a previous reconcile.
		// The user has since changed the CR name, so the account name needs updating.
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "updated-account",
				Namespace: namespace,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store:     store,
				AccountID: "RGW12345678901234567",
			},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW12345678901234567",
			},
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW12345678901234567", accountID)
	})

	t.Run("account uses spec name over CR name", func(t *testing.T) {
		uid := types.UID("b1c2d3e4-f5a6-4b7c-8d9e-0f1a2b3c4d5e")
		expectedID, err := generateDeterministicAccountID(uid)
		assert.NoError(t, err)
		accountJSON := fmt.Sprintf(`{"id": "%s", "name": "custom-name"}`, expectedID)

		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchKey","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				if req.Method == http.MethodPost {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountJSON))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cr-name",
				Namespace: namespace,
				UID:       uid,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
				Name:  "custom-name",
			},
		}

		s := scheme.Scheme
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()
		r := &ReconcileObjectStoreAccount{
			client: cl,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.NotEmpty(t, accountID)
	})

	t.Run("account uses ID from status when spec has no ID", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountCreateJSON))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		r := &ReconcileObjectStoreAccount{
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
			},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW12345678901234567",
			},
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW12345678901234567", accountID)
	})

	t.Run("crash recovery: finds existing account via annotation bookmark", func(t *testing.T) {
		// Simulate crash recovery: operator wrote the annotation, created the account,
		// but crashed before persisting the account ID to the CR status. On retry,
		// the annotation provides ownership proof and the deterministic ID allows
		// finding the existing account.
		uid := types.UID("a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d")
		expectedID, err := generateDeterministicAccountID(uid)
		assert.NoError(t, err)
		accountJSON := fmt.Sprintf(`{"id": "%s", "name": "my-account"}`, expectedID)

		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				// GET returns the account that was created before the "crash"
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountJSON))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		r := &ReconcileObjectStoreAccount{
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		// No status, but annotation was written before the crash
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				UID:         uid,
				Annotations: map[string]string{accountIDAnnotation: expectedID},
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
			},
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, expectedID, accountID)
	})

	t.Run("refuses to adopt foreign account without ownership proof", func(t *testing.T) {
		// spec.AccountID points to an existing account in Ceph, but there is no
		// annotation or status confirming ownership. The controller must refuse
		// regardless of whether the account name matches.
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"RGW12345678901234567","name":"foreign-account"}`))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		r := &ReconcileObjectStoreAccount{
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		// No annotation, no status — no ownership proof
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store:     store,
				AccountID: "RGW12345678901234567",
			},
		}

		_, err = r.reconcileAccount(account)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "refusing to adopt a foreign account")
	})

	t.Run("refuses to create account when already exists in RGW", func(t *testing.T) {
		// Our ID doesn't exist (GET returns 404), but CreateAccount returns
		// AccountAlreadyExists — indicating a conflict with a different account
		// (e.g., same name). The controller should fail with a clear error
		// instead of silently adopting the conflicting account.
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchKey","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				if req.Method == http.MethodPost {
					return &http.Response{
						StatusCode: 409,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"AccountAlreadyExists","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store:     store,
				AccountID: "RGW12345678901234567",
			},
		}

		s := scheme.Scheme
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()
		r := &ReconcileObjectStoreAccount{
			client: cl,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		_, err = r.reconcileAccount(account)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists in RGW")
	})

	t.Run("first reconcile with UID creates account with deterministic ID", func(t *testing.T) {
		// First reconcile: no account exists yet, but the deterministic ID
		// ensures the same ID is used if creation must be retried.
		uid := types.UID("b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e")
		expectedID, err := generateDeterministicAccountID(uid)
		assert.NoError(t, err)
		accountJSON := fmt.Sprintf(`{"id": "%s", "name": "my-account"}`, expectedID)

		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				// GET returns 404 — account does not exist yet
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchKey","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				// POST creates the account
				if req.Method == http.MethodPost {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountJSON))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       uid,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
			},
		}

		s := scheme.Scheme
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()
		r := &ReconcileObjectStoreAccount{
			client: cl,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, expectedID, accountID)
		// Verify annotation was persisted as creation bookmark
		assert.Equal(t, expectedID, account.Annotations[accountIDAnnotation])
	})
}

func TestDeleteAccount(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	ctx := context.TODO()

	t.Run("delete account successfully", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(""))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		r := &ReconcileObjectStoreAccount{
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW12345678901234567",
			},
		}

		err = r.deleteAccount(account)
		assert.NoError(t, err)
	})

	t.Run("delete account not found is idempotent", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete {
					return &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchKey","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		r := &ReconcileObjectStoreAccount{
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW12345678901234567",
			},
		}

		err = r.deleteAccount(account)
		assert.NoError(t, err)
	})

	t.Run("skip deletion when no account ID", func(t *testing.T) {
		r := &ReconcileObjectStoreAccount{
			opManagerContext: ctx,
		}

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}

		err := r.deleteAccount(account)
		assert.NoError(t, err)
	})

	t.Run("use spec account ID for deletion when status is nil", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(""))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: method %q path %q", req.Method, req.URL.Path)
			},
		}
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "access", "secret", mockClient)
		assert.NoError(t, err)

		r := &ReconcileObjectStoreAccount{
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store:     store,
				AccountID: "RGW12345678901234567",
			},
		}

		err = r.deleteAccount(account)
		assert.NoError(t, err)
	})
}

func TestUpdateStatus(t *testing.T) {
	ctx := context.TODO()
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion,
		&cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{},
	)

	account := &cephv1.CephObjectStoreAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.ObjectStoreAccountSpec{
			Store: store,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).WithStatusSubresource(account).Build()

	r := &ReconcileObjectStoreAccount{
		client:           cl,
		scheme:           s,
		opManagerContext: ctx,
		recorder:         events.NewFakeRecorder(50),
	}

	nsName := types.NamespacedName{Name: name, Namespace: namespace}

	t.Run("update status to ready", func(t *testing.T) {
		r.updateStatus(int64(1), nsName, k8sutil.ReadyStatus)
		updated := &cephv1.CephObjectStoreAccount{}
		err := r.client.Get(ctx, nsName, updated)
		assert.NoError(t, err)
		assert.NotNil(t, updated.Status)
		assert.Equal(t, k8sutil.ReadyStatus, updated.Status.Phase)
		assert.NotNil(t, updated.Status.ObservedGeneration)
		assert.Equal(t, int64(1), *updated.Status.ObservedGeneration)
	})

	t.Run("update status with account ID", func(t *testing.T) {
		r.updateStatusWithAccountID(int64(2), nsName, "RGW12345678901234567")
		updated := &cephv1.CephObjectStoreAccount{}
		err := r.client.Get(ctx, nsName, updated)
		assert.NoError(t, err)
		assert.NotNil(t, updated.Status)
		assert.Equal(t, k8sutil.ReadyStatus, updated.Status.Phase)
		assert.Equal(t, "RGW12345678901234567", updated.Status.AccountID)
		assert.NotNil(t, updated.Status.ObservedGeneration)
		assert.Equal(t, int64(2), *updated.Status.ObservedGeneration)
	})

	t.Run("update status for nonexistent resource does not panic", func(t *testing.T) {
		missingName := types.NamespacedName{Name: "does-not-exist", Namespace: namespace}
		assert.NotPanics(t, func() {
			r.updateStatus(int64(1), missingName, k8sutil.ReadyStatus)
		})
		assert.NotPanics(t, func() {
			r.updateStatusWithAccountID(int64(1), missingName, "some-id")
		})
	})
}

func TestReconcileObjectStoreAccountNotFound(t *testing.T) {
	ctx := context.TODO()
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion,
		&cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{},
		&cephv1.CephCluster{}, &cephv1.CephClusterList{},
	)

	// No account object in the fake client
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &ReconcileObjectStoreAccount{
		client:           cl,
		scheme:           s,
		opManagerContext: ctx,
		recorder:         events.NewFakeRecorder(50),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
}
