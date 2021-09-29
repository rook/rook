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
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_tlsSecretKeyToCheck(t *testing.T) {
	type args struct {
		tlsOption string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"certificate", args{tlsOption: "VAULT_CACERT"}, "cert"},
		{"client-certificate", args{tlsOption: "VAULT_CLIENT_CERT"}, "cert"},
		{"client-key", args{tlsOption: "VAULT_CLIENT_KEY"}, "key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tlsSecretKeyToCheck(tt.args.tlsOption); got != tt.want {
				t.Errorf("tlsSecretKeyToCheck() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_configTLS(t *testing.T) {
	ctx := context.TODO()
	config := map[string]string{
		"foo":                "bar",
		"KMS_PROVIDER":       "vault",
		"VAULT_ADDR":         "1.1.1.1",
		"VAULT_BACKEND_PATH": "vault",
	}
	ns := "rook-ceph"
	context := &clusterd.Context{Clientset: test.New(t, 3)}

	// No tls config
	_, err := configTLS(context, ns, config)
	assert.NoError(t, err)

	// TLS config with correct values
	config = map[string]string{
		"foo":                "bar",
		"KMS_PROVIDER":       "vault",
		"VAULT_ADDR":         "1.1.1.1",
		"VAULT_BACKEND_PATH": "vault",
		"VAULT_CACERT":       "/etc/vault/cacert",
		"VAULT_SKIP_VERIFY":  "false",
	}
	config, err = configTLS(context, ns, config)
	assert.NoError(t, err)
	assert.Equal(t, "/etc/vault/cacert", config["VAULT_CACERT"])

	// TLS config but no secret
	config = map[string]string{
		"foo":                "bar",
		"KMS_PROVIDER":       "vault",
		"VAULT_ADDR":         "1.1.1.1",
		"VAULT_BACKEND_PATH": "vault",
		"VAULT_CACERT":       "vault-ca-cert",
		"VAULT_SKIP_VERIFY":  "false",
	}
	_, err = configTLS(context, ns, config)
	assert.Error(t, err)
	assert.EqualError(t, err, "failed to fetch tls k8s secret \"vault-ca-cert\": secrets \"vault-ca-cert\" not found")

	// TLS config success!
	config = map[string]string{
		"foo":                "bar",
		"KMS_PROVIDER":       "vault",
		"VAULT_ADDR":         "1.1.1.1",
		"VAULT_BACKEND_PATH": "vault",
		"VAULT_CACERT":       "vault-ca-cert",
		"VAULT_SKIP_VERIFY":  "false",
	}
	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-ca-cert",
			Namespace: ns,
		},
		Data: map[string][]byte{"cert": []byte("bar")},
	}
	_, err = context.Clientset.CoreV1().Secrets(ns).Create(ctx, s, metav1.CreateOptions{})
	assert.NoError(t, err)
	config, err = configTLS(context, ns, config)
	assert.NoError(t, err)
	assert.NotEqual(t, "vault-ca-cert", config["VAULT_CACERT"])
	err = context.Clientset.CoreV1().Secrets(ns).Delete(ctx, s.Name, metav1.DeleteOptions{})
	assert.NoError(t, err)

	// All TLS success!
	config = map[string]string{
		"foo":                "bar",
		"KMS_PROVIDER":       "vault",
		"VAULT_ADDR":         "1.1.1.1",
		"VAULT_BACKEND_PATH": "vault",
		"VAULT_CACERT":       "vault-ca-cert",
		"VAULT_CLIENT_CERT":  "vault-client-cert",
		"VAULT_CLIENT_KEY":   "vault-client-key",
	}
	sCa := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-ca-cert",
			Namespace: ns,
		},
		Data: map[string][]byte{"cert": []byte("bar")},
	}
	sClCert := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-client-cert",
			Namespace: ns,
		},
		Data: map[string][]byte{"cert": []byte("bar")},
	}
	sClKey := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-client-key",
			Namespace: ns,
		},
		Data: map[string][]byte{"key": []byte("bar")},
	}
	_, err = context.Clientset.CoreV1().Secrets(ns).Create(ctx, sCa, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = context.Clientset.CoreV1().Secrets(ns).Create(ctx, sClCert, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = context.Clientset.CoreV1().Secrets(ns).Create(ctx, sClKey, metav1.CreateOptions{})
	assert.NoError(t, err)
	config, err = configTLS(context, ns, config)
	assert.NoError(t, err)
	assert.NotEqual(t, "vault-ca-cert", config["VAULT_CACERT"])
	assert.NotEqual(t, "vault-client-cert", config["VAULT_CLIENT_CERT"])
	assert.NotEqual(t, "vault-client-key", config["VAULT_CLIENT_KEY"])
}

func Test_buildKeyContext(t *testing.T) {
	t.Run("no vault namespace, return empty map and assignment is possible", func(t *testing.T) {
		config := map[string]string{
			"KMS_PROVIDER": "vault",
			"VAULT_ADDR":   "1.1.1.1",
		}
		context := buildKeyContext(config)
		assert.Len(t, context, 0)
		context["foo"] = "bar"
	})

	t.Run("vault namespace, return 1 single element in the map and assignment is possible", func(t *testing.T) {
		config := map[string]string{
			"KMS_PROVIDER":    "vault",
			"VAULT_ADDR":      "1.1.1.1",
			"VAULT_NAMESPACE": "vault-namespace",
		}
		context := buildKeyContext(config)
		assert.Len(t, context, 1)
		context["foo"] = "bar"
		assert.Len(t, context, 2)
	})
}
