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
	"io/ioutil"
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
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
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")
	ctx := context.TODO()
	ns := "rook-ceph"
	context := &clusterd.Context{Clientset: test.New(t, 3)}

	t.Run("no TLS config", func(t *testing.T) {
		config := map[string]string{
			"foo":                "bar",
			"KMS_PROVIDER":       "vault",
			"VAULT_ADDR":         "1.1.1.1",
			"VAULT_BACKEND_PATH": "vault",
		}
		// No tls config
		_, removeCertFiles, err := configTLS(ctx, context, ns, config)
		assert.NoError(t, err)
		defer removeCertFiles()
	})

	t.Run("TLS config with already populated cert path", func(t *testing.T) {
		config := map[string]string{
			"foo":                "bar",
			"KMS_PROVIDER":       "vault",
			"VAULT_ADDR":         "1.1.1.1",
			"VAULT_BACKEND_PATH": "vault",
			"VAULT_CACERT":       "/etc/vault/cacert",
			"VAULT_SKIP_VERIFY":  "false",
		}
		config, removeCertFiles, err := configTLS(ctx, context, ns, config)
		assert.NoError(t, err)
		assert.Equal(t, "/etc/vault/cacert", config["VAULT_CACERT"])
		defer removeCertFiles()
	})

	t.Run("TLS config but no secret", func(t *testing.T) {
		config := map[string]string{
			"foo":                "bar",
			"KMS_PROVIDER":       "vault",
			"VAULT_ADDR":         "1.1.1.1",
			"VAULT_BACKEND_PATH": "vault",
			"VAULT_CACERT":       "vault-ca-cert",
			"VAULT_SKIP_VERIFY":  "false",
		}
		_, removeCertFiles, err := configTLS(ctx, context, ns, config)
		assert.Error(t, err)
		assert.EqualError(t, err, "failed to fetch tls k8s secret \"vault-ca-cert\": secrets \"vault-ca-cert\" not found")
		assert.Nil(t, removeCertFiles)
	})

	t.Run("TLS config success!", func(t *testing.T) {
		config := map[string]string{
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
		_, err := context.Clientset.CoreV1().Secrets(ns).Create(ctx, s, metav1.CreateOptions{})
		assert.NoError(t, err)
		config, removeCertFiles, err := configTLS(ctx, context, ns, config)
		defer removeCertFiles()
		assert.NoError(t, err)
		assert.NotEqual(t, "vault-ca-cert", config["VAULT_CACERT"])
		err = context.Clientset.CoreV1().Secrets(ns).Delete(ctx, s.Name, metav1.DeleteOptions{})
		assert.NoError(t, err)
	})

	t.Run("advanced TLS config success!", func(t *testing.T) {
		config := map[string]string{
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
		_, err := context.Clientset.CoreV1().Secrets(ns).Create(ctx, sCa, metav1.CreateOptions{})
		assert.NoError(t, err)
		_, err = context.Clientset.CoreV1().Secrets(ns).Create(ctx, sClCert, metav1.CreateOptions{})
		assert.NoError(t, err)
		_, err = context.Clientset.CoreV1().Secrets(ns).Create(ctx, sClKey, metav1.CreateOptions{})
		assert.NoError(t, err)
		config, removeCertFiles, err := configTLS(ctx, context, ns, config)
		assert.NoError(t, err)
		assert.NotEqual(t, "vault-ca-cert", config["VAULT_CACERT"])
		assert.NotEqual(t, "vault-client-cert", config["VAULT_CLIENT_CERT"])
		assert.NotEqual(t, "vault-client-key", config["VAULT_CLIENT_KEY"])
		assert.FileExists(t, config["VAULT_CACERT"])
		assert.FileExists(t, config["VAULT_CLIENT_CERT"])
		assert.FileExists(t, config["VAULT_CLIENT_KEY"])
		removeCertFiles()
		assert.NoFileExists(t, config["VAULT_CACERT"])
		assert.NoFileExists(t, config["VAULT_CLIENT_CERT"])
		assert.NoFileExists(t, config["VAULT_CLIENT_KEY"])
	})

	t.Run("advanced TLS config success with timeout!", func(t *testing.T) {
		config := map[string]string{
			"foo":                "bar",
			"KMS_PROVIDER":       "vault",
			"VAULT_ADDR":         "1.1.1.1",
			"VAULT_BACKEND_PATH": "vault",
			"VAULT_CACERT":       "vault-ca-cert",
			"VAULT_CLIENT_CERT":  "vault-client-cert",
			"VAULT_CLIENT_KEY":   "vault-client-key",
		}
		config, removeCertFiles, err := configTLS(ctx, context, ns, config)
		assert.NoError(t, err)
		assert.NotEqual(t, "vault-ca-cert", config["VAULT_CACERT"])
		assert.NotEqual(t, "vault-client-cert", config["VAULT_CLIENT_CERT"])
		assert.NotEqual(t, "vault-client-key", config["VAULT_CLIENT_KEY"])
		assert.FileExists(t, config["VAULT_CACERT"])
		assert.FileExists(t, config["VAULT_CLIENT_CERT"])
		assert.FileExists(t, config["VAULT_CLIENT_KEY"])
		removeCertFiles()
		assert.NoFileExists(t, config["VAULT_CACERT"])
		assert.NoFileExists(t, config["VAULT_CLIENT_CERT"])
		assert.NoFileExists(t, config["VAULT_CLIENT_KEY"])
	})

	// This test verifies that if any of ioutil.TempFile or ioutil.WriteFile fail during the TLS
	// config loop we cleanup the already generated files. For instance, let's say we are at the
	// second iteration, a file has been created, and then ioutil.TempFile fails, we must cleanup
	// the previous file. Essentially we are verifying that defer does what it is supposed to do.
	// Also, in this situation the cleanup function will be 'nil' and the caller won't run it so the
	// configTLS() must do its own cleanup.
	t.Run("advanced TLS config with temp file creation error", func(t *testing.T) {
		createTmpFile = func(dir string, pattern string) (f *os.File, err error) {
			// Create a fake temp file
			ff, err := ioutil.TempFile("", "")
			if err != nil {
				logger.Error(err)
				return nil, err
			}

			// Add the file to the list of files to remove
			var fakeFilesToRemove []*os.File
			fakeFilesToRemove = append(fakeFilesToRemove, ff)
			getRemoveCertFiles = func(filesToRemove []*os.File) removeCertFilesFunction {
				return func() {
					filesToRemove = fakeFilesToRemove
					for _, f := range filesToRemove {
						t.Logf("removing file %q after failure from TempFile call", f.Name())
						f.Close()
						os.Remove(f.Name())
					}
				}
			}
			os.Setenv("ROOK_TMP_FILE", ff.Name())

			return ff, errors.New("error creating tmp file")
		}
		config := map[string]string{
			"foo":                "bar",
			"KMS_PROVIDER":       "vault",
			"VAULT_ADDR":         "1.1.1.1",
			"VAULT_BACKEND_PATH": "vault",
			"VAULT_CACERT":       "vault-ca-cert",
			"VAULT_CLIENT_CERT":  "vault-client-cert",
			"VAULT_CLIENT_KEY":   "vault-client-key",
		}
		_, _, err := configTLS(ctx, context, ns, config)
		assert.Error(t, err)
		assert.EqualError(t, err, "failed to generate temp file for k8s secret \"vault-ca-cert\" content: error creating tmp file")
		assert.NoFileExists(t, os.Getenv("ROOK_TMP_FILE"))
		os.Unsetenv("ROOK_TMP_FILE")
	})
}

func Test_buildVaultKeyContext(t *testing.T) {
	t.Run("no vault namespace, return empty map and assignment is possible", func(t *testing.T) {
		config := map[string]string{
			"KMS_PROVIDER": "vault",
			"VAULT_ADDR":   "1.1.1.1",
		}
		context := buildVaultKeyContext(config)
		assert.Len(t, context, 0)
		context["foo"] = "bar"
	})

	t.Run("vault namespace, return 1 single element in the map and assignment is possible", func(t *testing.T) {
		config := map[string]string{
			"KMS_PROVIDER":    "vault",
			"VAULT_ADDR":      "1.1.1.1",
			"VAULT_NAMESPACE": "vault-namespace",
		}
		context := buildVaultKeyContext(config)
		assert.Len(t, context, 1)
		context["foo"] = "bar"
		assert.Len(t, context, 2)
	})
}
