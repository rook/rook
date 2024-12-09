/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package radosnamespace

import (
	"context"
	"os"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
<<<<<<< HEAD

=======
	"github.com/rook/rook/pkg/operator/ceph/version"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"

	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
<<<<<<< HEAD
=======
	"k8s.io/client-go/kubernetes"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCephBlockPoolRadosNamespaceController(t *testing.T) {
	ctx := context.TODO()
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	var (
		name      = "namespace-a"
		namespace = "rook-ceph"
	)

	// A cephBlockPoolRadosNamespace resource with metadata and spec.
	cephBlockPoolRadosNamespace := &cephv1.CephBlockPoolRadosNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("c47cac40-9bee-4d52-823b-ccd803ba5bfe"),
		},
		Spec: cephv1.CephBlockPoolRadosNamespaceSpec{
			BlockPoolName: namespace,
		},
		Status: &cephv1.CephBlockPoolRadosNamespaceStatus{
			Phase: "",
		},
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		cephBlockPoolRadosNamespace,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}

			return "", nil
		},
	}
	c := &clusterd.Context{
		Executor:      executor,
		Clientset:     testop.New(t, 1),
		RookClientset: rookclient.NewSimpleClientset(),
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephClient{}, &cephv1.CephClusterList{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
	r := &ReconcileCephBlockPoolRadosNamespace{
		client:           cl,
		scheme:           s,
		context:          c,
		opManagerContext: ctx,
		opConfig:         opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
	}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Spec: cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
<<<<<<< HEAD
				Image:           "ceph/ceph:v20.0.0",
=======
				Image:           "ceph/ceph:v14.2.9",
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
				ImagePullPolicy: v1.PullIfNotPresent,
			},
		},
		Status: cephv1.ClusterStatus{
			Phase: "",
			CephVersion: &cephv1.ClusterVersion{
<<<<<<< HEAD
				Version: "20.0.0-0",
				Image:   "ceph/ceph:v20.0.0",
=======
				Version: "14.2.9-0",
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
			},
			CephStatus: &cephv1.CephStatus{
				Health: "",
			},
		},
	}
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	t.Run("error - no ceph cluster", func(t *testing.T) {
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - ceph cluster not ready", func(t *testing.T) {
		object = append(object, cephCluster)
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{client: cl, scheme: s, context: c, opManagerContext: context.TODO(), opConfig: opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"}}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)

		cephCluster.Status.Phase = cephv1.ConditionReady
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"
	})

	cephBlockPool := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Status: &cephv1.CephBlockPoolStatus{
			Phase: "",
		},
	}
	cephBlockPool.Spec.StatusCheck.Mirror.Disabled = true
	t.Run("error - ceph blockpool not ready", func(t *testing.T) {
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		cephBlockPool.Status.Phase = cephv1.ConditionReady
	})

	t.Run("success - ceph cluster ready, block pool rados namespace created", func(t *testing.T) {
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
		_, err := c.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)
		objects := []runtime.Object{
			cephBlockPoolRadosNamespace,
			cephCluster,
			cephBlockPool,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" && args[1] == "create" {
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" {
					return `{"mode":"disabled"}`, nil
				}

				return "", nil
			},
		}
		c.Executor = executor

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{
			client:                 cl,
			scheme:                 s,
			context:                c,
			opManagerContext:       context.TODO(),
			opConfig:               opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
			radosNamespaceContexts: make(map[string]*mirrorHealth),
		}
<<<<<<< HEAD
=======
		detectCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
			return &version.Reef, nil
		}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

		// Enable CSI
		csi.EnableRBD = true
		t.Setenv("POD_NAMESPACE", namespace)
		// Create CSI config map
		ownerRef := &metav1.OwnerReference{}
		ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, "")
		err = csi.CreateCsiConfigMap(context.TODO(), namespace, c.Clientset, ownerInfo)
		assert.NoError(t, err)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		err = r.client.Get(context.TODO(), req.NamespacedName, cephBlockPoolRadosNamespace)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, cephBlockPoolRadosNamespace.Status.Phase)

		// test that csi configmap is created
		cm, err := c.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, csi.ConfigName, metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotEmpty(t, cm.Data[csi.ConfigKey])
		assert.Contains(t, cm.Data[csi.ConfigKey], "clusterID")
		assert.Contains(t, cm.Data[csi.ConfigKey], name)
		err = c.Clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, csi.ConfigName, metav1.DeleteOptions{})
		assert.NoError(t, err)
	})

	t.Run("success - external mode csi config is updated", func(t *testing.T) {
		cephCluster.Spec.External.Enable = true
		objects := []runtime.Object{
			cephBlockPoolRadosNamespace,
			cephCluster,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{
			client:           cl,
			scheme:           s,
			context:          c,
			opManagerContext: ctx,
			opConfig:         opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
		}
<<<<<<< HEAD
=======
		detectCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
			return &version.Reef, nil
		}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

		// Enable CSI
		csi.EnableRBD = true
		t.Setenv("POD_NAMESPACE", namespace)
		// Create CSI config map
		ownerRef := &metav1.OwnerReference{}
		ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, "")
		err := csi.CreateCsiConfigMap(context.TODO(), namespace, c.Clientset, ownerInfo)
		assert.NoError(t, err)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		err = r.client.Get(ctx, req.NamespacedName, cephBlockPoolRadosNamespace)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, cephBlockPoolRadosNamespace.Status.Phase)
		assert.NotEmpty(t, cephBlockPoolRadosNamespace.Status.Info["clusterID"])

		// test that csi configmap is created
		cm, err := c.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, csi.ConfigName, metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotEmpty(t, cm.Data[csi.ConfigKey])
		assert.Contains(t, cm.Data[csi.ConfigKey], "clusterID")
		assert.Contains(t, cm.Data[csi.ConfigKey], name)
		cephCluster.Spec.External.Enable = false
	})

	t.Run("test rbd rados namespace mirroring enabled and blockpool mirroring disabled", func(t *testing.T) {
		remoteNamespace := ""
		cephBlockPoolRadosNamespace.Spec.Mirroring = &cephv1.RadosNamespaceMirroring{
			RemoteNamespace: &remoteNamespace,
			Mode:            "image",
		}
		objects := []runtime.Object{
			cephBlockPoolRadosNamespace,
			cephCluster,
			cephBlockPool,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" && args[1] == "create" {
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" {
					return `{"mode":""}`, nil
				}

				return "", nil
			},
		}
		c.Executor = executor

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{
			client:                 cl,
			scheme:                 s,
			context:                c,
			opManagerContext:       context.TODO(),
			opConfig:               opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
			radosNamespaceContexts: make(map[string]*mirrorHealth),
		}
<<<<<<< HEAD
=======
		detectCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
			return &version.Reef, nil
		}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

		res, err := r.Reconcile(ctx, req)
		assert.Error(t, err)
		assert.False(t, res.Requeue)
	})

	t.Run("test rbd rados namespace mirroring enabled and blockpool mirroring is also enabled but empty rados namespace", func(t *testing.T) {
		remoteNamespace := ""
		cephBlockPoolRadosNamespace.Spec.Mirroring = &cephv1.RadosNamespaceMirroring{
			RemoteNamespace: &remoteNamespace,
			Mode:            "image",
		}
		cephBlockPool.Spec.Mirroring.Enabled = true
		objects := []runtime.Object{
			cephBlockPoolRadosNamespace,
			cephCluster,
			cephBlockPool,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" && args[1] == "create" {
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" {
					return `{"mode":""}`, nil
				}

				return "", nil
			},
		}
		c.Executor = executor

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{
			client:                 cl,
			scheme:                 s,
			context:                c,
			opManagerContext:       context.TODO(),
			opConfig:               opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
			radosNamespaceContexts: make(map[string]*mirrorHealth),
		}
<<<<<<< HEAD

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
=======
		detectCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
			return &version.Reef, nil
		}

		res, err := r.Reconcile(ctx, req)
		assert.Error(t, err)
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
		assert.False(t, res.Requeue)
	})

	t.Run("test rbd rados namespace mirroring enabled and blockpool mirroring is also enabled and non empty rados namespace but less ceph version", func(t *testing.T) {
<<<<<<< HEAD
		cephCluster.Status.CephVersion.Version = "14.2.9"
=======
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
		remoteNamespace := "test-1"
		cephBlockPoolRadosNamespace.Spec.Mirroring = &cephv1.RadosNamespaceMirroring{
			RemoteNamespace: &remoteNamespace,
			Mode:            "image",
		}
		cephBlockPool.Spec.Mirroring.Enabled = true
		objects := []runtime.Object{
			cephBlockPoolRadosNamespace,
			cephCluster,
			cephBlockPool,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" && args[1] == "create" {
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "enable" {
					assert.Equal(t, cephBlockPoolRadosNamespace.Spec.Mirroring.RemoteNamespace, args[6])
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
					return `{}`, nil
				}
				return "", nil
			},
		}
		c.Executor = executor

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{
			client:                 cl,
			scheme:                 s,
			context:                c,
			opManagerContext:       context.TODO(),
			opConfig:               opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
			radosNamespaceContexts: make(map[string]*mirrorHealth),
		}
<<<<<<< HEAD
=======
		detectCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
			return &version.Reef, nil
		}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

		res, err := r.Reconcile(ctx, req)
		assert.Error(t, err)
		assert.False(t, res.Requeue)
<<<<<<< HEAD
		cephCluster.Status.CephVersion.Version = "20.0.0"
=======
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	})

	t.Run("test rbd rados namespace mirroring enabled and blockpool mirroring is also enabled and non empty rados namespace and correct ceph version", func(t *testing.T) {
		remoteNamespace := "test-1"
		cephBlockPoolRadosNamespace.Spec.Mirroring = &cephv1.RadosNamespaceMirroring{
			RemoteNamespace: &remoteNamespace,
			Mode:            "image",
		}
		cephBlockPool.Spec.Mirroring.Enabled = true
		objects := []runtime.Object{
			cephBlockPoolRadosNamespace,
			cephCluster,
			cephBlockPool,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" && args[1] == "create" {
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "enable" {
					assert.Equal(t, *cephBlockPoolRadosNamespace.Spec.Mirroring.RemoteNamespace, args[6])
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
					return `{}`, nil
				}
				return "", nil
			},
		}
		c.Executor = executor

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{
			client:                 cl,
			scheme:                 s,
			context:                c,
			opManagerContext:       context.TODO(),
			opConfig:               opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
			radosNamespaceContexts: make(map[string]*mirrorHealth),
		}
<<<<<<< HEAD
=======
		detectCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
			return &cephver.CephVersion{Major: 20, Minor: 0, Extra: 0}, nil
		}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		err = r.client.Get(ctx, req.NamespacedName, cephBlockPoolRadosNamespace)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, cephBlockPoolRadosNamespace.Status.Phase)
		assert.NotEmpty(t, cephBlockPoolRadosNamespace.Status.Info["clusterID"])
	})

	t.Run("test rbd rados namespace mirroring enabled and blockpool mirroring is also enabled and no remote rados namespace and correct ceph version", func(t *testing.T) {
		cephBlockPoolRadosNamespace.Spec.Mirroring = &cephv1.RadosNamespaceMirroring{
			Mode: "image",
		}
		cephBlockPool.Spec.Mirroring.Enabled = true
		objects := []runtime.Object{
			cephBlockPoolRadosNamespace,
			cephCluster,
			cephBlockPool,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" && args[1] == "create" {
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "enable" {
					assert.Equal(t, string(cephBlockPoolRadosNamespace.Spec.Mirroring.Mode), args[4])
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
					return `{}`, nil
				}
				return "", nil
			},
		}
		c.Executor = executor

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{
			client:                 cl,
			scheme:                 s,
			context:                c,
			opManagerContext:       context.TODO(),
			opConfig:               opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
			radosNamespaceContexts: make(map[string]*mirrorHealth),
		}
<<<<<<< HEAD
=======
		detectCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
			return &cephver.CephVersion{Major: 20, Minor: 0, Extra: 0}, nil
		}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		err = r.client.Get(ctx, req.NamespacedName, cephBlockPoolRadosNamespace)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, cephBlockPoolRadosNamespace.Status.Phase)
		assert.NotEmpty(t, cephBlockPoolRadosNamespace.Status.Info["clusterID"])
	})

	t.Run("test rbd rados namespace mirroring disabled", func(t *testing.T) {
		cephBlockPoolRadosNamespace.Spec.Mirroring = nil

		objects := []runtime.Object{
			cephBlockPoolRadosNamespace,
			cephCluster,
			cephBlockPool,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" && args[1] == "create" {
					return "", nil
				}
				// set mode = image as it was enabled earlier
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
					return `{"mode":"image"}`, nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "status" {
					return `{"images":[]}`, nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "peer" {
					return `{}`, nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "disable" {
					assert.Equal(t, cephBlockPool.Name+"/"+cephBlockPoolRadosNamespace.Name, args[3])
					return `{}`, nil
				}
				return "", nil
			},
		}
		c.Executor = executor

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPoolRadosNamespace object with the scheme and fake client.
		r = &ReconcileCephBlockPoolRadosNamespace{
			client:                 cl,
			scheme:                 s,
			context:                c,
			opManagerContext:       context.TODO(),
			opConfig:               opcontroller.OperatorConfig{Image: "ceph/ceph:v14.2.9"},
			radosNamespaceContexts: make(map[string]*mirrorHealth),
		}
<<<<<<< HEAD
=======
		detectCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, clientset kubernetes.Interface, cephClusterSpec *cephv1.ClusterSpec) (*cephver.CephVersion, error) {
			return &version.Reef, nil
		}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		err = r.client.Get(context.TODO(), req.NamespacedName, cephBlockPoolRadosNamespace)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, cephBlockPoolRadosNamespace.Status.Phase)
	})
}

func Test_buildClusterID(t *testing.T) {
	longName := "foooooooooooooooooooooooooooooooooooooooooooo"
	cephBlockPoolRadosNamespace := &cephv1.CephBlockPoolRadosNamespace{ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: longName}, Spec: cephv1.CephBlockPoolRadosNamespaceSpec{BlockPoolName: "replicapool"}}
	clusterID := buildClusterID(cephBlockPoolRadosNamespace)
	assert.Equal(t, "2a74e5201e6ff9d15916ce2109c4f868", clusterID)
}
