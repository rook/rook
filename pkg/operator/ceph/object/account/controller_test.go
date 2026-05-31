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
	"strings"
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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
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
		rootUserJSON := fmt.Sprintf(`{"user_id": "c1a2b3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "display_name": "root-my-account", "account_id": "%s", "keys": [{"access_key": "ROOT_ACCESS_KEY", "secret_key": "ROOT_SECRET_KEY"}]}`, deterministicID)
		newMultisiteAdminOpsCtxFunc = func(objContext *cephobject.Context, spec *cephv1.ObjectStoreSpec) (*cephobject.AdminOpsContext, error) {
			mockClient := &cephobject.MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					isUserAPI := strings.HasSuffix(req.URL.Path, "/admin/user")
					isAccountAPI := strings.HasSuffix(req.URL.Path, "/admin/account")

					if isAccountAPI {
						if req.Method == http.MethodGet {
							return &http.Response{
								StatusCode: 404,
								Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchKey","RequestId":"tx000","HostId":""}`))),
							}, nil
						}
						if req.Method == http.MethodPost {
							return &http.Response{
								StatusCode: 200,
								Body:       io.NopCloser(bytes.NewReader([]byte(newAccountJSON))),
							}, nil
						}
					}
					if isUserAPI {
						if req.Method == http.MethodGet {
							return &http.Response{
								StatusCode: 404,
								Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchUser","RequestId":"tx000","HostId":""}`))),
							}, nil
						}
						if req.Method == http.MethodPut {
							return &http.Response{
								StatusCode: 200,
								Body:       io.NopCloser(bytes.NewReader([]byte(rootUserJSON))),
							}, nil
						}
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
				// #nosec G101 -- fake test credential
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
		assert.Equal(t, "rook-ceph-object-root-user-my-account", freshAccount.Status.RootAccountSecretName)

		// Verify the root user secret was created
		rootSecret := &corev1.Secret{}
		err = freshR.client.Get(ctx, types.NamespacedName{Name: "rook-ceph-object-root-user-my-account", Namespace: namespace}, rootSecret)
		assert.NoError(t, err)
		assert.Equal(t, "ROOT_ACCESS_KEY", rootSecret.StringData["AccessKey"])
		assert.Equal(t, "ROOT_SECRET_KEY", rootSecret.StringData["SecretKey"])
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

	t.Run("does not generate ID from UID", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{UID: "d4e5f6a7-b8c9-4d0e-a1f2-3b4c5d6e7f80"},
			Spec:       cephv1.ObjectStoreAccountSpec{},
		}
		assert.Equal(t, "", getAccountID(account))
	})

	t.Run("returns empty when no spec or status", func(t *testing.T) {
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
		id, err := getOrGenerateAccountID(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW11111111111111111", id)
	})

	t.Run("generates deterministic ID from UID when no existing ID", func(t *testing.T) {
		uid := types.UID("d4e5f6a7-b8c9-4d0e-a1f2-3b4c5d6e7f80")
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{UID: uid},
			Spec:       cephv1.ObjectStoreAccountSpec{},
		}
		expected, err := generateDeterministicAccountID(uid)
		assert.NoError(t, err)
		id, err := getOrGenerateAccountID(account)
		assert.NoError(t, err)
		assert.Equal(t, expected, id)
	})

	t.Run("spec ID takes priority over UID-derived ID", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				UID: "d4e5f6a7-b8c9-4d0e-a1f2-3b4c5d6e7f80",
			},
			Spec: cephv1.ObjectStoreAccountSpec{AccountID: "RGW11111111111111111"},
		}
		id, err := getOrGenerateAccountID(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW11111111111111111", id)
	})

	t.Run("returns empty when no spec, status, or UID", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{},
		}
		id, err := getOrGenerateAccountID(account)
		assert.NoError(t, err)
		assert.Equal(t, "", id)
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
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).WithStatusSubresource(account).Build()
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
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).WithStatusSubresource(account).Build()
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
		// Verify account ID was persisted to status
		assert.NotNil(t, account.Status)
		assert.Equal(t, "RGW12345678901234567", account.Status.AccountID)
	})

	t.Run("account already exists with status confirming ownership", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(accountCreateJSON))),
					}, nil
				}
				if req.Method == http.MethodPut {
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

		// Status contains the account ID, confirming this CR created the account.
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
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
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).WithStatusSubresource(account).Build()
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
				if req.Method == http.MethodPut {
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

	t.Run("crash recovery: finds existing account via status bookmark", func(t *testing.T) {
		// Simulate crash recovery: operator persisted the account ID to status,
		// created the account, but crashed before setting the phase to Ready.
		// On retry, the status provides ownership proof and the ID allows
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
				if req.Method == http.MethodPut {
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

		// Status has the account ID (persisted before creation), but phase is not Ready
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       uid,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
			},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: expectedID,
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
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).WithStatusSubresource(account).Build()
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
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).WithStatusSubresource(account).Build()
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
		// Verify account ID was persisted to status as creation bookmark
		assert.NotNil(t, account.Status)
		assert.Equal(t, expectedID, account.Status.AccountID)
	})
}

func TestDeleteAccount(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	ctx := context.TODO()

	t.Run("delete account and root user successfully", func(t *testing.T) {
		userDeleteCalled := false
		accountDeleteCalled := false
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/user") {
					userDeleteCalled = true
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(""))),
					}, nil
				}
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/account") {
					accountDeleteCalled = true
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
				UID:       "test-uid",
			},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW12345678901234567",
			},
		}

		err = r.deleteAccount(account)
		assert.NoError(t, err)
		assert.True(t, userDeleteCalled, "should delete root user")
		assert.True(t, accountDeleteCalled, "should delete account")
	})

	t.Run("delete account with skipCreate root user still attempts root user deletion", func(t *testing.T) {
		userDeleteCalled := false
		accountDeleteCalled := false
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/user") {
					userDeleteCalled = true
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(""))),
					}, nil
				}
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/account") {
					accountDeleteCalled = true
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
				UID:       "test-uid",
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
				RootUser: &cephv1.AccountRootUserSpec{
					SkipCreate: ptr.To(true),
				},
			},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW12345678901234567",
			},
		}

		err = r.deleteAccount(account)
		assert.NoError(t, err)
		assert.True(t, userDeleteCalled, "should always attempt to delete root user even when skipCreate is true")
		assert.True(t, accountDeleteCalled, "should delete account")
	})

	t.Run("delete account when root user already gone is idempotent", func(t *testing.T) {
		accountDeleteCalled := false
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/user") {
					return &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchUser","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/account") {
					accountDeleteCalled = true
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
				UID:       "test-uid",
			},
			Status: &cephv1.ObjectStoreAccountStatus{
				AccountID: "RGW12345678901234567",
			},
		}

		err = r.deleteAccount(account)
		assert.NoError(t, err)
		assert.True(t, accountDeleteCalled, "should still delete account even if root user is gone")
	})

	t.Run("delete account not found is idempotent", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/user") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(""))),
					}, nil
				}
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/account") {
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
				UID:       "test-uid",
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

	t.Run("skip deletion of foreign account with spec ID but no ownership proof", func(t *testing.T) {
		deleteCalled := false
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete {
					deleteCalled = true
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

		// spec.AccountID is set but no status or annotation — no ownership proof
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
		assert.False(t, deleteCalled, "should not call RGW delete for a foreign account")
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

	t.Run("update status with account ID and secret name", func(t *testing.T) {
		r.updateStatusWithAccountID(int64(2), nsName, "RGW12345678901234567", "rook-ceph-object-root-user-my-account")
		updated := &cephv1.CephObjectStoreAccount{}
		err := r.client.Get(ctx, nsName, updated)
		assert.NoError(t, err)
		assert.NotNil(t, updated.Status)
		assert.Equal(t, k8sutil.ReadyStatus, updated.Status.Phase)
		assert.Equal(t, "RGW12345678901234567", updated.Status.AccountID)
		assert.Equal(t, "rook-ceph-object-root-user-my-account", updated.Status.RootAccountSecretName)
		assert.NotNil(t, updated.Status.ObservedGeneration)
		assert.Equal(t, int64(2), *updated.Status.ObservedGeneration)
	})

	t.Run("update status with empty secret name when root user skipped", func(t *testing.T) {
		r.updateStatusWithAccountID(int64(3), nsName, "RGW12345678901234567", "")
		updated := &cephv1.CephObjectStoreAccount{}
		err := r.client.Get(ctx, nsName, updated)
		assert.NoError(t, err)
		assert.NotNil(t, updated.Status)
		assert.Equal(t, k8sutil.ReadyStatus, updated.Status.Phase)
		assert.Equal(t, "", updated.Status.RootAccountSecretName)
	})

	t.Run("update status for nonexistent resource does not panic", func(t *testing.T) {
		missingName := types.NamespacedName{Name: "does-not-exist", Namespace: namespace}
		assert.NotPanics(t, func() {
			r.updateStatus(int64(1), missingName, k8sutil.ReadyStatus)
		})
		assert.NotPanics(t, func() {
			r.updateStatusWithAccountID(int64(1), missingName, "some-id", "some-secret")
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

func TestSkipRootUserCreation(t *testing.T) {
	t.Run("skip when skipCreate is true", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{
				RootUser: &cephv1.AccountRootUserSpec{SkipCreate: ptr.To(true)},
			},
		}
		assert.True(t, skipRootUserCreation(account))
	})

	t.Run("do not skip when skipCreate is false", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{
				RootUser: &cephv1.AccountRootUserSpec{SkipCreate: ptr.To(false)},
			},
		}
		assert.False(t, skipRootUserCreation(account))
	})

	t.Run("do not skip when rootUser is nil", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			Spec: cephv1.ObjectStoreAccountSpec{},
		}
		assert.False(t, skipRootUserCreation(account))
	})
}

func TestGetRootUserID(t *testing.T) {
	account := &cephv1.CephObjectStoreAccount{
		ObjectMeta: metav1.ObjectMeta{
			UID: "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
		},
	}
	assert.Equal(t, "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", getRootUserID(account))
}

func TestGetRootUserDisplayName(t *testing.T) {
	t.Run("returns custom display name when set", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "my-account", Namespace: "rook-ceph"},
			Spec: cephv1.ObjectStoreAccountSpec{
				RootUser: &cephv1.AccountRootUserSpec{DisplayName: "Custom Root"},
			},
		}
		assert.Equal(t, "Custom Root", getRootUserDisplayName(account))
	})

	t.Run("returns default display name when not set", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "my-account", Namespace: "rook-ceph"},
			Spec:       cephv1.ObjectStoreAccountSpec{},
		}
		assert.Equal(t, "root-my-account", getRootUserDisplayName(account))
	})

	t.Run("returns default display name when rootUser is nil", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec:       cephv1.ObjectStoreAccountSpec{},
		}
		assert.Equal(t, "root-test", getRootUserDisplayName(account))
	})

	t.Run("truncates generated display name to 64 characters", func(t *testing.T) {
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "a-very-long-account-name-that-exceeds-the-sixty-four-character-limit-for-rgw-display-names"},
			Spec:       cephv1.ObjectStoreAccountSpec{},
		}
		displayName := getRootUserDisplayName(account)
		assert.LessOrEqual(t, len(displayName), 64)
		assert.Equal(t, "root-a-very-long-account-name-that-exceeds-the-sixty-four-charac", displayName)
	})

	t.Run("does not truncate user-provided display name", func(t *testing.T) {
		longName := "This is a user-provided display name that is longer than sixty-four characters in total"
		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "my-account", Namespace: "rook-ceph"},
			Spec: cephv1.ObjectStoreAccountSpec{
				RootUser: &cephv1.AccountRootUserSpec{DisplayName: longName},
			},
		}
		assert.Equal(t, longName, getRootUserDisplayName(account))
	})
}

func TestGenerateRootUserSecretName(t *testing.T) {
	account := &cephv1.CephObjectStoreAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "my-account"},
		Spec:       cephv1.ObjectStoreAccountSpec{Store: "my-store"},
	}
	assert.Equal(t, "rook-ceph-object-root-user-my-account", generateRootUserSecretName(account))
}

func TestReconcileRootUser(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	ctx := context.TODO()

	objectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: store, Namespace: namespace},
		Spec: cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{Port: 80},
		},
	}

	t.Run("create root user successfully", func(t *testing.T) {
		uid := types.UID("a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d")
		rootUserJSON := `{"user_id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "display_name": "root-my-account", "keys": [{"access_key": "AK123", "secret_key": "SK456"}]}`

		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/admin/user") {
					return &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"Code":"NoSuchUser","RequestId":"tx000","HostId":""}`))),
					}, nil
				}
				if req.Method == http.MethodPut && strings.HasSuffix(req.URL.Path, "/admin/user") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(rootUserJSON))),
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
			Spec: cephv1.ObjectStoreAccountSpec{Store: store},
		}

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()
		r := &ReconcileObjectStoreAccount{
			client: cl,
			scheme: s,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
				Context:        cephobject.Context{Endpoint: "http://rook-ceph-rgw-my-store.rook-ceph:80"},
			},
			opManagerContext: ctx,
		}

		secretName, err := r.reconcileRootUser(account, "RGW12345678901234567", objectStore)
		assert.NoError(t, err)
		assert.Equal(t, "rook-ceph-object-root-user-my-account", secretName)

		// Verify the secret was created with correct data
		secret := &corev1.Secret{}
		err = r.client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
		assert.NoError(t, err)
		assert.Equal(t, "AK123", secret.StringData["AccessKey"])
		assert.Equal(t, "SK456", secret.StringData["SecretKey"])
		assert.Equal(t, "http://rook-ceph-rgw-my-store.rook-ceph:80", secret.StringData["Endpoint"])
	})

	t.Run("skip root user when skipCreate is true and delete existing root user and secret", func(t *testing.T) {
		deleteCalled := false
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/user") {
					deleteCalled = true
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

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       "some-uid",
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store:    store,
				RootUser: &cephv1.AccountRootUserSpec{SkipCreate: ptr.To(true)},
			},
		}

		// Pre-create the root user secret so we can verify it gets deleted
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      generateRootUserSecretName(account),
				Namespace: namespace,
			},
			StringData: map[string]string{
				"AccessKey": "AK123",
				"SecretKey": "SK456",
			},
		}
		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account, existingSecret).Build()

		r := &ReconcileObjectStoreAccount{
			client: cl,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}
		secretName, err := r.reconcileRootUser(account, "RGW12345678901234567", objectStore)
		assert.NoError(t, err)
		assert.Equal(t, "", secretName)
		assert.True(t, deleteCalled, "should attempt to delete root user when skipCreate is true")

		// Verify the secret was deleted
		secret := &corev1.Secret{}
		err = cl.Get(ctx, types.NamespacedName{Name: generateRootUserSecretName(account), Namespace: namespace}, secret)
		assert.Error(t, err)
		assert.True(t, kerrors.IsNotFound(err), "root user secret should be deleted when skipCreate is true")
	})

	t.Run("skip root user when skipCreate is true and secret does not exist", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete && strings.HasSuffix(req.URL.Path, "/admin/user") {
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

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       "some-uid",
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store:    store,
				RootUser: &cephv1.AccountRootUserSpec{SkipCreate: ptr.To(true)},
			},
		}

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()

		r := &ReconcileObjectStoreAccount{
			client: cl,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
			},
			opManagerContext: ctx,
		}
		secretName, err := r.reconcileRootUser(account, "RGW12345678901234567", objectStore)
		assert.NoError(t, err)
		assert.Equal(t, "", secretName, "should succeed even when secret does not exist")
	})

	t.Run("update root user display name", func(t *testing.T) {
		uid := types.UID("b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e")
		existingUserJSON := fmt.Sprintf(`{"user_id": "%s", "display_name": "Old Name", "keys": [{"access_key": "AK789", "secret_key": "SK012"}]}`, string(uid))
		updatedUserJSON := fmt.Sprintf(`{"user_id": "%s", "display_name": "New Custom Name", "keys": [{"access_key": "AK789", "secret_key": "SK012"}]}`, string(uid))

		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/admin/user") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(existingUserJSON))),
					}, nil
				}
				if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/admin/user") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(updatedUserJSON))),
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
				Store:    store,
				RootUser: &cephv1.AccountRootUserSpec{DisplayName: "New Custom Name"},
			},
		}

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()
		r := &ReconcileObjectStoreAccount{
			client: cl,
			scheme: s,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
				Context:        cephobject.Context{Endpoint: "http://rook-ceph-rgw-my-store.rook-ceph:80"},
			},
			opManagerContext: ctx,
		}

		secretName, err := r.reconcileRootUser(account, "RGW12345678901234567", objectStore)
		assert.NoError(t, err)
		assert.Equal(t, "rook-ceph-object-root-user-my-account", secretName)
	})

	t.Run("always updates even when display name matches", func(t *testing.T) {
		uid := types.UID("c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f")
		expectedDisplayName := fmt.Sprintf("root-%s", name)
		existingUserJSON := fmt.Sprintf(`{"user_id": "%s", "display_name": "%s", "keys": [{"access_key": "AK111", "secret_key": "SK222"}]}`, string(uid), expectedDisplayName)

		modifyCalled := false
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/admin/user") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(existingUserJSON))),
					}, nil
				}
				if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/admin/user") {
					modifyCalled = true
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(existingUserJSON))),
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
			Spec: cephv1.ObjectStoreAccountSpec{Store: store},
		}

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreAccount{}, &cephv1.CephObjectStoreAccountList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(account).Build()
		r := &ReconcileObjectStoreAccount{
			client: cl,
			scheme: s,
			objContext: &cephobject.AdminOpsContext{
				AdminOpsClient: adminClient,
				Context:        cephobject.Context{Endpoint: "http://rook-ceph-rgw-my-store.rook-ceph:80"},
			},
			opManagerContext: ctx,
		}

		secretName, err := r.reconcileRootUser(account, "RGW12345678901234567", objectStore)
		assert.NoError(t, err)
		assert.NotEmpty(t, secretName)
		assert.True(t, modifyCalled, "should always call modify to ensure desired state")
	})
}
