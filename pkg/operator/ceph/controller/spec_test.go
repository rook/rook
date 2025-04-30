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

package controller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodVolumes(t *testing.T) {
	clusterSpec := cephv1.ClusterSpec{
		DataDirHostPath: "/var/lib/rook/",
	}
	dataPathMap := config.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook")

	if err := test.VolumeIsEmptyDir(k8sutil.DataDirVolume, PodVolumes(dataPathMap, "", clusterSpec.DataDirHostPath, false)); err != nil {
		t.Errorf("PodVolumes(\"\") - data dir source is not EmptyDir: %s", err.Error())
	}
	if err := test.VolumeIsHostPath(k8sutil.DataDirVolume, "/dev/sdb", PodVolumes(dataPathMap, "/dev/sdb", clusterSpec.DataDirHostPath, false)); err != nil {
		t.Errorf("PodVolumes(\"/dev/sdb\") - data dir source is not HostPath: %s", err.Error())
	}
}

func TestMountsMatchVolumes(t *testing.T) {
	clusterSpec := cephv1.ClusterSpec{
		DataDirHostPath: "/var/lib/rook/",
	}

	dataPathMap := config.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook")

	volsMountsTestDef := test.VolumesAndMountsTestDefinition{
		VolumesSpec: &test.VolumesSpec{
			Moniker: "PodVolumes(\"/dev/sdc\")", Volumes: PodVolumes(dataPathMap, "/dev/sdc", clusterSpec.DataDirHostPath, false),
		},
		MountsSpecItems: []*test.MountsSpec{
			{Moniker: "CephVolumeMounts(true)", Mounts: CephVolumeMounts(dataPathMap, false)},
			{Moniker: "RookVolumeMounts(true)", Mounts: RookVolumeMounts(dataPathMap, false)},
		},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)

	volsMountsTestDef = test.VolumesAndMountsTestDefinition{
		VolumesSpec: &test.VolumesSpec{
			Moniker: "PodVolumes(\"/dev/sdc\")", Volumes: PodVolumes(dataPathMap, "/dev/sdc", clusterSpec.DataDirHostPath, true),
		},
		MountsSpecItems: []*test.MountsSpec{
			{Moniker: "CephVolumeMounts(false)", Mounts: CephVolumeMounts(dataPathMap, true)},
			{Moniker: "RookVolumeMounts(false)", Mounts: RookVolumeMounts(dataPathMap, true)},
		},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)
}

func TestCheckPodMemory(t *testing.T) {
	// This value is in MB
	const PodMinimumMemory uint64 = 1024
	name := "test"

	// A value for the memory used in the tests
	// nolint:gosec // G115 no overflow expected in the test
	memory_value := int64(PodMinimumMemory * 8 * uint64(math.Pow10(6)))

	// Case 1: No memory limits, no memory requested
	test_resource := v1.ResourceRequirements{}

	if err := CheckPodMemory(name, test_resource, PodMinimumMemory); err != nil {
		t.Errorf("Error case 1: %s", err.Error())
	}

	// Case 2: memory limit and memory requested
	test_resource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(memory_value, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(memory_value, resource.BinarySI),
		},
	}

	if err := CheckPodMemory(name, test_resource, PodMinimumMemory); err != nil {
		t.Errorf("Error case 2: %s", err.Error())
	}

	// Only memory requested
	test_resource = v1.ResourceRequirements{
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(memory_value, resource.BinarySI),
		},
	}

	if err := CheckPodMemory(name, test_resource, PodMinimumMemory); err != nil {
		t.Errorf("Error case 3: %s", err.Error())
	}
}

func TestBuildAdminSocketCommand(t *testing.T) {
	c := getDaemonConfig(config.OsdType, "")

	command := c.buildAdminSocketCommand()
	assert.Equal(t, "status", command)

	c.daemonType = config.MonType
	command = c.buildAdminSocketCommand()
	assert.Equal(t, "mon_status", command)
}

func TestBuildSocketName(t *testing.T) {
	daemonID := "0"
	c := getDaemonConfig(config.OsdType, daemonID)

	socketName := c.buildSocketName()
	assert.Equal(t, "ceph-osd.0.asok", socketName)

	c.daemonType = config.MonType
	c.daemonID = "a"
	socketName = c.buildSocketName()
	assert.Equal(t, "ceph-mon.a.asok", socketName)
}

func TestBuildSocketPath(t *testing.T) {
	daemonID := "0"
	c := getDaemonConfig(config.OsdType, daemonID)

	socketPath := c.buildSocketPath()
	assert.Equal(t, "/run/ceph/ceph-osd.0.asok", socketPath)
}

func TestGenerateLivenessProbeExecDaemon(t *testing.T) {
	daemonID := "0"
	probe := GenerateLivenessProbeExecDaemon(config.OsdType, daemonID)
	expectedCommand := []string{
		"env",
		"-i",
		"sh",
		"-c",
		fmt.Sprintf(osdLivenessProbeScript, "/run/ceph/ceph-osd.0.asok", "status"),
	}

	assert.Equal(t, expectedCommand, probe.ProbeHandler.Exec.Command)
	assert.Equal(t, livenessProbeInitialDelaySeconds, probe.InitialDelaySeconds)
	assert.Equal(t, livenessProbeTimeoutSeconds, probe.TimeoutSeconds)

	// test with a mon so the delay should be 10
	probe = GenerateLivenessProbeExecDaemon(config.MonType, "a")
	assert.Equal(t, livenessProbeInitialDelaySeconds, probe.InitialDelaySeconds)
	assert.Equal(t, livenessProbeTimeoutSeconds, probe.TimeoutSeconds)
}

func TestDaemonFlags(t *testing.T) {
	testcases := []struct {
		label       string
		clusterInfo *cephclient.ClusterInfo
		clusterSpec *cephv1.ClusterSpec
		daemonID    string
		expected    []string
	}{
		{
			label: "case 1: IPv6 enabled",
			clusterInfo: &cephclient.ClusterInfo{
				FSID: "id",
			},
			clusterSpec: &cephv1.ClusterSpec{
				Network: cephv1.NetworkSpec{
					IPFamily: "IPv6",
				},
			},
			daemonID: "daemon-id",
			expected: []string{
				"--fsid=id", "--keyring=/etc/ceph/keyring-store/keyring", "--default-log-to-stderr=true", "--default-err-to-stderr=true",
				"--default-mon-cluster-log-to-stderr=true", "--default-log-stderr-prefix=debug ", "--default-log-to-file=false", "--default-mon-cluster-log-to-file=false",
				"--mon-host=$(ROOK_CEPH_MON_HOST)", "--mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS)", "--id=daemon-id", "--setuser=ceph", "--setgroup=ceph",
				"--ms-bind-ipv4=false", "--ms-bind-ipv6=true",
			},
		},
		{
			label: "case 2: IPv6 disabled",
			clusterInfo: &cephclient.ClusterInfo{
				FSID: "id",
			},
			clusterSpec: &cephv1.ClusterSpec{},
			daemonID:    "daemon-id",
			expected: []string{
				"--fsid=id", "--keyring=/etc/ceph/keyring-store/keyring", "--default-log-to-stderr=true", "--default-err-to-stderr=true",
				"--default-mon-cluster-log-to-stderr=true", "--default-log-stderr-prefix=debug ", "--default-log-to-file=false", "--default-mon-cluster-log-to-file=false",
				"--mon-host=$(ROOK_CEPH_MON_HOST)", "--mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS)", "--id=daemon-id", "--setuser=ceph", "--setgroup=ceph",
			},
		},
	}

	for _, tc := range testcases {
		actual := DaemonFlags(tc.clusterInfo, tc.clusterSpec, tc.daemonID)
		assert.Equalf(t, tc.expected, actual, "[%s]: failed to get correct daemonset flags", tc.label)
	}
}

func TestNetworkBindingFlags(t *testing.T) {
	ipv4FlagTrue := "--ms-bind-ipv4=true"
	ipv4FlagFalse := "--ms-bind-ipv4=false"
	ipv6FlagTrue := "--ms-bind-ipv6=true"
	ipv6FlagFalse := "--ms-bind-ipv6=false"
	type args struct {
		cluster *cephclient.ClusterInfo
		spec    *cephv1.ClusterSpec
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{"ipv4", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Reef}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv4}}}, []string{ipv4FlagTrue, ipv6FlagFalse}},
		{"ipv6", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Reef}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv6}}}, []string{ipv4FlagFalse, ipv6FlagTrue}},
		{"dualstack-supported", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Reef}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv6, DualStack: true}}}, []string{ipv4FlagTrue, ipv6FlagTrue}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NetworkBindingFlags(tt.args.cluster, tt.args.spec); !reflect.DeepEqual(got, tt.want) {
				if len(got) != 0 && len(tt.want) != 0 {
					t.Errorf("NetworkBindingFlags() = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestExtractMgrIP(t *testing.T) {
	activeMgrRaw := "172.17.0.12:6801/2535462469"
	ip := extractMgrIP(activeMgrRaw)
	assert.Equal(t, "172.17.0.12", ip)
}

func TestConfigureExternalMetricsEndpoint(t *testing.T) {
	clusterInfo := cephclient.AdminTestClusterInfo("rook-ceph")
	t.Run("spec and current active mgr endpoint identical with no existing endpoint object", func(t *testing.T) {
		monitoringSpec := cephv1.MonitoringSpec{
			Enabled:              true,
			ExternalMgrEndpoints: []v1.EndpointAddress{{IP: "192.168.0.1"}},
		}
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[1] == "dump" {
					return fmt.Sprintf(`{"active_addr":"%s"}`, "192.168.0.1:6801/2535462469"), nil
				}
				return "", errors.New("unknown command")
			},
		}

		ctx := &clusterd.Context{
			Clientset:     test.New(t, 3),
			RookClientset: rookclient.NewSimpleClientset(),
			Executor:      executor,
		}

		err := ConfigureExternalMetricsEndpoint(ctx, monitoringSpec, clusterInfo, cephclient.NewMinimumOwnerInfo(t))
		assert.NoError(t, err)

		currentEndpoints, err := ctx.Clientset.CoreV1().Endpoints(namespace).Get(context.TODO(), "rook-ceph-mgr-external", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "192.168.0.1", currentEndpoints.Subsets[0].Addresses[0].IP, currentEndpoints)
	})

	t.Run("spec and current active mgr endpoint different with no existing endpoint object", func(t *testing.T) {
		monitoringSpec := cephv1.MonitoringSpec{
			Enabled:              true,
			ExternalMgrEndpoints: []v1.EndpointAddress{{IP: "192.168.0.1"}},
		}
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[1] == "dump" {
					return fmt.Sprintf(`{"active_addr":"%s"}`, "172.17.0.12:6801/2535462469"), nil
				}
				return "", errors.New("unknown command")
			},
		}
		ctx := &clusterd.Context{
			Clientset:     test.New(t, 3),
			RookClientset: rookclient.NewSimpleClientset(),
			Executor:      executor,
		}

		err := ConfigureExternalMetricsEndpoint(ctx, monitoringSpec, clusterInfo, cephclient.NewMinimumOwnerInfo(t))
		assert.NoError(t, err)

		currentEndpoints, err := ctx.Clientset.CoreV1().Endpoints(namespace).Get(context.TODO(), "rook-ceph-mgr-external", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "172.17.0.12", currentEndpoints.Subsets[0].Addresses[0].IP, currentEndpoints)
	})

	t.Run("spec and current active mgr endpoint different with existing endpoint object", func(t *testing.T) {
		monitoringSpec := cephv1.MonitoringSpec{
			Enabled:              true,
			ExternalMgrEndpoints: []v1.EndpointAddress{{IP: "192.168.0.1"}},
		}
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[1] == "dump" {
					return fmt.Sprintf(`{"active_addr":"%s"}`, "172.17.0.12:6801/2535462469"), nil
				}
				return "", errors.New("unknown command")
			},
		}
		ctx := &clusterd.Context{
			Clientset:     test.New(t, 3),
			RookClientset: rookclient.NewSimpleClientset(),
			Executor:      executor,
		}
		ownerInfo := cephclient.NewMinimumOwnerInfo(t)
		ep, err := createExternalMetricsEndpoints(clusterInfo.Namespace, monitoringSpec, ownerInfo)
		assert.NoError(t, err)
		_, err = ctx.Clientset.CoreV1().Endpoints(namespace).Create(context.TODO(), ep, metav1.CreateOptions{})
		assert.NoError(t, err)

		err = ConfigureExternalMetricsEndpoint(ctx, monitoringSpec, clusterInfo, ownerInfo)
		assert.NoError(t, err)

		currentEndpoints, err := ctx.Clientset.CoreV1().Endpoints(namespace).Get(context.TODO(), "rook-ceph-mgr-external", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "172.17.0.12", currentEndpoints.Subsets[0].Addresses[0].IP, currentEndpoints)
	})

	t.Run("spec and current active mgr endpoint identical with existing endpoint object", func(t *testing.T) {
		monitoringSpec := cephv1.MonitoringSpec{
			Enabled:              true,
			ExternalMgrEndpoints: []v1.EndpointAddress{{IP: "192.168.0.1"}},
		}
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[1] == "dump" {
					return fmt.Sprintf(`{"active_addr":"%s"}`, "192.168.0.1:6801/2535462469"), nil
				}
				return "", errors.New("unknown command")
			},
		}
		ctx := &clusterd.Context{
			Clientset:     test.New(t, 3),
			RookClientset: rookclient.NewSimpleClientset(),
			Executor:      executor,
		}
		ownerInfo := cephclient.NewMinimumOwnerInfo(t)
		ep, err := createExternalMetricsEndpoints(clusterInfo.Namespace, monitoringSpec, ownerInfo)
		assert.NoError(t, err)
		_, err = ctx.Clientset.CoreV1().Endpoints(namespace).Create(context.TODO(), ep, metav1.CreateOptions{})
		assert.NoError(t, err)

		err = ConfigureExternalMetricsEndpoint(ctx, monitoringSpec, clusterInfo, ownerInfo)
		assert.NoError(t, err)

		currentEndpoints, err := ctx.Clientset.CoreV1().Endpoints(namespace).Get(context.TODO(), "rook-ceph-mgr-external", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "192.168.0.1", currentEndpoints.Subsets[0].Addresses[0].IP, currentEndpoints)
	})
}

func TestLogCollectorContainer(t *testing.T) {
	daemonId := "ceph-mon-a"
	ns := "rook-ceph"

	t.Run("Periodicity 1d and no MaxlogSize", func(t *testing.T) {
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, Periodicity: "1d"}}
		got := LogCollectorContainer(daemonId, ns, c, nil)
		want := fmt.Sprintf(cronLogRotate, daemonId, "daily", "0", "7", "")
		assert.Equal(t, want, got.Command[5])
	})

	t.Run("Periodicity 1h and MaxlogSize 1M", func(t *testing.T) {
		maxsize, _ := resource.ParseQuantity("1M")
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, Periodicity: "1h", MaxLogSize: &maxsize}}
		got := LogCollectorContainer("ceph-client.rbd-mirror.a", ns, c, nil)
		want := fmt.Sprintf(cronLogRotate, "ceph-client.rbd-mirror.a", "hourly", "1M", "28", "")
		assert.Equal(t, want, got.Command[5])
	})

	t.Run("Periodicity weekly and 1G MaxlogSize", func(t *testing.T) {
		maxsize, _ := resource.ParseQuantity("1Gi")
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, Periodicity: "weekly", MaxLogSize: &maxsize}}
		got := LogCollectorContainer(daemonId, ns, c, nil)
		want := fmt.Sprintf(cronLogRotate, daemonId, "weekly", "1073M", "7", "")
		assert.Equal(t, want, got.Command[5])
	})

	t.Run("MaxlogSize 1M and no Periodicity", func(t *testing.T) {
		maxsize, _ := resource.ParseQuantity("1Mi")
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, MaxLogSize: &maxsize}}
		got := LogCollectorContainer(daemonId, ns, c, nil)
		want := fmt.Sprintf(cronLogRotate, daemonId, "daily", "1M", "7", "")
		assert.Equal(t, want, got.Command[5])
	})

	t.Run("10G MaxlogSize and Periodicity 1d", func(t *testing.T) {
		maxsize, _ := resource.ParseQuantity("10G")
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, Periodicity: "1d", MaxLogSize: &maxsize}}
		got := LogCollectorContainer(daemonId, ns, c, nil)
		want := fmt.Sprintf(cronLogRotate, daemonId, "daily", "10G", "7", "")
		assert.Equal(t, want, got.Command[5])
	})

	t.Run("10M MaxlogSize and Periodicity weekly", func(t *testing.T) {
		maxsize, _ := resource.ParseQuantity("10Mi")
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, Periodicity: "weekly", MaxLogSize: &maxsize}}
		got := LogCollectorContainer(daemonId, ns, c, nil)
		want := fmt.Sprintf(cronLogRotate, daemonId, "weekly", "10M", "7", "")
		assert.Equal(t, want, got.Command[5])
	})

	t.Run("1M MaxlogSize and Periodicity 1d", func(t *testing.T) {
		maxsize, _ := resource.ParseQuantity("1M")
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, Periodicity: "1d", MaxLogSize: &maxsize}}
		got := LogCollectorContainer(daemonId, ns, c, nil)
		want := fmt.Sprintf(cronLogRotate, daemonId, "daily", "1M", "7", "")
		assert.Equal(t, want, got.Command[5])
	})

	t.Run("500k MaxlogSize and Periodicity 1d", func(t *testing.T) {
		maxsize, _ := resource.ParseQuantity("500K")
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, Periodicity: "1d", MaxLogSize: &maxsize}}
		got := LogCollectorContainer(daemonId, ns, c, nil)
		want := fmt.Sprintf(cronLogRotate, daemonId, "daily", "1M", "7", "")
		assert.Equal(t, want, got.Command[5])
	})

	t.Run("AdditionalLogFile, 500k MaxlogSize and Periodicity 1d", func(t *testing.T) {
		additionalLogFile := "/tmp/ceph_test_log_500KB.log"
		maxsize, _ := resource.ParseQuantity("500K")
		c := cephv1.ClusterSpec{LogCollector: cephv1.LogCollectorSpec{Enabled: true, Periodicity: "1d", MaxLogSize: &maxsize}}
		got := LogCollectorContainer(daemonId, ns, c, nil, additionalLogFile)

		want := fmt.Sprintf(cronLogRotate, daemonId, "daily", "1M", "7", additionalLogFile)
		assert.Equal(t, want, got.Command[5])
	})
}

func TestGetContainerImagePullPolicy(t *testing.T) {
	t.Run("containerImagePullPolicy is set in cluster CR", func(t *testing.T) {
		containerImagePullPolicy := v1.PullAlways

		imagePullPolicy := GetContainerImagePullPolicy(containerImagePullPolicy)
		assert.Equal(t, containerImagePullPolicy, imagePullPolicy)
	})

	t.Run("containerImagePullPolicy is empty", func(t *testing.T) {
		exepctedImagePullPolicy := v1.PullIfNotPresent
		imagePullPolicy := GetContainerImagePullPolicy(v1.PullPolicy(""))
		assert.Equal(t, exepctedImagePullPolicy, imagePullPolicy)
	})
}

func TestDaemonEnvVars(t *testing.T) {
	// No network settings specified
	want := []v1.EnvVar{}

	clusterSpec := &cephv1.ClusterSpec{}
	got := ApplyNetworkEnv(clusterSpec)
	assert.Equal(t, want, got)

	// When Encryption is enabled
	connections := &cephv1.ConnectionsSpec{
		Encryption: &cephv1.EncryptionSpec{Enabled: true},
	}

	clusterSpec.Network = cephv1.NetworkSpec{Connections: connections}

	want = []v1.EnvVar{{
		Name:  "ROOK_MSGR2",
		Value: "msgr2_false_encryption_true_compression_false",
	}}

	got = ApplyNetworkEnv(clusterSpec)
	assert.Equal(t, want, got)

	// When Compression is enabled
	connections = &cephv1.ConnectionsSpec{
		Compression: &cephv1.CompressionSpec{
			Enabled: true,
		},
	}
	clusterSpec.Network = cephv1.NetworkSpec{Connections: connections}

	want = []v1.EnvVar{{
		Name:  "ROOK_MSGR2",
		Value: "msgr2_false_encryption_false_compression_true",
	}}

	got = ApplyNetworkEnv(clusterSpec)
	assert.Equal(t, want, got)

	// When Msgr2 is enabled
	connections = &cephv1.ConnectionsSpec{
		RequireMsgr2: true,
	}
	clusterSpec.Network = cephv1.NetworkSpec{Connections: connections}

	want = []v1.EnvVar{{
		Name:  "ROOK_MSGR2",
		Value: "msgr2_true_encryption_false_compression_false",
	}}

	got = ApplyNetworkEnv(clusterSpec)
	assert.Equal(t, want, got)

	// When Msgr2, Compression, Encryption are enabled
	connections = &cephv1.ConnectionsSpec{
		RequireMsgr2: true,
		Encryption: &cephv1.EncryptionSpec{
			Enabled: true,
		},
		Compression: &cephv1.CompressionSpec{
			Enabled: true,
		},
	}
	clusterSpec.Network = cephv1.NetworkSpec{Connections: connections}

	want = []v1.EnvVar{{
		Name:  "ROOK_MSGR2",
		Value: "msgr2_true_encryption_true_compression_true",
	}}

	got = ApplyNetworkEnv(clusterSpec)
	assert.Equal(t, want, got)

	// When Msgr2 is enabled but Compression, Encryption are disabled
	connections = &cephv1.ConnectionsSpec{
		RequireMsgr2: true,
		Encryption: &cephv1.EncryptionSpec{
			Enabled: false,
		},
		Compression: &cephv1.CompressionSpec{
			Enabled: false,
		},
	}
	clusterSpec.Network = cephv1.NetworkSpec{Connections: connections}

	want = []v1.EnvVar{{
		Name:  "ROOK_MSGR2",
		Value: "msgr2_true_encryption_false_compression_false",
	}}

	got = ApplyNetworkEnv(clusterSpec)
	assert.Equal(t, want, got)
}
