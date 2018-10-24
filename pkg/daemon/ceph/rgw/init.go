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

package rgw

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephrgw")

const (
	keyringTemplate = `[client.radosgw.gateway]
key = %s
caps mon = "allow rw"
caps osd = "allow *"
`
)

// Config contains the necessary parameters Rook needs to know to set up a rgw for a Ceph cluster.
type Config struct {
	Name            string
	Host            string
	Port            int
	SecurePort      int
	Keyring         string
	CertificatePath string
	ClusterInfo     *cephconfig.ClusterInfo
}

// Initialize generates configuration files for a Ceph rgw.
func Initialize(context *clusterd.Context, config *Config) error {
	// log the arguments for the rgw without leaking sensitive cluster info.
	logger.Infof("Creating config for rgw %s - config. %+v", config.Name,
		copyConfigWithoutClusterInfo(config))
	config.ClusterInfo.Log(logger) // log cluster info safely

	configPath := cephconfig.DefaultConfigFilePath()
	keyringPath := cephconfig.DaemonKeyringFilePath(cephconfig.VarLibCephDir, "rgw", config.Name)
	runDir := cephconfig.DaemonRunDir(cephconfig.VarLibCephDir, "rgw", config.Name)
	username := "client.radosgw.gateway"
	settings := map[string]string{
		"host":                           config.Host,
		"rgw log nonexistent bucket":     "true",
		"rgw intent log object name utc": "true",
		"rgw enable usage log":           "true",
		"rgw_frontends":                  fmt.Sprintf("civetweb port=%s", portString(config)),
		"rgw_zone":                       config.Name,
		"rgw_zonegroup":                  config.Name,
	}
	err := cephconfig.GenerateConfigFile(context, config.ClusterInfo,
		configPath, username, keyringPath, runDir, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create config file. %+v", err)
	}

	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, key)
	}
	if err := cephconfig.WriteKeyring(keyringPath, config.Keyring, keyringEval); err != nil {
		return fmt.Errorf("failed to save keyring. %+v", err)
	}

	// create the rgw run directory (where the mime types file goes)
	if err := os.MkdirAll(runDir, 0744); err != nil {
		logger.Warningf("failed to create data directory %s. %+v", runDir, err)
	}

	// write the mime types config
	mimeTypesPath := GetMimeTypesPath(runDir)
	logger.Infof("Writing mime types to %s", mimeTypesPath)
	if err := ioutil.WriteFile(mimeTypesPath, []byte(mimeTypes), 0644); err != nil {
		return fmt.Errorf("failed to write mime types to %s. %+v", mimeTypesPath, err)
	}

	return nil
}

func copyConfigWithoutClusterInfo(c *Config) *Config {
	r := new(Config)
	*r = *c
	r.ClusterInfo = nil
	return r
}

func portString(config *Config) string {
	var portString string
	if config.Port != 0 {
		portString = strconv.Itoa(config.Port)
	}
	if config.SecurePort != 0 && config.CertificatePath != "" {
		var separator string
		if config.Port != 0 {
			separator = "+"
		}
		// the suffix is intended to be appended to the end of the rgw_frontends arg, immediately after the port.
		// with ssl enabled, the port number must end with the letter s.
		portString = fmt.Sprintf("%s%s%ds ssl_certificate=%s", portString, separator, config.SecurePort, config.CertificatePath)
	}

	return portString
}

// GetMimeTypesPath returns the path to the mime types file for the rgw in its run dir.
func GetMimeTypesPath(runDir string) string {
	return path.Join(runDir, "mime.types")
}
