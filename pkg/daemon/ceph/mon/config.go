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
	"net"
	"path"
	"path/filepath"
	"strings"

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

// GetMonRunDirPath returns the path of a given monitor's run dir. The run dir is where a mon's
// monmap file should be stored.
func GetMonRunDirPath(configDir, monName string) string {
	if strings.Index(monName, "mon") == -1 {
		// if the mon name doesn't have "mon" in it, include it in the directory
		return path.Join(configDir, "mon-"+monName)
	}
	return path.Join(configDir, monName)
}

// GetMonDataDirPath returns the path of a given monitor's data dir. The data dir is where a mon's
// '--mon-data' flag should be configured.
func GetMonDataDirPath(configDir, monName string) string {
	return filepath.Join(GetMonRunDirPath(configDir, monName), "data")
}

func writeMonKeyring(context *clusterd.Context, c *cephconfig.ClusterInfo, name string) error {
	keyringPath := getMonKeyringPath(context.ConfigDir, name)
	keyring := fmt.Sprintf(monitorKeyringTemplate, c.MonitorSecret, cephconfig.AdminKeyring(c))
	return cephconfig.WriteKeyring(keyringPath, "", func(_ string) string { return keyring })
}

// getMonKeyringPath gets the path of a given monitor's keyring
func getMonKeyringPath(configDir, monName string) string {
	return filepath.Join(GetMonRunDirPath(configDir, monName), cephconfig.DefaultKeyringFile)
}

// generateConnectionConfigFile generates and writes the monitor config file to disk
func generateConnectionConfigFile(context *clusterd.Context, config *Config, pathRoot, keyringPath string) (string, error) {
	// public_bind_addr is set from the pod IP which can only be known at runtime, so set this
	// at config init int the Ceph config file.
	// See pkg/operator/ceph/cluster/mon/spec.go - makeMonDaemonContainer() comment notes for more
	privateAddr := net.JoinHostPort(context.NetworkInfo.ClusterAddr, fmt.Sprintf("%d", config.Port))
	settings := map[string]string{
		"public bind addr": privateAddr,
	}

	// The user is "mon.<mon-name>" for mon config items
	clientUser := fmt.Sprintf("mon.%s", config.Name)

	return cephconfig.GenerateConfigFile(
		context,
		config.Cluster,
		pathRoot, clientUser, keyringPath,
		nil,
		settings,
	)
}
