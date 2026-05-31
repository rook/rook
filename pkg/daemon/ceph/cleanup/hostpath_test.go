/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package cleanup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_cleanCSIDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid CSI directory
	rbdDriverDir := filepath.Join(tmpDir, "rbd.csi.ceph.com")
	err := os.MkdirAll(rbdDriverDir, 0o600)
	assert.NoError(t, err)

	cephFSDriverDir := filepath.Join(tmpDir, "cephfs.csi.ceph.com")
	err = os.MkdirAll(cephFSDriverDir, 0o600)
	assert.NoError(t, err)

	cleanCSIDirs(tmpDir)

	_, err = os.Stat(rbdDriverDir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(rbdDriverDir)
	assert.True(t, os.IsNotExist(err))
}

func Test_cleanExporterDir(t *testing.T) {
	tmpDir := t.TempDir()
	exporterDir := filepath.Join(tmpDir, "exporter")

	// Create exporter directory
	err := os.MkdirAll(exporterDir, 0o600)
	assert.NoError(t, err)

	cleanExporterDir(tmpDir)

	_, err = os.Stat(exporterDir)
	assert.True(t, os.IsNotExist(err))
}

func Test_monDir(t *testing.T) {
	// Test for key match
	testKey := "mysecret"
	validKeyring := `[client.admin]
	key = mysecret
`
	tmpDir := t.TempDir()
	monDir := filepath.Join(tmpDir, "mon-a")
	dataDir := filepath.Join(monDir, "data")
	err := os.MkdirAll(dataDir, 0o755)
	assert.NoError(t, err)

	// Create keyring file with matching key
	keyring := filepath.Join(dataDir, "keyring")
	err = os.WriteFile(keyring, []byte(validKeyring), 0o600)
	assert.NoError(t, err)

	cleanMonDirs(tmpDir, testKey)

	_, err = os.Stat(monDir)
	assert.True(t, os.IsNotExist(err))
	// Test for key mismatch
	// Wrong secret in keyring
	keyring = filepath.Join(dataDir, "keyring")
	err = os.MkdirAll(dataDir, 0o755)
	assert.NoError(t, err)
	err = os.WriteFile(keyring, []byte(validKeyring), 0o600)
	assert.NoError(t, err)

	cleanMonDirs(tmpDir, "wrongsecret")

	_, err = os.Stat(monDir)
	assert.False(t, os.IsNotExist(err))
}

func Test_secretKeyMatch(t *testing.T) {
	testKey := "mysecret"
	validKeyring := `[client.admin]
	key = mysecret`
	// key match
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	err := os.MkdirAll(dataDir, 0o755)
	assert.NoError(t, err)

	keyring := filepath.Join(dataDir, "keyring")
	err = os.WriteFile(keyring, []byte(validKeyring), 0o600)
	assert.NoError(t, err)

	match, err := secretKeyMatch(tmpDir, testKey)
	assert.NoError(t, err)
	assert.True(t, match)

	// key mismatch
	match, err = secretKeyMatch(tmpDir, "invalidKey")
	assert.NoError(t, err)
	assert.False(t, match)
}
