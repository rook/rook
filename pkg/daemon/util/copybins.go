/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	"io"
	"os"
	"os/exec"
	"path"
)

// override these for unit testing
var defaultRookDir = "/usr/local/bin"
var defaultTiniDir = "/"

// CopyBinaries copies the "tini" and "rook" binaries to a shared volume at the target path.
func CopyBinaries(target string) error {
	if err := copyBinary(defaultRookDir, target, "rook"); err != nil {
		return err
	}
	return copyBinary(defaultTiniDir, target, "tini")
}

func copyBinary(sourceDir, targetDir, filename string) error {
	sourcePath := path.Join(sourceDir, filename)
	targetPath := path.Join(targetDir, filename)
	logger.Infof("copying %s to %s", sourcePath, targetPath)

	// Check if the source path exists, and look in PATH if it doesn't
	if _, err := os.Stat(sourcePath); err != nil {
		if sourcePath, err = exec.LookPath(filename); err != nil {
			return err
		}
	}

	// Check if the target path exists, and skip the copy if it does
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer destinationFile.Close()
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return err
	}

	return os.Chmod(targetPath, 0755)
}
