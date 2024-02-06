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

package operator

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestOperatorController(t *testing.T) {
	ctx := context.TODO()
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")
	name := controller.OperatorSettingConfigMapName
	namespace := "rook-ceph"
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	t.Run("success - normal run", func(t *testing.T) {
		fakeClientSet := test.New(t, 1)
		test.SetFakeKubernetesVersion(fakeClientSet, "v1.21.0")
		c := &clusterd.Context{
			Clientset:     fakeClientSet,
			RookClientset: rookclient.NewSimpleClientset(),
		}

		// Register operator types with the runtime scheme.
		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &v1.ConfigMap{}, &v1.ConfigMapList{})

		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).Build()

		// Create a ReconcileCSI object with the scheme and fake client.
		r := &ReconcileConfig{
			client:  cl,
			context: c,
			config: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
		}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
	})

	t.Run("success - env set for command timeout", func(t *testing.T) {
		fakeClientSet := test.New(t, 1)
		test.SetFakeKubernetesVersion(fakeClientSet, "v1.21.0")
		c := &clusterd.Context{
			Clientset:     fakeClientSet,
			RookClientset: rookclient.NewSimpleClientset(),
		}

		// Register operator types with the runtime scheme.
		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &v1.ConfigMap{}, &v1.ConfigMapList{})

		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).Build()

		// Create a ReconcileCSI object with the scheme and fake client.
		r := &ReconcileConfig{
			client:  cl,
			context: c,
			config: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
		}
		t.Setenv("ROOK_CEPH_COMMANDS_TIMEOUT_SECONDS", "10")
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.Equal(t, time.Second*10, exec.CephCommandsTimeout)
	})

	t.Run("success - cm set for command timeout", func(t *testing.T) {
		fakeClientSet := test.New(t, 1)
		test.SetFakeKubernetesVersion(fakeClientSet, "v1.21.0")
		c := &clusterd.Context{
			Clientset:     fakeClientSet,
			RookClientset: rookclient.NewSimpleClientset(),
		}

		// Register operator types with the runtime scheme.
		// s := scheme.Scheme
		s := clientgoscheme.Scheme
		// s.AddKnownTypes(cephv1.SchemeGroupVersion, &v1.ConfigMap{})

		opConfigCM := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      controller.OperatorSettingConfigMapName,
				Namespace: namespace,
			},
			Data: map[string]string{
				"ROOK_CEPH_COMMANDS_TIMEOUT_SECONDS": "20",
			},
		}

		object := []runtime.Object{
			opConfigCM,
		}
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

		// Create a ReconcileCSI object with the scheme and fake client.
		r := &ReconcileConfig{
			client:  cl,
			context: c,
			config: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
		}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.Equal(t, time.Second*20, exec.CephCommandsTimeout)
	})

	t.Run("success - env set for discovery daemon", func(t *testing.T) {
		fakeClientSet := test.New(t, 1)
		test.SetFakeKubernetesVersion(fakeClientSet, "v1.21.0")
		c := &clusterd.Context{
			Clientset:     fakeClientSet,
			RookClientset: rookclient.NewSimpleClientset(),
		}

		// Register operator types with the runtime scheme.
		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &v1.ConfigMap{}, &v1.ConfigMapList{})

		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).Build()

		// Create a ReconcileCSI object with the scheme and fake client.
		r := &ReconcileConfig{
			client:  cl,
			context: c,
			config: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
		}
		t.Setenv("ROOK_ENABLE_DISCOVERY_DAEMON", "true")
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		ds, err := c.Clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(ds.Items), ds)
		assert.Equal(t, "rook-discover", ds.Items[0].Name, ds)
	})

	t.Run("success - cm set for allowing loop devices", func(t *testing.T) {
		fakeClientSet := test.New(t, 1)
		test.SetFakeKubernetesVersion(fakeClientSet, "v1.21.0")
		c := &clusterd.Context{
			Clientset:     fakeClientSet,
			RookClientset: rookclient.NewSimpleClientset(),
		}

		// Register operator types with the runtime scheme.
		// s := scheme.Scheme
		s := clientgoscheme.Scheme
		// s.AddKnownTypes(cephv1.SchemeGroupVersion, &v1.ConfigMap{})

		opConfigCM := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      controller.OperatorSettingConfigMapName,
				Namespace: namespace,
			},
			Data: map[string]string{
				"ROOK_CEPH_ALLOW_LOOP_DEVICES": "true",
			},
		}

		object := []runtime.Object{
			opConfigCM,
		}
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

		// Create a ReconcileCSI object with the scheme and fake client.
		r := &ReconcileConfig{
			client:  cl,
			context: c,
			config: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
		}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.True(t, controller.LoopDevicesAllowed())
	})
}
