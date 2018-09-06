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

package mon

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

const (
	// The final string field is for the admin keyring
	monitorKeyringTemplate = `
	[mon.]
		key = %s
		caps mon = "allow *"

	%s`
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephmon")

// GetMonRunDirPath returns the path of a given monitor's run dir
func GetMonRunDirPath(configDir, monName string) string {
	if strings.Index(monName, "mon") == -1 {
		// if the mon name doesn't have "mon" in it, include it in the directory
		return path.Join(configDir, "mon-"+monName)
	}
	return path.Join(configDir, monName)
}

// get the path of a given monitor's keyring
func getMonKeyringPath(configDir, monName string) string {
	return filepath.Join(GetMonRunDirPath(configDir, monName), cephconfig.DefaultKeyringFile)
}

// get the path of a given monitor's data dir
func getMonDataDirPath(configDir, monName string) string {
	return filepath.Join(GetMonRunDirPath(configDir, monName), "data")
}

func writeMonKeyring(context *clusterd.Context, c *cephconfig.ClusterInfo, name string) error {
	keyringPath := getMonKeyringPath(context.ConfigDir, name)
	keyring := fmt.Sprintf(monitorKeyringTemplate, c.MonitorSecret, cephconfig.AdminKeyring(c))
	return cephconfig.WriteKeyring(keyringPath, "", func(_ string) string { return keyring })
}

// generates and writes the monitor config file to disk
func generateConnectionConfigFile(context *clusterd.Context, cluster *cephconfig.ClusterInfo, pathRoot, user, keyringPath string) (string, error) {
	return cephconfig.GenerateConfigFile(context, cluster, pathRoot, user, keyringPath, nil, nil)
}

// writes the monitor backend file to disk
func writeBackendFile(monDataDir, backend string) error {
	backendPath := filepath.Join(monDataDir, "kv_backend")
	if err := ioutil.WriteFile(backendPath, []byte(backend), 0644); err != nil {
		return fmt.Errorf("failed to write kv_backend to %s: %+v", backendPath, err)
	}
	return nil
}

func generateMonMap(context *clusterd.Context, cluster *cephconfig.ClusterInfo, folder string) (string, error) {
	path := path.Join(folder, "monmap")
	args := []string{path, "--create", "--clobber", "--fsid", cluster.FSID}
	for _, mon := range cluster.Monitors {
		args = append(args, "--add", mon.Name, mon.Endpoint)
	}

	err := context.Executor.ExecuteCommand(false, "", "monmaptool", args...)
	if err != nil {
		return "", fmt.Errorf("failed to generate monmap. %+v", err)
	}

	return path, nil
}
