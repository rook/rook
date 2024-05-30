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
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/nodedaemon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/telemetry"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
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
		{"missing stretch zones", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.MonZoneSpec{
			{Name: "a"},
		}}}}}}, true},
		{"missing arbiter", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.MonZoneSpec{
			{Name: "a"},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, true},
		{"missing zone name", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.MonZoneSpec{
			{Arbiter: true},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, true},
		{"valid stretch cluster", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{Count: 3, StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.MonZoneSpec{
			{Name: "a", Arbiter: true},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, false},
		{"not enough stretch nodes", args{&cluster{ClusterInfo: cephclient.AdminTestClusterInfo("rook-ceph"), context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{Count: 5, StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.MonZoneSpec{
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

func TestConfigureMsgr2(t *testing.T) {
	type fields struct {
		expectedGlobalConfigSettings map[string]string
		cephVersion                  cephver.CephVersion
		Spec                         *cephv1.ClusterSpec
	}

	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "default settings",
			fields: fields{
				expectedGlobalConfigSettings: nil,
				cephVersion:                  cephver.CephVersion{Major: 15},
				Spec:                         &cephv1.ClusterSpec{},
			},
		},
		{
			name: "encryption enabled",
			fields: fields{
				expectedGlobalConfigSettings: map[string]string{
					"ms_cluster_mode":         "secure",
					"ms_service_mode":         "secure",
					"ms_client_mode":          "secure",
					"rbd_default_map_options": "ms_mode=secure",
				},
				cephVersion: cephver.CephVersion{Major: 16},
				Spec: &cephv1.ClusterSpec{
					Network: cephv1.NetworkSpec{
						Connections: &cephv1.ConnectionsSpec{
							Encryption: &cephv1.EncryptionSpec{
								Enabled: true,
							},
						},
					},
				},
			},
		},
		{
			name: "compression enabled old version",
			fields: fields{
				expectedGlobalConfigSettings: map[string]string{
					"rbd_default_map_options": "ms_mode=prefer-crc",
				},
				cephVersion: cephver.CephVersion{Major: 16},
				Spec: &cephv1.ClusterSpec{
					Network: cephv1.NetworkSpec{
						Connections: &cephv1.ConnectionsSpec{
							Compression: &cephv1.CompressionSpec{
								Enabled: true,
							},
						},
					},
				},
			},
		},
		{
			name: "compression enabled good version",
			fields: fields{
				expectedGlobalConfigSettings: map[string]string{
					"rbd_default_map_options": "ms_mode=prefer-crc",
				},
				cephVersion: cephver.CephVersion{Major: 17},
				Spec: &cephv1.ClusterSpec{
					Network: cephv1.NetworkSpec{
						Connections: &cephv1.ConnectionsSpec{
							Compression: &cephv1.CompressionSpec{
								Enabled: true,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configFile *ini.File

			clusterInfo := cephclient.AdminTestClusterInfo("rook-ceph")
			clusterInfo.CephVersion = tt.fields.cephVersion

			c := &cluster{
				ClusterInfo: clusterInfo,
				Namespace:   "rook-ceph",
				Spec:        tt.fields.Spec,
				context: &clusterd.Context{
					Clientset: testop.New(t, 3),
					Executor: &exectest.MockExecutor{
						MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
							joinedArgs := strings.Join(args, " ")
							switch {
							case strings.HasPrefix(joinedArgs, "config assimilate-conf"):
								fs := flag.NewFlagSet("", flag.ContinueOnError)
								inputFile := fs.String("i", "", "")

								if err := fs.Parse(args[2:4]); err != nil {
									return "", fmt.Errorf("parse flags: %w", err)
								}

								f, err := ini.Load(*inputFile)
								if err != nil {
									return "", fmt.Errorf("load ini file: %w", err)
								}
								configFile = f

								fallthrough
							case
								strings.HasPrefix(joinedArgs, "config rm"),
								strings.HasPrefix(joinedArgs, "config get global rbd_default_map_options"):
								return "", nil
							}
							return "", errors.Errorf("unexpected ceph command %q", args)
						},
					},
				},
			}

			err := c.configureMsgr2()
			require.NoError(t, err)

			if assert.Equal(t, tt.fields.expectedGlobalConfigSettings == nil, configFile == nil) {
				return
			}

			section := configFile.Section("global")
			for k, v := range tt.fields.expectedGlobalConfigSettings {
				assert.Equal(t, v, section.Key(k).String(), k)
			}
			assert.Len(t, section.Keys(), len(tt.fields.expectedGlobalConfigSettings))
		})
	}
}

func TestTelemetry(t *testing.T) {
	var expectedSettings map[string]string
	clientset := testop.New(t, 3)
	context := &clusterd.Context{Clientset: clientset}
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "config-key" && args[1] == "set" {
				key := args[2]
				if key == telemetry.RookVersionKey {
					// the rook version will vary depending on the build, so it just shouldn't be blank
					assert.NotEqual(t, "", args[3])
				} else {
					assert.Equal(t, expectedSettings[key], args[3], key)
				}
				return "", nil
			}
			return "", errors.New("mock error to simulate failure of mon store config")
		},
	}
	c := cluster{
		context:     context,
		ClusterInfo: cephclient.AdminTestClusterInfo("cluster"),
		mons:        &mon.Cluster{},
		Namespace:   "rook-ceph",
	}

	t.Run("normal cluster", func(t *testing.T) {
		ns := "rook-ceph"
		c.Spec = &cephv1.ClusterSpec{
			Mon: cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true},
			Storage: cephv1.StorageScopeSpec{
				StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{
					{Name: "one", Count: 3, Portable: true},
					{Name: "two", Count: 3, Portable: true},
					{Name: "three", Count: 3, Portable: false},
				},
			},
			Network:        cephv1.NetworkSpec{Provider: "host"},
			CrashCollector: cephv1.CrashCollectorSpec{Disable: false},
		}
		expectedSettings = map[string]string{
			telemetry.K8sVersionKey:              "v0.0.0-master+$Format:%H$",
			telemetry.MonMaxIDKey:                "0",
			telemetry.MonCountKey:                "3",
			telemetry.MonAllowMultiplePerNodeKey: "true",
			telemetry.MonPVCEnabledKey:           "false",
			telemetry.MonStretchEnabledKey:       "false",
			telemetry.DeviceSetTotalKey:          "3",
			telemetry.DeviceSetPortableKey:       "2",
			telemetry.DeviceSetNonPortableKey:    "1",
			telemetry.NetworkProviderKey:         "host",
			telemetry.ExternalModeEnabledKey:     "false",
			telemetry.K8sNodeCount:               "3",
			telemetry.CephFSNodeCount:            "0",
			telemetry.RBDNodeCount:               "0",
			telemetry.NFSNodeCount:               "0",
			telemetry.CephNodeCount:              "0",
		}
		c.reportTelemetry()

		// add some pods to the nodes
		labels := make(map[string]string)
		labels["app"] = nodedaemon.CrashCollectorAppName
		testop.FakeCustomisePodCreate(t, clientset, "crash-collector", ns, labels)
		labels["app"] = csi.CsiRBDPlugin
		testop.FakeCustomisePodCreate(t, clientset, "csi-rbdplugin", ns, labels)
		expectedSettings = map[string]string{
			telemetry.K8sVersionKey:              "v0.0.0-master+$Format:%H$",
			telemetry.MonMaxIDKey:                "0",
			telemetry.MonCountKey:                "3",
			telemetry.MonAllowMultiplePerNodeKey: "true",
			telemetry.MonPVCEnabledKey:           "false",
			telemetry.MonStretchEnabledKey:       "false",
			telemetry.DeviceSetTotalKey:          "3",
			telemetry.DeviceSetPortableKey:       "2",
			telemetry.DeviceSetNonPortableKey:    "1",
			telemetry.NetworkProviderKey:         "host",
			telemetry.ExternalModeEnabledKey:     "false",
			telemetry.K8sNodeCount:               "3",
			telemetry.CephFSNodeCount:            "0",
			telemetry.RBDNodeCount:               "1", //should be 1
			telemetry.NFSNodeCount:               "0",
			telemetry.CephNodeCount:              "1", //should be 1
		}
		c.reportTelemetry()
	})

	t.Run("external cluster", func(t *testing.T) {
		c.Spec = &cephv1.ClusterSpec{
			External:       cephv1.ExternalSpec{Enable: true},
			CrashCollector: cephv1.CrashCollectorSpec{Disable: true},
		}
		expectedSettings = map[string]string{
			telemetry.K8sVersionKey:              "v0.0.0-master+$Format:%H$",
			telemetry.MonMaxIDKey:                "0",
			telemetry.MonCountKey:                "0",
			telemetry.MonAllowMultiplePerNodeKey: "false",
			telemetry.MonPVCEnabledKey:           "false",
			telemetry.MonStretchEnabledKey:       "false",
			telemetry.DeviceSetTotalKey:          "0",
			telemetry.DeviceSetPortableKey:       "0",
			telemetry.DeviceSetNonPortableKey:    "0",
			telemetry.NetworkProviderKey:         "",
			telemetry.ExternalModeEnabledKey:     "true",
			telemetry.K8sNodeCount:               "3",
			telemetry.CephFSNodeCount:            "0",
			telemetry.RBDNodeCount:               "1",
			telemetry.NFSNodeCount:               "0",
			telemetry.CephNodeCount:              "-1", // checking the disable scenario
		}
		c.reportTelemetry()
	})
}
func TestClusterFullSettings(t *testing.T) {
	actualFullRatio := 0.95
	actualBackfillFullRatio := 0.90
	actualNearFullRatio := 0.85
	setFullRatio := false
	setBackfillFullRatio := false
	setNearFullRatio := false
	clientset := testop.New(t, 1)
	context := &clusterd.Context{Clientset: clientset}
	c := cluster{
		context:     context,
		ClusterInfo: cephclient.AdminTestClusterInfo("cluster"),
		Spec:        &cephv1.ClusterSpec{},
	}
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "osd" {
				if args[1] == "dump" {
					return fmt.Sprintf(
						`{	"full_ratio": %.2f,
					"backfillfull_ratio": %.2f,
					"nearfull_ratio": %.2f}`, actualFullRatio, actualBackfillFullRatio, actualNearFullRatio), nil
				}
				if args[1] == "set-full-ratio" {
					assert.Equal(t, fmt.Sprintf("%.2f", *c.Spec.Storage.FullRatio), args[2])
					setFullRatio = true
					return "", nil
				}
				if args[1] == "set-nearfull-ratio" {
					assert.Equal(t, fmt.Sprintf("%.2f", *c.Spec.Storage.NearFullRatio), args[2])
					setNearFullRatio = true
					return "", nil
				}
				if args[1] == "set-backfillfull-ratio" {
					assert.Equal(t, fmt.Sprintf("%.2f", *c.Spec.Storage.BackfillFullRatio), args[2])
					setBackfillFullRatio = true
					return "", nil
				}
			}
			return "", errors.New("mock error to simulate failure of mon store config")
		},
	}
	t.Run("no settings", func(t *testing.T) {
		err := c.configureStorageSettings()
		assert.NoError(t, err)
		assert.False(t, setFullRatio)
		assert.False(t, setNearFullRatio)
		assert.False(t, setBackfillFullRatio)
	})

	val91 := 0.91
	val90 := 0.90
	val85 := 0.85
	val80 := 0.80

	t.Run("all settings applied", func(t *testing.T) {
		c.Spec.Storage.FullRatio = &val90
		c.Spec.Storage.NearFullRatio = &val80
		c.Spec.Storage.BackfillFullRatio = &val85
		err := c.configureStorageSettings()
		assert.NoError(t, err)
		assert.True(t, setFullRatio)
		assert.True(t, setNearFullRatio)
		assert.True(t, setBackfillFullRatio)
	})

	t.Run("no settings changed", func(t *testing.T) {
		setFullRatio = false
		setBackfillFullRatio = false
		setNearFullRatio = false
		c.Spec.Storage.FullRatio = &actualFullRatio
		c.Spec.Storage.NearFullRatio = &actualNearFullRatio
		c.Spec.Storage.BackfillFullRatio = &actualBackfillFullRatio
		err := c.configureStorageSettings()
		assert.NoError(t, err)
		assert.False(t, setFullRatio)
		assert.False(t, setNearFullRatio)
		assert.False(t, setBackfillFullRatio)
	})

	t.Run("one setting applied", func(t *testing.T) {
		setFullRatio = false
		setBackfillFullRatio = false
		setNearFullRatio = false
		c.Spec.Storage.FullRatio = &val91
		c.Spec.Storage.NearFullRatio = nil
		c.Spec.Storage.BackfillFullRatio = nil
		err := c.configureStorageSettings()
		assert.NoError(t, err)
		assert.True(t, setFullRatio)
		assert.False(t, setNearFullRatio)
		assert.False(t, setBackfillFullRatio)
	})
}
