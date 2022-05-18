/*
Copyright 2022 The Rook Authors. All rights reserved.

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

// Package telemetry codifes the Rook telemetry spec used to record Rook information for Ceph
// telemetry. See: https://docs.ceph.com/en/latest/mgr/telemetry/
package telemetry

import (
	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	rookversion "github.com/rook/rook/pkg/version"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "telemetry")

const (
	RookVersionKey = `rook/version`
)

// SetRookVersion sets the Rook version in Ceph's config-key store to allow Rook clusters that
// enable Ceph telemetry to be identified easily as Rook clusters while additionally providing
// useful information about the version of Rook that is managing the cluster.
// e.g., rook/version=v1.8.7
func SetRookVersion(context *clusterd.Context, clusterInfo *client.ClusterInfo) {
	ms := config.GetMonStore(context, clusterInfo)
	if err := ms.SetKeyValue(RookVersionKey, rookversion.Version); err != nil {
		logger.Warningf("failed to set telemetry key; this cluster may not be identifiable by ceph telemetry as a Rook cluster. %v", err)
	}
}
