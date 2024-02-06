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

// Package telemetry codifies the Rook telemetry spec used to record Rook information for Ceph
// telemetry. See: https://docs.ceph.com/en/latest/mgr/telemetry/
package telemetry

import (
	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "telemetry")

const (
	RookVersionKey             = "rook/version"
	K8sVersionKey              = "rook/kubernetes/version"
	CSIVersionKey              = "rook/csi/version"
	MonMaxIDKey                = "rook/cluster/mon/max-id"
	MonCountKey                = "rook/cluster/mon/count"
	MonAllowMultiplePerNodeKey = "rook/cluster/mon/allow-multiple-per-node"
	MonPVCEnabledKey           = "rook/cluster/mon/pvc/enabled"
	MonStretchEnabledKey       = "rook/cluster/mon/stretch/enabled"
	DeviceSetTotalKey          = "rook/cluster/storage/device-set/count/total"
	DeviceSetPortableKey       = "rook/cluster/storage/device-set/count/portable"
	DeviceSetNonPortableKey    = "rook/cluster/storage/device-set/count/non-portable"
	NetworkProviderKey         = "rook/cluster/network/provider"
	ExternalModeEnabledKey     = "rook/cluster/external-mode"
	K8sNodeCount               = "rook/node/count/kubernetes-total"
	CephNodeCount              = "rook/node/count/with-ceph-daemons"
	RBDNodeCount               = "rook/node/count/with-csi-rbd-plugin"
	CephFSNodeCount            = "rook/node/count/with-csi-cephfs-plugin"
	NFSNodeCount               = "rook/node/count/with-csi-nfs-plugin"
)

var CSIVersion string

func ReportKeyValue(context *clusterd.Context, clusterInfo *client.ClusterInfo, key, value string) {
	ms := config.GetMonStore(context, clusterInfo)
	if err := ms.SetKeyValue(key, value); err != nil {
		logger.Warningf("failed to set telemetry key %q. %v", key, err)
		return
	}
	logger.Debugf("set telemetry key: %s=%s", key, value)
}
