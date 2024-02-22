/*
Copyright 2024 The Rook Authors. All rights reserved.

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
	"testing"

	"github.com/libopenstorage/secrets/azure"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_AzureKVCert(t *testing.T) {
	ctx := context.TODO()
	ns := "rook-ceph"
	context := &clusterd.Context{Clientset: test.New(t, 3)}
	t.Run("azure secret name not provided in config", func(t *testing.T) {
		config := map[string]string{
			"KMS_PROVIDER": "azure-kv",
		}
		_, _, err := azureKVCert(ctx, context, ns, config)
		assert.Error(t, err)
	})

	t.Run("invalid azure secret name in config", func(t *testing.T) {
		config := map[string]string{
			"KMS_PROVIDER":            "azure-kv",
			azureClientCertSecretName: "invalid-name",
		}

		cert := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "azure-cert",
				Namespace: ns,
			},
			Data: map[string][]byte{"CLIENT_CERT": []byte("bar")},
		}

		_, err := context.Clientset.CoreV1().Secrets(ns).Create(ctx, cert, metav1.CreateOptions{})
		assert.NoError(t, err)
		_, _, err = azureKVCert(ctx, context, ns, config)
		assert.Error(t, err)
	})

	t.Run("valid azure cert secret is available", func(t *testing.T) {
		config := map[string]string{
			"KMS_PROVIDER":            "azure-kv",
			azureClientCertSecretName: "azure-cert-2",
		}

		cert := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "azure-cert-2",
				Namespace: ns,
			},
			Data: map[string][]byte{"CLIENT_CERT": []byte("bar")},
		}

		_, err := context.Clientset.CoreV1().Secrets(ns).Create(ctx, cert, metav1.CreateOptions{})
		assert.NoError(t, err)
		newConfig, removeCertFile, err := azureKVCert(ctx, context, ns, config)
		assert.NoError(t, err)
		assert.FileExists(t, newConfig[azure.AzureClientCertPath])
		removeCertFile()
		assert.NoFileExists(t, newConfig[azure.AzureClientCertPath])
	})
}
