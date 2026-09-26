/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// Package realm to manage a rook object realm.
package realm

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	name         = "realm-a"
	namespace    = "rook-ceph"
	realmGetJSON = `{
		"id": "237e6250-5f7d-4b85-9359-8cb2b1848507",
		"name": "realm-a",
		"current_period": "df665ecb-1762-47a9-9c66-f938d251c02a",
		"epoch": 2
	}`
)

func TestCephObjectRealmController(t *testing.T) {
	ctx := context.TODO()
	//
	// TEST 1 SETUP
	//
	// FAILURE because no CephCluster
	//
	// A Pool resource with metadata and spec.
	r, objectRealm := getObjectRealmAndReconcileObjectRealm(t)
	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)

	//
	// TEST 2:
	//
	// FAILURE we have a cluster but it's not ready
	//
	cephCluster := &cephv1.CephCluster{
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

	object := []runtime.Object{
		objectRealm,
		cephCluster,
	}
	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(r.scheme).WithRuntimeObjects(object...).Build()
	// Create a ReconcileObjectRealm object with the scheme and fake client.
	r = &ReconcileObjectRealm{client: cl, scheme: r.scheme, context: r.context, recorder: events.NewFakeRecorder(50)}
	res, err = r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)

	//
	// TEST 3:
	//
	// SUCCESS! The CephCluster is ready and Object Realm is Created
	//

	// Mock clusterInfo
	secrets := map[string][]byte{
		"fsid":         []byte(name),
		"mon-secret":   []byte("monsecret"),
		"admin-secret": []byte("adminsecret"),
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-mon",
			Namespace: namespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}
	_, err = r.context.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Add ready status to the CephCluster
	cephCluster.Status.Phase = k8sutil.ReadyStatus
	cephCluster.Status.CephStatus.Health = "HEALTH_OK"

	// Create a fake client to mock API calls.
	cl = fake.NewClientBuilder().WithScheme(r.scheme).WithRuntimeObjects(object...).Build()

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "realm" && args[1] == "get" {
				return realmGetJSON, nil
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "realm" && args[1] == "get" {
				return realmGetJSON, nil
			}
			return "", nil
		},
	}

	r.context.Executor = executor

	// Create a ReconcileObjectRealm object with the scheme and fake client.
	r = &ReconcileObjectRealm{client: cl, scheme: r.scheme, context: r.context, recorder: events.NewFakeRecorder(50)}

	res, err = r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
	err = r.client.Get(context.TODO(), req.NamespacedName, objectRealm)
	assert.NoError(t, err)
}

func TestPullCephRealm(t *testing.T) {
	ctx := context.TODO()
	r, objectRealm := getObjectRealmAndReconcileObjectRealm(t)

	secrets := map[string][]byte{
		"access-key": []byte("akey"),
		"secret-key": []byte("skey"),
	}

	secretName := objectRealm.Name + "-keys"
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: objectRealm.Namespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}

	_, err := r.context.Clientset.CoreV1().Secrets(objectRealm.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	objectRealm.Spec.Pull.Endpoint = "http://10.2.1.164:80"
	res, err := r.pullCephRealm(objectRealm)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
}

func TestCreateRealmKeys(t *testing.T) {
	r, objectRealm := getObjectRealmAndReconcileObjectRealm(t)

	res, err := r.createRealmKeys(objectRealm)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
}

func TestCreateCephRealm(t *testing.T) {
	r, objectRealm := getObjectRealmAndReconcileObjectRealm(t)

	res, err := r.createCephRealm(objectRealm)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
}

func getObjectRealmAndReconcileObjectRealm(t *testing.T) (*ReconcileObjectRealm, *cephv1.CephObjectRealm) {
	objectRealm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectRealm",
		},
		Spec: cephv1.ObjectRealmSpec{},
	}
	cephCluster := &cephv1.CephCluster{}

	// Objects to track in the fake client.
	object := []runtime.Object{
		objectRealm,
		cephCluster,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "realm" && args[1] == "get" {
				return realmGetJSON, nil
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
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectRealm{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	// Create a ReconcileObjectRealm object with the scheme and fake client.
	clusterInfo := cephclient.AdminTestClusterInfo("rook")
	r := &ReconcileObjectRealm{client: cl, scheme: s, context: c, clusterInfo: clusterInfo, recorder: events.NewFakeRecorder(50)}

	return r, objectRealm
}

func TestReconcileObjectRealm_createRealmKeys(t *testing.T) {
	ctx := context.TODO()
	realmName := "my-realm"
	ns := "my-ns"

	scheme := scheme.Scheme
	scheme.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectRealm{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	realm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realmName,
			Namespace: ns,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectRealm",
		},
	}

	t.Run("should be idempotent", func(t *testing.T) {
		r := ReconcileObjectRealm{
			context: &clusterd.Context{
				Clientset: k8sfake.NewClientset(),
			},
			scheme:   scheme,
			recorder: events.NewFakeRecorder(50),
		}

		for _, tName := range []string{"first reconcile", "second reconcile"} {
			// the output should be the same on the first and subsequent reconciles
			t.Run(tName, func(t *testing.T) {
				res, err := r.createRealmKeys(realm)
				assert.NoError(t, err)
				assert.True(t, res.IsZero())

				secret, err := r.context.Clientset.CoreV1().Secrets(ns).Get(ctx, realmName+"-keys", metav1.GetOptions{})
				assert.NoError(t, err)
				assert.Contains(t, secret.Data, "access-key")
				assert.Contains(t, secret.Data, "secret-key")
			})
		}
	})

	t.Run("generated keys are URL-safe", func(t *testing.T) {
		// the access key is embedded in the '/'-delimited SigV4 credential scope, so a '/'
		// anywhere in it makes "radosgw-admin realm pull" fail with "(22) Invalid argument"
		urlSafe := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
		for range 100 {
			r := ReconcileObjectRealm{
				context: &clusterd.Context{
					Clientset: k8sfake.NewClientset(),
				},
				scheme:   scheme,
				recorder: events.NewFakeRecorder(50),
			}

			res, err := r.createRealmKeys(realm)
			require.NoError(t, err)
			assert.True(t, res.IsZero())

			secret, err := r.context.Clientset.CoreV1().Secrets(ns).Get(ctx, realmName+"-keys", metav1.GetOptions{})
			require.NoError(t, err)
			assert.Regexp(t, urlSafe, string(secret.Data["access-key"]))
			assert.Regexp(t, urlSafe, string(secret.Data["secret-key"]))
		}
	})

	t.Run("should fail if the secret doesn't have the necessary keys", func(t *testing.T) {
		secret := &v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: v1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      realmName + "-keys",
			},
			Data: map[string][]byte{
				"access-key": []byte("my-access-key"),
				// missing "secret-key"
			},
		}

		r := ReconcileObjectRealm{
			context: &clusterd.Context{
				Clientset: k8sfake.NewClientset(secret),
			},
			scheme:   scheme,
			recorder: events.NewFakeRecorder(50),
		}

		_, err := r.createRealmKeys(realm)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user likely created or modified the secret manually and should add the missing key back into the secret")
	})
}

func TestCephObjectRealmFinalizer(t *testing.T) {
	ctx := context.TODO()
	finalizer := "cephobjectrealm.ceph.rook.io"
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectRealm{}, &cephv1.CephObjectRealmList{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	newRealm := func() *cephv1.CephObjectRealm {
		return &cephv1.CephObjectRealm{
			TypeMeta:   metav1.TypeMeta{Kind: "CephObjectRealm"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
	}
	newDeletingRealm := func() *cephv1.CephObjectRealm {
		realm := newRealm()
		realm.Finalizers = []string{finalizer}
		realm.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		return realm
	}
	newCephCluster := func(health string) *cephv1.CephCluster {
		return &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{Name: namespace, Namespace: namespace},
			Status:     cephv1.ClusterStatus{CephStatus: &cephv1.CephStatus{Health: health}},
		}
	}
	newZoneGroup := func(zoneGroupName, zoneGroupNamespace, realmName string) *cephv1.CephObjectZoneGroup {
		return &cephv1.CephObjectZoneGroup{
			ObjectMeta: metav1.ObjectMeta{Name: zoneGroupName, Namespace: zoneGroupNamespace},
			Spec:       cephv1.ObjectZoneGroupSpec{Realm: realmName},
		}
	}
	newReconciler := func(rookClient *rookclient.Clientset, objects ...runtime.Object) (*ReconcileObjectRealm, *events.FakeRecorder) {
		recorder := events.NewFakeRecorder(50)
		return &ReconcileObjectRealm{
			client:           test.NewFakeClientWithKind(s, objects...),
			scheme:           s,
			context:          &clusterd.Context{Clientset: k8sfake.NewClientset(), RookClientset: rookClient},
			opManagerContext: ctx,
			recorder:         recorder,
		}, recorder
	}
	drainEvents := func(recorder *events.FakeRecorder) []string {
		var got []string
		for len(recorder.Events) > 0 {
			got = append(got, <-recorder.Events)
		}
		return got
	}

	t.Run("reconcile adds the finalizer", func(t *testing.T) {
		r, _ := newReconciler(rookclient.NewSimpleClientset(), newRealm())

		_, err := r.Reconcile(ctx, req)
		require.NoError(t, err)

		realm := &cephv1.CephObjectRealm{}
		require.NoError(t, r.client.Get(ctx, req.NamespacedName, realm))
		assert.Contains(t, realm.Finalizers, finalizer)
	})

	// With a destructive cleanup policy, a CephCluster that is being deleted counts as gone
	destroyedCluster := newCephCluster("HEALTH_OK")
	destroyedCluster.Finalizers = []string{"cephcluster.ceph.rook.io"}
	destroyedCluster.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	destroyedCluster.Spec.CleanupPolicy.Confirmation = cephv1.DeleteDataDirOnHostsConfirmation

	dependent := newZoneGroup("zonegroup-a", namespace, name)

	tests := []struct {
		name        string
		cephCluster *cephv1.CephCluster // nil if no CephCluster exists
		zoneGroups  []runtime.Object
		wantResult  reconcile.Result
		released    bool
		blocked     bool // the deletion is reported as blocked by the zone groups
	}{
		{
			name:        "deletion is blocked while a zone group references the realm",
			cephCluster: newCephCluster("HEALTH_OK"),
			zoneGroups:  []runtime.Object{dependent},
			wantResult:  opcontroller.WaitForRequeueIfFinalizerBlocked,
			blocked:     true,
		},
		{
			name:        "finalizer is removed when no zone group references the realm",
			cephCluster: newCephCluster("HEALTH_OK"),
			zoneGroups:  []runtime.Object{newZoneGroup("zonegroup-b", namespace, "realm-b"), newZoneGroup("zonegroup-a", "other-namespace", name)},
			released:    true,
		},
		{
			name:        "finalizer is removed when there are no zone groups",
			cephCluster: newCephCluster("HEALTH_OK"),
			released:    true,
		},
		{
			name:        "deletion waits while the CephCluster is not ready",
			cephCluster: newCephCluster(""),
			zoneGroups:  []runtime.Object{dependent},
			wantResult:  opcontroller.WaitForRequeueIfCephClusterNotReady,
		},
		{
			name:       "finalizer is removed when there is no CephCluster",
			zoneGroups: []runtime.Object{dependent},
			released:   true,
		},
		{
			name:        "finalizer is removed when the CephCluster is deleted with a cleanup policy",
			cephCluster: destroyedCluster,
			zoneGroups:  []runtime.Object{dependent},
			released:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{newDeletingRealm()}
			if tt.cephCluster != nil {
				objects = append(objects, tt.cephCluster.DeepCopy())
			}
			r, recorder := newReconciler(rookclient.NewSimpleClientset(tt.zoneGroups...), objects...)

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantResult, res)

			got := &cephv1.CephObjectRealm{}
			err = r.client.Get(ctx, req.NamespacedName, got)
			if tt.released {
				assert.True(t, kerrors.IsNotFound(err), "realm CR should be gone, got err: %v", err)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, got.Finalizers, finalizer)

			if tt.blocked {
				require.NotNil(t, got.Status)
				cond := cephv1.FindStatusCondition(got.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
				require.NotNil(t, cond)
				assert.Equal(t, v1.ConditionTrue, cond.Status)
				assert.Equal(t, cephv1.ObjectHasDependentsReason, cond.Reason)

				events := drainEvents(recorder)
				require.Len(t, events, 1)
				assert.Contains(t, events[0], "Warning ReconcileFailed")
				assert.Contains(t, events[0], "will not be deleted until all dependents are removed: CephObjectZoneGroups: [zonegroup-a]")
			}
		})
	}

	t.Run("deletion reports the deleting event once no zone group references the realm", func(t *testing.T) {
		r, recorder := newReconciler(rookclient.NewSimpleClientset(), newDeletingRealm(), newCephCluster("HEALTH_OK"))

		_, err := r.Reconcile(ctx, req)
		require.NoError(t, err)

		events := drainEvents(recorder)
		assert.Condition(t, func() bool {
			for _, e := range events {
				if strings.HasPrefix(e, "Normal Deleting ") {
					return true
				}
			}
			return false
		}, "expected a Normal Deleting event, got %v", events)
	})

	t.Run("deletion waits when the zone groups cannot be listed", func(t *testing.T) {
		rookClient := rookclient.NewSimpleClientset()
		rookClient.PrependReactor("list", "cephobjectzonegroups", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("list failed")
		})
		r, _ := newReconciler(rookClient, newDeletingRealm(), newCephCluster("HEALTH_OK"))

		_, err := r.Reconcile(ctx, req)
		assert.ErrorContains(t, err, "list failed")

		got := &cephv1.CephObjectRealm{}
		require.NoError(t, r.client.Get(ctx, req.NamespacedName, got))
		assert.Contains(t, got.Finalizers, finalizer)
	})
}
