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
	"syscall"

	"github.com/pkg/errors"
	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/dependents"
	kexec "github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const subvolumeGroupDependentType = "filesystem subvolume groups that contain subvolumes (could be from CephFilesystem PVCs or CephNFS exports)"

// the empty string is used to represent "no group". Use a clear string for users when reporting
// subvolume dependents in no group to prevent confusion
const noGroupDependentName = "<no group>"

// there are special subvolume groups that should not contain valid subvolumes. skip these during
// subvolume dependency checking. there is some possibility users could manually put subvolumes into
// these groups, but that should be exceedingly rare. future ceph versions may stop reporting these
// groups. ignore "_nogroup" in favor of explicitly listing subvolumes not in any group for forwards
// compatibility with ceph versions that do not list "_nogroup"
var ignoredDependentSubvolumeGroups = []string{"_nogroup", "_index", "_legacy", "_deleting"}

// CephFilesystemDependents returns the subvolume group(s) which exist in the ceph filesystem that
// should block deletion.
//
// No RBD images are created by normal operations on filesystems, so there will be no images present
// to check if a filesystem has user data in it. Therefore, we need some other check for user data.
// We approximate such a check here by checking for subvolume groups that have subvolumes. Subvolume
// groups with no subvolumes don't block deletion.
var CephFilesystemDependents = cephFilesystemDependents

// check filesystem whether it exists
func filesystemExists(clusterdCtx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, name string, nsName types.NamespacedName) (bool, error) {
	_, err := cephclient.GetFilesystem(clusterdCtx, clusterInfo, name)
	if err != nil {
		if code, ok := kexec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			log.NamedInfo(nsName, logger, "filesystem deletion will continue without checking for dependencies since the the filesystem does not exist within Ceph")
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to check for existence of CephFilesystem %q", nsName)
	}
	return true, nil
}

// with above, allow this to be overridden for unit testing
func cephFilesystemDependents(clusterdCtx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, filesystem *v1.CephFilesystem) (*dependents.DependentList, error) {
	nsName := controller.NsName(filesystem.Namespace, filesystem.Name)
	baseErrMsg := fmt.Sprintf("failed to get dependents of CephFilesystem %q", nsName)

	deps := dependents.NewDependentList()
	fsExists, err := filesystemExists(clusterdCtx, clusterInfo, filesystem.Name, nsName)
	if err != nil {
		return deps, nil
	}

	// subvolume groups that contain subvolumes
	if fsExists {
		deps, err = subvolumeGroupDependents(clusterdCtx, clusterInfo, filesystem)
		if err != nil {
			return deps, errors.Wrapf(err, "%s", baseErrMsg)
		}
	}

	// CephFilesystemSubVolumeGroups
	subVolumeGroups, err := clusterdCtx.RookClientset.CephV1().CephFilesystemSubVolumeGroups(filesystem.Namespace).List(clusterInfo.Context, metav1.ListOptions{})
	if err != nil {
		return deps, errors.Wrapf(err, "%s. failed to list CephFilesystemSubVolumeGroups for CephFilesystem %q", baseErrMsg, nsName)
	}
	for _, subVolumeGroup := range subVolumeGroups.Items {
		if subVolumeGroup.Spec.FilesystemName == filesystem.Name {
			deps.Add("CephFilesystemSubVolumeGroups", subVolumeGroup.Name)
		}
		log.NamedDebug(nsName, logger, "found CephFilesystemSubVolumeGroups %q that does not depend on CephFilesystem", subVolumeGroup.Name)
	}

	return deps, nil
}

// return subvolume groups that have 1 or more subvolumes present in them
func subvolumeGroupDependents(clusterdCtx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, filesystem *v1.CephFilesystem) (*dependents.DependentList, error) {
	baseErr := "failed to get Ceph subvolume groups containing subvolumes"

	deps := dependents.NewDependentList()

	svgs, err := cephclient.ListSubvolumeGroups(clusterdCtx, clusterInfo, filesystem.Name)
	if err != nil {
		return deps, errors.Wrap(err, baseErr)
	}

	// also check the case where subvolumes are not in a group
	svgs = append(svgs, cephclient.SubvolumeGroup{Name: cephclient.NoSubvolumeGroup})

	errs := []error{}
	for _, svg := range svgs {
		if ignoreSVG(svg.Name) {
			continue
		}

		svs, err := cephclient.ListSubvolumesInGroup(clusterdCtx, clusterInfo, filesystem.Name, svg.Name)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to list subvolumes in subvolume group %q", svg.Name))
		}

		if len(svs) > 0 {
			name := svg.Name
			if name == cephclient.NoSubvolumeGroup {
				// identify the "no group" case clearly for users
				name = noGroupDependentName
			}
			deps.Add(subvolumeGroupDependentType, name)
		}
	}

	outErr := util.AggregateErrors(errs, "failed to list subvolumes in filesystem %q for one or more subvolume groups; "+
		"a timeout might indicate there are many subvolumes in a subvolume group", filesystem.Name)

	return deps, outErr
}

func ignoreSVG(name string) bool {
	for _, ignore := range ignoredDependentSubvolumeGroups {
		if name == ignore {
			log.NamespacedDebug(name, logger, "skipping dependency check for subvolumes in subvolumegroup %q", ignore)
			return true
		}
	}
	return false
}
