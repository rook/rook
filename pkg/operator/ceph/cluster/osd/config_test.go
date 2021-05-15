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
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
	assert.Equal(t, "/dev/mapper/set1-data-0-6rqdn-block-dmcrypt", encryptionDMPath("set1-data-0-6rqdn", DmcryptBlockType))
}

func TestEncryptionDMName(t *testing.T) {
	assert.Equal(t, "set1-data-0-6rqdn-block-dmcrypt", encryptionDMName("set1-data-0-6rqdn", DmcryptBlockType))
}

func TestClusterIsCephVolumeRAwModeSupported(t *testing.T) {
	type fields struct {
		context      *clusterd.Context
		clusterInfo  *cephclient.ClusterInfo
		rookVersion  string
		spec         cephv1.ClusterSpec
		ValidStorage cephv1.StorageScopeSpec
		kv           *k8sutil.ConfigMapKVStore
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{"nok-14.2.4", fields{&clusterd.Context{}, &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 4}}, "", cephv1.ClusterSpec{}, cephv1.StorageScopeSpec{}, &k8sutil.ConfigMapKVStore{}}, false},
		{"ok-14.2.11", fields{&clusterd.Context{}, &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 11}}, "", cephv1.ClusterSpec{}, cephv1.StorageScopeSpec{}, &k8sutil.ConfigMapKVStore{}}, true},
		{"nok-15.2.4", fields{&clusterd.Context{}, &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 4}}, "", cephv1.ClusterSpec{}, cephv1.StorageScopeSpec{}, &k8sutil.ConfigMapKVStore{}}, false},
		{"ok-15.2.5", fields{&clusterd.Context{}, &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 5}}, "", cephv1.ClusterSpec{}, cephv1.StorageScopeSpec{}, &k8sutil.ConfigMapKVStore{}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Cluster{
				context:      tt.fields.context,
				clusterInfo:  tt.fields.clusterInfo,
				rookVersion:  tt.fields.rookVersion,
				spec:         tt.fields.spec,
				ValidStorage: tt.fields.ValidStorage,
				kv:           tt.fields.kv,
			}
			if got := c.isCephVolumeRawModeSupported(); got != tt.want {
				t.Errorf("Cluster.isCephVolumeRAwModeSupported() = %v, want %v", got, tt.want)
			}
		})
	}
}
