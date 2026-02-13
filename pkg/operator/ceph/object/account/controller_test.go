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
	"k8s.io/client-go/tools/record"
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
	r := &ReconcileObjectStoreAccount{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}

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
		r = &ReconcileObjectStoreAccount{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}
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

		r = &ReconcileObjectStoreAccount{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}
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
		r = &ReconcileObjectStoreAccount{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}

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
				Finalizers: []string{"cephobjectstoreaccount.ceph.rook.io"},
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "CephObjectStoreAccount",
			},
		}
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
		_, err := freshC.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)

		freshR := &ReconcileObjectStoreAccount{client: freshCl, scheme: s, context: freshC, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}

		newMultisiteAdminOpsCtxFunc = func(objContext *cephobject.Context, spec *cephv1.ObjectStoreSpec) (*cephobject.AdminOpsContext, error) {
			mockClient := &cephobject.MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					// Handle account creation
					if req.Method == http.MethodPost {
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewReader([]byte(accountCreateJSON))),
						}, nil
					}
					// Handle account retrieval
					if req.Method == http.MethodGet {
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewReader([]byte(accountCreateJSON))),
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
		assert.Equal(t, "RGW12345678901234567", freshAccount.Status.AccountID)
	})
}

func TestReconcileAccount(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	ctx := context.TODO()

	t.Run("create account without ID", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
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
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW12345678901234567", accountID)
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

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW12345678901234567", accountID)
	})

	t.Run("account already exists with same name", func(t *testing.T) {
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
				Store:     store,
				AccountID: "RGW12345678901234567",
			},
		}

		accountID, err := r.reconcileAccount(account)
		assert.NoError(t, err)
		assert.Equal(t, "RGW12345678901234567", accountID)
	})

	t.Run("account exists with different name triggers update", func(t *testing.T) {
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

		account := &cephv1.CephObjectStoreAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "updated-account",
				Namespace: namespace,
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

	t.Run("account uses spec name over CR name", func(t *testing.T) {
		mockClient := &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodPost {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"RGW99999999999999999","name":"custom-name"}`))),
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
				Name:      "cr-name",
				Namespace: namespace,
			},
			Spec: cephv1.ObjectStoreAccountSpec{
				Store: store,
				Name:  "custom-name",
			},
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
		recorder:         record.NewFakeRecorder(5),
	}

	nsName := types.NamespacedName{Name: name, Namespace: namespace}

	t.Run("update status to ready", func(t *testing.T) {
		r.updateStatus(int64(1), nsName, k8sutil.ReadyStatus)
		updated := &cephv1.CephObjectStoreAccount{}
		err := r.client.Get(ctx, nsName, updated)
		assert.NoError(t, err)
		assert.NotNil(t, updated.Status)
		assert.Equal(t, k8sutil.ReadyStatus, updated.Status.Phase)
		assert.Equal(t, int64(1), updated.Status.ObservedGeneration)
	})

	t.Run("update status with account ID", func(t *testing.T) {
		r.updateStatusWithAccountID(int64(2), nsName, "RGW12345678901234567")
		updated := &cephv1.CephObjectStoreAccount{}
		err := r.client.Get(ctx, nsName, updated)
		assert.NoError(t, err)
		assert.NotNil(t, updated.Status)
		assert.Equal(t, k8sutil.ReadyStatus, updated.Status.Phase)
		assert.Equal(t, "RGW12345678901234567", updated.Status.AccountID)
		assert.Equal(t, int64(2), updated.Status.ObservedGeneration)
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
		recorder:         record.NewFakeRecorder(5),
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
