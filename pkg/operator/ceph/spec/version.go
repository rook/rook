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

package spec

import (
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
)

// ValidateCephVersionsBetweenLocalAndExternalClusters makes sure an external cluster can be connected
// by checking the external ceph versions available and comparing it with the local image provided
func ValidateCephVersionsBetweenLocalAndExternalClusters(context *clusterd.Context, namespace string, localVersion cephver.CephVersion) (cephver.CephVersion, error) {
	// health check should tell us if the external cluster has been upgraded and display a message
	externalVersion, err := client.GetCephMonVersion(context, namespace)
	if err != nil {
		return cephver.CephVersion{}, errors.Wrapf(err, "failed to get ceph mon version")
	}

	return *externalVersion, cephver.ValidateCephVersionsBetweenLocalAndExternalClusters(localVersion, *externalVersion)
}
