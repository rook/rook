package cosi

import (
	"context"
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	cephObjectStoreName = "my-store"
	namespace           = "rook-ceph"
)

func TestCephCOSIDriverController(t *testing.T) {
	ctx := context.TODO()
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")
	os.Setenv("POD_NAMESPACE", namespace)

	setupNewEnvironment := func(objects ...runtime.Object) *ReconcileCephCOSIDriver {
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
			RookClientset: rookfake.NewSimpleClientset(),
			Clientset:     clientset,
		}

		// Register operator types with the runtime scheme.
		s := scheme.Scheme
		err := appsv1.AddToScheme(s)
		if err != nil {
			assert.Fail(t, "failed to build scheme")
		}

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStore{})
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCOSIDriver{})
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreList{})

		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		return &ReconcileCephCOSIDriver{
			client:           cl,
			scheme:           s,
			context:          c,
			recorder:         &record.FakeRecorder{},
			opManagerContext: ctx,
		}
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      CephCOSIDriverName,
			Namespace: namespace,
		},
	}
	t.Run("no requeue no object store or cosi driver exists", func(t *testing.T) {
		r := setupNewEnvironment()
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, false, res.Requeue)
	})

	t.Run("object store exists", func(t *testing.T) {
		objectStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cephObjectStoreName,
				Namespace: namespace,
			},
		}
		r := setupNewEnvironment(objectStore)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, false, res.Requeue)
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(ctx, types.NamespacedName{Name: CephCOSIDriverName, Namespace: namespace}, cephCOSIDriverDeployment)
		assert.True(t, kerrors.IsNotFound(err))
	})

	t.Run("ceph cosi driver CRD with disabled mode without any object stores", func(t *testing.T) {
		cephCOSIDriver := &cephv1.CephCOSIDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      CephCOSIDriverName,
				Namespace: namespace,
			},
			Spec: cephv1.CephCOSIDriverSpec{
				DeploymentStrategy: cephv1.COSIDeploymentStrategyNever,
			},
		}
		r := setupNewEnvironment(cephCOSIDriver)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, false, res.Requeue)
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(ctx, types.NamespacedName{Name: cephCOSIDriver.Name, Namespace: cephCOSIDriver.Namespace}, cephCOSIDriverDeployment)
		assert.Error(t, err)
	})

	t.Run("ceph cosi driver CRD with disabled mode with object stores", func(t *testing.T) {
		cephCOSIDriver := &cephv1.CephCOSIDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      CephCOSIDriverName,
				Namespace: namespace,
			},
			Spec: cephv1.CephCOSIDriverSpec{
				DeploymentStrategy: cephv1.COSIDeploymentStrategyNever,
			},
		}
		objectStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cephObjectStoreName,
				Namespace: namespace,
			},
		}
		r := setupNewEnvironment(cephCOSIDriver, objectStore)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, false, res.Requeue)
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(ctx, types.NamespacedName{Name: cephCOSIDriver.Name, Namespace: cephCOSIDriver.Namespace}, cephCOSIDriverDeployment)
		assert.Error(t, err)
	})

	t.Run("ceph cosi driver CRD with enforced mode without any object stores", func(t *testing.T) {
		cephCOSIDriver := &cephv1.CephCOSIDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      CephCOSIDriverName,
				Namespace: namespace,
			},
			Spec: cephv1.CephCOSIDriverSpec{
				DeploymentStrategy: cephv1.COSIDeploymentStrategyAlways,
			},
		}
		r := setupNewEnvironment(cephCOSIDriver)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, false, res.Requeue)
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(ctx, types.NamespacedName{Name: cephCOSIDriver.Name, Namespace: cephCOSIDriver.Namespace}, cephCOSIDriverDeployment)
		assert.NoError(t, err)
	})

	t.Run("ceph cosi driver CRD with auto mode without object stores", func(t *testing.T) {
		cephCOSIDriver := &cephv1.CephCOSIDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      CephCOSIDriverName,
				Namespace: namespace,
			},
			Spec: cephv1.CephCOSIDriverSpec{
				DeploymentStrategy: cephv1.COSIDeploymentStrategyAuto,
			},
		}
		r := setupNewEnvironment(cephCOSIDriver)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, true, res.Requeue)
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(ctx, types.NamespacedName{Name: cephCOSIDriver.Name, Namespace: cephCOSIDriver.Namespace}, cephCOSIDriverDeployment)
		assert.Error(t, err)
	})

	t.Run("ceph cosi driver CRD with auto mode with object stores", func(t *testing.T) {
		cephCOSIDriver := &cephv1.CephCOSIDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      CephCOSIDriverName,
				Namespace: namespace,
			},
			Spec: cephv1.CephCOSIDriverSpec{
				DeploymentStrategy: cephv1.COSIDeploymentStrategyAuto,
			},
		}
		objectStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cephObjectStoreName,
				Namespace: namespace,
			},
		}
		r := setupNewEnvironment(cephCOSIDriver, objectStore)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, false, res.Requeue)
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(ctx, types.NamespacedName{Name: cephCOSIDriver.Name, Namespace: cephCOSIDriver.Namespace}, cephCOSIDriverDeployment)
		assert.NoError(t, err)
	})

	t.Run("ceph cosi driver CRD with custom values and no object stores", func(t *testing.T) {
		cephCOSIDriver := &cephv1.CephCOSIDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      CephCOSIDriverName,
				Namespace: namespace,
			},
			Spec: cephv1.CephCOSIDriverSpec{
				Image:              "quay.io/ceph/cosi:custom",
				DeploymentStrategy: cephv1.COSIDeploymentStrategyAuto,
			},
		}
		r := setupNewEnvironment(cephCOSIDriver)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, true, res.Requeue)
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(ctx, types.NamespacedName{Name: cephCOSIDriver.Name, Namespace: cephCOSIDriver.Namespace}, cephCOSIDriverDeployment)
		assert.Error(t, err)
	})

	t.Run("ceph cosi driver CRD with custom values and object stores", func(t *testing.T) {
		cephCOSIDriver := &cephv1.CephCOSIDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      CephCOSIDriverName,
				Namespace: namespace,
			},
			Spec: cephv1.CephCOSIDriverSpec{
				Image:              "quay.io/ceph/cosi:custom",
				DeploymentStrategy: cephv1.COSIDeploymentStrategyAuto,
			},
		}
		objectStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cephObjectStoreName,
				Namespace: namespace,
			},
		}
		r := setupNewEnvironment(cephCOSIDriver, objectStore)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, false, res.Requeue)
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(ctx, types.NamespacedName{Name: cephCOSIDriver.Name, Namespace: cephCOSIDriver.Namespace}, cephCOSIDriverDeployment)
		assert.NoError(t, err)
		assert.Equal(t, "quay.io/ceph/cosi:custom", cephCOSIDriverDeployment.Spec.Template.Spec.Containers[0].Image)
	})
}
