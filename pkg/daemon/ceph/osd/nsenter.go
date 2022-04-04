/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package osd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	// mountNsPath is the default mount namespace of the host
	mountNsPath = "/rootfs/proc/1/ns/mnt"
	// nsenterCmd is the nsenter command
	nsenterCmd = "nsenter"
	rootFSPath = "/rootfs"
)

var (
	binPathsToCheck = []string{"/usr/sbin", "/sbin/", "/run/current-system/sw/bin", "/run/current-system/sw/sbin"}
)

// NSEnter is an nsenter object
type NSEnter struct {
	context    *clusterd.Context
	binary     string
	binaryArgs []string
}

// NewNsenter returns an instance of the NSEnter object
func NewNsenter(context *clusterd.Context, binary string, binaryArgs []string) *NSEnter {
	return &NSEnter{
		context:    context,
		binary:     binary,
		binaryArgs: binaryArgs,
	}
}

func (ne *NSEnter) buildNsEnterCLI(binPath string) []string {
	baseArgs := []string{fmt.Sprintf("--mount=%s", mountNsPath), "--", binPath}
	baseArgs = append(baseArgs, ne.binaryArgs...)

	return baseArgs
}

func (ne *NSEnter) callNsEnter(binPath string) error {
	args := ne.buildNsEnterCLI(binPath)
	op, err := ne.context.Executor.ExecuteCommandWithCombinedOutput(nsenterCmd, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to execute nsenter. output: %s", op)
	}

	logger.Info("successfully called nsenter")
	return nil
}

func (ne *NSEnter) checkIfBinaryExistsOnHost() error {
	for _, path := range binPathsToCheck {
		binPath := filepath.Join(path, ne.binary)
		// Check with nsenter first
		err := ne.callNsEnter(binPath)
		if err != nil {
			logger.Debugf("failed to call nsenter. %v", err)
			// If nsenter failed, let's try with the rootfs directly but only lookup the binary and do not execute it
			// This avoids mismatch libraries between the container and the host while executing
			rootFSBinPath := filepath.Join(rootFSPath, binPath)
			_, err := os.Stat(rootFSBinPath)
			if err != nil {
				logger.Debugf("failed to lookup binary path %q on the host rootfs. %v", rootFSBinPath, err)
				continue
			}
			binPath = rootFSBinPath
		}
		logger.Infof("binary %q found on the host, proceeding with osd preparation", binPath)
		return nil
	}

	return errors.Errorf("binary %q does not exist on the host", ne.binary)
}
