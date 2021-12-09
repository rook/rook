/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package file

import (
	"fmt"

	"github.com/pkg/errors"
	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/dependents"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephFilesystemDependents returns the subvolume group(s) which exist in the ceph filesystem that should block
// deletion.
func CephFilesystemDependents(clusterdCtx *clusterd.Context, clusterInfo *client.ClusterInfo, filesystem *v1.CephFilesystem) (*dependents.DependentList, error) {
	nsName := fmt.Sprintf("%s/%s", filesystem.Namespace, filesystem.Name)
	baseErrMsg := fmt.Sprintf("failed to get dependents of CephFilesystem %q", nsName)

	deps := dependents.NewDependentList()

	// CephFilesystemSubVolumeGroups
	subVolumeGroups, err := clusterdCtx.RookClientset.CephV1().CephFilesystemSubVolumeGroups(filesystem.Namespace).List(clusterInfo.Context, metav1.ListOptions{})
	if err != nil {
		return deps, errors.Wrapf(err, "%s. failed to list CephFilesystemSubVolumeGroups for CephFilesystem %q", baseErrMsg, nsName)
	}
	for _, subVolumeGroup := range subVolumeGroups.Items {
		if subVolumeGroup.Spec.FilesystemName == filesystem.Name {
			deps.Add("CephFilesystemSubVolumeGroups", subVolumeGroup.Name)
		}
		logger.Debugf("found CephFilesystemSubVolumeGroups %q that does not depend on CephFilesystem %q", subVolumeGroup.Name, nsName)
	}

	return deps, nil
}
