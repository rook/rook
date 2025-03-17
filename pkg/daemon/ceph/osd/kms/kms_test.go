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

	"github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/azure"
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
	kms := &cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{}}
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
	kmipSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kmip-token",
			Namespace: ns,
		},
		Data: map[string][]byte{
			"CLIENT_CERT": []byte("bar"),
			"CLIENT_KEY":  []byte("bar"),
		},
	}
	tlsSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-ca-secret",
			Namespace: ns,
		},
	}
	ibmKMSSpec := &cephv1.KeyManagementServiceSpec{
		ConnectionDetails: map[string]string{
			"KMS_PROVIDER": TypeIBM,
		},
	}
	kmipKMSSpec := &cephv1.KeyManagementServiceSpec{
		ConnectionDetails: map[string]string{
			"KMS_PROVIDER": TypeKMIP,
		},
		TokenSecretName: "kmip-token",
	}
	azureKMSSpec := &cephv1.KeyManagementServiceSpec{
		ConnectionDetails: map[string]string{
			"KMS_PROVIDER": secrets.TypeAzure,
		},
	}

	t.Run("no kms provider given", func(t *testing.T) {
		err := ValidateConnectionDetails(ctx, clusterdContext, kms, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms config \"KMS_PROVIDER\". cannot be empty")
		kms.ConnectionDetails["KMS_PROVIDER"] = "vault"
	})

	t.Run("kmip - missing ca cert", func(t *testing.T) {
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Create(ctx, kmipSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, kmipKMSSpec, ns)
		assert.Error(t, err, ErrKMIPEndpointNotSet)
	})

	t.Run("kmip - missing endpoint", func(t *testing.T) {
		kmipSecret.Data[KmipCACert] = []byte("foo")
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, kmipSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, kmipKMSSpec, ns)
		assert.Error(t, err, ErrKMIPEndpointNotSet)
	})

	t.Run("kmip - success", func(t *testing.T) {
		kmipKMSSpec.ConnectionDetails[kmipEndpoint] = "pykmip.local"
		err := ValidateConnectionDetails(ctx, clusterdContext, kmipKMSSpec, ns)
		assert.NoError(t, err)
		assert.Equal(t, "foo", kmipKMSSpec.ConnectionDetails[KmipCACert])
		assert.Equal(t, "bar", kmipKMSSpec.ConnectionDetails[KmipClientCert])
		assert.Equal(t, "bar", kmipKMSSpec.ConnectionDetails[KmipClientKey])
	})

	t.Run("vault - no token object", func(t *testing.T) {
		kms.TokenSecretName = "vault-token"
		err := ValidateConnectionDetails(ctx, clusterdContext, kms, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to fetch kms token secret \"vault-token\": secrets \"vault-token\" not found")
	})

	t.Run("vault - token secret present but empty content", func(t *testing.T) {
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Create(ctx, vaultSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, kms, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to read k8s kms secret \"token\" key \"vault-token\" (not found or empty)")
	})

	t.Run("vault - token key does not exist", func(t *testing.T) {
		vaultSecret.Data = map[string][]byte{"foo": []byte("bar")}
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, vaultSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, kms, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to read k8s kms secret \"token\" key \"vault-token\" (not found or empty)")
	})

	// Success: token content is ok
	t.Run("vault - token content is ok", func(t *testing.T) {
		vaultSecret.Data["token"] = []byte("token")
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, vaultSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, kms, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find connection details \"VAULT_ADDR\"")
		kms.ConnectionDetails["VAULT_ADDR"] = "https://1.1.1.1:8200"
	})

	t.Run("vault - TLS is configured but secrets do not exist", func(t *testing.T) {
		kms.ConnectionDetails["VAULT_CACERT"] = "vault-ca-secret"
		err := ValidateConnectionDetails(ctx, clusterdContext, kms, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection details k8s secret \"vault-ca-secret\": secrets \"vault-ca-secret\" not found")
	})

	t.Run("vault - TLS secret exists but empty key", func(t *testing.T) {
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Create(ctx, tlsSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, kms, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate vault connection details: failed to find TLS connection key \"cert\" for \"VAULT_CACERT\" in k8s secret \"vault-ca-secret\"")
	})

	t.Run("vault - success TLS config is correct", func(t *testing.T) {
		tlsSecret.Data = map[string][]byte{"cert": []byte("envnrevbnbvsbjkrtn")}
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, tlsSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, kms, ns)
		assert.NoError(t, err, "")
	})

	t.Run("ibm kp - fail no token specified, only token is supported", func(t *testing.T) {
		err := ValidateConnectionDetails(ctx, clusterdContext, ibmKMSSpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms configuration (missing token in spec)")
		ibmKMSSpec.TokenSecretName = "ibm-token"
	})

	t.Run("ibm kp - token present but no key for service key", func(t *testing.T) {
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Create(ctx, ibmSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, ibmKMSSpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to read k8s kms secret \"IBM_KP_SERVICE_API_KEY\" key \"ibm-token\" (not found or empty)")
	})

	t.Run("ibm kp - token ok but no instance id", func(t *testing.T) {
		ibmSecret.Data["IBM_KP_SERVICE_API_KEY"] = []byte("foo")
		_, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Update(ctx, ibmSecret, metav1.UpdateOptions{})
		assert.NoError(t, err)
		err = ValidateConnectionDetails(ctx, clusterdContext, ibmKMSSpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms config \"IBM_KP_SERVICE_INSTANCE_ID\". cannot be empty")
		ibmKMSSpec.ConnectionDetails["IBM_KP_SERVICE_INSTANCE_ID"] = "foo"
	})

	t.Run("ibm kp - success", func(t *testing.T) {
		err := ValidateConnectionDetails(ctx, clusterdContext, ibmKMSSpec, ns)
		assert.NoError(t, err, "")
		// IBM_KP_SERVICE_API_KEY must be appended to the details so that the client can be built with
		// all the details
		assert.Equal(t, ibmKMSSpec.ConnectionDetails["IBM_KP_SERVICE_API_KEY"], "foo")
	})

	t.Run("azure kms - vault URL is missing ", func(t *testing.T) {
		err := ValidateConnectionDetails(ctx, clusterdContext, azureKMSSpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms config \"AZURE_VAULT_URL\". cannot be empty")
	})

	t.Run("azure kms - tenant ID is missing ", func(t *testing.T) {
		azureKMSSpec.ConnectionDetails[azure.AzureVaultURL] = "test"
		err := ValidateConnectionDetails(ctx, clusterdContext, azureKMSSpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms config \"AZURE_TENANT_ID\". cannot be empty")
	})

	t.Run("azure kms - client ID is missing ", func(t *testing.T) {
		azureKMSSpec.ConnectionDetails[azure.AzureTenantID] = "test"
		err := ValidateConnectionDetails(ctx, clusterdContext, azureKMSSpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms config \"AZURE_CLIENT_ID\". cannot be empty")
	})

	t.Run("azure kms - cert secret is missing ", func(t *testing.T) {
		azureKMSSpec.ConnectionDetails[azure.AzureClientID] = "test"
		err := ValidateConnectionDetails(ctx, clusterdContext, azureKMSSpec, ns)
		assert.Error(t, err, "")
		assert.EqualError(t, err, "failed to validate kms config \"AZURE_CERT_SECRET_NAME\". cannot be empty")
	})

	t.Run("azure kms - success", func(t *testing.T) {
		azureKMSSpec.ConnectionDetails[azureClientCertSecretName] = "test"
		err := ValidateConnectionDetails(ctx, clusterdContext, azureKMSSpec, ns)
		assert.NoError(t, err)
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
