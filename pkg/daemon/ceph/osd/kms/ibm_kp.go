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
	"github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/ibm"
)

var (
	kmsIBMKeyProtectMandatoryTokenDetails      = []string{ibm.IbmServiceApiKey}
	kmsIBMKeyProtectMandatoryConnectionDetails = []string{ibm.IbmInstanceIdKey}
)

// InitKeyProtect initializes the KeyProtect KMS.
// With native go client directly "github.com/IBM/keyprotect-go-client"
func InitKeyProtect(config map[string]string) (*kp.Client, error) {
	// Create a cloud API Key
	// https://www.ibm.com/docs/en/app-connect/containers_cd?topic=servers-creating-cloud-api-key
	serviceApiKey := GetParam(config, ibm.IbmServiceApiKey)
	if serviceApiKey == "" {
		return nil, ibm.ErrIbmServiceApiKeyNotSet
	}

	instanceId := GetParam(config, ibm.IbmInstanceIdKey)
	if instanceId == "" {
		return nil, ibm.ErrIbmInstanceIdKeyNotSet
	}

	baseUrl := GetParam(config, ibm.IbmBaseUrlKey)
	if baseUrl == "" {
		baseUrl = kp.DefaultBaseURL
	}

	tokenUrl := GetParam(config, ibm.IbmTokenUrlKey)
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

// IsKeyProtect determines whether the configured KMS is IBM Key Protect
func (c *Config) IsKeyProtect() bool { return c.Provider == secrets.TypeIBM }
