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

package client

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	// AdminKeyringTemplate is a string template of Ceph keyring settings which allow connection
	// as admin. The key value must be filled in by the admin auth key for the cluster.
	AdminKeyringTemplate = `
[client.admin]
	key = %s
	caps mds = "allow *"
	caps mon = "allow *"
	caps osd = "allow *"
	caps mgr = "allow *"
`

	// UserKeyringTemplate is a string template of Ceph keyring settings which allow connection.
	UserKeyringTemplate = `
[%s]
	key = %s
`
)

// CephKeyring returns the filled-out user keyring
func CephKeyring(cred CephCred) string {
	if cred.Username == AdminUsername {
		return fmt.Sprintf(AdminKeyringTemplate, cred.Secret)
	}
	return fmt.Sprintf(UserKeyringTemplate, cred.Username, cred.Secret)
}

// CreateKeyring creates a keyring for access to the cluster with the desired set of privileges
// and writes it to disk at the keyring path
func CreateKeyring(context *clusterd.Context, clusterInfo *ClusterInfo, username, keyringPath, keyType string, access []string, generateContents func(string) string) error {
	_, err := os.Stat(keyringPath)
	if err == nil {
		// no error, the file exists, bail out with no error
		logger.Debugf("keyring already exists at %s", keyringPath)
		return nil
	} else if !os.IsNotExist(err) {
		// some other error besides "does not exist", bail out with error
		return errors.Wrapf(err, "failed to stat %s", keyringPath)
	}

	// get-or-create-key for the user account
	key, err := AuthGetOrCreateKey(context, clusterInfo, username, keyType, access)
	if err != nil {
		return errors.Wrapf(err, "failed to get or create auth key for %s", username)
	}

	contents := generateContents(key)
	return WriteKeyring(keyringPath, contents)
}

// Writes the keyring to disk, creating parent dirs as necessary
func WriteKeyring(keyringPath, keyring string) error {
	// save the keyring to the given path
	if err := os.MkdirAll(filepath.Dir(keyringPath), 0o700); err != nil {
		return errors.Wrapf(err, "failed to create keyring directory for %s", keyringPath)
	}
	if err := os.WriteFile(keyringPath, []byte(keyring), 0o600); err != nil {
		return errors.Wrapf(err, "failed to write monitor keyring to %s", keyringPath)
	}
	return nil
}

// IsKeyringBase64Encoded returns whether the keyring is valid
func IsKeyringBase64Encoded(keyring string) bool {
	// If the keyring is not base64 we fail
	_, err := base64.StdEncoding.DecodeString(keyring)
	if err != nil {
		logger.Errorf("key is not base64 encoded. %v", err)
		return false
	}

	return true
}
