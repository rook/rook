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

package cluster

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/config"
)

const (
	// STSKeyLength is the required length of the STS key in bytes (32 hex characters = 16 bytes)
	// Ceph uses AES-128 encryption which requires a 16-byte (128-bit) key
	STSKeyLength = 16
)

// ensureSTSConfiguration ensures that STS is properly configured for RGW
// It checks if an STS key exists in the Ceph config, generates one if needed,
// and automatically enables the STS authentication engine
func (c *cluster) ensureSTSConfiguration() error {
	// Inject STS configuration into CephConfig if not already present
	if c.Spec.CephConfig == nil {
		c.Spec.CephConfig = make(map[string]map[string]string)
	}
	if c.Spec.CephConfig["global"] == nil {
		c.Spec.CephConfig["global"] = make(map[string]string)
	}

	// Check if user has already configured the STS key
	if _, exists := c.Spec.CephConfig["global"]["rgw_sts_key"]; !exists {
		// Check if key exists in Ceph config store
		monStore := config.GetMonStore(c.context, c.ClusterInfo)
		existingKey, err := monStore.Get("global", "rgw_sts_key")
		if err != nil || existingKey == "" {
			// Generate a new STS key
			logger.Info("generating new RGW STS encryption key")
			stsKey, err := generateSTSKey()
			if err != nil {
				return errors.Wrap(err, "failed to generate STS encryption key")
			}
			c.Spec.CephConfig["global"]["rgw_sts_key"] = stsKey
			logger.Infof("automatically configured rgw_sts_key for STS support")
		} else {
			// Use existing key from config store
			c.Spec.CephConfig["global"]["rgw_sts_key"] = existingKey
			logger.Debug("using existing rgw_sts_key from Ceph config store")
		}
	}

	// Enable STS authentication if not explicitly configured by user
	if _, exists := c.Spec.CephConfig["global"]["rgw_s3_auth_use_sts"]; !exists {
		c.Spec.CephConfig["global"]["rgw_s3_auth_use_sts"] = "true"
		logger.Info("automatically enabled rgw_s3_auth_use_sts for STS support")
	}

	return nil
}

// generateSTSKey generates a cryptographically secure random 16-character hex string
// suitable for use as an RGW STS encryption key
func generateSTSKey() (string, error) {
	bytes := make([]byte, STSKeyLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.Wrap(err, "failed to generate random bytes for STS key")
	}
	return hex.EncodeToString(bytes), nil
}

// ValidateSTSKey validates that an STS key is in the correct format
func ValidateSTSKey(key string) error {
	if len(key) != STSKeyLength*2 {
		return fmt.Errorf("STS key must be exactly %d hex characters, got %d", STSKeyLength*2, len(key))
	}

	// Verify it's valid hex
	_, err := hex.DecodeString(key)
	if err != nil {
		return fmt.Errorf("STS key must be a valid hexadecimal string: %w", err)
	}

	return nil
}

// Made with Bob
