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
	"testing"

	kp "github.com/IBM/keyprotect-go-client"
	"github.com/stretchr/testify/assert"
)

func TestInitKeyProtect(t *testing.T) {
	config := map[string]string{
		"foo": "1",
	}

	t.Run("IBM_KP_SERVICE_API_KEY not set", func(t *testing.T) {
		_, err := InitKeyProtect(config)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrIbmServiceApiKeyNotSet)
		config[IbmKeyProtectServiceApiKey] = "foo"
	})

	t.Run("IBM_KP_SERVICE_INSTANCE_ID not set", func(t *testing.T) {
		_, err := InitKeyProtect(config)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrIbmInstanceIdKeyNotSet)
		config[IbmKeyProtectInstanceIdKey] = "foo"
	})

	t.Run("default base URL", func(t *testing.T) {
		c, err := InitKeyProtect(config)
		assert.NoError(t, err)
		assert.Equal(t, kp.DefaultBaseURL, c.Config.BaseURL)
	})

	t.Run("different base URL", func(t *testing.T) {
		config[IbmKeyProtectBaseUrlKey] = "https://us-west.kms.cloud.ibm.com"
		c, err := InitKeyProtect(config)
		assert.NoError(t, err)
		assert.Equal(t, "https://us-west.kms.cloud.ibm.com", c.Config.BaseURL)
	})

	t.Run("default base token URL", func(t *testing.T) {
		c, err := InitKeyProtect(config)
		assert.NoError(t, err)
		assert.Equal(t, kp.DefaultTokenURL, c.Config.TokenURL)
	})

	t.Run("different base URL", func(t *testing.T) {
		config[IbmKeyProtectTokenUrlKey] = "new"
		c, err := InitKeyProtect(config)
		assert.NoError(t, err)
		assert.Equal(t, "new", c.Config.TokenURL)
	})
}
