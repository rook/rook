/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package client

import (
	"context"
	cryptotls "crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rook/rook/tests/framework/utils"
)

// InsecureHTTPClient returns an http.Client that skips TLS verification, for
// reaching a test CephObjectStore served with a cluster-signed cert the test
// process does not trust.
func InsecureHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			// nolint:gosec // skip TLS verification as this is a test
			TLSClientConfig: &cryptotls.Config{InsecureSkipVerify: true},
		},
	}
}

// GenerateRgwTLSCertSecret generates a cluster-signed TLS certificate for
// rgwServiceName and stores it as an Opaque Secret named secretName in
// namespace. The secret holds the concatenated key, certificate, and CA bundle
// under the data key "cert", which is the format a CephObjectStore expects when
// referenced via Gateway.SSLCertificateRef.
func GenerateRgwTLSCertSecret(t *testing.T, k8sh *utils.K8sHelper, namespace, secretName, rgwServiceName string) {
	ctx := context.TODO()

	root, err := utils.FindRookRoot()
	require.NoError(t, err, "failed to get rook root")

	tlscertdir := t.TempDir()
	cmdArgs := utils.CommandArgs{
		Command: filepath.Join(root, "tests/scripts/generate-tls-config.sh"),
		CmdArgs: []string{tlscertdir, rgwServiceName, namespace},
	}
	cmdOut := utils.ExecuteCommand(cmdArgs)
	require.NoError(t, cmdOut.Err)

	tlsKeyIn, err := os.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".key"))
	require.NoError(t, err)
	tlsCertIn, err := os.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".crt"))
	require.NoError(t, err)
	tlsCaCertIn, err := os.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".ca"))
	require.NoError(t, err)

	secretCertOut := fmt.Sprintf("%s%s%s", tlsKeyIn, tlsCertIn, tlsCaCertIn)
	tlsK8sSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"cert": []byte(secretCertOut),
		},
	}
	_, err = k8sh.Clientset.CoreV1().Secrets(namespace).Create(ctx, tlsK8sSecret, metav1.CreateOptions{})
	require.NoError(t, err)
}
