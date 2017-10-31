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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
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

	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/util/version"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	UnixSocketName           = ".rook.sock"
	FlexvolumeVendor         = "rook.io"
	FlexvolumeDriver         = "rook"
	flexvolumeDriverFileName = "rookflex"
	usrBinDir                = "/usr/local/bin/"
	serverVersionV170        = "v1.7.0"
	serverVersionV180        = "v1.8.0"
)

var flexVolumeDriverDir = fmt.Sprintf("/flexmnt/%s~%s", FlexvolumeVendor, FlexvolumeDriver)
var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook-flexvolume")

// FlexvolumeServer start a unix domain socket server to interact with the flexvolume driver
type FlexvolumeServer struct {
	context    *clusterd.Context
	controller *FlexvolumeController
	listener   net.Listener
}

// NewFlexvolumeServer creates an Flexvolume server
func NewFlexvolumeServer(context *clusterd.Context, volumeAttachmentClient rest.Interface, manager VolumeManager) (*FlexvolumeServer, error) {
	controller, err := newFlexvolumeController(context, volumeAttachmentClient, manager)
	if err != nil {
		return nil, err
	}
	return &FlexvolumeServer{
		context:    context,
		controller: controller,
	}, nil
}

// Start configures the flexvolume driver on the host and starts the unix domain socket server to communicate with the driver
func (s *FlexvolumeServer) Start() error {

	// first install the flexvolume driver
	driverFile := path.Join(usrBinDir, flexvolumeDriverFileName) // /usr/local/bin/rookflex
	err := configureFlexVolume(driverFile, flexVolumeDriverDir)
	if err != nil {
		return fmt.Errorf("unable to configure flexvolume: %v", err)
	}

	err = rpc.Register(s.controller)
	if err != nil {
		return fmt.Errorf("unable to register rpc: %v", err)
	}
	logger.Info("Rook Flexvolume configured")

	unixSocketFile := path.Join(flexVolumeDriverDir, path.Join(UnixSocketName)) // /flextmnt/rook.io~rook/.rook.sock

	// remove unix socket if it existed previously
	if _, err := os.Stat(unixSocketFile); !os.IsNotExist(err) {
		logger.Info("Deleting unix domain socket file.")
		os.Remove(unixSocketFile)
	}

	s.listener, err = net.Listen("unix", unixSocketFile)
	if err != nil {
		return fmt.Errorf("unable to listen at %s: %v", unixSocketFile, err)
	}

	if err := os.Chmod(unixSocketFile, 0770); err != nil {
		return fmt.Errorf("unable to set file permission to unix socket %s: %v", unixSocketFile, err)
	}

	go rpc.Accept(s.listener)

	logger.Info("Listening on unix socket for Kubernetes volume attach commands.")

	// flexvolume driver was installed OK.  If running on pre 1.8 Kubernetes, then remind the user
	// to restart the Kubelet. We do this last so that it's the last message in the log, making it
	// harder for the user to miss.
	checkIfKubeletRestartRequired(s.context)

	return nil
}

// Stop the unix domain socket server and deletes the socket file
func (s *FlexvolumeServer) Stop() {
	if s.listener != nil {
		logger.Info("Stopping unix socket rpc server.")
		if err := s.listener.Close(); err != nil {
			logger.Errorf("Failed to stop unix socket rpc server: %+v", err)
		}
	}
	// closing the listener should remove the unix socket file. But lets try it remove it just in case.
	unixSocketFile := path.Join(flexVolumeDriverDir, path.Join(UnixSocketName)) // /flextmnt/rook.io~rook/.rook.sock
	if _, err := os.Stat(unixSocketFile); !os.IsNotExist(err) {
		logger.Info("Deleting unix domain socket file.")
		os.Remove(unixSocketFile)
	}

}

func configureFlexVolume(driverFile, driverDir string) error {
	// copying flex volume
	if _, err := os.Stat(driverDir); os.IsNotExist(err) {
		os.Mkdir(driverDir, 0755)
	}

	destFile := path.Join(driverDir, "."+FlexvolumeDriver)             // /flextmnt/rook.io~rook/.rook
	finalDestFile := path.Join(driverDir, path.Join(FlexvolumeDriver)) // /flextmnt/rook.io~rook/rook
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
			"https://rook.io/docs/rook/master/common-problems.html#kubelet-restart")
	}
}
