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
	_ "embed"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func configureSSE(t *testing.T, c *clusterConfig, kms bool, s3 bool) {
	c.store.Spec.Security = &cephv1.ObjectStoreSecuritySpec{}
	if kms {
		c.store.Spec.Security.KeyManagementService = cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{}}
		c.store.Spec.Security.KeyManagementService.TokenSecretName = "vault-token"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["KMS_PROVIDER"] = "vault"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_ADDR"] = "https://1.1.1.1:8200"
	}
	if s3 {
		c.store.Spec.Security.ServerSideEncryptionS3 = cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{}}
		c.store.Spec.Security.ServerSideEncryptionS3.TokenSecretName = "vault-token"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["KMS_PROVIDER"] = "vault"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_ADDR"] = "https://1.1.1.1:8200"
	}
	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-token",
			Namespace: c.store.Namespace,
		},
		Data: map[string][]byte{
			"token": []byte("myt-otkenbenvqrev"),
		},
	}
	_, err := c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Create(c.clusterInfo.Context, s, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		assert.Error(t, err)
	}
}

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
	info.CephVersion = cephver.Squid
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")

	executorFunc := func(command string, args ...string) (string, error) {
		idResponse := `{"id":"test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return idResponse, nil
	}
	executorFuncTimeout := func(timeout time.Duration, command string, args ...string) (string, error) {
		testResponse := `{"id": "test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return testResponse, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput:         executorFunc,
		MockExecuteCommandWithCombinedOutput: executorFunc,
		MockExecuteCommandWithTimeout:        executorFuncTimeout,
	}

	c := &clusterConfig{
		context:     &clusterd.Context{Executor: executor},
		clusterInfo: info,
		store:       store,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v15"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
			DataDirHostPath: "/var/lib/rook",
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
		DaemonID:     "default",
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
		"my-priority-class", "default", "cephobjectstores.ceph.rook.io", "ceph-rgw")

	t.Run(("check rgw ConfigureProbe"), func(t *testing.T) {
		c.store.Spec.HealthCheck.StartupProbe = &cephv1.ProbeSpec{Disabled: false, Probe: &v1.Probe{InitialDelaySeconds: 1000}}
		deployment, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.NotNil(t, deployment.StartupProbe)                                 // YES startup probe
		assert.Equal(t, int32(1000), deployment.StartupProbe.InitialDelaySeconds) //  ^ and modified
		assert.NotNil(t, deployment.ReadinessProbe)                               // YES readiness probe
		assert.Nil(t, deployment.LivenessProbe)                                   // NO liveness probe
	})
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
	info.CephVersion = cephver.Squid
	info.Namespace = store.Namespace
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")
	store.Spec.Gateway.SecurePort = 443
	executorFunc := func(command string, args ...string) (string, error) {
		idResponse := `{"id":"test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return idResponse, nil
	}
	executorFuncTimeout := func(timeout time.Duration, command string, args ...string) (string, error) {
		testResponse := `{"id": "test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return testResponse, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput:         executorFunc,
		MockExecuteCommandWithCombinedOutput: executorFunc,
		MockExecuteCommandWithTimeout:        executorFuncTimeout,
	}
	context.Executor = executor

	c := &clusterConfig{
		clusterInfo: info,
		store:       store,
		context:     context,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v19"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
			DataDirHostPath: "/var/lib/rook",
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
		DaemonID:     "default",
	}
	_, err := c.makeRGWPodSpec(rgwConfig)
	// No TLS certs specified, will return error
	assert.Error(t, err)

	// Using SSLCertificateRef

	c.store.Spec.Gateway.SSLCertificateRef = "bogusCert"
	// Secret missing, will return error
	_, err = c.makeRGWPodSpec(rgwConfig)
	assert.Error(t, err)

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
		"my-priority-class", "default", "cephobjectstores.ceph.rook.io", "ceph-rgw")
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
		"my-priority-class", "default", "cephobjectstores.ceph.rook.io", "ceph-rgw")
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
		"my-priority-class", "default", "cephobjectstores.ceph.rook.io", "ceph-rgw")

	assert.True(t, s.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, s.Spec.DNSPolicy)

	// add user-defined volume mount
	c.store.Spec.Gateway.AdditionalVolumeMounts = cephv1.AdditionalVolumeMounts{
		{
			SubPath: "ldap",
			VolumeSource: &cephv1.ConfigFileVolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: "my-rgw-ldap-secret",
				},
			},
		},
	}
	s, err = c.makeRGWPodSpec(rgwConfig)
	assert.NoError(t, err)
	podTemplate = cephtest.NewPodTemplateSpecTester(t, &s) // checks that vols have corresponding mounts
	podTemplate.RunFullSuite(cephconfig.RgwType, "default", "rook-ceph-rgw", "mycluster", "quay.io/ceph/ceph:myversion",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class", "default", "cephobjectstores.ceph.rook.io", "ceph-rgw")
	assert.True(t, hasSecretVolWithName(s.Spec.Volumes, "my-rgw-ldap-secret"))
}

func hasSecretVolWithName(vols []v1.Volume, secretName string) bool {
	for _, v := range vols {
		if v.Secret != nil && v.Secret.SecretName == secretName {
			return true
		}
	}
	return false
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
	s.Spec.Gateway.ExternalRgwEndpoints = []cephv1.EndpointAddress{
		{
			IP: "192.168.0.1",
		},
	}
	err = r.validateStore(s)
	assert.Nil(t, err)
}

func TestDefaultProbes(t *testing.T) {
	type storeSpec struct {
		Port        int32
		SecurePort  int32
		HostNetwork bool
	}
	type want struct {
		Port     int32
		Protocol ProtocolType
	}
	type test struct {
		probeType   ProbeType
		name        string
		environment storeSpec
		want        want
	}

	tests := []test{}
	// run the same tests for each probe type
	// matrix:
	//   - internal vs host net
	//   - HTTP vs HTTPS vs both
	for _, typ := range []ProbeType{ReadinessProbeType, StartupProbeType} {
		tests = append(tests, []test{
			{
				typ, "internal HTTP",
				storeSpec{Port: 80},
				want{Port: 8080, Protocol: HTTPProtocol},
			},
			{
				typ, "host net HTTP",
				storeSpec{Port: 8088, HostNetwork: true},
				want{Port: 8088, Protocol: HTTPProtocol},
			},
			{
				typ, "internal HTTPS",
				storeSpec{SecurePort: 8443},
				want{Port: 8443, Protocol: HTTPSProtocol},
			},
			{
				typ, "host net HTTPS",
				storeSpec{SecurePort: 10443, HostNetwork: true},
				want{Port: 10443, Protocol: HTTPSProtocol},
			},
			{
				typ, "internal HTTP+HTTPS",
				storeSpec{Port: 80, SecurePort: 443},
				want{Port: 8080, Protocol: HTTPProtocol},
			}, // uses HTTP
			{
				typ, "host net HTTP+HTTPS",
				storeSpec{Port: 10080, SecurePort: 10443, HostNetwork: true},
				want{Port: 10080, Protocol: HTTPProtocol},
			}, // uses HTTP
		}...)
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s with %s", tt.probeType, tt.name), func(t *testing.T) {
			store := simpleStore()
			store.Spec.Gateway.Port = tt.environment.Port
			store.Spec.Gateway.SecurePort = tt.environment.SecurePort
			store.Spec.Gateway.SSLCertificateRef = "foo"
			c := &clusterConfig{
				store: store,
				clusterSpec: &cephv1.ClusterSpec{
					Network: cephv1.NetworkSpec{
						HostNetwork: tt.environment.HostNetwork,
					},
				},
			}

			var got *v1.Probe
			var err error
			switch tt.probeType {
			case StartupProbeType:
				got, err = c.defaultStartupProbe()
			case ReadinessProbeType:
				got, err = c.defaultReadinessProbe()
			default:
				panic(fmt.Sprintf("unsupported probe type %q", tt.probeType))
			}

			assert.NoError(t, err)

			// must be an exec command, called by bash; then validate core parts of the script.
			// no need to validate start/period/timeout timings b/c those are business logic things
			// that affect runtime stability & best verified by ci
			assert.NotNil(t, got.Exec)
			cmd := got.Exec.Command
			assert.Equal(t, "bash", cmd[0])
			assert.Equal(t, "-c", cmd[1])
			script := cmd[2]

			// test that script config env vars are set up correctly
			assert.Contains(t, script, fmt.Sprintf(`PROBE_TYPE="%s"`, tt.probeType))
			assert.Contains(t, script, fmt.Sprintf(`PROBE_PORT="%d"`, tt.want.Port))
			assert.Contains(t, script, fmt.Sprintf(`PROBE_PROTOCOL="%s"`, tt.want.Protocol))
		})
	}
}

func TestCheckRGWKMS(t *testing.T) {
	ctx := context.TODO()
	setupTest := func() *clusterConfig {
		context := &clusterd.Context{Clientset: test.New(t, 3)}
		store := simpleStore()
		return &clusterConfig{
			context:     context,
			store:       store,
			clusterInfo: &client.ClusterInfo{Context: ctx},
		}
	}

	t.Run("KMS is disabled", func(t *testing.T) {
		c := setupTest()
		b, err := c.CheckRGWKMS()
		assert.False(t, b)
		assert.NoError(t, err)
	})

	t.Run("Vault Secret Engine is missing", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, true, false)
		b, err := c.CheckRGWKMS()
		assert.False(t, b)
		assert.Error(t, err)
	})

	t.Run("Vault Secret Engine is kv with v1", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, true, false)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "kv"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_BACKEND"] = "v1"
		b, err := c.CheckRGWKMS()
		assert.False(t, b)
		assert.Error(t, err)
	})

	t.Run("Vault Secret Engine is kv with v2", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, true, false)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "kv"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_BACKEND"] = "v2"
		b, err := c.CheckRGWKMS()
		assert.True(t, b)
		assert.NoError(t, err)
	})

	t.Run("Vault Secret Engine is transit", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, true, false)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		b, err := c.CheckRGWKMS()
		assert.True(t, b)
		assert.NoError(t, err)
	})

	t.Run("TLS is configured but secrets do not exist", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, true, false)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		b, err := c.CheckRGWKMS()
		assert.False(t, b)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection details k8s secret \"vault-ca-secret\": secrets \"vault-ca-secret\" not found")
	})

	t.Run("TLS secret exists but empty key", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, true, false)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		tlsSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vault-ca-secret",
				Namespace: c.store.Namespace,
			},
		}
		_, err := c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Create(context.TODO(), tlsSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		b, err := c.CheckRGWKMS()
		assert.False(t, b)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection key \"cert\" for \"VAULT_CACERT\" in k8s secret \"vault-ca-secret\"")
	})

	t.Run("TLS config is valid", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, true, false)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		tlsSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vault-ca-secret",
				Namespace: c.store.Namespace,
			},
		}
		tlsSecret.Data = map[string][]byte{"cert": []byte("envnrevbnbvsbjkrtn")}
		_, err := c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Create(context.TODO(), tlsSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		b, err := c.CheckRGWKMS()
		assert.True(t, b)
		assert.NoError(t, err, "")
	})
}

func TestCheckRGWSSES3Enabled(t *testing.T) {
	ctx := context.TODO()
	setupTest := func() *clusterConfig {
		context := &clusterd.Context{Clientset: test.New(t, 3)}
		store := simpleStore()
		return &clusterConfig{
			context:     context,
			store:       store,
			clusterInfo: &client.ClusterInfo{Context: ctx, CephVersion: cephver.CephVersion{Major: 17, Minor: 2, Extra: 3}},
			clusterSpec: &cephv1.ClusterSpec{
				CephVersion:     cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v19"},
				DataDirHostPath: "/var/lib/rook",
			},
		}
	}

	t.Run("KMS is disabled", func(t *testing.T) {
		c := setupTest()
		b, err := c.CheckRGWSSES3Enabled()
		assert.False(t, b)
		assert.NoError(t, err)
	})

	t.Run("Vault Secret Engine is missing", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, false, true)
		b, err := c.CheckRGWSSES3Enabled()
		assert.False(t, b)
		assert.Error(t, err)
	})

	t.Run("Vault Secret Engine is kv with v1", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, false, true)
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "kv"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_BACKEND"] = "v1"
		b, err := c.CheckRGWSSES3Enabled()
		assert.False(t, b)
		assert.Error(t, err)
	})

	t.Run("Vault Secret Engine is kv with v2", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, false, true)
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "kv"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_BACKEND"] = "v2"
		b, err := c.CheckRGWSSES3Enabled()
		assert.False(t, b)
		assert.Error(t, err)
	})

	t.Run("Vault Secret Engine is transit", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, false, true)
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		b, err := c.CheckRGWSSES3Enabled()
		assert.True(t, b)
		assert.NoError(t, err)
	})

	t.Run("TLS is configured but secrets do not exist", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, false, true)
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		b, err := c.CheckRGWSSES3Enabled()
		assert.False(t, b)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection details k8s secret \"vault-ca-secret\": secrets \"vault-ca-secret\" not found")
	})

	t.Run("TLS secret exists but empty key", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, false, true)
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		tlsSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vault-ca-secret",
				Namespace: c.store.Namespace,
			},
		}
		_, err := c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Create(context.TODO(), tlsSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		b, err := c.CheckRGWSSES3Enabled()
		assert.False(t, b)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection key \"cert\" for \"VAULT_CACERT\" in k8s secret \"vault-ca-secret\"")
	})

	t.Run("TLS config is valid", func(t *testing.T) {
		c := setupTest()
		configureSSE(t, c, false, true)
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		tlsSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vault-ca-secret",
				Namespace: c.store.Namespace,
			},
		}
		tlsSecret.Data = map[string][]byte{"cert": []byte("envnrevbnbvsbjkrtn")}
		_, err := c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Create(context.TODO(), tlsSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		b, err := c.CheckRGWSSES3Enabled()
		assert.True(t, b)
		assert.NoError(t, err, "")
	})
}

func TestGetDaemonName(t *testing.T) {
	context := &clusterd.Context{Clientset: test.New(t, 3)}
	store := simpleStore()
	tests := []struct {
		storeName      string
		testDaemonName string
		daemonID       string
	}{
		{"default", "ceph-client.rgw.default.a", "a"},
		{"my-store", "ceph-client.rgw.my.store.b", "b"},
	}
	for _, tt := range tests {
		t.Run(tt.storeName, func(t *testing.T) {
			c := &clusterConfig{
				context: context,
				store:   store,
			}
			c.store.Name = tt.storeName
			daemonName := fmt.Sprintf("%s-%s", c.store.Name, tt.daemonID)
			resourceName := fmt.Sprintf("%s-%s", AppName, daemonName)
			rgwconfig := &rgwConfig{
				ResourceName: resourceName,
				DaemonID:     daemonName,
			}
			daemon := getDaemonName(rgwconfig)
			assert.Equal(t, tt.testDaemonName, daemon)
		})
	}
}

func TestMakeRGWPodSpec(t *testing.T) {
	store := simpleStore()
	info := clienttest.CreateTestClusterInfo(1)
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")
	executorFunc := func(command string, args ...string) (string, error) {
		idResponse := `{"id":"test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return idResponse, nil
	}
	executorFuncTimeout := func(timeout time.Duration, command string, args ...string) (string, error) {
		testResponse := `{"id": "test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return testResponse, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput:         executorFunc,
		MockExecuteCommandWithCombinedOutput: executorFunc,
		MockExecuteCommandWithTimeout:        executorFuncTimeout,
	}
	c := &clusterConfig{
		context:     &clusterd.Context{Executor: executor},
		store:       store,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v15"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
		},
		clusterInfo: info,
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
		DaemonID:     "default",
	}
	tests := []struct {
		name    string
		isSet   bool
		enabled bool
	}{
		{"host networking is not set to rgw daemon", false, false},
		{"host networking is set to rgw daemon but not enabled", true, false},
		{"host networking is enabled for the rgw daemon", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isSet {
				if tt.enabled {
					c.store.Spec.Gateway.HostNetwork = func(hostNetwork bool) *bool { return &hostNetwork }(true)
				} else {
					c.store.Spec.Gateway.HostNetwork = new(bool)
				}
			}
			podTemplateSpec, _ := c.makeRGWPodSpec(rgwConfig)

			if tt.isSet {
				assert.Equal(t, *c.store.Spec.Gateway.HostNetwork, podTemplateSpec.Spec.HostNetwork)
			} else {
				assert.Equal(t, c.clusterSpec.Network.IsHost(), podTemplateSpec.Spec.HostNetwork)
			}
		})
	}
}

func TestAWSServerSideEncryption(t *testing.T) {
	ctx := context.TODO()
	// Placeholder
	context := &clusterd.Context{Clientset: test.New(t, 3)}

	store := simpleStore()
	info := clienttest.CreateTestClusterInfo(1)
	info.CephVersion = cephver.CephVersion{Major: 17, Minor: 2, Extra: 3}
	info.Namespace = store.Namespace
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")
	executorFunc := func(command string, args ...string) (string, error) {
		idResponse := `{"id":"test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return idResponse, nil
	}
	executorFuncTimeout := func(timeout time.Duration, command string, args ...string) (string, error) {
		testResponse := `{"id": "test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return testResponse, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput:         executorFunc,
		MockExecuteCommandWithCombinedOutput: executorFunc,
		MockExecuteCommandWithTimeout:        executorFuncTimeout,
	}
	context.Executor = executor

	c := &clusterConfig{
		clusterInfo: info,
		store:       store,
		context:     context,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v19.3"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
		DaemonID:     "default",
	}
	checkRGWOptions := func(allRGWOptions, optionsNeeded []string) bool {
		if len(optionsNeeded) == 0 {
			return false
		}
		optionMap := make(map[string]bool)
		for _, option := range allRGWOptions {
			optionMap[option] = true
		}

		for _, option := range optionsNeeded {
			if _, found := optionMap[option]; !found {
				return false
			}
		}

		return true
	}

	t.Run("SecuritySpec is not configured no options will be configured", func(t *testing.T) {
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSDefaultOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3DefaultOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTokenOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTokenOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTLSOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTLSOptions(false)))
	})

	configureSSE(t, c, true, false)
	t.Run("Invalid Security Spec configured, no options will be configured", func(t *testing.T) {
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.Error(t, err)
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSDefaultOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3DefaultOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTokenOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTokenOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTLSOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTLSOptions(false)))
	})

	t.Run("Security Spec configured with kms only,so kms options will be configured", func(t *testing.T) {
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSDefaultOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3DefaultOptions(false)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTokenOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTokenOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTLSOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTLSOptions(false)))
	})

	t.Run("Security Spec configured with s3 only, so s3 options will be configured", func(t *testing.T) {
		configureSSE(t, c, false, true)
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSDefaultOptions(false)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3DefaultOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTokenOptions(false)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTokenOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTLSOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTLSOptions(false)))
	})

	t.Run("Security Spec configured with both kms and s3 settings, so both kms and s3 options will be configured", func(t *testing.T) {
		configureSSE(t, c, true, true)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSDefaultOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3DefaultOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTokenOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTokenOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTLSOptions(false)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTLSOptions(false)))
	})

	tlsSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-ca-secret",
			Namespace: c.store.Namespace,
		},
	}
	tlsSecret.Data = map[string][]byte{"cert": []byte("envnrevbnbvsbjkrtn")}
	_, err := c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Create(ctx, tlsSecret, metav1.CreateOptions{})
	assert.NoError(t, err)

	t.Run("Security Spec configured with kms along with TLS, so kms including TLS options will be configured", func(t *testing.T) {
		configureSSE(t, c, true, false)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSDefaultOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3DefaultOptions(false)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTokenOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTokenOptions(false)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTLSOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTLSOptions(false)))
	})

	t.Run("Security Spec configured with s3 along with TLS, so s3 including TLS options will be configured", func(t *testing.T) {
		configureSSE(t, c, false, true)
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSDefaultOptions(false)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3DefaultOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTokenOptions(false)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTokenOptions(true)))
		assert.False(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTLSOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTLSOptions(true)))
	})

	t.Run("Security Spec configured both kms and s3 aloong with TLS, all the serverr side encryption related options will be configured", func(t *testing.T) {
		configureSSE(t, c, true, true)
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_SECRET_ENGINE"] = "transit"
		c.store.Spec.Security.KeyManagementService.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSDefaultOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3DefaultOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTokenOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTokenOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseKMSVaultTLSOptions(true)))
		assert.True(t, checkRGWOptions(rgwContainer.Args, c.sseS3VaultTLSOptions(true)))
	})
}

func TestRgwCommandFlags(t *testing.T) {
	// ctx := context.TODO()
	// Placeholder
	context := &clusterd.Context{Clientset: test.New(t, 3)}

	store := simpleStore()
	info := clienttest.CreateTestClusterInfo(1)
	info.CephVersion = cephver.CephVersion{Major: 18, Minor: 2, Extra: 0}
	info.Namespace = store.Namespace
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")
	executorFunc := func(command string, args ...string) (string, error) {
		idResponse := `{"id":"test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return idResponse, nil
	}
	executorFuncTimeout := func(timeout time.Duration, command string, args ...string) (string, error) {
		testResponse := `{"id": "test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return testResponse, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput:         executorFunc,
		MockExecuteCommandWithCombinedOutput: executorFunc,
		MockExecuteCommandWithTimeout:        executorFuncTimeout,
	}
	context.Executor = executor

	c := &clusterConfig{
		clusterInfo: info,
		store:       store,
		context:     context,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v19.3"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
		DaemonID:     "default",
	}

	rgwContainer, err := c.makeDaemonContainer(rgwConfig)
	assert.NoError(t, err)
	baseArgs := rgwContainer.Args // args without rgwCommandFlags set

	t.Run("empty map adds no flags", func(t *testing.T) {
		c.store.Spec.Gateway.RgwCommandFlags = map[string]string{}
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		assert.Equal(t, baseArgs, rgwContainer.Args)
	})

	t.Run("add flag with space", func(t *testing.T) {
		c.store.Spec.Gateway.RgwCommandFlags = map[string]string{"rgw option": "val spaced"}
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		l := len(rgwContainer.Args)
		assert.Equal(t, "--rgw-option=val spaced", rgwContainer.Args[l-1])
	})

	t.Run("add flag with dashes", func(t *testing.T) {
		c.store.Spec.Gateway.RgwCommandFlags = map[string]string{"rgw-option": "val-dashed"}
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		l := len(rgwContainer.Args)
		assert.Equal(t, "--rgw-option=val-dashed", rgwContainer.Args[l-1])
	})

	t.Run("add flag with underscores", func(t *testing.T) {
		c.store.Spec.Gateway.RgwCommandFlags = map[string]string{"rgw_option": "val_underscored"}
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		l := len(rgwContainer.Args)
		assert.Equal(t, "--rgw-option=val_underscored", rgwContainer.Args[l-1])
	})

	t.Run("add all flags", func(t *testing.T) {
		c.store.Spec.Gateway.RgwCommandFlags = map[string]string{
			"rgw option": "val spaced",
			"rgw-option": "val-dashed",
			"rgw_option": "val_underscored",
		}
		rgwContainer, err := c.makeDaemonContainer(rgwConfig)
		assert.NoError(t, err)
		// ordering of additional args amongst themselves doesn't matter (and is random b/c of
		// underlying map data type), but it is still important that the additional args are always
		// after the args Rook normally applies
		l := len(rgwContainer.Args)
		addedArgs := rgwContainer.Args[l-3:]
		assert.ElementsMatch(t, []string{"--rgw-option=val spaced", "--rgw-option=val-dashed", "--rgw-option=val_underscored"}, addedArgs)
	})
}

func TestAddDNSNamesToRGWPodSpec(t *testing.T) {
	setupTest := func(zoneName string, cephvers cephver.CephVersion, dnsNames, customEndpoints []string) *clusterConfig {
		store := simpleStore()
		info := clienttest.CreateTestClusterInfo(1)
		data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")
		ctx := &clusterd.Context{RookClientset: rookclient.NewSimpleClientset()}
		info.CephVersion = cephvers
		store.Spec.Hosting = &cephv1.ObjectStoreHostingSpec{
			DNSNames: dnsNames,
		}
		if zoneName != "" {
			store.Spec.Zone.Name = zoneName
			zone := &cephv1.CephObjectZone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      zoneName,
					Namespace: store.Namespace,
				},
				Spec: cephv1.ObjectZoneSpec{},
			}
			if len(customEndpoints) > 0 {
				zone.Spec.CustomEndpoints = customEndpoints
			}
			_, err := ctx.RookClientset.CephV1().CephObjectZones(store.Namespace).Create(context.TODO(), zone, metav1.CreateOptions{})
			assert.NoError(t, err)
		}
		return &clusterConfig{
			clusterInfo: info,
			store:       store,
			context:     ctx,
			rookVersion: "rook/rook:myversion",
			clusterSpec: &cephv1.ClusterSpec{
				CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v18"},
			},
			DataPathMap: data,
		}
	}

	cephV18 := cephver.CephVersion{Major: 18, Minor: 0, Extra: 0}

	tests := []struct {
		name            string
		dnsNames        []string
		expectedDNSArg  string
		cephvers        cephver.CephVersion
		zoneName        string
		CustomEndpoints []string
		wantErr         bool
	}{
		{"no dns names ceph v18", []string{}, "", cephV18, "", []string{}, false},
		{"no dns names with zone ceph v18", []string{}, "", cephV18, "myzone", []string{}, false},
		{"no dns names with zone and custom endpoints ceph v18", []string{}, "", cephV18, "myzone", []string{"http://my.custom.endpoint1:80", "http://my.custom.endpoint2:80"}, false},
		{"one dns name ceph v18", []string{"my.dns.name"}, "--rgw-dns-name=my.dns.name,rook-ceph-rgw-default.mycluster.svc", cephV18, "", []string{}, false},
		{"multiple dns names ceph v18", []string{"my.dns.name1", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc", cephV18, "", []string{}, false},
		{"duplicate dns names ceph v18", []string{"my.dns.name1", "my.dns.name2", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc", cephV18, "", []string{}, false},
		{"invalid dns name ceph v18", []string{"!my.invalid-dns.com"}, "", cephV18, "", []string{}, true},
		{"mixed invalid and valid dns names ceph v18", []string{"my.dns.name", "!my.invalid-dns.name"}, "", cephV18, "", []string{}, true},
		{"dns name with zone without custom endpoints ceph v18", []string{"my.dns.name1", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc", cephV18, "myzone", []string{}, false},
		{"dns name with zone with custom endpoints ceph v18", []string{"my.dns.name1", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc,my.custom.endpoint1,my.custom.endpoint2", cephV18, "myzone", []string{"http://my.custom.endpoint1:80", "http://my.custom.endpoint2:80"}, false},
		{"dns name with zone with custom invalid endpoints ceph v18", []string{"my.dns.name1", "my.dns.name2"}, "", cephV18, "myzone", []string{"http://my.custom.endpoint:80", "http://!my.invalid-custom.endpoint:80"}, true},
		{"dns name with zone with mixed invalid and valid dnsnames/custom endpoint ceph v18", []string{"my.dns.name", "!my.dns.name"}, "", cephV18, "myzone", []string{"http://my.custom.endpoint1:80", "http://my.custom.endpoint2:80:80"}, true},
		{"no dns names ceph v19", []string{}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, false},
		{"one dns name ceph v19", []string{"my.dns.name"}, "--rgw-dns-name=my.dns.name,rook-ceph-rgw-default.mycluster.svc", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, false},
		{"multiple dns names ceph v19", []string{"my.dns.name1", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, false},
		{"duplicate dns names ceph v19", []string{"my.dns.name1", "my.dns.name2", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, false},
		{"invalid dns name ceph v19", []string{"!my.invalid-dns.name"}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, true},
		{"mixed invalid and valid dns names ceph v19", []string{"my.dns.name", "!my.invalid-dns.name"}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, true},
		{"no dns names ceph v19", []string{}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, false},
		{"no dns names with zone ceph v19", []string{}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "myzone", []string{}, false},
		{"no dns names with zone and custom endpoints ceph v19", []string{}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "myzone", []string{"http://my.custom.endpoint1:80", "http://my.custom.endpoint2:80"}, false},
		{"one dns name ceph v19", []string{"my.dns.name"}, "--rgw-dns-name=my.dns.name,rook-ceph-rgw-default.mycluster.svc", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, false},
		{"multiple dns names ceph v19", []string{"my.dns.name1", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, false},
		{"duplicate dns names ceph v19", []string{"my.dns.name1", "my.dns.name2", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, false},
		{"invalid dns name ceph v19", []string{"!my.invalid-dns.name"}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, true},
		{"mixed invalid and valid dns names ceph v19", []string{"my.dns.name", "!my.invalid-dns.name"}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "", []string{}, true},
		{"dns name with zone without custom endpoints ceph v19", []string{"my.dns.name1", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "myzone", []string{}, false},
		{"dns name with zone with custom endpoints ceph v19", []string{"my.dns.name1", "my.dns.name2"}, "--rgw-dns-name=my.dns.name1,my.dns.name2,rook-ceph-rgw-default.mycluster.svc,my.custom.endpoint1,my.custom.endpoint2", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "myzone", []string{"http://my.custom.endpoint1:80", "http://my.custom.endpoint2:80"}, false},
		{"dns name with zone with custom invalid endpoints ceph v19", []string{"my.dns.name1", "my.dns.name2"}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "myzone", []string{"http://my.custom.endpoint:80", "http://!my.custom.invalid-endpoint:80"}, true},
		{"dns with zone with mixed invalid and valid dnsnames/custom endpoint ceph v19", []string{"my.dns.name", "!my.invalid-dns.name"}, "", cephver.CephVersion{Major: 19, Minor: 0, Extra: 0}, "myzone", []string{"http://my.custom.endpoint1:80", "http://!my.custom.invalid-endpoint:80"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := setupTest(tt.zoneName, tt.cephvers, tt.dnsNames, tt.CustomEndpoints)
			res, err := c.addDNSNamesToRGWServer()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedDNSArg, res)
		})
	}

	t.Run("advertiseEndpoint http, no dnsNames", func(t *testing.T) {
		c := setupTest("", cephV18, []string{}, []string{})
		c.store.Spec.Hosting = &cephv1.ObjectStoreHostingSpec{
			AdvertiseEndpoint: &cephv1.ObjectEndpointSpec{
				DnsName: "my.endpoint.com",
				Port:    80,
			},
		}
		res, err := c.addDNSNamesToRGWServer()
		assert.NoError(t, err)
		assert.Equal(t, "--rgw-dns-name=my.endpoint.com,rook-ceph-rgw-default.mycluster.svc", res)
	})

	t.Run("advertiseEndpoint https, no dnsNames", func(t *testing.T) {
		c := setupTest("", cephV18, []string{}, []string{})
		c.store.Spec.Hosting = &cephv1.ObjectStoreHostingSpec{
			AdvertiseEndpoint: &cephv1.ObjectEndpointSpec{
				DnsName: "my.endpoint.com",
				Port:    443,
				UseTls:  true,
			},
		}
		res, err := c.addDNSNamesToRGWServer()
		assert.NoError(t, err)
		assert.Equal(t, "--rgw-dns-name=my.endpoint.com,rook-ceph-rgw-default.mycluster.svc", res)
	})

	t.Run("advertiseEndpoint is svc", func(t *testing.T) {
		c := setupTest("", cephV18, []string{}, []string{})
		c.store.Spec.Hosting = &cephv1.ObjectStoreHostingSpec{
			AdvertiseEndpoint: &cephv1.ObjectEndpointSpec{
				DnsName: "rook-ceph-rgw-default.mycluster.svc",
				Port:    443,
				UseTls:  true,
			},
		}
		res, err := c.addDNSNamesToRGWServer()
		assert.NoError(t, err)
		// ensures duplicates are removed
		assert.Equal(t, "--rgw-dns-name=rook-ceph-rgw-default.mycluster.svc", res)
	})

	t.Run("advertiseEndpoint https, no dnsNames, with zone custom endpoint", func(t *testing.T) {
		c := setupTest("my-zone", cephV18, []string{}, []string{"multisite.endpoint.com"})
		c.store.Spec.Hosting = &cephv1.ObjectStoreHostingSpec{
			AdvertiseEndpoint: &cephv1.ObjectEndpointSpec{
				DnsName: "my.endpoint.com",
				Port:    443,
				UseTls:  true,
			},
		}
		res, err := c.addDNSNamesToRGWServer()
		assert.NoError(t, err)
		assert.Equal(t, "--rgw-dns-name=my.endpoint.com,rook-ceph-rgw-default.mycluster.svc,multisite.endpoint.com", res)
	})

	t.Run("advertiseEndpoint https, with dnsNames, with zone custom endpoint", func(t *testing.T) {
		c := setupTest("my-zone", cephV18, []string{}, []string{"multisite.endpoint.com"})
		c.store.Spec.Hosting = &cephv1.ObjectStoreHostingSpec{
			AdvertiseEndpoint: &cephv1.ObjectEndpointSpec{
				DnsName: "my.endpoint.com",
				Port:    443,
				UseTls:  true,
			},
			DNSNames: []string{"extra.endpoint.com", "extra.endpoint.net"},
		}
		res, err := c.addDNSNamesToRGWServer()
		assert.NoError(t, err)
		assert.Equal(t, "--rgw-dns-name=my.endpoint.com,extra.endpoint.com,extra.endpoint.net,rook-ceph-rgw-default.mycluster.svc,multisite.endpoint.com", res)
	})

	t.Run("advertiseEndpoint https, with dnsNames, with zone custom endpoint, duplicates", func(t *testing.T) {
		c := setupTest("my-zone", cephV18, []string{}, []string{"extra.endpoint.com"})
		c.store.Spec.Hosting = &cephv1.ObjectStoreHostingSpec{
			AdvertiseEndpoint: &cephv1.ObjectEndpointSpec{
				DnsName: "my.endpoint.com",
				Port:    443,
				UseTls:  true,
			},
			DNSNames: []string{"my.endpoint.com", "extra.endpoint.com"},
		}
		res, err := c.addDNSNamesToRGWServer()
		assert.NoError(t, err)
		t.Log(res)
		assert.Equal(t, "--rgw-dns-name=my.endpoint.com,extra.endpoint.com,rook-ceph-rgw-default.mycluster.svc", res)
	})
}

func TestGetHostnameFromEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
		wantErr  bool
	}{
		{"empty endpoint", "", "", true},
		{"http endpoint", "http://my.custom.endpoint1:80", "my.custom.endpoint1", false},
		{"https endpoint", "https://my.custom.endpoint1:80", "my.custom.endpoint1", false},
		{"http endpoint without port", "http://my.custom.endpoint1", "my.custom.endpoint1", false},
		{"https endpoint without port", "https://my.custom.endpoint1", "my.custom.endpoint1", false},
		{"multiple portocol endpoint case 1", "http://https://my.custom.endpoint1:80", "", true},
		{"multiple portocol endpoint case 2", "https://http://my.custom.endpoint1:80", "", true},
		{"ftp endpoint", "ftp://my.custom.endpoint1:80", "my.custom.endpoint1", false},
		{"custom protocol endpoint", "custom://my.custom.endpoint1:80", "my.custom.endpoint1", false},
		{"custom protocol endpoint without port", "custom://my.custom.endpoint1", "my.custom.endpoint1", false},
		{"invalid endpoint", "http://!my.custom.endpoint1:80", "", true},
		{"invalid endpoint multiple ports", "http://my.custom.endpoint1:80:80", "", true},
		{"invalid http endpoint with upper case", "http://MY.CUSTOM.ENDPOINT1:80", "", true},
		{"invalid http endpoint with upper case without port", "http://MY.CUSTOM.ENDPOINT1", "", true},
		{"invalid https endpoint with upper case", "https://MY.CUSTOM.ENDPOINT1:80", "", true},
		{"invalid https endpoint with upper case without port", "https://MY.CUSTOM.ENDPOINT1", "", true},
		{"invalid http endpoint with camel case", "http://myCustomEndpoint1:80", "", true},
		{"invalid http endpoint with camel case without port", "http://myCustomEndpoint1", "", true},
		{"invalid https endpoint with camel case", "https://myCustomEndpoint1:80", "", true},
		{"invalid https endpoint with camel case without port", "https://myCustomEndpoint1", "", true},
		{"endpoint without protocol", "my.custom.endpoint1:80", "my.custom.endpoint1", false},
		{"endpoint without protocol without port", "my.custom.endpoint1", "my.custom.endpoint1", false},
		{"invalid endpoint without protocol with upper case", "MY.CUSTOM.ENDPOINT1:80", "", true},
		{"invalid endpoint without protocol with upper case without port", "MY.CUSTOM.ENDPOINT1", "", true},
		{"invalid endpoint without protocol with camel case", "myCustomEndpoint1:80", "", true},
		{"invalid endpoint without protocol with camel case without port", "myCustomEndpoint1", "", true},
		{"invalid endpoint without protocol ending :", "my.custom.endpoint1:", "", true},
		{"invalid endpoint without protocol and multiple ports", "my.custom.endpoint1:80:80", "", true},
		{"http endpoint containing ip address", "http://1.1.1.1:80", "1.1.1.1", false},
		{"http endpoint containing ip address without port", "http://1.1.1.1", "1.1.1.1", false},
		{"https endpoint containing ip address", "https://1.1.1.1:80", "1.1.1.1", false},
		{"https endpoint containing ip address without port", "https://1.1.1.1:80", "1.1.1.1", false},
		{"invalid endpoint ending ://", "my.custom.endpoint1://", "", true},
		{"invalid endpoint ending : without port", "http://my.custom.endpoint1:", "", true},
		{"http endpoint with user and password", "http://user:password@mycustomendpoint1:80", "mycustomendpoint1", false},
		{"http endpoint with user and password without port", "http://user:password@mycustomendpoint1", "mycustomendpoint1", false},
		{"https endpoint with user and password", "https://user:password@mycustomendpoint1:80", "mycustomendpoint1", false},
		{"https endpoint with user and password without port", "https://user:password@mycustomendpoint1", "mycustomendpoint1", false},
		{"endpoint with user and password without protocol", "user:password@mycustomendpoint:80", "mycustomendpoint", false},
		{"endpoint with user and password without protocol without port", "user:password@mycustomendpoint", "mycustomendpoint", false},
		{"invalid endpoint with user and password ending ://", "user:password@mycustomendpoint1://", "", true},
		{"invalid endpoint with user and password ending : without port", "user:password@mycustomendpoint1:", "", true},
		{"invalid endpoint with user and password ending  with multiple ports", "user:password@mycustomendpoint1:80:80", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := GetHostnameFromEndpoint(tt.endpoint)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, res)
		})
	}
}

func strPtr(in string) *string {
	return &in
}

func Test_buildRGWEnableAPIsConfigVal(t *testing.T) {
	type args struct {
		protocolSpec cephv1.ProtocolSpec
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "nothing set",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{},
			},
			want: nil,
		},
		{
			name: "only admin enabled",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						"admin",
					},
				},
			},
			want: []string{"admin"},
		},
		{
			name: "admin and s3 enabled",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						"admin",
						"s3",
					},
				},
			},
			want: []string{"admin", "s3"},
		},
		{
			name: "whitespaces trimmed",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						" s3 ",
						" swift ",
					},
				},
			},
			want: []string{"s3", "swift"},
		},
		{
			name: "s3 disabled",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					S3: &cephv1.S3Spec{
						Enabled: new(bool),
					},
				},
			},
			want: rgwAPIwithoutS3,
		},
		{
			name: "s3 disabled when swift prefix is '/'",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					Swift: &cephv1.SwiftSpec{
						UrlPrefix: strPtr("/"),
					},
				},
			},
			want: rgwAPIwithoutS3,
		},
		{
			name: "s3 is not disabled when swift prefix is not '/'",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					Swift: &cephv1.SwiftSpec{
						UrlPrefix: strPtr("object/"),
					},
				},
			},
			want: nil,
		},
		{
			name: "enableAPIs overrides s3 option",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						" s3 ",
						" swift ",
					},
					S3: &cephv1.S3Spec{
						Enabled: new(bool),
					},
				},
			},
			want: []string{"s3", "swift"},
		},
		{
			name: "enableAPIs overrides swift prefix option",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						" s3 ",
						" swift ",
					},
					Swift: &cephv1.SwiftSpec{
						UrlPrefix: strPtr("/"),
					},
				},
			},
			want: []string{"s3", "swift"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRGWEnableAPIsConfigVal(tt.args.protocolSpec)
			assert.ElementsMatch(t, got, tt.want)
		})
	}
}

func Test_buildRGWConfigFlags(t *testing.T) {
	type args struct {
		objectStore *cephv1.CephObjectStore
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "nothing set",
			args: args{
				objectStore: &cephv1.CephObjectStore{},
			},
			want: nil,
		},
		{
			name: "enabled APIs set",
			args: args{
				objectStore: &cephv1.CephObjectStore{
					Spec: cephv1.ObjectStoreSpec{
						Protocols: cephv1.ProtocolSpec{
							EnableAPIs: []cephv1.ObjectStoreAPI{
								"swift",
								"admin",
							},
						},
					},
				},
			},
			want: []string{
				"--rgw-enable-apis=swift,admin",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildRGWConfigFlags(tt.args.objectStore); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildRGWConfigFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getRGWProbePathAndCode(t *testing.T) {
	type args struct {
		protocolSpec cephv1.ProtocolSpec
	}
	tests := []struct {
		name        string
		args        args
		wantPath    string
		wantDisable bool
	}{
		{
			name: "all apis enabled - return s3 probe",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{},
					S3:         &cephv1.S3Spec{},
					Swift:      &cephv1.SwiftSpec{},
				},
			},
			wantPath:    "",
			wantDisable: false,
		},
		{
			name: "s3 disabled - return default swift probe",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{},
					S3: &cephv1.S3Spec{
						Enabled: new(bool),
					},
					Swift: &cephv1.SwiftSpec{},
				},
			},
			wantPath:    "/swift/info",
			wantDisable: false,
		},
		{
			name: "s3 disabled in api list - return default swift probe",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						"swift",
						"admin",
					},
				},
			},
			wantPath:    "/swift/info",
			wantDisable: false,
		},
		{
			name: "s3 disabled - return swift with custom prefix probe",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						"swift",
						"admin",
					},
					Swift: &cephv1.SwiftSpec{
						UrlPrefix: strPtr("some-path"),
					},
				},
			},
			wantPath:    "/some-path/info",
			wantDisable: false,
		},
		{
			name: "s3 disabled - return swift with root prefix probe",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					Swift: &cephv1.SwiftSpec{
						UrlPrefix: strPtr("/"),
					},
				},
			},
			wantPath:    "/info",
			wantDisable: false,
		},
		{
			name: "s3 and swift disabled - disable probe",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						"admin",
					},
				},
			},
			wantPath:    "",
			wantDisable: true,
		},
		{
			name: "no suitable api enabled - disable probe",
			args: args{
				protocolSpec: cephv1.ProtocolSpec{
					EnableAPIs: []cephv1.ObjectStoreAPI{
						"sts",
					},
				},
			},
			wantPath:    "",
			wantDisable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotDisable := getRGWProbePath(tt.args.protocolSpec)
			if gotPath != tt.wantPath {
				t.Errorf("getRGWProbePath() gotPath = %v, want %v", gotPath, tt.wantPath)
			}
			if gotDisable != tt.wantDisable {
				t.Errorf("getRGWProbePath() gotDisable = %v, want %v", gotDisable, tt.wantDisable)
			}
		})
	}
}

func TestRgwReadAffinity(t *testing.T) {
	context := &clusterd.Context{Clientset: test.New(t, 3)}

	store := simpleStore()
	info := clienttest.CreateTestClusterInfo(1)
	info.Namespace = store.Namespace
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "default", "rook-ceph", "/var/lib/rook/")

	executorFunc := func(command string, args ...string) (string, error) {
		idResponse := `{"id":"test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return idResponse, nil
	}
	executorFuncTimeout := func(timeout time.Duration, command string, args ...string) (string, error) {
		testResponse := `{"id": "test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return testResponse, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput:         executorFunc,
		MockExecuteCommandWithCombinedOutput: executorFunc,
		MockExecuteCommandWithTimeout:        executorFuncTimeout,
	}
	context.Executor = executor
	c := &clusterConfig{
		clusterInfo: info,
		store:       store,
		context:     context,
		rookVersion: "rook/rook:myversion",
		clusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v19.3"},
			Network: cephv1.NetworkSpec{
				HostNetwork: true,
			},
		},
		DataPathMap: data,
	}

	resourceName := fmt.Sprintf("%s-%s", AppName, c.store.Name)
	rgwConfig := &rgwConfig{
		ResourceName: resourceName,
		DaemonID:     "default",
	}

	tests := []struct {
		name                  string
		cephVersion           cephver.CephVersion
		readAffinity          string
		isReadAffinityArgSet  bool
		isCrushLocationArgSet bool
	}{
		{
			name:                  "ceph version is less than v.20",
			cephVersion:           cephver.CephVersion{Major: 17, Minor: 2, Extra: 3},
			readAffinity:          "localize",
			isReadAffinityArgSet:  false,
			isCrushLocationArgSet: false,
		},
		{
			name:                  "ceph version is v.20 and localized read affinity is set",
			cephVersion:           cephver.CephVersion{Major: 20, Minor: 0, Extra: 0},
			readAffinity:          "localize",
			isReadAffinityArgSet:  true,
			isCrushLocationArgSet: true,
		},
		{
			name:                  "ceph version is v.20 and balanced read affinity is set",
			cephVersion:           cephver.CephVersion{Major: 20, Minor: 0, Extra: 0},
			readAffinity:          "balance",
			isReadAffinityArgSet:  true,
			isCrushLocationArgSet: false,
		},
		{
			name:                  "ceph version is v.20 and default read affinity is set",
			cephVersion:           cephver.CephVersion{Major: 20, Minor: 0, Extra: 0},
			readAffinity:          "default",
			isReadAffinityArgSet:  true,
			isCrushLocationArgSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.clusterInfo.CephVersion = tt.cephVersion
			c.store.Spec.Gateway.ReadAffinity = &cephv1.RgwReadAffinity{Type: tt.readAffinity}
			container, err := c.makeDaemonContainer(rgwConfig)
			assert.NoError(t, err)
			assert.Equal(t, tt.isReadAffinityArgSet, slices.Contains(container.Args, fmt.Sprintf("--rados-replica-read-policy=%s", tt.readAffinity)))
			assert.Equal(t, tt.isCrushLocationArgSet, slices.Contains(container.Command, `exec radosgw --crush-location="host=${NODE_NAME//./-}" "$@"`))
		})
	}
}
