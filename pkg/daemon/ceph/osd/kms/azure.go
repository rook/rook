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
	"fmt"
	"os"

	"github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/azure"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	//#nosec G101 -- This is only the k8s secret name
	azureClientCertSecretName = "AZURE_CERT_SECRET_NAME"
)

var kmsAzureManadatoryConnectionDetails = []string{azure.AzureVaultURL, azure.AzureTenantID, azure.AzureClientID, azureClientCertSecretName}

// IsAzure determines whether the configured KMS is Azure Key Vault
func (c *Config) IsAzure() bool {
	return c.Provider == secrets.TypeAzure
}

// InitAzure initializes azure key vault client
func InitAzure(ctx context.Context, context *clusterd.Context, namespace string, config map[string]string) (secrets.Secrets, error) {
	azureKVConfig, removecertFiles, err := azureKVCert(ctx, context, namespace, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to setup azure client cert authentication")
	}
	defer removecertFiles()

	// Convert map string to map interface
	secretConfig := make(map[string]interface{})
	for key, value := range azureKVConfig {
		secretConfig[key] = string(value)
	}

	secrets, err := azure.New(secretConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to initialize azure client")
	}

	return secrets, nil
}

// azureKVCert retrivies azure client cert from the secret and stores that in a file
func azureKVCert(ctx context.Context, context *clusterd.Context, namespace string, config map[string]string) (newConfig map[string]string, removeCertFiles removeCertFilesFunction, retErr error) {
	var filesToRemove []*os.File
	defer func() {
		removeCertFiles = getRemoveCertFiles(filesToRemove)
		if retErr != nil {
			removeCertFiles()
			removeCertFiles = nil
		}
	}()

	clientCertSecretName := config[azureClientCertSecretName]
	if clientCertSecretName == "" {
		return nil, removeCertFiles, fmt.Errorf("azure cert secret name is not provided in the connection details")
	}

	secret, err := context.Clientset.CoreV1().Secrets(namespace).Get(ctx, clientCertSecretName, v1.GetOptions{})
	if err != nil {
		return nil, removeCertFiles, errors.Wrapf(err, "failed to fetch tls k8s secret %q", clientCertSecretName)
	}
	// Generate a temp file
	file, err := createTmpFile("", "cert.pem")
	if err != nil {
		return nil, removeCertFiles, errors.Wrapf(err, "failed to generate temp file for k8s secret %q content", clientCertSecretName)
	}

	err = os.WriteFile(file.Name(), secret.Data["CLIENT_CERT"], 0400)
	if err != nil {
		return nil, removeCertFiles, errors.Wrapf(err, "failed to write k8s secret %q content to a file", clientCertSecretName)
	}

	// Update the env var with the path
	config[azure.AzureClientCertPath] = file.Name()

	filesToRemove = append(filesToRemove, file)

	return config, removeCertFiles, nil
}
