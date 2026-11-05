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

package peermap

import (
	"context"
	"testing"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const csiOperatorResourcesSetting = "ROOK_CREATE_CSI_OPERATOR_RESOURCES"

func TestReconcilePoolIDMapClientProfileMapping(t *testing.T) {
	t.Setenv(k8sutil.PodNameEnvVar, "rook-ceph-operator")
	t.Setenv(k8sutil.PodNamespaceEnvVar, ns)

	s := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(s))
	assert.NoError(t, csiopv1.AddToScheme(s))

	ctx := t.Context()
	clusterInfo := cephclient.AdminTestClusterInfo(ns)
	fakeContext := &clusterd.Context{
		Client:    fake.NewClientBuilder().WithScheme(s).Build(),
		Executor:  mockExecutor,
		Clientset: test.New(t, 3),
	}

	// the operator pod and replicaset are needed for the owner reference of the mapping configmap
	_, err := fakeContext.Clientset.CoreV1().Pods(ns).Create(ctx, test.FakeOperatorPod(ns), metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = fakeContext.Clientset.AppsV1().ReplicaSets(ns).Create(ctx, test.FakeReplicaSet(ns), metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = fakeContext.Clientset.CoreV1().Secrets(ns).Create(ctx, &peer1Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	request := types.NamespacedName{Name: "rook-ceph-primary.mirrorPool1", Namespace: ns}

	t.Run("the mapping is created when csi operator resources are enabled", func(t *testing.T) {
		t.Setenv(csiOperatorResourcesSetting, "true")
		err := ReconcilePoolIDMap(ctx, fakeContext, clusterInfo, &fakeSinglePeerCephBlockPool)
		assert.NoError(t, err)

		actual := &csiopv1.ClientProfileMapping{}
		err = fakeContext.Client.Get(ctx, request, actual)
		assert.NoError(t, err)
		expected := []csiopv1.MappingsSpec{
			{
				LocalClientProfile:  ns,
				RemoteClientProfile: "peer1",
				BlockPoolIdMapping:  []csiopv1.BlockPoolIdPair{{"1", "2"}},
			},
		}
		assert.Equal(t, expected, actual.Spec.Mappings)
	})

	t.Run("the mapping is deleted when csi operator resources are disabled", func(t *testing.T) {
		t.Setenv(csiOperatorResourcesSetting, "false")
		err := ReconcilePoolIDMap(ctx, fakeContext, clusterInfo, &fakeSinglePeerCephBlockPool)
		assert.NoError(t, err)

		err = fakeContext.Client.Get(ctx, request, &csiopv1.ClientProfileMapping{})
		assert.True(t, kerrors.IsNotFound(err))
	})

	t.Run("no failure when csi operator resources are disabled and the CRDs are not installed", func(t *testing.T) {
		t.Setenv(csiOperatorResourcesSetting, "false")
		noMatchErr := &meta.NoKindMatchError{
			GroupKind:        schema.GroupKind{Group: csiopv1.GroupVersion.Group, Kind: "ClientProfileMapping"},
			SearchedVersions: []string{csiopv1.GroupVersion.Version},
		}
		// the api server does not know the ClientProfileMapping kind
		fakeContext.Client = fake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*csiopv1.ClientProfileMapping); ok {
					return noMatchErr
				}
				return c.Get(ctx, key, obj, opts...)
			},
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*csiopv1.ClientProfileMapping); ok {
					return noMatchErr
				}
				return c.Create(ctx, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*csiopv1.ClientProfileMapping); ok {
					return noMatchErr
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()

		err := ReconcilePoolIDMap(ctx, fakeContext, clusterInfo, &fakeSinglePeerCephBlockPool)
		assert.NoError(t, err)

		// the same reconcile fails when the resources are enabled but the CRDs are missing
		t.Setenv(csiOperatorResourcesSetting, "true")
		err = ReconcilePoolIDMap(ctx, fakeContext, clusterInfo, &fakeSinglePeerCephBlockPool)
		assert.Error(t, err)
	})
}

func TestGenerateClientProfileMappingSpec(t *testing.T) {
	mappings := &PeerIDMappings{
		{
			ClusterIDMapping: map[string]string{"peer1": ns},
			RBDPoolIDMapping: []map[string]string{{"2": "1"}, {"4": "3"}},
		},
		{
			ClusterIDMapping: map[string]string{"peer2": ns},
			RBDPoolIDMapping: []map[string]string{{"3": "1"}},
		},
		// a bootstrap peer token that was not created by Rook has no peer cluster ID
		{
			ClusterIDMapping: map[string]string{"": ns},
			RBDPoolIDMapping: []map[string]string{{"5": "1"}},
		},
		// no pool ID mappings
		{
			ClusterIDMapping: map[string]string{"peer3": ns},
		},
	}

	expected := csiopv1.ClientProfileMappingSpec{
		Mappings: []csiopv1.MappingsSpec{
			{
				LocalClientProfile:  ns,
				RemoteClientProfile: "peer1",
				BlockPoolIdMapping:  []csiopv1.BlockPoolIdPair{{"1", "2"}, {"3", "4"}},
			},
			{
				LocalClientProfile:  ns,
				RemoteClientProfile: "peer2",
				BlockPoolIdMapping:  []csiopv1.BlockPoolIdPair{{"1", "3"}},
			},
		},
	}

	assert.Equal(t, expected, generateClientProfileMappingSpec(mappings))
	assert.Empty(t, generateClientProfileMappingSpec(&PeerIDMappings{}).Mappings)
}

func TestReconcileClientProfileMapping(t *testing.T) {
	opNamespace := "rook-ceph-operator"
	poolName := "mirrorPool1"
	t.Setenv(k8sutil.PodNamespaceEnvVar, opNamespace)

	s := runtime.NewScheme()
	assert.NoError(t, csiopv1.AddToScheme(s))
	c := fake.NewClientBuilder().WithScheme(s).Build()
	ctx := t.Context()

	// the resource is created in the operator namespace and is named after the cluster namespace and the pool
	request := types.NamespacedName{Name: "rook-ceph-primary.mirrorPool1", Namespace: opNamespace}
	mappings := &PeerIDMappings{
		{
			ClusterIDMapping: map[string]string{"peer1": ns},
			RBDPoolIDMapping: []map[string]string{{"2": "1"}},
		},
	}

	t.Run("no resource is created without mappings", func(t *testing.T) {
		err := reconcileClientProfileMapping(ctx, c, ns, poolName, &PeerIDMappings{})
		assert.NoError(t, err)
		err = c.Get(ctx, request, &csiopv1.ClientProfileMapping{})
		assert.True(t, kerrors.IsNotFound(err))
	})

	t.Run("create", func(t *testing.T) {
		err := reconcileClientProfileMapping(ctx, c, ns, poolName, mappings)
		assert.NoError(t, err)

		actual := &csiopv1.ClientProfileMapping{}
		err = c.Get(ctx, request, actual)
		assert.NoError(t, err)
		assert.Equal(t, generateClientProfileMappingSpec(mappings), actual.Spec)
	})

	t.Run("no update when the mappings are unchanged", func(t *testing.T) {
		before := &csiopv1.ClientProfileMapping{}
		err := c.Get(ctx, request, before)
		assert.NoError(t, err)

		err = reconcileClientProfileMapping(ctx, c, ns, poolName, mappings)
		assert.NoError(t, err)

		after := &csiopv1.ClientProfileMapping{}
		err = c.Get(ctx, request, after)
		assert.NoError(t, err)
		assert.Equal(t, before.ResourceVersion, after.ResourceVersion)
	})

	t.Run("update when the peer pool ID changes", func(t *testing.T) {
		(*mappings)[0].RBDPoolIDMapping = []map[string]string{{"7": "1"}}
		err := reconcileClientProfileMapping(ctx, c, ns, poolName, mappings)
		assert.NoError(t, err)

		actual := &csiopv1.ClientProfileMapping{}
		err = c.Get(ctx, request, actual)
		assert.NoError(t, err)
		assert.Equal(t, []csiopv1.BlockPoolIdPair{{"1", "7"}}, actual.Spec.Mappings[0].BlockPoolIdMapping)
	})

	t.Run("existing resource is kept when the mappings are empty", func(t *testing.T) {
		err := reconcileClientProfileMapping(ctx, c, ns, poolName, &PeerIDMappings{})
		assert.NoError(t, err)
		err = c.Get(ctx, request, &csiopv1.ClientProfileMapping{})
		assert.NoError(t, err)
	})

	t.Run("delete", func(t *testing.T) {
		err := DeleteClientProfileMapping(ctx, c, ns, poolName)
		assert.NoError(t, err)
		err = c.Get(ctx, request, &csiopv1.ClientProfileMapping{})
		assert.True(t, kerrors.IsNotFound(err))

		// deleting a resource that is already gone is not an error
		err = DeleteClientProfileMapping(ctx, c, ns, poolName)
		assert.NoError(t, err)
	})
}
