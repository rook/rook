/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package mds to manage a rook file system.
package mds

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilesystemChanged(t *testing.T) {
	// no change
	old := FilesystemSpec{MetadataServer: MetadataServerSpec{ActiveCount: 1, ActiveStandby: true}}
	new := FilesystemSpec{MetadataServer: MetadataServerSpec{ActiveCount: 1, ActiveStandby: true}}
	changed := filesystemChanged(old, new)
	assert.False(t, changed)

	// changed properties
	new = FilesystemSpec{MetadataServer: MetadataServerSpec{ActiveCount: 2, ActiveStandby: true}}
	assert.True(t, filesystemChanged(old, new))

	new = FilesystemSpec{MetadataServer: MetadataServerSpec{ActiveCount: 1, ActiveStandby: false}}
	assert.True(t, filesystemChanged(old, new))
}
