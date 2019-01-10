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
	"github.com/rook/rook/pkg/util"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephrgw")

const (
	keyringTemplate = `[client.radosgw.gateway]
key = %s
caps mon = "allow rw"
caps osd = "allow *"
`
)

type Config struct {
	Name            string
	Host            string
	Port            int
	SecurePort      int
	Keyring         string
	CertificatePath string
	ClusterInfo     *cephconfig.ClusterInfo
}

func Initialize(context *clusterd.Context, config *Config) error {

	err := generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate rgw config files. %+v", err)
	}

	return err
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

func generateConfigFiles(context *clusterd.Context, config *Config) error {

	// create the rgw data directory
	dataDir := path.Join(getRGWConfDir(context.ConfigDir), "data")
	if err := os.MkdirAll(dataDir, 0744); err != nil {
		logger.Warningf("failed to create data directory %s: %+v", dataDir, err)
	}

	settings := map[string]string{
		"host":                           config.Host,
		"rgw data":                       dataDir,
		"rgw log nonexistent bucket":     "true",
		"rgw intent log object name utc": "true",
		"rgw enable usage log":           "true",
		"rgw_frontends":                  fmt.Sprintf("civetweb port=%s", portString(config)),
		"rgw_zone":                       config.Name,
		"rgw_zonegroup":                  config.Name,
	}
	configFile, err := cephconfig.GenerateConfigFile(context, config.ClusterInfo, getRGWConfDir(context.ConfigDir),
		"client.radosgw.gateway", getRGWKeyringPath(context.ConfigDir), nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create config file. %+v", err)
	}
	util.WriteFileToLog(logger, configFile)

	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, key)
	}

	// create rgw config
	err = cephconfig.WriteKeyring(getRGWKeyringPath(context.ConfigDir), config.Keyring, keyringEval)
	if err != nil {
		return fmt.Errorf("failed to save keyring. %+v", err)
	}

	// write the mime types config
	mimeTypesPath := GetMimeTypesPath(context.ConfigDir)
	logger.Infof("Writing mime types to: %s", mimeTypesPath)
	if err := ioutil.WriteFile(mimeTypesPath, []byte(mimeTypes), 0644); err != nil {
		return fmt.Errorf("failed to write mime types to %s: %+v", mimeTypesPath, err)
	}

	return nil
}

func getRGWConfDir(configDir string) string {
	return path.Join(configDir, "rgw")
}

func getRGWKeyringPath(configDir string) string {
	return path.Join(getRGWConfDir(configDir), "keyring")
}

func GetMimeTypesPath(configDir string) string {
	return path.Join(getRGWConfDir(configDir), "mime.types")
}
