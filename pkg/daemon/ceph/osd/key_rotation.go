/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package osd

import (
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
)

const (
	slotZero string = "0"
	slotOne  string = "1"
)

func RotateKeyEncryptionKey(context *clusterd.Context, kms *kms.Config, secretName string, devicePaths []string) error {
	logger.Info("fetching the current key")
	// Fetch the currentKey.
	currentKey, err := kms.GetSecret(secretName)
	if err != nil {
		return errors.Wrapf(err, "failed to get secret %q", secretName)
	}

	if currentKey == "" {
		return errors.Errorf("the secret %q is empty, cannot rotate the key", secretName)
	}

	// Ensure currentKey is in slot 1.
	for _, devicePath := range devicePaths {
		logger.Infof("adding the current key to slot %q of the device %q", slotOne, devicePath)
		err = addEncryptionKey(context, devicePath, currentKey, currentKey, slotOne)
		if err != nil {
			return errors.Wrapf(err, "failed to add the current key to slot %q of the device", slotOne)
		}
	}

	logger.Info("generating new key")
	// Generate new key.
	newKey, err := oposd.GenerateDmCryptKey()
	if err != nil {
		return errors.Wrapf(err, "failed to generate new key")
	}

	// Add newKey to slot 0.
	for _, devicePath := range devicePaths {
		logger.Infof("removing key slot %q, if found, of the device %q", slotZero, devicePath)
		err = removeEncryptionKeySlot(context, devicePath, currentKey, slotZero)
		if err != nil {
			return errors.Wrapf(err, "failed to remove key slot %q of the device %q", slotZero, devicePath)
		}
		logger.Infof("adding new key to slot %q of the device %q", slotZero, devicePath)
		err = addEncryptionKey(context, devicePath, currentKey, newKey, slotZero)
		if err != nil {
			return errors.Wrapf(err, "failed to add new key to slot %q of the device %q", slotZero, devicePath)
		}
	}

	logger.Info("updating the new key in the KMS")
	// Update new key.
	err = kms.UpdateSecret(secretName, newKey)
	if err != nil {
		return errors.Wrapf(err, "failed to update secret %q with new key", secretName)
	}

	logger.Info("fetching the key from the KMS to verify it.")
	// Fetch key to verify its the new key.
	keyInKMS, err := kms.GetSecret(secretName)
	if err != nil {
		return errors.Wrapf(err, "failed to get secret %q", secretName)
	}

	if keyInKMS != newKey {
		return errors.New("failed to verify the new key in the KMS, the fetched key is not the same as the new key")
	}

	// Remove old key from slot 1.
	for _, devicePath := range devicePaths {
		logger.Infof("removing the old key from the slot %q of the device %q", slotOne, devicePath)
		err = removeEncryptionKeySlot(context, devicePath, newKey, slotOne)
		if err != nil {
			return errors.Wrapf(err, "failed to remove the old key from the slot %q of the device %q", slotOne, devicePath)
		}
	}
	logger.Infof("Successfully rotated the key")

	return nil
}
