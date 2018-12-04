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

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"path"

	k8smount "k8s.io/kubernetes/pkg/util/mount"

	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/spf13/cobra"
)

const (
	cephFS = "ceph"
)

// RootCmd the rookflex volume plugin cobra root command
var RootCmd = &cobra.Command{
	Use:           "rookflex",
	Short:         "Rook Flex volume plugin",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func getRPCClient() (*rpc.Client, error) {
	driverDir, err := getDriverDir()
	if err != nil {
		return nil, err
	}

	unixSocketFile := path.Join(driverDir, flexvolume.UnixSocketName) // /usr/libexec/kubernetes/kubelet-plugins/volume/exec/rook.io~rook/rook/.rook.sock
	conn, err := net.Dial("unix", unixSocketFile)
	if err != nil {
		return nil, fmt.Errorf("error connecting to socket %s: %+v", unixSocketFile, err)
	}
	return rpc.NewClient(conn), nil
}

func getDriverDir() (string, error) {
	ex, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("error getting path of the Rook flexvolume driver: %v", err)
	}

	return path.Dir(ex), nil
}

func getMounter() *k8smount.SafeFormatAndMount {
	return &k8smount.SafeFormatAndMount{
		Interface: k8smount.New("" /* default mount path */),
		Exec:      k8smount.NewOsExec(),
	}
}

func log(client *rpc.Client, message string, isError bool) {
	var log = &flexvolume.LogMessage{
		Message: message,
		IsError: isError,
	}
	client.Call("Controller.Log", log, nil)
}

// redirectStdout redirects the stdout for the fn function to the driver logger
func redirectStdout(client *rpc.Client, fn func() error) error {
	// keep backup of the real stdout and stderr
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w

	// restoring the real stdout and stderr
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	err := fn()
	w.Close()

	var buf bytes.Buffer
	io.Copy(&buf, r)
	log(client, buf.String(), false)
	return err
}
