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
	kp "github.com/IBM/keyprotect-go-client"
	"github.com/pkg/errors"
)

const (
	TypeIBM = "ibmkeyprotect"
	//nolint:gosec // IbmKeyProtectServiceApiKey is the IBM Key Protect service API key
	IbmKeyProtectServiceApiKey = "IBM_KP_SERVICE_API_KEY"
	//nolint:gosec // IbmKeyProtectInstanceIdKey is the IBM Key Protect instance id
	IbmKeyProtectInstanceIdKey = "IBM_KP_SERVICE_INSTANCE_ID"
	//nolint:gosec // IbmKeyProtectBaseUrlKey is the IBM Key Protect base url
	IbmKeyProtectBaseUrlKey = "IBM_KP_BASE_URL"
	//nolint:gosec // IbmKeyProtectTokenUrlKey is the IBM Key Protect token url
	IbmKeyProtectTokenUrlKey = "IBM_KP_TOKEN_URL"
)

var (
	kmsIBMKeyProtectMandatoryTokenDetails      = []string{IbmKeyProtectServiceApiKey}
	kmsIBMKeyProtectMandatoryConnectionDetails = []string{IbmKeyProtectInstanceIdKey, IbmKeyProtectServiceApiKey}
	// ErrIbmServiceApiKeyNotSet is returned when IBM_KP_SERVICE_API_KEY is not set
	ErrIbmServiceApiKeyNotSet = errors.Errorf("%s not set.", IbmKeyProtectServiceApiKey)
	// ErrIbmInstanceIdKeyNotSet is returned when IBM_KP_SERVICE_INSTANCE_ID is not set
	ErrIbmInstanceIdKeyNotSet = errors.Errorf("%s not set.", IbmKeyProtectInstanceIdKey)
)

// InitKeyProtect initializes the KeyProtect KMS.
// With native go client directly "github.com/IBM/keyprotect-go-client"
func InitKeyProtect(config map[string]string) (*kp.Client, error) {
	serviceApiKey := GetParam(config, IbmKeyProtectServiceApiKey)
	if serviceApiKey == "" {
		return nil, ErrIbmServiceApiKeyNotSet
	}

	instanceId := GetParam(config, IbmKeyProtectInstanceIdKey)
	if instanceId == "" {
		return nil, ErrIbmInstanceIdKeyNotSet
	}

	baseUrl := GetParam(config, IbmKeyProtectBaseUrlKey)
	if baseUrl == "" {
		baseUrl = kp.DefaultBaseURL
	}

	tokenUrl := GetParam(config, IbmKeyProtectTokenUrlKey)
	if tokenUrl == "" {
		tokenUrl = kp.DefaultTokenURL
	}

	cc := kp.ClientConfig{
		BaseURL:    baseUrl,
		APIKey:     serviceApiKey,
		TokenURL:   tokenUrl,
		InstanceID: instanceId,
		Verbose:    kp.VerboseAll,
	}

	kp, err := kp.New(cc, nil)
	if err != nil {
		return nil, err
	}

	return kp, nil
}

// IsIBMKeyProtect determines whether the configured KMS is IBM Key Protect
func (c *Config) IsIBMKeyProtect() bool { return c.Provider == TypeIBM }
