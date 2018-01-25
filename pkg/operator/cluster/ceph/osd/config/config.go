/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package config for OSD config managed by the operator
package config

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
)

const (
	configStoreNameFmt = "rook-ceph-osd-%s-config"
	osdDirsKeyName     = "osd-dirs"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "osd-config")

func GetConfigStoreName(nodeName string) string {
	return fmt.Sprintf(configStoreNameFmt, nodeName)
}
