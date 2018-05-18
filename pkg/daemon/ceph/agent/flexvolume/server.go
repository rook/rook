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
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"path"
	"regexp"
	"strings"

	"k8s.io/kubernetes/pkg/util/version"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	UnixSocketName           = ".rook.sock"
	FlexvolumeVendor         = "ceph.rook.io"
	FlexvolumeVendorLegacy   = "rook.io"
	FlexDriverName           = "rook"
	flexvolumeDriverFileName = "rookflex"
	flexMountPath            = "/flexmnt/%s~%s"
	usrBinDir                = "/usr/local/bin/"
	serverVersionV180        = "v1.8.0"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "flexvolume")

// FlexvolumeServer start a unix domain socket server to interact with the flexvolume driver
type FlexvolumeServer struct {
	context    *clusterd.Context
	controller *Controller
	listeners  map[string]net.Listener
}

// NewFlexvolumeServer creates an Flexvolume server
func NewFlexvolumeServer(context *clusterd.Context, controller *Controller, manager VolumeManager) *FlexvolumeServer {
	return &FlexvolumeServer{
		context:    context,
		controller: controller,
		listeners:  make(map[string]net.Listener),
	}
}

// Start configures the flexvolume driver on the host and starts the unix domain socket server to communicate with the driver
func (s *FlexvolumeServer) Start(driverVendor, driverName string) error {

	// first install the flexvolume driver
	// /usr/local/bin/rookflex
	driverFile := path.Join(usrBinDir, flexvolumeDriverFileName)
	// /flexmnt/rook.io~rook-system
	flexVolumeDriverDir := fmt.Sprintf(flexMountPath, driverVendor, driverName)

	err := configureFlexVolume(driverFile, flexVolumeDriverDir, driverName)
	if err != nil {
		return fmt.Errorf("unable to configure flexvolume %s: %v", flexVolumeDriverDir, err)
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
		return fmt.Errorf("unable to listen at %s: %v", unixSocketFile, err)
	}
	s.listeners[unixSocketFile] = listener

	if err := os.Chmod(unixSocketFile, 0770); err != nil {
		return fmt.Errorf("unable to set file permission to unix socket %s: %v", unixSocketFile, err)
	}

	go rpc.Accept(listener)

	logger.Infof("Listening on unix socket for Kubernetes volume attach commands: %s", unixSocketFile)

	// flexvolume driver was installed OK.  If running on pre 1.8 Kubernetes, then remind the user
	// to restart the Kubelet. We do this last so that it's the last message in the log, making it
	// harder for the user to miss.
	checkIfKubeletRestartRequired(s.context)

	return nil
}

// Stop the unix domain socket server and deletes the socket file
func (s *FlexvolumeServer) StopAll() {
	logger.Infof("Stopping %d unix socket rpc server(s).", len(s.listeners))
	for unixSocketFile, listener := range s.listeners {
		if err := listener.Close(); err != nil {
			logger.Errorf("Failed to stop unix socket rpc server: %+v", err)
		}

		// closing the listener should remove the unix socket file. But lets try it remove it just in case.
		if _, err := os.Stat(unixSocketFile); !os.IsNotExist(err) {
			logger.Infof("Deleting unix domain socket file %s.", unixSocketFile)
			os.Remove(unixSocketFile)
		}
	}
	s.listeners = make(map[string]net.Listener)
}

func RookDriverName(context *clusterd.Context) (string, error) {
	kubeVersion, err := k8sutil.GetK8SVersion(context.Clientset)
	if err != nil {
		return "", fmt.Errorf("Error getting server version: %v", err)
	}
	// K8s 1.7 returns an error when trying to run multiple drivers under the same rook.io provider,
	// so we will fall back to the rook driver name in that case.
	if kubeVersion.AtLeast(version.MustParseSemantic(serverVersionV180)) {
		// the driver name needs to be the same as the namespace so that we can support multiple namespaces
		// without the drivers conflicting with each other
		return os.Getenv(k8sutil.PodNamespaceEnvVar), nil
	}
	// fall back to the rook driver name where multiple system namespaces are not supported
	return FlexDriverName, nil
}

func configureFlexVolume(driverFile, driverDir, driverName string) error {
	// copying flex volume
	if _, err := os.Stat(driverDir); os.IsNotExist(err) {
		err := os.Mkdir(driverDir, 0755)
		if err != nil {
			logger.Errorf("failed to create dir %s. %+v", driverDir, err)
		}
	}

	destFile := path.Join(driverDir, "."+driverName)  // /flextmnt/rook.io~rook-system/.rook-system
	finalDestFile := path.Join(driverDir, driverName) // /flextmnt/rook.io~rook-system/rook-system
	err := copyFile(driverFile, destFile)
	if err != nil {
		return fmt.Errorf("unable to copy flexvolume from %s to %s: %+v", driverFile, destFile, err)
	}

	// renaming flex volume. Rename is an atomic execution while copying is not.
	if _, err := os.Stat(finalDestFile); !os.IsNotExist(err) {
		// Delete old plugin if it exists
		err = os.Remove(finalDestFile)
		if err != nil {
			logger.Warningf("Could not delete old Rook Flexvolume driver at %s: %v", finalDestFile, err)
		}

	}

	if err := os.Rename(destFile, finalDestFile); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %+v", destFile, finalDestFile, err)
	}

	return nil
}

func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening source file %s: %v", src, err)
	}
	defer srcFile.Close()

	destFile, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755) // creates if file doesn't exist
	if err != nil {
		return fmt.Errorf("error creating destination file %s: %v", dest, err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("error copying file from %s to %s: %v", src, dest, err)
	}
	return destFile.Sync()
}

func checkIfKubeletRestartRequired(context *clusterd.Context) {
	kubeVersion, err := k8sutil.GetK8SVersion(context.Clientset)
	if err != nil || kubeVersion.LessThan(version.MustParseSemantic(serverVersionV180)) {
		logger.Warning("NOTE: The Kubelet must be restarted on this node since this pod appears to " +
			"be running on a Kubernetes version prior to 1.8. More details can be found in the Rook docs at " +
			"https://rook.io/docs/rook/master/common-issues.html#kubelet-restart")
	}
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
				return "", "", fmt.Errorf("unexpected number of items in flex driver info %+v from path %s", flexInfo, flexDriverPath)
			}

			return flexInfo[0], flexInfo[1], nil
		}
	}

	return "", "", fmt.Errorf("failed to find flex driver info from path %s", flexDriverPath)
}
