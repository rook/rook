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

// Package objectuser to manage a rook object store.
package objectuser

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
)

func TestGetObjectStoreUserObject(t *testing.T) {
	// get a current version objectstoreuser object, should return with no error
	objectuser, err := getObjectStoreUserObject(&cephv1.CephObjectStoreUser{})
	assert.NotNil(t, objectuser)
	assert.Nil(t, err)

	// try to get an object that isn't a objectstoreuser, should return with an error
	objectuser, err = getObjectStoreUserObject(&map[string]string{})
	assert.Nil(t, objectuser)
	assert.NotNil(t, err)
}
