/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package util

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/coreos/pkg/capnslog"
)

const (
	RBDSysBusPathDefault = "/sys/bus/rbd"
	RBDDevicesDir        = "devices"
	RBDDevicePathPrefix  = "/dev/rbd"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-ceph-util")

// FindRBDMappedFile search for the mapped RBD volume and returns its device path
func FindRBDMappedFile(imageName, poolName, sysBusDir string) (string, error) {

	sysBusDeviceDir := filepath.Join(sysBusDir, RBDDevicesDir)
	// if sysPath does not exist, no attachments has happened
	if _, err := os.Stat(sysBusDeviceDir); os.IsNotExist(err) {
		return "", nil
	}

	files, err := ioutil.ReadDir(sysBusDeviceDir)
	if err != nil {
		return "", fmt.Errorf("failed to read rbd device dir: %+v", err)
	}

	for _, idFile := range files {
		nameContent, err := ioutil.ReadFile(filepath.Join(sysBusDeviceDir, idFile.Name(), "name"))
		if err == nil && imageName == strings.TrimSpace(string(nameContent)) {
			// the image for the current rbd device matches, now try to match pool
			poolContent, err := ioutil.ReadFile(filepath.Join(sysBusDeviceDir, idFile.Name(), "pool"))
			if err == nil && poolName == strings.TrimSpace(string(poolContent)) {
				// match current device matches both image name and pool name, return the device
				return idFile.Name(), nil
			}
		}
	}
	return "", nil
}

// GetIPFromEndpoint return the IP from an endpoint string (192.168.0.1:6789)
func GetIPFromEndpoint(endpoint string) string {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		logger.Errorf("failed to split ip and port for endpoint %s", endpoint)
	}
	return host
}

// GetPortFromEndpoint return the port from an endpoint string (192.168.0.1:6789)
func GetPortFromEndpoint(endpoint string) int32 {
	var port int
	_, portString, err := net.SplitHostPort(endpoint)
	if err != nil {
		logger.Errorf("failed to split host and port for endpoint %s, assuming default Ceph port %s", endpoint, portString)
	} else {
		port, err = strconv.Atoi(portString)
		if err != nil {
			logger.Errorf("failed to convert %s to integer", portString)
		}
	}
	return int32(port)
}
