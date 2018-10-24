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

// writes the keyring to disk
//
// For all daemons except mon, the keyring file should be written to both the daemon's data dir
// (e.g., /var/lib/ceph/mgr-a) as well as the default location. The mon keyring has the admin key on
// it, so persisting it to disk is less secure. When specifying "keyring" in the Ceph config file or
// "--keyring" on the CLI for the Ceph daemon, daemons still insist on looking for their keyring in
// their data dir. Therefore, the keyring code cannot be simplified for daemons by writing the
// keyring only to /etc/ceph. :sad-face:
func writeKeyring(keyring, keyringPath string) error {
	logger.Infof("writing keyring to %s", keyringPath)
	// save the keyring to the given path
	if err := os.MkdirAll(filepath.Dir(keyringPath), 0744); err != nil {
		return fmt.Errorf("failed to create keyring directory for %s. %+v", keyringPath, err)
	}
	if err := ioutil.WriteFile(keyringPath, []byte(keyring), 0644); err != nil {
		return fmt.Errorf("failed to write monitor keyring to %s. %+v", keyringPath, err)
	}

	// Save the keyring to the default path. This allows the user to any pod to easily execute Ceph commands.
	// It is recommended to connect to the operator pod rather than monitors and OSDs since the operator always has the latest configuration files.
	// The mon and OSD pods will only re-create the config files when the pod is restarted. If a monitor fails over, the config
	// in the other mon and osd pods may be out of date. This could cause your ceph commands to timeout connecting to invalid mons.
	// Note that the running mon and osd daemons are not affected by this issue because of their live connection to the mon quorum.
	// If you have multiple Rook clusters, it is preferred to connect to the Rook toolbox for a specific cluster. Otherwise, your ceph commands
	// may connect to the wrong cluster.
	if err := os.MkdirAll(EtcCephDir, 0744); err != nil {
		logger.Warningf("failed to create default directory %s. %+v", EtcCephDir, err)
		return nil
	}
	defaultPath := DefaultKeyringFilePath()
	logger.Infof("copying keyring to default location %s", defaultPath)
	if err := ioutil.WriteFile(defaultPath, []byte(keyring), 0644); err != nil {
		logger.Warningf("failed to copy keyring to %s. %+v", defaultPath, err)
		return nil
	}

	return nil
}
