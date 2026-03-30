/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertToIntIgnoreErr(t *testing.T) {
	assert.Equal(t, 0, convertToIntIgnoreErr(""))
	assert.Equal(t, 0, convertToIntIgnoreErr("abc"))
	assert.Equal(t, 0, convertToIntIgnoreErr("1.5"))
	assert.Equal(t, 0, convertToIntIgnoreErr("0"))
	assert.Equal(t, 1, convertToIntIgnoreErr("1"))
	assert.Equal(t, -1, convertToIntIgnoreErr("-1"))
	assert.Equal(t, 1024, convertToIntIgnoreErr("1024"))
}

func TestToStoreConfig(t *testing.T) {
	t.Run("empty config returns defaults", func(t *testing.T) {
		cfg := ToStoreConfig(map[string]string{})
		assert.Equal(t, 1, cfg.OSDsPerDevice)
		assert.Equal(t, 0, cfg.WalSizeMB)
		assert.Equal(t, 0, cfg.DatabaseSizeMB)
		assert.Equal(t, false, cfg.EncryptedDevice)
		assert.Equal(t, "", cfg.MetadataDevice)
	})

	t.Run("all valid fields", func(t *testing.T) {
		cfg := ToStoreConfig(map[string]string{
			WalSizeMBKey:       "512",
			DatabaseSizeMBKey:  "1024",
			OSDsPerDeviceKey:   "3",
			EncryptedDeviceKey: "true",
			MetadataDeviceKey:  "sdb",
			DeviceClassKey:     "ssd",
			InitialWeightKey:   "1.5",
			PrimaryAffinityKey: "0.5",
		})
		assert.Equal(t, 512, cfg.WalSizeMB)
		assert.Equal(t, 1024, cfg.DatabaseSizeMB)
		assert.Equal(t, 3, cfg.OSDsPerDevice)
		assert.Equal(t, true, cfg.EncryptedDevice)
		assert.Equal(t, "sdb", cfg.MetadataDevice)
		assert.Equal(t, "ssd", cfg.DeviceClass)
		assert.Equal(t, "1.5", cfg.InitialWeight)
		assert.Equal(t, "0.5", cfg.PrimaryAffinity)
	})

	t.Run("OSDsPerDevice rejects zero and negative", func(t *testing.T) {
		cfg := ToStoreConfig(map[string]string{
			OSDsPerDeviceKey: "0",
		})
		assert.Equal(t, 1, cfg.OSDsPerDevice, "zero should keep default of 1")

		cfg = ToStoreConfig(map[string]string{
			OSDsPerDeviceKey: "-5",
		})
		assert.Equal(t, 1, cfg.OSDsPerDevice, "negative should keep default of 1")
	})

	t.Run("non-numeric walSizeMB silently becomes 0", func(t *testing.T) {
		cfg := ToStoreConfig(map[string]string{
			WalSizeMBKey: "not-a-number",
		})
		assert.Equal(t, 0, cfg.WalSizeMB)
	})

	t.Run("encrypted device requires exact string true", func(t *testing.T) {
		cfg := ToStoreConfig(map[string]string{
			EncryptedDeviceKey: "TRUE",
		})
		assert.Equal(t, false, cfg.EncryptedDevice, "only lowercase 'true' is accepted")

		cfg = ToStoreConfig(map[string]string{
			EncryptedDeviceKey: "1",
		})
		assert.Equal(t, false, cfg.EncryptedDevice, "numeric 1 is not accepted")
	})

	t.Run("unknown keys are ignored", func(t *testing.T) {
		cfg := ToStoreConfig(map[string]string{
			"unknownKey": "value",
		})
		assert.Equal(t, 1, cfg.OSDsPerDevice)
	})
}

func TestMetadataDevice(t *testing.T) {
	assert.Equal(t, "sdb", MetadataDevice(map[string]string{MetadataDeviceKey: "sdb"}))
	assert.Equal(t, "", MetadataDevice(map[string]string{"other": "value"}))
	assert.Equal(t, "", MetadataDevice(map[string]string{}))
}
