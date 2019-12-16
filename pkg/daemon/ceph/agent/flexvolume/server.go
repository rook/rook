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

// Package flexvolume to manage Kubernetes storage attach events.
package flexvolume

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/kubernetes/pkg/volume/flexvolume"
)

const (
	UnixSocketName           = ".rook.sock"
	FlexvolumeVendor         = "ceph.rook.io"
	FlexvolumeVendorLegacy   = "rook.io"
	FlexDriverName           = "rook"
	flexvolumeDriverFileName = "rookflex"
	flexMountPath            = "/flexmnt/%s~%s"
	usrBinDir                = "/usr/local/bin/"
	settingsFilename         = "flex.config"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "flexvolume")

// FlexvolumeServer start a unix domain socket server to interact with the flexvolume driver
type FlexvolumeServer struct {
	context    *clusterd.Context
	controller *Controller
	listeners  map[string]net.Listener
}

// NewFlexvolumeServer creates an Flexvolume server
func NewFlexvolumeServer(context *clusterd.Context, controller *Controller) *FlexvolumeServer {
	return &FlexvolumeServer{
		context:    context,
		controller: controller,
		listeners:  make(map[string]net.Listener),
	}
}

// Start configures the flexvolume driver on the host and starts the unix domain socket server to communicate with the driver
func (s *FlexvolumeServer) Start(driverVendor, driverName string) error {
	driverFile := path.Join(getRookFlexBinaryPath(), flexvolumeDriverFileName)
	// /flexmnt/rook.io~rook-system
	flexVolumeDriverDir := fmt.Sprintf(flexMountPath, driverVendor, driverName)

	err := configureFlexVolume(driverFile, flexVolumeDriverDir, driverName)
	if err != nil {
		return errors.Wrapf(err, "unable to configure flexvolume %s", flexVolumeDriverDir)
	}

	unixSocketFile := path.Join(flexVolumeDriverDir, UnixSocketName) // /flextmnt/rook.io~rook-system/.rook.sock
	if _, ok := s.listeners[unixSocketFile]; ok {
		logger.Infof("flex server already running at %s", unixSocketFile)
		return nil
	}

	// remove unix socket if it existed previously
	if _, err := os.Stat(unixSocketFile); !os.IsNotExist(err) {
		logger.Info("Deleting unix domain socket file.")
		os.Remove(unixSocketFile)
	}

	listener, err := net.Listen("unix", unixSocketFile)
	if err != nil {
		return errors.Wrapf(err, "unable to listen at %q", unixSocketFile)
	}
	s.listeners[unixSocketFile] = listener

	if err := os.Chmod(unixSocketFile, 0770); err != nil {
		return errors.Wrapf(err, "unable to set file permission to unix socket %q", unixSocketFile)
	}

	go rpc.Accept(listener)

	logger.Infof("listening on unix socket for Kubernetes volume attach commands %q", unixSocketFile)
	return nil
}

// StopAll Stop the unix domain socket server and deletes the socket file
func (s *FlexvolumeServer) StopAll() {
	logger.Infof("Stopping %d unix socket rpc server(s).", len(s.listeners))
	for unixSocketFile, listener := range s.listeners {
		if err := listener.Close(); err != nil {
			logger.Errorf("failed to stop unix socket rpc server. %v", err)
		}

		// closing the listener should remove the unix socket file. But lets try it remove it just in case.
		if _, err := os.Stat(unixSocketFile); !os.IsNotExist(err) {
			logger.Infof("deleting unix domain socket file %q.", unixSocketFile)
			os.Remove(unixSocketFile)
		}
	}
	s.listeners = make(map[string]net.Listener)
}

// RookDriverName return the Kubernetes version appropriate Rook driver name
func RookDriverName(context *clusterd.Context) (string, error) {
	// the driver name needs to be the same as the namespace so that we can support multiple namespaces
	// without the drivers conflicting with each other
	return os.Getenv(k8sutil.PodNamespaceEnvVar), nil
}

// TouchFlexDrivers causes k8s to reload the flex volumes. Needed periodically due to a k8s race condition with flex driver loading.
func TouchFlexDrivers(vendor, driverName string) {
	filename := path.Join(fmt.Sprintf(flexMountPath, vendor, driverName), driverName)
	logger.Debugf("reloading flex drivers. touching %q", filename)

	currenttime := time.Now().Local()
	err := os.Chtimes(filename, currenttime, currenttime)
	if err != nil {
		logger.Warningf("failed to touch file %s", filename)
	}
}

// Encode the flex settings in json
func generateFlexSettings(enableSELinuxRelabeling, enableFSGroup bool) ([]byte, error) {
	status := flexvolume.DriverStatus{
		Status: flexvolume.StatusSuccess,
		Capabilities: &flexvolume.DriverCapabilities{
			Attach: false,
			// Required for metrics
			SupportsMetrics: true,
			// Required for any mount performed on a host running selinux
			SELinuxRelabel:   enableSELinuxRelabeling,
			FSGroup:          enableFSGroup,
			RequiresFSResize: true,
		},
	}
	result, err := json.Marshal(status)
	if err != nil {
		return nil, errors.Wrapf(err, "Invalid flex settings")
	}
	return result, nil
}

// The flex settings must be loaded from a file next to the flex driver since there is context
// that can be used other than the directory where the flex driver is running.
// This method cannot write to stdout since it is running in the context of the kubelet
// which only expects the json settings to be output.
func LoadFlexSettings(directory string) []byte {
	// Load the settings from the expected config file, ensure they are valid settings, then return them in
	// a json string to the caller
	var status flexvolume.DriverStatus
	if output, err := ioutil.ReadFile(path.Join(directory, settingsFilename)); err == nil {
		if err := json.Unmarshal(output, &status); err == nil {
			if output, err = json.Marshal(status); err == nil {
				return output
			}
		}
	}

	// If there is an error loading settings, set the defaults
	settings, err := generateFlexSettings(true, true)
	if err != nil {
		// Never expect this to happen since we'll validate settings in the build
		return nil
	}
	return settings
}

func configureFlexVolume(driverFile, driverDir, driverName string) error {
	// copying flex volume
	if _, err := os.Stat(driverDir); os.IsNotExist(err) {
		err := os.Mkdir(driverDir, 0755)
		if err != nil {
			logger.Errorf("failed to create dir %q. %v", driverDir, err)
		}
	}

	destFile := path.Join(driverDir, "."+driverName)  // /flextmnt/rook.io~rook-system/.rook-system
	finalDestFile := path.Join(driverDir, driverName) // /flextmnt/rook.io~rook-system/rook-system
	err := copyFile(driverFile, destFile)
	if err != nil {
		return errors.Wrapf(err, "unable to copy flexvolume from %q to %q", driverFile, destFile)
	}

	// renaming flex volume. Rename is an atomic execution while copying is not.
	if _, err := os.Stat(finalDestFile); !os.IsNotExist(err) {
		// Delete old plugin if it exists
		err = os.Remove(finalDestFile)
		if err != nil {
			logger.Warningf("Could not delete old Rook Flexvolume driver at %q. %v", finalDestFile, err)
		}

	}

	if err := os.Rename(destFile, finalDestFile); err != nil {
		return errors.Wrapf(err, "failed to rename %q to %q", destFile, finalDestFile)
	}

	// Write the flex configuration
	enableSELinuxRelabeling, err := strconv.ParseBool(os.Getenv(agent.RookEnableSelinuxRelabelingEnv))
	if err != nil {
		logger.Errorf("invalid value for disabling SELinux relabeling. %v", err)
		enableSELinuxRelabeling = true
	}
	enableFSGroup, err := strconv.ParseBool(os.Getenv(agent.RookEnableFSGroupEnv))
	if err != nil {
		logger.Errorf("invalid value for disabling fs group. %v", err)
		enableFSGroup = true
	}
	settings, err := generateFlexSettings(enableSELinuxRelabeling, enableFSGroup)
	if err != nil {
		logger.Errorf("invalid flex settings. %v", err)
	} else {
		if err := ioutil.WriteFile(path.Join(driverDir, settingsFilename), settings, 0644); err != nil {
			logger.Errorf("failed to write settings file %q. %v", settingsFilename, err)
		} else {
			logger.Debugf("flex settings: %q", string(settings))
		}
	}

	return nil
}

func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return errors.Wrapf(err, "error opening source file %s", src)
	}
	defer srcFile.Close()

	destFile, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755) // creates if file doesn't exist
	if err != nil {
		return errors.Wrapf(err, "error creating destination file %s", dest)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return errors.Wrapf(err, "error copying file from %s to %s", src, dest)
	}
	return destFile.Sync()
}

// Gets the flex driver info (vendor, driver name) from a given path where the flex driver exists.
// The given path may look something like this:
// /usr/libexec/kubernetes/kubelet-plugins/volume/exec/rook.io~rook/rook/
// In which case, the vendor is rook.io and the driver name is rook.
func getFlexDriverInfo(flexDriverPath string) (vendor, driver string, err error) {
	parts := strings.Split(flexDriverPath, string(os.PathSeparator))
	for i := len(parts) - 1; i >= 0; i-- {
		p := parts[i]
		if matched, _ := regexp.Match(".+~.+", []byte(p)); matched {
			// found a match for the flex driver directory name pattern
			flexInfo := strings.Split(p, "~")
			if len(flexInfo) > 2 {
				return "", "", errors.Errorf("unexpected number of items in flex driver info %+v from path %s", flexInfo, flexDriverPath)
			}

			return flexInfo[0], flexInfo[1], nil
		}
	}

	return "", "", errors.Errorf("failed to find flex driver info from path %s", flexDriverPath)
}

// getRookFlexBinaryPath returns the path of rook flex volume driver
func getRookFlexBinaryPath() string {
	p, err := exec.LookPath(flexvolumeDriverFileName)
	if err != nil {
		return usrBinDir
	}
	return path.Dir(p)
}
