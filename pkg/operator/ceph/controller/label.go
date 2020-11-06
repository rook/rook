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
	"fmt"

	"github.com/rook/rook/pkg/operator/ceph/version"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// CephVersionLabelKey is the key used for reporting the Ceph version which Rook has detected is
	// configured for the labeled resource.
	CephVersionLabelKey = "ceph-version"
)

// Add the Ceph version to the given labels.  This should *not* be used on pod specifications,
// because this will result in the deployment/daemonset/etc. recreating all of its pods even if an
// update wouldn't otherwise be required. Upgrading unnecessarily increases risk for loss of data
// reliability, even if only briefly.
func addCephVersionLabel(cephVersion version.CephVersion, labels map[string]string) {
	// cephVersion.String() returns a string with a space in it, and labels in k8s are limited to
	// alphanum characters plus '-', '_', '.'
	labels[CephVersionLabelKey] = GetCephVersionLabel(cephVersion)
}

// GetCephVersionLabel returns a formatted serialization of a provided CephVersion for use in resource labels.
func GetCephVersionLabel(cephVersion version.CephVersion) string {
	return fmt.Sprintf("%d.%d.%d-%d",
		cephVersion.Major, cephVersion.Minor, cephVersion.Extra, cephVersion.Build)
}

// ExtractCephVersionFromLabel returns a CephVersion struct deserialized from a provided version label.
func ExtractCephVersionFromLabel(labelVersion string) (*version.CephVersion, error) {
	return version.ExtractCephVersion(fmt.Sprintf("ceph version %s", labelVersion))
}

// AddCephVersionLabelToDeployment adds a label reporting the Ceph version which Rook has detected is
// running in the Deployment's pods.
func AddCephVersionLabelToDeployment(cephVersion version.CephVersion, d *apps.Deployment) {
	if d == nil {
		return
	}
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	addCephVersionLabel(cephVersion, d.Labels)
}

// AddCephVersionLabelToDaemonSet adds a label reporting the Ceph version which Rook has detected is
// running in the DaemonSet's pods.
func AddCephVersionLabelToDaemonSet(cephVersion version.CephVersion, d *apps.DaemonSet) {
	if d == nil {
		return
	}
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	addCephVersionLabel(cephVersion, d.Labels)
}

// AddCephVersionLabelToJob adds a label reporting the Ceph version which Rook has detected is
// running in the Job's pods.
func AddCephVersionLabelToJob(cephVersion version.CephVersion, j *batch.Job) {
	if j == nil {
		return
	}
	if j.Labels == nil {
		j.Labels = map[string]string{}
	}
	addCephVersionLabel(cephVersion, j.Labels)
}

func AddCephVersionLabelToObjectMeta(cephVersion version.CephVersion, meta *metav1.ObjectMeta) {
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	addCephVersionLabel(cephVersion, meta.Labels)
}
