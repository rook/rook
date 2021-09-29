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

package kms

import (
	"context"
	"testing"

	kv "github.com/hashicorp/vault-plugin-secrets-kv"
	"github.com/hashicorp/vault/api"
	vaulthttp "github.com/hashicorp/vault/http"
	"github.com/hashicorp/vault/sdk/logical"
	"github.com/hashicorp/vault/vault"
	"github.com/libopenstorage/secrets/vault/utils"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBackendVersion(t *testing.T) {
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

	// Set up the kv store
	if err := client.Sys().Mount("rook/", &api.MountInput{
		Type:    "kv",
		Options: map[string]string{"version": "1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Sys().Mount("rookv2/", &api.MountInput{
		Type:    "kv-v2",
		Options: map[string]string{"version": "2"},
	}); err != nil {
		t.Fatal(err)
	}

	type args struct {
		secretConfig map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"v1 is set explicitly", args{map[string]string{"VAULT_BACKEND": "v1"}}, "v1", false},
		{"v2 is set explicitly", args{map[string]string{"VAULT_BACKEND": "v2"}}, "v2", false},
		{"v1 is set auto-discovered", args{map[string]string{"VAULT_ADDR": client.Address(), "VAULT_BACKEND_PATH": "rook"}}, "v1", false},
		{"v2 is set auto-discovered", args{map[string]string{"VAULT_ADDR": client.Address(), "VAULT_BACKEND_PATH": "rookv2"}}, "v2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BackendVersion(&clusterd.Context{}, "ns", tt.args.secretConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("BackendVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("BackendVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func fakeVaultServer(t *testing.T) *vault.TestCluster {
	cluster := vault.NewTestCluster(t, &vault.CoreConfig{
		DevToken:        "token",
		LogicalBackends: map[string]logical.Factory{"kv": kv.Factory},
	},
		&vault.TestClusterOptions{
			HandlerFunc: vaulthttp.Handler,
			NumCores:    1,
		})

	return cluster
}

func TestTLSConfig(t *testing.T) {
	ns := "rook-ceph"
	ctx := context.TODO()
	context := &clusterd.Context{Clientset: test.New(t, 3)}
	secretConfig := map[string]string{
		"foo":                "bar",
		"KMS_PROVIDER":       "vault",
		"VAULT_ADDR":         "1.1.1.1",
		"VAULT_BACKEND_PATH": "vault",
		"VAULT_CACERT":       "vault-ca-cert",
		"VAULT_CLIENT_CERT":  "vault-client-cert",
		"VAULT_CLIENT_KEY":   "vault-client-key",
	}

	// DefaultConfig uses the environment variables if present.
	config := api.DefaultConfig()

	// Convert map string to map interface
	c := make(map[string]interface{})
	for k, v := range secretConfig {
		c[k] = v
	}

	sCa := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-ca-cert",
			Namespace: ns,
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
	}

	sClCert := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-client-cert",
			Namespace: ns,
		},
		Data: map[string][]byte{"cert": []byte(`-----BEGIN CERTIFICATE-----
MIIBEDCBuwIBATANBgkqhkiG9w0BAQUFADAaMQswCQYDVQQGEwJGUjELMAkGA1UE
CAwCRlIwHhcNMjEwOTMwMDgwNDA1WhcNMjQwNjI2MDgwNDA1WjANMQswCQYDVQQG
EwJGUjBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQCpWJqKhSES3BiFkt2M82xy3tkB
plDS8DM0s/+VkqfZlVG18KbbIVDHi1lsPjjs/Aja7lWymw0ycV4KGEcqxdmNAgMB
AAEwDQYJKoZIhvcNAQEFBQADQQC5esmoTqp4uEWyC+GKbTTFp8ngMUywAtZJs4nS
wdoF3ZJJzo4ps0saP1ww5LBdeeXUURscxyaFfCFmGODaHJJn
-----END CERTIFICATE-----`)},
	}

	sClKey := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-client-key",
			Namespace: ns,
		},
		Data: map[string][]byte{"key": []byte(`-----BEGIN PRIVATE KEY-----
MIIBVgIBADANBgkqhkiG9w0BAQEFAASCAUAwggE8AgEAAkEAqViaioUhEtwYhZLd
jPNsct7ZAaZQ0vAzNLP/lZKn2ZVRtfCm2yFQx4tZbD447PwI2u5VspsNMnFeChhH
KsXZjQIDAQABAkARlCv+oxEq1wQIoZUz83TXe8CFBlGvg9Wc6+5lBWM9F7K4by7i
IB5hQ2oaTNN+1Kxzf+XRM9R7sMPP9qFEp0LhAiEA0PzsQqbvNUVEx8X16Hed6V/Z
yvL1iZeHvc2QIbGjZGkCIQDPcM7U0frsFIPuMY4zpX2b6w4rpxZN7Kybp9/3l0tX
hQIhAJVWVsGeJksLr4WNuRYf+9BbNPdoO/rRNCd2L+tT060ZAiEAl0uontITl9IS
s0yTcZm29lxG9pGkE+uVrOWQ1W0Ud10CIQDJ/L+VCQgjO+SviUECc/nMwhWDMT+V
cjLxGL8tcZjHKg==
-----END PRIVATE KEY-----`)},
	}

	for _, s := range []*v1.Secret{sCa, sClCert, sClKey} {
		if secret, err := context.Clientset.CoreV1().Secrets(ns).Create(ctx, s, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		} else {
			defer func() {
				err := context.Clientset.CoreV1().Secrets(ns).Delete(ctx, secret.Name, metav1.DeleteOptions{})
				if err != nil {
					logger.Errorf("failed to delete secret %s: %v", secret.Name, err)
				}
			}()
		}
	}

	// Populate TLS config
	newConfigWithTLS, removeCertFiles, err := configTLS(context, ns, secretConfig)
	assert.NoError(t, err)
	defer removeCertFiles()

	// Populate TLS config
	for key, value := range newConfigWithTLS {
		c[key] = string(value)
	}

	// Configure TLS
	err = utils.ConfigureTLS(config, c)
	assert.NoError(t, err)
}
