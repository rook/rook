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

package bucket

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCephBucketController(t *testing.T) {
	var (
		name      = "rook-ceph"
		namespace = "rook-ceph"
	)
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
	c := &clusterd.Context{
		Clientset: test.New(t, 1),
	}
	t.Run("do nothing since there is no CephCluster", func(t *testing.T) {
		// Register operator types with the runtime scheme.
		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &v1.ConfigMap{})

		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).Build()
		c.Client = cl

		// Create a ReconcileBucket object with the scheme and fake client.
		r := &ReconcileBucket{
			client:  cl,
			context: c,
			opConfig: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
			opManagerContext: context.TODO(),
		}
		ctx := context.TODO()
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
	})

	t.Run("success bucket prov deployment", func(t *testing.T) {
		var err error

		// It's a bit annoying that lib-bucket-prov only supports passing a rest.Config{} which is really
		// hard to mock since it's the simplest expression of a kube config. Typically when mocking we pass
		// a fake client which is a bit easier to mock.
		// Also, mocking the &provisioner.Provisioner{} is not possible since not all the fields
		// like informerFactory are exported... I'm leaving the failed attempt below just in
		// case lib-bucket-prov changes in the future. For now, the client will start but will fail
		// to connect to the API server, we can see that by running the test.
		//
		// fakeKubeClient := testclient.NewSimpleClientset()
		// newBucketController = func(cfg *rest.Config, p *Provisioner, data map[string]string) (*provisioner.Provisioner, error) {
		// 	return &provisioner.Provisioner{
		// 		Name:            "prov",
		// 		informerFactory: informers.NewSharedInformerFactory(fakeKubeClient, 1*time.Second),

		// 		claimController: provisioner.NewController(
		// 			provisionerName,
		// 			NewProvisioner(c, nil), // change cluster info
		// 			fakeKubeClient,
		// 			libClientset,
		// 			informerFactory.Objectbucket().V1alpha1().ObjectBucketClaims(),
		// 			informerFactory.Objectbucket().V1alpha1().ObjectBuckets()),
		// 	}, nil
		// }

		assert.NoError(t, err)
		c.KubeConfig = &rest.Config{}
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
		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &v1alpha1.ObjectBucketClaim{}, &v1alpha1.ObjectBucket{})

		object := []runtime.Object{
			cephCluster,
		}
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		c.Client = cl

		// Create a ReconcileBucket object with the scheme and fake client.
		ctx, cancel := context.WithCancel(context.TODO())
		// defer cancel()

		r := &ReconcileBucket{
			client:  cl,
			context: c,
			opConfig: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
			opManagerContext: ctx,
		}

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
		_, err = c.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		// wait a few seconds for the manager to start
		time.Sleep(2 * time.Second)
		cancel()
	})
}
