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

package ganesha

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/util"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ganesha")

const (
	cephConfigPath = "/etc/ceph/ceph.conf"
)

func Initialize(context *clusterd.Context, clusterInfo *cephconfig.ClusterInfo) error {
	// write the latest config to the config dir
	if err := cephconfig.GenerateAdminConnectionConfig(context, clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}
	util.WriteFileToLog(logger, cephconfig.DefaultConfigFilePath())

	return nil
}
