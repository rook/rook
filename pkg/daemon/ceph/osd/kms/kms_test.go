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
	context := &clusterd.Context{Clientset: test.New(t, 3)}
	securitySpec := &cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{}}}
	ns := "rook-ceph"

	// Error: no token in spec
	err := ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.Error(t, err, "")
	assert.EqualError(t, err, "failed to validate kms configuration (missing token in spec)")

	securitySpec.KeyManagementService.TokenSecretName = "vault-token"

	err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.Error(t, err, "")
	assert.EqualError(t, err, "failed to fetch kms token secret \"vault-token\": secrets \"vault-token\" not found")

	// Error: token secret present but empty content
	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      securitySpec.KeyManagementService.TokenSecretName,
			Namespace: ns,
		},
	}
	_, err = context.Clientset.CoreV1().Secrets(ns).Create(ctx, s, metav1.CreateOptions{})
	assert.NoError(t, err)
	err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.Error(t, err, "")
	assert.EqualError(t, err, "failed to read k8s kms secret \"token\" key \"vault-token\" (not found or empty)")

	// Error: token key does not exist
	s.Data = map[string][]byte{"foo": []byte("bar")}
	_, err = context.Clientset.CoreV1().Secrets(ns).Update(ctx, s, metav1.UpdateOptions{})
	assert.NoError(t, err)
	err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.Error(t, err, "")
	assert.EqualError(t, err, "failed to read k8s kms secret \"token\" key \"vault-token\" (not found or empty)")

	// Success: token content is ok
	s.Data["token"] = []byte("token")
	_, err = context.Clientset.CoreV1().Secrets(ns).Update(ctx, s, metav1.UpdateOptions{})
	assert.NoError(t, err)
	err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.Error(t, err, "")
	assert.EqualError(t, err, "failed to validate kms config \"KMS_PROVIDER\". cannot be empty")
	securitySpec.KeyManagementService.ConnectionDetails["KMS_PROVIDER"] = "vault"

	// Error: Data has a KMS_PROVIDER but missing details
	err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.Error(t, err, "")
	assert.EqualError(t, err, "failed to validate vault connection details: failed to find connection details \"VAULT_ADDR\"")

	// Error: connection details are correct but the token secret does not exist
	securitySpec.KeyManagementService.ConnectionDetails["VAULT_ADDR"] = "https://1.1.1.1:8200"

	// Error: TLS is configured but secrets do not exist
	securitySpec.KeyManagementService.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
	err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.Error(t, err, "")
	assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection details k8s secret \"vault-ca-secret\"")

	// Error: TLS secret exists but empty key
	tlsSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-ca-secret",
			Namespace: ns,
		},
	}
	_, err = context.Clientset.CoreV1().Secrets(ns).Create(ctx, tlsSecret, metav1.CreateOptions{})
	assert.NoError(t, err)
	err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.Error(t, err, "")
	assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection key \"cert\" for \"VAULT_CACERT\" in k8s secret \"vault-ca-secret\"")

	// Success: TLS config is correct
	tlsSecret.Data = map[string][]byte{"cert": []byte("envnrevbnbvsbjkrtn")}
	_, err = context.Clientset.CoreV1().Secrets(ns).Update(ctx, tlsSecret, metav1.UpdateOptions{})
	assert.NoError(t, err)
	err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
	assert.NoError(t, err, "")

	// test with vauult server
	t.Run("success - auto detect kv version and set it", func(t *testing.T) {
		cluster := fakeVaultServer(t)
		cluster.Start()
		defer cluster.Cleanup()
		core := cluster.Cores[0].Core
		vault.TestWaitActive(t, core)
		client := cluster.Cores[0].Client
		// Mock the client here
		vaultClient = func(clusterdContext *clusterd.Context, namespace string, secretConfig map[string]string) (*api.Client, error) {
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
		err = ValidateConnectionDetails(ctx, context, securitySpec, ns)
		assert.NoError(t, err, "")
		assert.Equal(t, securitySpec.KeyManagementService.ConnectionDetails["VAULT_BACKEND"], "v2")
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

	err = SetTokenToEnvVar(context, secretName, "vault", ns)
	assert.NoError(t, err)
	assert.Equal(t, os.Getenv("VAULT_TOKEN"), "toto")
	os.Unsetenv("VAULT_TOKEN")
}
