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

	"path"
	"path/filepath"

	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

const (
	keyringFileName     = "keyring"
	bootstrapOsdKeyring = "bootstrap-osd/ceph.keyring"
)

// get the bootstrap OSD root dir
func getBootstrapOSDDir(configDir string) string {
	return path.Join(configDir, "bootstrap-osd")
}

func getOSDRootDir(root string, osdID int) string {
	return filepath.Join(root, fmt.Sprintf("osd%d", osdID))
}

// get the full path to the given OSD's config file
func getOSDConfFilePath(osdDataPath, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", osdDataPath, clusterName)
}

// get the full path to the given OSD's keyring
func getOSDKeyringPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, keyringFileName)
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
func createOSDBootstrapKeyring(context *clusterd.Context, clusterName, rootDir string) error {
	username := "client.bootstrap-osd"
	keyringPath := path.Join(rootDir, bootstrapOsdKeyring)
	access := []string{"mon", "allow profile bootstrap-osd"}
	keyringEval := func(key string) string {
		return fmt.Sprintf(bootstrapOSDKeyringTemplate, key)
	}

	return cephconfig.CreateKeyring(context, clusterName, username, keyringPath, access, keyringEval)
}
