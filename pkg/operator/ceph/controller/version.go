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

package controller

import (
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
)

// ValidateCephVersionsBetweenLocalAndExternalClusters makes sure an external cluster can be connected
// by checking the external ceph versions available and comparing it with the local image provided
func ValidateCephVersionsBetweenLocalAndExternalClusters(context *clusterd.Context, clusterInfo *client.ClusterInfo) (cephver.CephVersion, error) {
	// health check should tell us if the external cluster has been upgraded and display a message
	externalVersion, err := client.GetCephMonVersion(context, clusterInfo)
	if err != nil {
		return cephver.CephVersion{}, errors.Wrap(err, "failed to get ceph mon version")
	}

	return *externalVersion, cephver.ValidateCephVersionsBetweenLocalAndExternalClusters(clusterInfo.CephVersion, *externalVersion)
}

// GetImageVersion returns the CephVersion registered for a specified image (if any) and whether any image was found.
func GetImageVersion(cephCluster cephv1.CephCluster) (*cephver.CephVersion, error) {
	// If the Ceph cluster has not yet recorded the image and version for the current image in its spec, then the Crash
	// controller should wait for the version to be detected.
	if cephCluster.Status.CephVersion != nil && cephCluster.Spec.CephVersion.Image == cephCluster.Status.CephVersion.Image {
		logger.Debugf("ceph version found %q", cephCluster.Status.CephVersion.Version)
		return ExtractCephVersionFromLabel(cephCluster.Status.CephVersion.Version)
	}

	return nil, errors.New("attempt to determine ceph version for the current cluster image timed out")
}
