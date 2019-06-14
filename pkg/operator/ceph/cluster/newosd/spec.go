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

package newosd

import (
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
)

const (
	osdLabelKey                    = "ceph-osd-id"
	cephOsdPodMinimumMemory uint64 = 4096 // minimum amount of memory in MB to run the pod
)

func newInt32(i int32) *int32 { return &i }

func newInt64(i int64) *int64 { return &i }

func newBool(b bool) *bool { return &b }

func (c *Controller) osdAppLabels(osdID string) map[string]string {
	labels := opspec.PodLabels(osdAppName, c.namespace, "osd", osdID)
	// Add "ceph_osd_id: <osd ID>" for legacy"
	labels[osdLabelKey] = osdID
	return labels
}
