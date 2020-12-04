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

package csi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCephCSIKeyringRBDNodeCaps(t *testing.T) {
	caps := cephCSIKeyringRBDNodeCaps()
	assert.Equal(t, caps, []string{"mon", "profile rbd", "mgr", "allow rw", "osd", "profile rbd"})
}

func TestCephCSIKeyringRBDProvisionerCaps(t *testing.T) {
	caps := cephCSIKeyringRBDProvisionerCaps()
	assert.Equal(t, caps, []string{"mon", "profile rbd", "mgr", "allow rw", "osd", "profile rbd"})
}

func TestCephCSIKeyringCephFSNodeCaps(t *testing.T) {
	caps := cephCSIKeyringCephFSNodeCaps()
	assert.Equal(t, caps, []string{"mon", "allow r", "mgr", "allow rw", "osd", "allow rw tag cephfs *=*", "mds", "allow rw"})
}

func TestCephCSIKeyringCephFSProvisionerCaps(t *testing.T) {
	caps := cephCSIKeyringCephFSProvisionerCaps()
	assert.Equal(t, caps, []string{"mon", "allow r", "mgr", "allow rw", "osd", "allow rw tag cephfs metadata=*"})
}
