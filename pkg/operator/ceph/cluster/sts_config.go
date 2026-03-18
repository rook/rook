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
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// STSKeySecretName is the name of the secret that stores the RGW STS encryption key
	STSKeySecretName = "rook-ceph-rgw-sts-key"
	// STSKeySecretKey is the key in the secret that contains the STS encryption key
	STSKeySecretKey = "sts-key"
	// STSKeyLength is the required length of the STS key in bytes (16 hex characters = 8 bytes)
	STSKeyLength = 8
)

// ensureSTSConfiguration ensures that STS is properly configured for RGW
// It generates and stores an STS encryption key if one doesn't exist,
// and automatically adds the required Ceph configuration settings
func (c *cluster) ensureSTSConfiguration() error {
	ctx := context.TODO()

	// Check if STS key secret already exists
	secret, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(ctx, STSKeySecretName, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to check for existing STS key secret")
		}

		// Secret doesn't exist, generate a new STS key
		logger.Info("generating new RGW STS encryption key")
		stsKey, err := generateSTSKey()
		if err != nil {
			return errors.Wrap(err, "failed to generate STS encryption key")
		}

		// Create the secret
		secret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      STSKeySecretName,
				Namespace: c.Namespace,
			},
			Type: v1.SecretTypeOpaque,
			Data: map[string][]byte{
				STSKeySecretKey: []byte(stsKey),
			},
		}

		// Set owner reference
		err = c.ownerInfo.SetControllerReference(secret)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference on STS key secret")
		}

		_, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to create STS key secret")
		}

		logger.Infof("created STS key secret %q in namespace %q", STSKeySecretName, c.Namespace)
	} else {
		logger.Debugf("STS key secret %q already exists", STSKeySecretName)
	}

	// Get the STS key from the secret
	stsKeyBytes, ok := secret.Data[STSKeySecretKey]
	if !ok {
		return errors.Errorf("STS key secret %q is missing key %q", STSKeySecretName, STSKeySecretKey)
	}
	stsKey := string(stsKeyBytes)

	// Validate the STS key format (must be 16 hex characters)
	if len(stsKey) != STSKeyLength*2 {
		return errors.Errorf("STS key must be exactly %d hex characters, got %d", STSKeyLength*2, len(stsKey))
	}

	// Inject STS configuration into CephConfig if not already present
	if c.Spec.CephConfig == nil {
		c.Spec.CephConfig = make(map[string]map[string]string)
	}
	if c.Spec.CephConfig["global"] == nil {
		c.Spec.CephConfig["global"] = make(map[string]string)
	}

	// Only set if not already configured by the user
	if _, exists := c.Spec.CephConfig["global"]["rgw_sts_key"]; !exists {
		c.Spec.CephConfig["global"]["rgw_sts_key"] = stsKey
		logger.Info("automatically configured rgw_sts_key for STS support")
	}

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

// GetSTSKey retrieves the STS encryption key from the secret
// This can be used by other components that need access to the key
func GetSTSKey(clusterdContext *clusterd.Context, namespace string) (string, error) {
	ctx := context.TODO()
	secret, err := clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(ctx, STSKeySecretName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get STS key secret %q", STSKeySecretName)
	}

	stsKeyBytes, ok := secret.Data[STSKeySecretKey]
	if !ok {
		return "", errors.Errorf("STS key secret %q is missing key %q", STSKeySecretName, STSKeySecretKey)
	}

	return string(stsKeyBytes), nil
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
