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
	"fmt"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpecs(t *testing.T) {
	store := simpleStore()
	store.Spec.Gateway.Resources = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(500.0, resource.BinarySI),
		},
	}
	store.Spec.Gateway.PriorityClassName = "my-priority-class"
	info := clienttest.CreateTestClusterInfo(1)
	info.CephVersion = cephver.Nautilus
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")

	c := &clusterConfig{
		clusterInfo: info,
		store:       store,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v15"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
	}

	s, err := c.makeRGWPodSpec(rgwConfig)
	assert.NoError(t, err)

	// Check pod anti affinity is well added to be compliant with HostNetwork setting
	assert.Equal(t,
		1,
		len(s.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution))
	assert.Equal(t,
		getLabels(c.store.Name, c.store.Namespace, false),
		s.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector.MatchLabels)

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &s)
	podTemplate.RunFullSuite(cephconfig.RgwType, "default", "rook-ceph-rgw", "mycluster", "quay.io/ceph/ceph:myversion",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class")
}

func TestSSLPodSpec(t *testing.T) {
	ctx := context.TODO()
	// Placeholder
	context := &clusterd.Context{Clientset: test.New(t, 3)}

	store := simpleStore()
	store.Spec.Gateway.Resources = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(500.0, resource.BinarySI),
		},
	}
	store.Spec.Gateway.PriorityClassName = "my-priority-class"
	info := clienttest.CreateTestClusterInfo(1)
	info.CephVersion = cephver.Nautilus
	info.Namespace = store.Namespace
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")
	store.Spec.Gateway.SecurePort = 443

	c := &clusterConfig{
		clusterInfo: info,
		store:       store,
		context:     context,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v15"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
	}
	_, err := c.makeRGWPodSpec(rgwConfig)
	// No TLS certs specified, will return error
	assert.Error(t, err)

	// Using SSLCertificateRef
	// Opaque Secret
	c.store.Spec.Gateway.SSLCertificateRef = "mycert"
	rgwtlssecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.store.Spec.Gateway.SSLCertificateRef,
			Namespace: c.store.Namespace,
		},
		Data: map[string][]byte{
			"cert": []byte("tlssecrettesting"),
		},
		Type: v1.SecretTypeOpaque,
	}
	_, err = c.context.Clientset.CoreV1().Secrets(store.Namespace).Create(ctx, rgwtlssecret, metav1.CreateOptions{})
	assert.NoError(t, err)
	secretVolSrc, err := c.generateVolumeSourceWithTLSSecret()
	assert.NoError(t, err)
	assert.Equal(t, secretVolSrc.SecretName, "mycert")
	s, err := c.makeRGWPodSpec(rgwConfig)
	assert.NoError(t, err)
	podTemplate := cephtest.NewPodTemplateSpecTester(t, &s)
	podTemplate.RunFullSuite(cephconfig.RgwType, "default", "rook-ceph-rgw", "mycluster", "quay.io/ceph/ceph:myversion",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class")
	// TLS Secret
	c.store.Spec.Gateway.SSLCertificateRef = "tlscert"
	rgwtlssecret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.store.Spec.Gateway.SSLCertificateRef,
			Namespace: c.store.Namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("tlssecrettestingcert"),
			"tls.key": []byte("tlssecrettestingkey"),
		},
		Type: v1.SecretTypeTLS,
	}
	_, err = c.context.Clientset.CoreV1().Secrets(store.Namespace).Create(ctx, rgwtlssecret, metav1.CreateOptions{})
	assert.NoError(t, err)
	secretVolSrc, err = c.generateVolumeSourceWithTLSSecret()
	assert.NoError(t, err)
	assert.Equal(t, secretVolSrc.SecretName, "tlscert")
	s, err = c.makeRGWPodSpec(rgwConfig)
	assert.NoError(t, err)
	podTemplate = cephtest.NewPodTemplateSpecTester(t, &s)
	podTemplate.RunFullSuite(cephconfig.RgwType, "default", "rook-ceph-rgw", "mycluster", "quay.io/ceph/ceph:myversion",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class")
	// Using service serving cert
	c.store.Spec.Gateway.SSLCertificateRef = ""
	c.store.Spec.Gateway.Service = &(cephv1.RGWServiceSpec{Annotations: cephv1.Annotations{cephv1.ServiceServingCertKey: "rgw-cert"}})
	secretVolSrc, err = c.generateVolumeSourceWithTLSSecret()
	assert.NoError(t, err)
	assert.Equal(t, secretVolSrc.SecretName, "rgw-cert")
	// Using caBundleRef
	// Opaque Secret
	c.store.Spec.Gateway.CaBundleRef = "mycabundle"
	cabundlesecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.store.Spec.Gateway.CaBundleRef,
			Namespace: c.store.Namespace,
		},
		Data: map[string][]byte{
			"cabundle": []byte("cabundletesting"),
		},
		Type: v1.SecretTypeOpaque,
	}
	_, err = c.context.Clientset.CoreV1().Secrets(store.Namespace).Create(ctx, cabundlesecret, metav1.CreateOptions{})
	assert.NoError(t, err)
	caBundleVolSrc, err := c.generateVolumeSourceWithCaBundleSecret()
	assert.NoError(t, err)
	assert.Equal(t, caBundleVolSrc.SecretName, "mycabundle")
	s, err = c.makeRGWPodSpec(rgwConfig)
	assert.NoError(t, err)
	podTemplate = cephtest.NewPodTemplateSpecTester(t, &s)
	podTemplate.RunFullSuite(cephconfig.RgwType, "default", "rook-ceph-rgw", "mycluster", "quay.io/ceph/ceph:myversion",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class")

	assert.True(t, s.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, s.Spec.DNSPolicy)

}

func TestValidateSpec(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "crush" && args[2] == "dump" {
			return `{"types":[{"type_id": 0,"name": "osd"}, {"type_id": 1,"name": "host"}],"buckets":[{"id": -1,"name":"default"},{"id": -2,"name":"good"}, {"id": -3,"name":"host"}]}`, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	context := &clusterd.Context{Executor: executor}

	r := &ReconcileCephObjectStore{
		context: context,
		clusterSpec: &cephv1.ClusterSpec{
			External: cephv1.ExternalSpec{
				Enable: false,
			},
		},
		clusterInfo: &client.ClusterInfo{
			CephCred: client.CephCred{
				Username: "client.admin",
			},
		},
	}

	// valid store
	s := simpleStore()
	err := r.validateStore(s)
	assert.Nil(t, err)

	// no name
	s.Name = ""
	err = r.validateStore(s)
	assert.NotNil(t, err)
	s.Name = "default"
	err = r.validateStore(s)
	assert.Nil(t, err)

	// no namespace
	s.Namespace = ""
	err = r.validateStore(s)
	assert.NotNil(t, err)
	s.Namespace = "mycluster"
	err = r.validateStore(s)
	assert.Nil(t, err)

	// no replication or EC is valid
	s.Spec.MetadataPool.Replicated.Size = 0
	err = r.validateStore(s)
	assert.Nil(t, err)
	s.Spec.MetadataPool.Replicated.Size = 1
	err = r.validateStore(s)
	assert.Nil(t, err)

	// external with endpoints, success
	s.Spec.Gateway.ExternalRgwEndpoints = []v1.EndpointAddress{
		{
			IP: "192.168.0.1",
		},
	}
	err = r.validateStore(s)
	assert.Nil(t, err)
}

func TestGenerateLiveProbe(t *testing.T) {
	store := simpleStore()
	c := &clusterConfig{
		store: store,
		clusterSpec: &cephv1.ClusterSpec{
			Network: cephv1.NetworkSpec{
				HostNetwork: false,
			},
		},
	}

	// No SSL - HostNetwork is disabled - using internal port
	p := c.generateLiveProbe()
	assert.Equal(t, int32(8080), p.Handler.HTTPGet.Port.IntVal)
	assert.Equal(t, v1.URISchemeHTTP, p.Handler.HTTPGet.Scheme)

	// No SSL - HostNetwork is enabled
	c.store.Spec.Gateway.Port = 123
	c.store.Spec.Gateway.SecurePort = 0
	c.clusterSpec.Network.HostNetwork = true
	p = c.generateLiveProbe()
	assert.Equal(t, int32(123), p.Handler.HTTPGet.Port.IntVal)

	// SSL - HostNetwork is enabled
	c.store.Spec.Gateway.Port = 0
	c.store.Spec.Gateway.SecurePort = 321
	c.store.Spec.Gateway.SSLCertificateRef = "foo"
	p = c.generateLiveProbe()
	assert.Equal(t, int32(321), p.Handler.HTTPGet.Port.IntVal)

	// Both Non-SSL and SSL are enabled
	// liveprobe just on Non-SSL
	c.store.Spec.Gateway.Port = 123
	c.store.Spec.Gateway.SecurePort = 321
	c.store.Spec.Gateway.SSLCertificateRef = "foo"
	p = c.generateLiveProbe()
	assert.Equal(t, v1.URISchemeHTTP, p.Handler.HTTPGet.Scheme)
	assert.Equal(t, int32(123), p.Handler.HTTPGet.Port.IntVal)
}

func TestCheckRGWKMS(t *testing.T) {
	ctx := context.TODO()
	// Placeholder
	context := &clusterd.Context{Clientset: test.New(t, 3)}
	store := simpleStore()
	store.Spec.Security = &cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{}}}
	c := &clusterConfig{
		context: context,
		store:   store,
	}

	// without KMS
	b, err := c.CheckRGWKMS()
	assert.False(t, b)
	assert.NoError(t, err)

	// setting KMS configurations
	c.store.Spec.Security.KeyManagementService.TokenSecretName = "vault-token"
	c.store.Spec.Security.KeyManagementService.ConnectionDetails["KMS_PROVIDER"] = "vault"
	c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_ADDR"] = "https://1.1.1.1:8200"
	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      store.Spec.Security.KeyManagementService.TokenSecretName,
			Namespace: store.Namespace,
		},
		Data: map[string][]byte{
			"token": []byte("myt-otkenbenvqrev"),
		},
	}
	_, err = context.Clientset.CoreV1().Secrets(store.Namespace).Create(ctx, s, metav1.CreateOptions{})
	assert.NoError(t, err)

	// no secret engine set, will fail
	b, err = c.CheckRGWKMS()
	assert.False(t, b)
	assert.Error(t, err)

	// kv engine version v1, will fail
	c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "kv"
	c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_BACKEND"] = "v1"
	b, err = c.CheckRGWKMS()
	assert.False(t, b)
	assert.Error(t, err)

	// kv engine version v2, will pass
	c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_BACKEND"] = "v2"
	b, err = c.CheckRGWKMS()
	assert.True(t, b)
	assert.NoError(t, err)

	// transit engine, will pass
	c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
	c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_BACKEND"] = ""
	b, err = c.CheckRGWKMS()
	assert.True(t, b)
	assert.NoError(t, err)
}
