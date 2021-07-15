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
	"os"
	"strings"

	"github.com/libopenstorage/secrets/vault"
	"github.com/libopenstorage/secrets/vault/utils"
	"github.com/pkg/errors"

	"github.com/hashicorp/vault/api"
)

const (
	kvVersionKey = "version"
	kvVersion1   = "kv"
	kvVersion2   = "kv-v2"
)

// newVaultClient returns a vault client, there is no need for any secretConfig validation
// Since this is called after an already validated call InitVault()
func newVaultClient(secretConfig map[string]string) (*api.Client, error) {
	// DefaultConfig uses the environment variables if present.
	config := api.DefaultConfig()

	// Convert map string to map interface
	c := make(map[string]interface{})
	for k, v := range secretConfig {
		c[k] = v
	}

	// Configure TLS
	if err := utils.ConfigureTLS(config, c); err != nil {
		return nil, err
	}

	// Initialize the vault client
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	// Set the token if provided, token should be set by ValidateConnectionDetails() if applicable
	client.SetToken(strings.TrimSuffix(os.Getenv(api.EnvVaultToken), "\n"))

	// Set Vault address, was validated by ValidateConnectionDetails()
	err = client.SetAddress(strings.TrimSuffix(secretConfig[api.EnvVaultAddress], "\n"))
	if err != nil {
		return nil, err
	}

	return client, nil
}

func IsKvBackendV2(secretConfig map[string]string) (bool, error) {
	backendPath := GetParam(secretConfig, vault.VaultBackendPathKey)
	if backendPath == "" {
		backendPath = vault.DefaultBackendPath
	}

	backend := GetParam(secretConfig, vault.VaultBackendKey)
	switch backend {
	case kvVersion1, "v1":
		return false, nil
	case kvVersion2, "v2":
		return true, nil
	default:
		// Initialize Vault client
		vaultClient, err := newVaultClient(secretConfig)
		if err != nil {
			return false, errors.Wrap(err, "failed to initialize vault client, assuming kv version 1")
		}

		mounts, err := vaultClient.Sys().ListMounts()
		if err != nil {
			return false, errors.Wrap(err, "failed to list vault system mounts, assuming kv version 1")
		}

		for path, mount := range mounts {
			// path is represented as 'path/'
			if trimSlash(path) == trimSlash(backendPath) {
				version := mount.Options[kvVersionKey]
				if version == "2" {
					return true, nil
				}
				return false, nil
			}
		}
	}

	return false, errors.Errorf("secrets engine with mount path %q not found", backendPath)
}

func trimSlash(in string) string {
	return strings.Trim(in, "/")
}

func BackendVersion(secretConfig map[string]string) (string, error) {
	v1 := "v1"
	v2 := "v2"

	// Check if kv backend is v2
	isKvBackendV2, err := IsKvBackendV2(secretConfig)
	if err != nil {
		return v1, err
	}

	if isKvBackendV2 {
		return v2, nil
	}

	return v1, nil
}
