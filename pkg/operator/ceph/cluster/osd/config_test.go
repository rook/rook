/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package osd

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
)

func TestOsdOnSDNFlag(t *testing.T) {
	network := cephv1.NetworkSpec{}

	args := osdOnSDNFlag(network)
	assert.NotEmpty(t, args)

	network.Provider = "host"
	args = osdOnSDNFlag(network)
	assert.Empty(t, args)
}

func TestEncryptionKeyPath(t *testing.T) {
	assert.Equal(t, "/etc/ceph/luks_key", encryptionKeyPath())
}

func TestEncryptionBlockDestinationCopy(t *testing.T) {
	m := "/var/lib/ceph/osd/ceph-0"
	assert.Equal(t, "/var/lib/ceph/osd/ceph-0/block-tmp", encryptionBlockDestinationCopy(m, bluestoreBlockName))
	assert.Equal(t, "/var/lib/ceph/osd/ceph-0/block.db-tmp", encryptionBlockDestinationCopy(m, bluestoreMetadataName))
	assert.Equal(t, "/var/lib/ceph/osd/ceph-0/block.wal-tmp", encryptionBlockDestinationCopy(m, bluestoreWalName))
}

func TestEncryptionDMPath(t *testing.T) {
	assert.Equal(t, "/dev/mapper/set1-data-0-6rqdn-block-dmcrypt", EncryptionDMPath("set1-data-0-6rqdn", DmcryptBlockType))
}

func TestEncryptionDMName(t *testing.T) {
	assert.Equal(t, "set1-data-0-6rqdn-block-dmcrypt", EncryptionDMName("set1-data-0-6rqdn", DmcryptBlockType))
}
