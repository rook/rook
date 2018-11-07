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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

const (
	// AdminKeyringTemplate is a string template of Ceph keyring settings which allow connection
	// as admin. The key value must be filled in by the admin auth key for the cluster.
	AdminKeyringTemplate = `
	[client.admin]
		key = %s
		auid = 0
		caps mds = "allow *"
		caps mon = "allow *"
		caps osd = "allow *"
		caps mgr = "allow *"
	`
)

// AdminKeyring returns the filled-out admin keyring
func AdminKeyring(c *ClusterInfo) string {
	return fmt.Sprintf(AdminKeyringTemplate, c.AdminSecret)
}

// WriteKeyring calls the generate contents function with auth key as an argument then saves the
// output of the generateContents function to disk at the keyring path
// TODO: Kludgey; can keyring files be generated w/ go-ini package or using the '-o' option to
// 'ceph auth get-or-create ...'?
func WriteKeyring(keyringPath, authKey string, generateContents func(string) string) error {
	contents := generateContents(authKey)
	return writeKeyring(contents, keyringPath)
}

// CreateKeyring creates a keyring for access to the cluster with the desired set of privileges
// and writes it to disk at the keyring path
func CreateKeyring(context *clusterd.Context, clusterName, username, keyringPath string, access []string, generateContents func(string) string) error {
	_, err := os.Stat(keyringPath)
	if err == nil {
		// no error, the file exists, bail out with no error
		logger.Debugf("keyring already exists at %s", keyringPath)
		return nil
	} else if !os.IsNotExist(err) {
		// some other error besides "does not exist", bail out with error
		return fmt.Errorf("failed to stat %s: %+v", keyringPath, err)
	}

	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	return WriteKeyring(keyringPath, key, generateContents)
}

// writes the keyring to disk
// TODO: Write keyring only to the default ceph config location since we are in a container
func writeKeyring(keyring, keyringPath string) error {
	// save the keyring to the given path
	if err := os.MkdirAll(filepath.Dir(keyringPath), 0744); err != nil {
		return fmt.Errorf("failed to create keyring directory for %s: %+v", keyringPath, err)
	}
	if err := ioutil.WriteFile(keyringPath, []byte(keyring), 0644); err != nil {
		return fmt.Errorf("failed to write monitor keyring to %s: %+v", keyringPath, err)
	}

	// Save the keyring to the default path. This allows the user to any pod to easily execute Ceph commands.
	// It is recommended to connect to the operator pod rather than monitors and OSDs since the operator always has the latest configuration files.
	// The mon and OSD pods will only re-create the config files when the pod is restarted. If a monitor fails over, the config
	// in the other mon and osd pods may be out of date. This could cause your ceph commands to timeout connecting to invalid mons.
	// Note that the running mon and osd daemons are not affected by this issue because of their live connection to the mon quorum.
	// If you have multiple Rook clusters, it is preferred to connect to the Rook toolbox for a specific cluster. Otherwise, your ceph commands
	// may connect to the wrong cluster.
	if err := os.MkdirAll(DefaultConfigDir, 0744); err != nil {
		logger.Warningf("failed to create default directory %s: %+v", DefaultConfigDir, err)
		return nil
	}
	defaultPath := DefaultKeyringFilePath()
	if err := ioutil.WriteFile(defaultPath, []byte(keyring), 0644); err != nil {
		logger.Warningf("failed to copy keyring to %s: %+v", defaultPath, err)
		return nil
	}

	return nil
}
