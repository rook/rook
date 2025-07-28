/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package subvolumegroup

import (
	"context"
	"os"
	"testing"

	"github.com/pkg/errors"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestFilesystemSubvolumeGroupController(t *testing.T) {
	ctx := context.TODO()
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	var (
		name      = "group-a"
		namespace = "rook-ceph"
	)

	// A cephFilesystemSubVolumeGroup resource with metadata and spec.
	cephFilesystemSubVolumeGroup := &cephv1.CephFilesystemSubVolumeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			UID:        types.UID("c47cac40-9bee-4d52-823b-ccd803ba5bfe"),
			Finalizers: []string{"cephfilesystemsubvolumegroup.ceph.rook.io"},
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephFilesystemSubvolumeGroup",
		},
		Spec: cephv1.CephFilesystemSubVolumeGroupSpec{
			FilesystemName: namespace,
		},
		Status: &cephv1.CephFilesystemSubVolumeGroupStatus{
			Phase: "",
		},
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		cephFilesystemSubVolumeGroup,
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

	// Create a ReconcileCephFilesystemSubVolumeGroup object with the scheme and fake client.
	r := &ReconcileCephFilesystemSubVolumeGroup{
		client:           cl,
		scheme:           s,
		context:          c,
		opManagerContext: ctx,
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
		Status: cephv1.ClusterStatus{
			Phase: "",
			CephVersion: &cephv1.ClusterVersion{
				Version: "14.2.9-0",
			},
			CephStatus: &cephv1.CephStatus{
				Health: "",
			},
		},
	}
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	cephFilesystem := &cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Status: &cephv1.CephFilesystemStatus{
			Phase: "",
		},
	}

	t.Run("error - no ceph cluster", func(t *testing.T) {
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - ceph cluster not ready", func(t *testing.T) {
		object = append(object, cephCluster)
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		// Create a ReconcileCephFilesystem object with the scheme and fake client.
		r = &ReconcileCephFilesystemSubVolumeGroup{client: cl, scheme: s, context: c, opManagerContext: context.TODO()}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)

		cephCluster.Status.Phase = cephv1.ConditionReady
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"
	})

	t.Run("error - ceph filesystem cluster not ready", func(t *testing.T) {
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		cephFilesystem.Status.Phase = cephv1.ConditionReady
	})

	t.Run("success - ceph cluster ready, mds are running and subvolumegroup created", func(t *testing.T) {
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
			cephFilesystemSubVolumeGroup,
			cephCluster,
			cephFilesystem,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "fs" && args[1] == "subvolumegroup" && args[2] == "create" {
					return "", nil
				} else if args[0] == "fs" && args[1] == "subvolumegroup" && args[2] == "pin" {
					return "", nil
				}

				return "", errors.Errorf("unknown command. %v", args)
			},
		}
		c.Executor = executor

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephFilesystemSubVolumeGroup object with the scheme and fake client.
		r = &ReconcileCephFilesystemSubVolumeGroup{
			client:           cl,
			scheme:           s,
			context:          c,
			opManagerContext: ctx,
		}

		// Enable CSI
		csi.EnableRBD = true
		t.Setenv("POD_NAMESPACE", namespace)
		// Create CSI config map
		ownerRef := &metav1.OwnerReference{}
		ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, "")
		err = csi.CreateCsiConfigMap(ctx, namespace, c.Clientset, ownerInfo)
		assert.NoError(t, err)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		err = r.client.Get(ctx, req.NamespacedName, cephFilesystemSubVolumeGroup)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, cephFilesystemSubVolumeGroup.Status.Phase)
		assert.NotEmpty(t, cephFilesystemSubVolumeGroup.Status.Info["clusterID"])

		// test that csi configmap is created
		cm, err := c.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, csi.ConfigName, metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotEmpty(t, cm.Data[csi.ConfigKey])
		assert.Contains(t, cm.Data[csi.ConfigKey], "clusterID")
		assert.Contains(t, cm.Data[csi.ConfigKey], "group-a")
		err = c.Clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, csi.ConfigName, metav1.DeleteOptions{})
		assert.NoError(t, err)
	})

	t.Run("success - external mode csi config is updated", func(t *testing.T) {
		cephCluster.Spec.External.Enable = true
		csiOpClientProfile := &csiopv1.ClientProfile{}

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{}, &csiopv1.ClientProfile{})
		objects := []runtime.Object{
			cephFilesystemSubVolumeGroup,
			cephCluster,
			csiOpClientProfile,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		// Create a ReconcileCephFilesystemSubVolumeGroup object with the scheme and fake client.
		r = &ReconcileCephFilesystemSubVolumeGroup{
			client:           cl,
			scheme:           s,
			context:          c,
			opManagerContext: ctx,
		}

		// Enable CSI
		csi.EnableRBD = true
		t.Setenv("POD_NAMESPACE", namespace)
		// Create CSI config map
		ownerRef := &metav1.OwnerReference{}
		ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, "")
		err := csi.CreateCsiConfigMap(ctx, namespace, c.Clientset, ownerInfo)
		assert.NoError(t, err)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		err = r.client.Get(ctx, req.NamespacedName, cephFilesystemSubVolumeGroup)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, cephFilesystemSubVolumeGroup.Status.Phase)
		assert.NotEmpty(t, cephFilesystemSubVolumeGroup.Status.Info["clusterID"])

		// test that csi configmap is created
		cm, err := c.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, csi.ConfigName, metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotEmpty(t, cm.Data[csi.ConfigKey])
		assert.Contains(t, cm.Data[csi.ConfigKey], "clusterID")
		assert.Contains(t, cm.Data[csi.ConfigKey], "group-a")
	})

	t.Run("success - test with multus ceph cluster", func(t *testing.T) {
		cephCluster.Spec.External.Enable = false
		cephCluster.Spec.Network.Provider = "multus"
		cephCluster.Status.Phase = cephv1.ConditionReady
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"
		objects := []runtime.Object{
			cephFilesystemSubVolumeGroup,
			cephCluster,
			cephFilesystem,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		c.Client = cl

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{}, &v1.ConfigMap{})
		// Create a ReconcileCephFilesystemSubVolumeGroup object with the scheme and fake client.
		r = &ReconcileCephFilesystemSubVolumeGroup{
			client:           cl,
			scheme:           s,
			context:          c,
			opManagerContext: ctx,
		}

		// Enable CSI
		csi.EnableRBD = true
		t.Setenv("POD_NAMESPACE", namespace)
		// Create CSI config map
		ownerRef := &metav1.OwnerReference{}
		ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, "")
		err := csi.CreateCsiConfigMap(ctx, namespace, c.Clientset, ownerInfo)
		assert.NoError(t, err)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		// test that csi configmap is created
		cm, err := c.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, csi.ConfigName, metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotEmpty(t, cm.Data[csi.ConfigKey])
		assert.Contains(t, cm.Data[csi.ConfigKey], "clusterID")
		assert.Contains(t, cm.Data[csi.ConfigKey], "group-a")
		assert.Contains(t, cm.Data[csi.ConfigKey], "netNamespaceFilePath")
	})
}

func Test_buildClusterID(t *testing.T) {
	longName := "foooooooooooooooooooooooooooooooooooooooooooo"
	cephFilesystemSubVolumeGroup := &cephv1.CephFilesystemSubVolumeGroup{ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: longName}, Spec: cephv1.CephFilesystemSubVolumeGroupSpec{FilesystemName: "myfs"}}
	clusterID := buildClusterID(cephFilesystemSubVolumeGroup)
	assert.Equal(t, "29e92135b7e8c014079b9f9f3566777d", clusterID)

	inputClusterID := "test-clusterId"
	cephFilesystemSubVolumeGroup = &cephv1.CephFilesystemSubVolumeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "rook-ceph",
			Name:      longName,
		},
		Spec: cephv1.CephFilesystemSubVolumeGroupSpec{
			FilesystemName: "myfs",
			ClusterID:      inputClusterID,
		},
	}
	clusterID = buildClusterID(cephFilesystemSubVolumeGroup)
	assert.Equal(t, inputClusterID, clusterID)
}

func Test_formatPinning(t *testing.T) {
	pinning := &cephv1.CephFilesystemSubVolumeGroupSpecPinning{}
	pinningStatus := formatPinning(*pinning)
	assert.Equal(t, "distributed=1", pinningStatus)

	distributedValue := 0
	pinning.Distributed = &distributedValue
	pinningStatus = formatPinning(*pinning)
	assert.Equal(t, "distributed=0", pinningStatus)

	pinning.Distributed = nil

	exportValue := 42
	pinning.Export = &exportValue
	pinningStatus = formatPinning(*pinning)
	assert.Equal(t, "export=42", pinningStatus)

	pinning.Export = nil

	randomValue := 0.31
	pinning.Random = &randomValue
	pinningStatus = formatPinning(*pinning)
	assert.Equal(t, "random=0.31", pinningStatus)
}
