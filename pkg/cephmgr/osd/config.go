/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"fmt"

	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/rook/rook/pkg/cephmgr/client"
)

// get the bootstrap OSD root dir
func getBootstrapOSDDir(configDir string) string {
	return path.Join(configDir, "bootstrap-osd")
}

func getOSDRootDir(root string, osdID int) string {
	return fmt.Sprintf("%s/osd%d", root, osdID)
}

// get the full path to the bootstrap OSD keyring
func getBootstrapOSDKeyringPath(configDir, clusterName string) string {
	return fmt.Sprintf("%s/%s.keyring", getBootstrapOSDDir(configDir), clusterName)
}

// get the full path to the given OSD's config file
func getOSDConfFilePath(osdDataPath, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", osdDataPath, clusterName)
}

// get the full path to the given OSD's keyring
func getOSDKeyringPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "keyring")
}

// get the full path to the given OSD's journal
func getOSDJournalPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "journal")
}

// get the full path to the given OSD's temporary mon map
func getOSDTempMonMapPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "tmp", "activate.monmap")
}

// create a keyring for the bootstrap-osd client, it gets a limited set of privileges
func createOSDBootstrapKeyring(conn client.Connection, configDir, clusterName string) error {
	bootstrapOSDKeyringPath := getBootstrapOSDKeyringPath(configDir, clusterName)
	_, err := os.Stat(bootstrapOSDKeyringPath)
	if err == nil {
		// no error, the file exists, bail out with no error
		logger.Debugf("bootstrap OSD keyring already exists at %s", bootstrapOSDKeyringPath)
		return nil
	} else if !os.IsNotExist(err) {
		// some other error besides "does not exist", bail out with error
		return fmt.Errorf("failed to stat %s: %+v", bootstrapOSDKeyringPath, err)
	}

	// get-or-create-key for client.bootstrap-osd
	bootstrapOSDKey, err := client.AuthGetOrCreateKey(conn, "client.bootstrap-osd", []string{"mon", "allow profile bootstrap-osd"})
	if err != nil {
		return fmt.Errorf("failed to get or create osd auth key %s. %+v", bootstrapOSDKeyringPath, err)
	}

	logger.Debugf("succeeded bootstrap OSD get/create key, bootstrapOSDKey: %s", bootstrapOSDKey)

	// write the bootstrap-osd keyring to disk
	bootstrapOSDKeyringDir := filepath.Dir(bootstrapOSDKeyringPath)
	if err := os.MkdirAll(bootstrapOSDKeyringDir, 0744); err != nil {
		return fmt.Errorf("failed to create bootstrap OSD keyring dir at %s: %+v", bootstrapOSDKeyringDir, err)
	}

	bootstrapOSDKeyring := fmt.Sprintf(bootstrapOSDKeyringTemplate, bootstrapOSDKey)
	logger.Debugf("Writing osd keyring to: %s", bootstrapOSDKeyring)
	if err := ioutil.WriteFile(bootstrapOSDKeyringPath, []byte(bootstrapOSDKeyring), 0644); err != nil {
		return fmt.Errorf("failed to write bootstrap-osd keyring to %s: %+v", bootstrapOSDKeyringPath, err)
	}

	return nil
}
