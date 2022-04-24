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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestPreClusterStartValidation(t *testing.T) {
	type args struct {
		cluster *cluster
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"no settings", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), Spec: &cephv1.ClusterSpec{}, context: &clusterd.Context{Clientset: testop.New(t, 3)}}}, false},
		{"even mons", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{Count: 2}}}}, false},
		{"missing stretch zones", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Name: "a"},
		}}}}}}, true},
		{"missing arbiter", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Name: "a"},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, true},
		{"missing zone name", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Arbiter: true},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, true},
		{"valid stretch cluster", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{Count: 3, StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Name: "a", Arbiter: true},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, false},
		{"not enough stretch nodes", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{Count: 5, StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Name: "a", Arbiter: true},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := preClusterStartValidation(tt.args.cluster); (err != nil) != tt.wantErr {
				t.Errorf("ClusterController.preClusterStartValidation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPreMonChecks(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	setSkipSanity := false
	unsetSkipSanity := false
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "config" {
			if args[1] == "set" {
				setSkipSanity = true
				assert.Equal(t, "mon", args[2])
				assert.Equal(t, "mon_mds_skip_sanity", args[3])
				assert.Equal(t, "1", args[4])
				return "", nil
			}
			if args[1] == "rm" {
				unsetSkipSanity = true
				assert.Equal(t, "mon", args[2])
				assert.Equal(t, "mon_mds_skip_sanity", args[3])
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	c := cluster{context: context, ClusterInfo: cephclient.AdminTestClusterInfo("cluster")}

	t.Run("no upgrade", func(t *testing.T) {
		v := cephver.CephVersion{Major: 16, Minor: 2, Extra: 7}
		c.isUpgrade = false
		err := c.preMonStartupActions(v)
		assert.NoError(t, err)
		assert.False(t, setSkipSanity)
		assert.False(t, unsetSkipSanity)
	})

	t.Run("upgrade below version", func(t *testing.T) {
		setSkipSanity = false
		unsetSkipSanity = false
		v := cephver.CephVersion{Major: 16, Minor: 2, Extra: 6}
		c.isUpgrade = true
		err := c.preMonStartupActions(v)
		assert.NoError(t, err)
		assert.False(t, setSkipSanity)
		assert.False(t, unsetSkipSanity)
	})

	t.Run("upgrade to applicable version", func(t *testing.T) {
		setSkipSanity = false
		unsetSkipSanity = false
		v := cephver.CephVersion{Major: 16, Minor: 2, Extra: 7}
		c.isUpgrade = true
		err := c.preMonStartupActions(v)
		assert.NoError(t, err)
		assert.True(t, setSkipSanity)
		assert.False(t, unsetSkipSanity)

		// This will be called during the post mon checks
		err = c.skipMDSSanityChecks(false)
		assert.NoError(t, err)
		assert.True(t, unsetSkipSanity)
	})

	t.Run("upgrade to quincy", func(t *testing.T) {
		setSkipSanity = false
		unsetSkipSanity = false
		v := cephver.CephVersion{Major: 17, Minor: 2, Extra: 0}
		c.isUpgrade = true
		err := c.preMonStartupActions(v)
		assert.NoError(t, err)
		assert.False(t, setSkipSanity)
		assert.False(t, unsetSkipSanity)
	})
}

func TestConfigureMsgr2(t *testing.T) {
	type fields struct {
		encryptionExpected   bool
		compressionExpected  bool
		disabledBothExpected bool
		cephVersion          cephver.CephVersion
		Spec                 *cephv1.ClusterSpec
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{"default settings", fields{false, false, false, cephver.CephVersion{Major: 15}, &cephv1.ClusterSpec{}}},
		{"encryption enabled", fields{true, false, false, cephver.CephVersion{Major: 16}, &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{Connections: &cephv1.ConnectionsSpec{Encryption: &cephv1.EncryptionSpec{Enabled: true}}}}}},
		{"compression enabled old version", fields{false, false, false, cephver.CephVersion{Major: 16}, &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{Connections: &cephv1.ConnectionsSpec{Compression: &cephv1.CompressionSpec{Enabled: true}}}}}},
		{"compression enabled good version", fields{false, true, false, cephver.CephVersion{Major: 17}, &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{Connections: &cephv1.ConnectionsSpec{Compression: &cephv1.CompressionSpec{Enabled: true}}}}}},
		{"both enabled", fields{true, true, false, cephver.CephVersion{Major: 17}, &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{Connections: &cephv1.ConnectionsSpec{Encryption: &cephv1.EncryptionSpec{Enabled: true}, Compression: &cephv1.CompressionSpec{Enabled: true}}}}}},
		{"both disabled", fields{false, false, true, cephver.CephVersion{Major: 17}, &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{Connections: &cephv1.ConnectionsSpec{Encryption: &cephv1.EncryptionSpec{Enabled: false}, Compression: &cephv1.CompressionSpec{Enabled: false}}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabledCompression := false
			disabledCompression := false
			enabledEncryption := false
			disabledEncryption := false
			executor := &exectest.MockExecutor{
				MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
					logger.Infof("Execute: %s %v", command, args)
					if args[0] == "config" && args[1] == "set" {
						if args[3] == "ms_osd_compress_mode" {
							if args[4] == "force" {
								enabledCompression = true
							} else if args[4] == "none" {
								disabledCompression = true
							}
						}
						if args[3] == "ms_cluster_mode" {
							if args[4] == "secure" {
								enabledEncryption = true
							} else if args[4] == "crc secure" {
								disabledEncryption = true
							}
						}

						return "", nil
					}
					return "", errors.Errorf("unrecognized command")
				},
			}
			clusterInfo := cephclient.AdminTestClusterInfo("rook-ceph")
			clusterInfo.CephVersion = tt.fields.cephVersion
			context := &clusterd.Context{Clientset: testop.New(t, 3), Executor: executor}
			c := &cluster{
				ClusterInfo: clusterInfo,
				Spec:        tt.fields.Spec,
				context:     context,
			}

			err := c.configureMsgr2()
			assert.NoError(t, err)
			assert.Equal(t, tt.fields.compressionExpected, enabledCompression)
			assert.Equal(t, tt.fields.encryptionExpected, enabledEncryption)
			assert.Equal(t, tt.fields.disabledBothExpected, disabledCompression)
			assert.Equal(t, tt.fields.disabledBothExpected, disabledEncryption)
		})
	}
}
