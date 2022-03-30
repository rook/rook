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

package kms

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/vault"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateConnectionDetails(t *testing.T) {
	ctx := context.TODO()
	// Placeholder
	clusterdContext := &clusterd.Context{Clientset: test.New(t, 3)}
	securitySpec := &cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{}}}
	ns := "rook-ceph"
	vaultSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-token",
			Namespace: ns,
		},
	}
	ibmSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibm-token",
			Namespace: ns,
		},
		Data: map[string][]byte{"foo": []byte("bar")},
	}
	tlsSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-ca-secret",
			Namespace: ns,
		},
	}
	ibmSecuritySpec := &cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{
		ConnectionDetails: map[string]string{
			"KMS_PROVIDER": TypeIBM,
		},
	}}

	t.Run("no kms provider given", func(t *testing.T) {
		err := ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms config \"KMS_PROVIDER\". cannot be empty")
		securitySpec.KeyManagementService.ConnectionDetails["KMS_PROVIDER"] = "vault"
	})

	t.Run("vault - no token object", func(t *testing.T) {
		securitySpec.KeyManagementService.TokenSecretName = "vault-token"
		err := ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to fetch kms token secret \"vault-token\": secrets \"vault-token\" not found")
	})

	t.Run("vault - token secret present but empty content", func(t *testing.T) {
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Create(ctx, vaultSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to read k8s kms secret \"token\" key \"vault-token\" (not found or empty)")
	})

	t.Run("vault - token key does not exist", func(t *testing.T) {
		vaultSecret.Data = map[string][]byte{"foo": []byte("bar")}
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, vaultSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to read k8s kms secret \"token\" key \"vault-token\" (not found or empty)")
	})

	// Success: token content is ok
	t.Run("vault - token content is ok", func(t *testing.T) {
		vaultSecret.Data["token"] = []byte("token")
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, vaultSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find connection details \"VAULT_ADDR\"")
		securitySpec.KeyManagementService.ConnectionDetails["VAULT_ADDR"] = "https://1.1.1.1:8200"
	})

	t.Run("vault - TLS is configured but secrets do not exist", func(t *testing.T) {
		securitySpec.KeyManagementService.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		err := ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection details k8s secret \"vault-ca-secret\"")
	})

	t.Run("vault - TLS secret exists but empty key", func(t *testing.T) {
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Create(ctx, tlsSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection key \"cert\" for \"VAULT_CACERT\" in k8s secret \"vault-ca-secret\"")
	})

	t.Run("vault - success TLS config is correct", func(t *testing.T) {
		tlsSecret.Data = map[string][]byte{"cert": []byte("envnrevbnbvsbjkrtn")}
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, tlsSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.NoError(t, err, "")
	})

	// test with vault server
	t.Run("success - auto detect kv version and set it", func(t *testing.T) {
		cluster := fakeVaultServer(t)
		cluster.Start()
		defer cluster.Cleanup()
		core := cluster.Cores[0].Core
		vault.TestWaitActive(t, core)
		client := cluster.Cores[0].Client
		// Mock the client here
		vaultClient = func(ctx context.Context, clusterdContext *clusterd.Context, namespace string, secretConfig map[string]string) (*api.Client, error) {
			return client, nil
		}
		if err := client.Sys().Mount("rook/", &api.MountInput{
			Type:    "kv-v2",
			Options: map[string]string{"version": "2"},
		}); err != nil {
			t.Fatal(err)
		}
		securitySpec := &cephv1.SecuritySpec{
			KeyManagementService: cephv1.KeyManagementServiceSpec{
				ConnectionDetails: map[string]string{
					"VAULT_SECRET_ENGINE": "kv",
					"KMS_PROVIDER":        "vault",
					"VAULT_ADDR":          client.Address(),
					"VAULT_BACKEND_PATH":  "rook",
				},
				TokenSecretName: "vault-token",
			},
		}
		err := ValidateConnectionDetails(ctx, clusterdContext, securitySpec, ns)
		assert.NoError(t, err, "")
		assert.Equal(t, securitySpec.KeyManagementService.ConnectionDetails["VAULT_BACKEND"], "v2")
	})

	t.Run("ibm kp - fail no token specified, only token is supported", func(t *testing.T) {
		err := ValidateConnectionDetails(ctx, clusterdContext, ibmSecuritySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms configuration (missing token in spec)")
		ibmSecuritySpec.KeyManagementService.TokenSecretName = "ibm-token"

	})

	t.Run("ibm kp - token present but no key for service key", func(t *testing.T) {
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Create(ctx, ibmSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, ibmSecuritySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to read k8s kms secret \"IBM_KP_SERVICE_API_KEY\" key \"ibm-token\" (not found or empty)")
	})

	t.Run("ibm kp - token ok but no instance id", func(t *testing.T) {
		ibmSecret.Data["IBM_KP_SERVICE_API_KEY"] = []byte("foo")
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, ibmSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, ibmSecuritySpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms config \"IBM_KP_SERVICE_INSTANCE_ID\". cannot be empty")
		ibmSecuritySpec.KeyManagementService.ConnectionDetails["IBM_KP_SERVICE_INSTANCE_ID"] = "foo"
	})

	t.Run("ibm kp - success", func(t *testing.T) {
		err := ValidateConnectionDetails(ctx, clusterdContext, ibmSecuritySpec, ns)
		assert.NoError(t, err, "")
		// IBM_KP_SERVICE_API_KEY must be appended to the details so that the client can be built with
		// all the details
		assert.Equal(t, ibmSecuritySpec.KeyManagementService.ConnectionDetails["IBM_KP_SERVICE_API_KEY"], "foo")
	})
}

func TestSetTokenToEnvVar(t *testing.T) {
	ctx := context.TODO()
	context := &clusterd.Context{Clientset: test.New(t, 3)}
	secretName := "vault-secret"
	ns := "rook-ceph"
	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns,
		},
		Data: map[string][]byte{"token": []byte("toto")},
	}
	_, err := context.Clientset.CoreV1().Secrets(ns).Create(ctx, s, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = SetTokenToEnvVar(ctx, context, secretName, "vault", ns)
	assert.NoError(t, err)
	assert.Equal(t, os.Getenv("VAULT_TOKEN"), "toto")
	os.Unsetenv("VAULT_TOKEN")
}
