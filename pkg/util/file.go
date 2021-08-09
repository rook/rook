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

package util

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "util")

func WriteFile(filePath string, contentBuffer bytes.Buffer) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0744); err != nil {
		return fmt.Errorf("failed to create config file directory at %s: %+v", dir, err)
	}
	if err := ioutil.WriteFile(filePath, contentBuffer.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file to %s: %+v", filePath, err)
	}

	return nil
}

func WriteFileToLog(logger *capnslog.PackageLogger, path string) {
	contents, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		logger.Warningf("failed to write file %s to log: %+v", path, err)
		return
	}

	logger.Infof("Config file %s:\n%s", path, string(contents))
}

// PathToProjectRoot returns the path to the root of the rook repo on the current host.
// This is primarily useful for tests.
func PathToProjectRoot() string {
	_, path, _, _ := runtime.Caller(0) // get path to current file (<root>/pkg/util/file.go)
	util := filepath.Dir(path)         // <root>/pkg/util
	pkg := filepath.Dir(util)          // <root>/pkg
	root := filepath.Dir(pkg)          // <root>
	return root
}

// CreateTempFile creates a temporary file with content passed as an argument
func CreateTempFile(content string) (*os.File, error) {
	// Generate a temp file
	file, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate temp file")
	}

	// Write content into file
	err = ioutil.WriteFile(file.Name(), []byte(content), 0440)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write content into file")
	}
	return file, nil
}
