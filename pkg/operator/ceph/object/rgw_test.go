/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package object

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fclient "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStartRGW(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 3)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return `{"key":"mysecurekey"}`, nil
			}
			return `{"id":"test-id"}`, nil
		},
	}

	configDir := t.TempDir()
	info := clienttest.CreateTestClusterInfo(1)
	context := &clusterd.Context{Clientset: clientset, Executor: executor, ConfigDir: configDir}
	store := simpleStore()
	store.Spec.Gateway.Instances = 1
	version := "v1.1.0"
	data := config.NewStatelessDaemonDataPathMap(config.RgwType, "my-fs", "rook-ceph", "/var/lib/rook/")

	s := scheme.Scheme
	object := []runtime.Object{&cephv1.CephObjectStore{}}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	r := &ReconcileCephObjectStore{client: cl, scheme: s}

	// start a basic cluster
	ownerInfo := client.NewMinimumOwnerInfoWithOwnerRef()
	c := &clusterConfig{context, info, store, version, &cephv1.ClusterSpec{}, ownerInfo, data, r.client}
	err := c.startRGWPods(store.Name, store.Name, store.Name)
	assert.Nil(t, err)

	validateStart(ctx, t, c, clientset)
}

func validateStart(ctx context.Context, t *testing.T, c *clusterConfig, clientset *fclient.Clientset) {
	rgwName := instanceName(c.store.Name) + "-a"
	r, err := clientset.AppsV1().Deployments(c.store.Namespace).Get(ctx, rgwName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, rgwName, r.Name)
}

func TestCreateObjectStore(t *testing.T) {
	commandWithOutputFunc := func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if command == "ceph" {
			if args[1] == "erasure-code-profile" {
				return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
			}
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return `{"key":"mykey"}`, nil
			}
		} else {
			return `{"realms": []}`, nil
		}
		return "", nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: commandWithOutputFunc,
		MockExecuteCommandWithOutput:         commandWithOutputFunc,
	}

	store := simpleStore()
	clientset := test.New(t, 3)
	context := &clusterd.Context{Executor: executor, Clientset: clientset}
	info := clienttest.CreateTestClusterInfo(1)
	data := config.NewStatelessDaemonDataPathMap(config.RgwType, "my-fs", "rook-ceph", "/var/lib/rook/")

	// create the pools
	s := scheme.Scheme
	object := []runtime.Object{&cephv1.CephObjectStore{}}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	r := &ReconcileCephObjectStore{client: cl, scheme: s}
	ownerInfo := client.NewMinimumOwnerInfoWithOwnerRef()
	c := &clusterConfig{context, info, store, "1.2.3.4", &cephv1.ClusterSpec{}, ownerInfo, data, r.client}
	err := c.createOrUpdateStore(store.Name, store.Name, store.Name)
	assert.Nil(t, err)
}

func simpleStore() *cephv1.CephObjectStore {
	return &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "mycluster"},
		Spec: cephv1.ObjectStoreSpec{
			MetadataPool: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			DataPool:     cephv1.PoolSpec{ErasureCoded: cephv1.ErasureCodedSpec{CodingChunks: 1, DataChunks: 2}},
			Gateway:      cephv1.GatewaySpec{Port: 123},
		},
	}
}

func TestGenerateSecretName(t *testing.T) {
	cl := fake.NewClientBuilder().Build()

	// start a basic cluster
	c := &clusterConfig{&clusterd.Context{},
		&client.ClusterInfo{},
		&cephv1.CephObjectStore{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "mycluster"}},
		"v1.1.0",
		&cephv1.ClusterSpec{},
		&k8sutil.OwnerInfo{},
		&config.DataPathMap{},
		cl}
	secret := c.generateSecretName("a")
	assert.Equal(t, "rook-ceph-rgw-default-a-keyring", secret)
}

func TestEmptyPoolSpec(t *testing.T) {
	assert.True(t, emptyPool(cephv1.PoolSpec{}))

	p := cephv1.PoolSpec{FailureDomain: "foo"}
	assert.False(t, emptyPool(p))

	p = cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1}}
	assert.False(t, emptyPool(p))

	p = cephv1.PoolSpec{ErasureCoded: cephv1.ErasureCodedSpec{CodingChunks: 1}}
	assert.False(t, emptyPool(p))
}

func TestBuildDomainNameAndEndpoint(t *testing.T) {
	name := "my-store"
	ns := "rook-ceph"
	dns := BuildDomainName(name, ns)
	assert.Equal(t, "rook-ceph-rgw-my-store.rook-ceph.svc", dns)

	// non-secure endpoint
	var port int32 = 80
	ep := BuildDNSEndpoint(dns, port, false)
	assert.Equal(t, "http://rook-ceph-rgw-my-store.rook-ceph.svc:80", ep)

	// Secure endpoint
	var securePort int32 = 443
	ep = BuildDNSEndpoint(dns, securePort, true)
	assert.Equal(t, "https://rook-ceph-rgw-my-store.rook-ceph.svc:443", ep)
}

func TestGetTlsCaCert(t *testing.T) {
	objContext := &Context{
		Context: &clusterd.Context{
			Clientset: test.New(t, 3),
		},
		clusterInfo: client.AdminTestClusterInfo("rook-ceph"),
	}
	objectStore := simpleStore()

	t.Run("no gateway cert ref", func(t *testing.T) {
		tlsCert, insesure, err := GetTlsCaCert(objContext, &objectStore.Spec)
		assert.NoError(t, err)
		assert.False(t, insesure)
		assert.Nil(t, tlsCert)
	})

	t.Run("gateway cert ref but secret no found", func(t *testing.T) {
		objectStore.Spec.Gateway.SSLCertificateRef = "my-secret"
		tlsCert, insesure, err := GetTlsCaCert(objContext, &objectStore.Spec)
		assert.Error(t, err)
		assert.False(t, insesure)
		assert.Nil(t, tlsCert)
	})

	t.Run("gateway cert ref and secret found but no key and wrong type", func(t *testing.T) {
		s := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-secret",
				Namespace: "rook-ceph",
			},
			Type: "Yolo",
		}
		_, err := objContext.Context.Clientset.CoreV1().Secrets(objContext.clusterInfo.Namespace).Create(context.TODO(), s, metav1.CreateOptions{})
		assert.NoError(t, err)
		objectStore.Spec.Gateway.SSLCertificateRef = "my-secret"
		tlsCert, insesure, err := GetTlsCaCert(objContext, &objectStore.Spec)
		assert.Error(t, err)
		assert.EqualError(t, err, "failed to get TLS certificate from secret, unknown secret type \"Yolo\"")
		assert.False(t, insesure)
		assert.Nil(t, tlsCert)
		err = objContext.Context.Clientset.CoreV1().Secrets(objContext.clusterInfo.Namespace).Delete(context.TODO(), s.Name, metav1.DeleteOptions{})
		assert.NoError(t, err)
	})

	t.Run("gateway cert ref and Opaque secret found and no key is present", func(t *testing.T) {
		s := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-secret",
				Namespace: "rook-ceph",
			},
			Type: "Opaque",
		}
		_, err := objContext.Context.Clientset.CoreV1().Secrets(objContext.clusterInfo.Namespace).Create(context.TODO(), s, metav1.CreateOptions{})
		assert.NoError(t, err)
		objectStore.Spec.Gateway.SSLCertificateRef = "my-secret"
		tlsCert, insesure, err := GetTlsCaCert(objContext, &objectStore.Spec)
		assert.Error(t, err)
		assert.EqualError(t, err, "failed to get TLS certificate from secret, token is \"Opaque\" but key \"cert\" does not exist")
		assert.False(t, insesure)
		assert.Nil(t, tlsCert)
		err = objContext.Context.Clientset.CoreV1().Secrets(objContext.clusterInfo.Namespace).Delete(context.TODO(), s.Name, metav1.DeleteOptions{})
		assert.NoError(t, err)
	})

	t.Run("gateway cert ref and Opaque secret found and key is present", func(t *testing.T) {
		s := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-secret",
				Namespace: "rook-ceph",
			},
			Data: map[string][]byte{"cert": []byte(`-----BEGIN CERTIFICATE-----
MIIBJTCB0AIJAPNFNz1CNlDOMA0GCSqGSIb3DQEBCwUAMBoxCzAJBgNVBAYTAkZS
MQswCQYDVQQIDAJGUjAeFw0yMTA5MzAwODAzNDBaFw0yNDA2MjYwODAzNDBaMBox
CzAJBgNVBAYTAkZSMQswCQYDVQQIDAJGUjBcMA0GCSqGSIb3DQEBAQUAA0sAMEgC
QQDHeZ47hVBcryl6SCghM8Zj3Q6DQzJzno1J7EjPXef5m+pIVAEylS9sQuwKtFZc
vv3qS/OVFExmMdbrvfKEIfbBAgMBAAEwDQYJKoZIhvcNAQELBQADQQAAnflLuUM3
4Dq0v7If4cgae2mr7jj3U/lIpHVtFbF7kVjC/eqmeN1a9u0UbRHKkUr+X1mVX3rJ
BvjQDN6didwQ
-----END CERTIFICATE-----`)},
			Type: "Opaque",
		}
		_, err := objContext.Context.Clientset.CoreV1().Secrets(objContext.clusterInfo.Namespace).Create(context.TODO(), s, metav1.CreateOptions{})
		assert.NoError(t, err)
		objectStore.Spec.Gateway.SSLCertificateRef = "my-secret"
		tlsCert, insesure, err := GetTlsCaCert(objContext, &objectStore.Spec)
		assert.NoError(t, err)
		assert.False(t, insesure)
		assert.NotNil(t, tlsCert)
		err = objContext.Context.Clientset.CoreV1().Secrets(objContext.clusterInfo.Namespace).Delete(context.TODO(), s.Name, metav1.DeleteOptions{})
		assert.NoError(t, err)
	})
}
