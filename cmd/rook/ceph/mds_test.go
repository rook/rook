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
package ceph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractMdsName(t *testing.T) {
	name := extractMdsID("random")
	assert.Equal(t, "random", name)

	name = extractMdsID("rook-ceph-mds")
	assert.Equal(t, "rook-ceph-mds", name)

	name = extractMdsID("rook-ceph-mds-a")
	assert.Equal(t, "a", name)

	name = extractMdsID("rook-ceph-mds-rook-ceph-mds-foo")
	assert.Equal(t, "rook-ceph-mds-foo", name)

	name = extractMdsID("rook-ceph-mds-myfs-64b66569f6-5tz2s")
	assert.Equal(t, "myfs-64b66569f6-5tz2s", name)
}
