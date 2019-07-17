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
*/

// Package mds to manage a rook filesystem.
package file

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"

	"github.com/stretchr/testify/assert"
)

func TestFilesystemChanged(t *testing.T) {
	// no change
	old := cephv1.FilesystemSpec{MetadataServer: cephv1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: true}}
	new := cephv1.FilesystemSpec{MetadataServer: cephv1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: true}}
	changed := filesystemChanged(old, new)
	assert.False(t, changed)

	// changed properties
	new = cephv1.FilesystemSpec{MetadataServer: cephv1.MetadataServerSpec{ActiveCount: 2, ActiveStandby: true}}
	assert.True(t, filesystemChanged(old, new))

	new = cephv1.FilesystemSpec{MetadataServer: cephv1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: false}}
	assert.True(t, filesystemChanged(old, new))
}

func TestGetFilesystemObject(t *testing.T) {
	// get a current version filesystem object, should return with no error and no migration needed
	filesystem, err := getFilesystemObject(&cephv1.CephFilesystem{})
	assert.NotNil(t, filesystem)
	assert.Nil(t, err)

	// try to get an object that isn't a filesystem, should return with an error
	filesystem, err = getFilesystemObject(&map[string]string{})
	assert.Nil(t, filesystem)
	assert.NotNil(t, err)
}
