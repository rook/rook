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
	"io/ioutil"

	"github.com/coreos/pkg/capnslog"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "util")

func WriteFileToLog(logger *capnslog.PackageLogger, path string) {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Warningf("failed to write file %s to log: %+v", path, err)
		return
	}

	logger.Infof("Config file %s:\n%s", path, string(contents))
}
