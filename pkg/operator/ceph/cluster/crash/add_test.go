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

package crash

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCephPod(t *testing.T) {
	labels := make(map[string]string)

	labels["foo"] = "bar"
	podName := "ceph"
	b := isCephPod(labels, podName)
	assert.False(t, b)

	// Label is correct but this is a canary pod, this is not valid!
	podName = "rook-ceph-mon-b-canary-664f5bf8cd-697hh"
	labels["rook_cluster"] = "rook-ceph"
	b = isCephPod(labels, podName)
	assert.False(t, b)

	// Label is correct and this is not a canary pod
	podName = "rook-ceph-mon"
	b = isCephPod(labels, podName)
	assert.True(t, b)
}
