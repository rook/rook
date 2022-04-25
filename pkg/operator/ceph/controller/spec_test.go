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
	dataPathMap := config.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook")

	if err := test.VolumeIsEmptyDir(k8sutil.DataDirVolume, PodVolumes(dataPathMap, "", false)); err != nil {
		t.Errorf("PodVolumes(\"\") - data dir source is not EmptyDir: %s", err.Error())
	}
	if err := test.VolumeIsHostPath(k8sutil.DataDirVolume, "/dev/sdb", PodVolumes(dataPathMap, "/dev/sdb", false)); err != nil {
		t.Errorf("PodVolumes(\"/dev/sdb\") - data dir source is not HostPath: %s", err.Error())
	}
}

func TestMountsMatchVolumes(t *testing.T) {

	dataPathMap := config.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook")

	volsMountsTestDef := test.VolumesAndMountsTestDefinition{
		VolumesSpec: &test.VolumesSpec{
			Moniker: "PodVolumes(\"/dev/sdc\")", Volumes: PodVolumes(dataPathMap, "/dev/sdc", false)},
		MountsSpecItems: []*test.MountsSpec{
			{Moniker: "CephVolumeMounts(true)", Mounts: CephVolumeMounts(dataPathMap, false)},
			{Moniker: "RookVolumeMounts(true)", Mounts: RookVolumeMounts(dataPathMap, false)}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)

	volsMountsTestDef = test.VolumesAndMountsTestDefinition{
		VolumesSpec: &test.VolumesSpec{
			Moniker: "PodVolumes(\"/dev/sdc\")", Volumes: PodVolumes(dataPathMap, "/dev/sdc", true)},
		MountsSpecItems: []*test.MountsSpec{
			{Moniker: "CephVolumeMounts(false)", Mounts: CephVolumeMounts(dataPathMap, true)},
			{Moniker: "RookVolumeMounts(false)", Mounts: RookVolumeMounts(dataPathMap, true)}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)
}

func TestCheckPodMemory(t *testing.T) {
	// This value is in MB
	const PodMinimumMemory uint64 = 1024
	name := "test"

	// A value for the memory used in the tests
	var memory_value = int64(PodMinimumMemory * 8 * uint64(math.Pow10(6)))

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
	expectedCommand := []string{"env",
		"-i",
		"sh",
		"-c",
		"ceph --admin-daemon /run/ceph/ceph-osd.0.asok status",
	}

	assert.Equal(t, expectedCommand, probe.ProbeHandler.Exec.Command)
	assert.Equal(t, livenessProbeInitialDelaySeconds, probe.InitialDelaySeconds)

	// test with a mon so the delay should be 10
	probe = GenerateLivenessProbeExecDaemon(config.MonType, "a")
	assert.Equal(t, livenessProbeInitialDelaySeconds, probe.InitialDelaySeconds)
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
			expected: []string{"--fsid=id", "--keyring=/etc/ceph/keyring-store/keyring", "--log-to-stderr=true", "--err-to-stderr=true",
				"--mon-cluster-log-to-stderr=true", "--log-stderr-prefix=debug ", "--default-log-to-file=false", "--default-mon-cluster-log-to-file=false",
				"--mon-host=$(ROOK_CEPH_MON_HOST)", "--mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS)", "--id=daemon-id", "--setuser=ceph", "--setgroup=ceph",
				"--ms-bind-ipv4=false", "--ms-bind-ipv6=true"},
		},
		{
			label: "case 2: IPv6 disabled",
			clusterInfo: &cephclient.ClusterInfo{
				FSID: "id",
			},
			clusterSpec: &cephv1.ClusterSpec{},
			daemonID:    "daemon-id",
			expected: []string{"--fsid=id", "--keyring=/etc/ceph/keyring-store/keyring", "--log-to-stderr=true", "--err-to-stderr=true",
				"--mon-cluster-log-to-stderr=true", "--log-stderr-prefix=debug ", "--default-log-to-file=false", "--default-mon-cluster-log-to-file=false",
				"--mon-host=$(ROOK_CEPH_MON_HOST)", "--mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS)", "--id=daemon-id", "--setuser=ceph", "--setgroup=ceph"},
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
		{"octopus-ipv4", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Octopus}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv4}}}, []string{ipv4FlagTrue, ipv6FlagFalse}},
		{"octopus-ipv6", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Octopus}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv6}}}, []string{ipv4FlagFalse, ipv6FlagTrue}},
		{"octopus-dualstack-unsupported", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Octopus}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv4, DualStack: true}}}, []string{}},
		{"octopus-dualstack-unsupported-by-ipv6", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Octopus}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv6, DualStack: true}}}, []string{ipv6FlagTrue}},
		{"pacific-ipv4", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Pacific}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv4}}}, []string{ipv4FlagTrue, ipv6FlagFalse}},
		{"pacific-ipv6", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Pacific}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv6}}}, []string{ipv4FlagFalse, ipv6FlagTrue}},
		{"pacific-dualstack-supported", args{cluster: &cephclient.ClusterInfo{CephVersion: version.Pacific}, spec: &cephv1.ClusterSpec{Network: cephv1.NetworkSpec{IPFamily: cephv1.IPv6, DualStack: true}}}, []string{ipv4FlagTrue, ipv6FlagTrue}},
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
