/*
Copyright 2023 The Rook Authors. All rights reserved.

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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type mockCmdReporter struct {
	mock.Mock

	job *batch.Job
}

func (m *mockCmdReporter) Job() *batch.Job {
	m.Called()
	return m.job
}

func (m *mockCmdReporter) Run(ctx context.Context, timeout time.Duration) (stdout, stderr string, retcode int, retErr error) {
	args := m.Called(ctx, timeout)
	return args.String(0), args.String(1), args.Int(2), args.Error(3)
}

// returns a newCmdReporter function that returns the given mockCmdReporter and error
func mockNewCmdReporter(m *mockCmdReporter, err error) func(clientset kubernetes.Interface, ownerInfo *k8sutil.OwnerInfo, appName string, jobName string, jobNamespace string, cmd []string, args []string, rookImage string, runImage string, imagePullPolicy v1.PullPolicy, resources cephv1.ResourceSpec) (cmdreporter.CmdReporterInterface, error) {
	return func(clientset kubernetes.Interface, ownerInfo *k8sutil.OwnerInfo, appName, jobName, jobNamespace string, cmd, args []string, rookImage, runImage string, imagePullPolicy v1.PullPolicy, resources cephv1.ResourceSpec) (cmdreporter.CmdReporterInterface, error) {
		job, err := cmdreporter.MockCmdReporterJob(clientset, ownerInfo, appName, jobName, jobNamespace, cmd, args, rookImage, runImage, imagePullPolicy, resources)
		if err != nil {
			// okay to panic here because this is a unit test setup failure, not part of code testing
			panic(fmt.Sprintf("error setting up mock CmdReporter job: %v", err))
		}
		m.job = job
		return m, err
	}
}

func newTestConfigsWithNetworkSpec(n cephv1.NetworkSpec) (*clusterd.Context, *cephv1.ClusterSpec, *client.ClusterInfo) {
	clusterdCtx := &clusterd.Context{}
	clusterSpec := &cephv1.ClusterSpec{
		Network: n,
	}
	clusterInfo := &client.ClusterInfo{
		Namespace:   "ns",
		NetworkSpec: clusterSpec.Network,
		OwnerInfo:   k8sutil.NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{}, "ns"),
	}
	return clusterdCtx, clusterSpec, clusterInfo
}

func Test_discoverAddressRanges(t *testing.T) {
	oldNewCmdReporter := newCmdReporter
	defer func() { newCmdReporter = oldNewCmdReporter }()
	t.Run("public network selected", func(t *testing.T) {
		clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
			cephv1.NetworkSpec{
				Provider: cephv1.NetworkProviderMultus,
				Selectors: map[cephv1.CephNetworkType]string{
					cephv1.CephNetworkPublic: "macvlan-public",
				},
				AddressRanges: nil, // should auto-detect
			})

		t.Run("discover public cidrs", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("public", "2000::1") +
				"\n" + separator() + "\n" +
				ipAddrOutput("public", "2000::1/112")
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.NoError(t, err)
			assert.Equal(t, []string{"2000::/112"}, ranges)
		})
		t.Run("discover cluster cidrs", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			// no cmd reporter methods should be called b/c cluster net is not selected
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkCluster)
			assert.NoError(t, err)
			assert.Equal(t, []string{}, ranges)
		})
		t.Run("public ip mismatch", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("public", "192.168.1.1") +
				"\n" + separator() + "\n" +
				ipAddrOutput("public", "192.168.1.2/24") // IP is one more than above
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.Error(t, err)
			assert.Equal(t, []string{}, ranges)
		})
		t.Run("public ip iface mismatch", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("public", "192.168.1.1") +
				"\n" + separator() + "\n" +
				ipAddrOutput("cluster", "192.168.1.1/24") // cluster != public above
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.Error(t, err)
			assert.Equal(t, []string{}, ranges)
		})
		t.Run("iface wrong", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("cluster", "192.168.1.1") +
				"\n" + separator() + "\n" +
				ipAddrOutput("cluster", "192.168.1.1/24") // cluster != public above
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.Error(t, err)
			assert.Equal(t, []string{}, ranges)
		})
		t.Run("no net status", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := "" + // no net status
				"\n" + separator() + "\n" +
				ipAddrOutput("public", "192.168.1.1/24")
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.Error(t, err)
			assert.Equal(t, []string{}, ranges)
		})
		t.Run("no ip addr output", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("public", "2000::1") +
				"\n" + separator() + "\n" +
				"" // no ip addr output
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.Error(t, err)
			assert.Equal(t, []string{}, ranges)
		})
		t.Run("cmdreporter run failure", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return("", "", 0, errors.New("fake err"))
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.Error(t, err)
			assert.Equal(t, []string{}, ranges)
		})
		t.Run("cmdreporter nonzero exit", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return("some garbage", "", 1, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.Error(t, err)
			assert.Equal(t, []string{}, ranges)
		})
	})

	t.Run("cluster network selected", func(t *testing.T) {
		clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
			cephv1.NetworkSpec{
				Provider: cephv1.NetworkProviderMultus,
				Selectors: map[cephv1.CephNetworkType]string{
					cephv1.CephNetworkCluster: "macvlan-cluster",
				},
				AddressRanges: nil, // should auto-detect
			})

		t.Run("discover cluster cidrs", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("cluster", "10.144.1.5") +
				"\n" + separator() + "\n" +
				ipAddrOutput("cluster", "10.144.1.5/16")
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkCluster)
			assert.NoError(t, err)
			assert.Equal(t, []string{"10.144.0.0/16"}, ranges)
		})
		t.Run("discover public cidrs", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			// no cmd reporter methods should be called b/c public net is not selected
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.NoError(t, err)
			assert.Equal(t, []string{}, ranges)
		})
	})

	t.Run("public and cluster networks selected", func(t *testing.T) {
		clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
			cephv1.NetworkSpec{
				Provider: cephv1.NetworkProviderMultus,
				Selectors: map[cephv1.CephNetworkType]string{
					cephv1.CephNetworkPublic:  "macvlan-public",
					cephv1.CephNetworkCluster: "macvlan-cluster",
				},
				AddressRanges: nil, // should auto-detect
			})

		t.Run("discover public cidrs", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("public", "10.144.1.5") +
				"\n" + separator() + "\n" +
				ipAddrOutput("public", "10.144.1.5/16")
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.NoError(t, err)
			assert.Equal(t, []string{"10.144.0.0/16"}, ranges)
		})
		t.Run("discover cluster cidrs", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("cluster", "1abc::1") +
				"\n" + separator() + "\n" +
				ipAddrOutput("cluster", "1abc::1/112")
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkCluster)
			assert.NoError(t, err)
			assert.Equal(t, []string{"1abc::/112"}, ranges)
		})
		t.Run("public multiple ips", func(t *testing.T) {
			cmdReporter := new(mockCmdReporter)
			cmdReporter.On("Job")
			goodOutput := netStatus("public", "10.144.1.5", "fe05::6") +
				"\n" + separator() + "\n" +
				ipAddrOutput("public", "10.144.1.5/16", "fe05::6/96")
			cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
			newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

			ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			assert.NoError(t, err)
			assert.Equal(t, []string{"10.144.0.0/16", "fe05::/96"}, ranges)
		})
	})

	t.Run("cluster config with all placement", func(t *testing.T) {
		tolerations := []v1.Toleration{
			{
				Key:      "testkey",
				Operator: v1.TolerationOpExists,
			},
		}
		clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
			cephv1.NetworkSpec{
				Provider: cephv1.NetworkProviderMultus,
				Selectors: map[cephv1.CephNetworkType]string{
					cephv1.CephNetworkPublic:  "macvlan-public",
					cephv1.CephNetworkCluster: "macvlan-cluster",
				},
				AddressRanges: nil, // should auto-detect
			})
		clusterSpec.Placement = map[cephv1.KeyType]cephv1.Placement{
			cephv1.KeyAll: {Tolerations: tolerations},
		}

		cmdReporter := new(mockCmdReporter)
		cmdReporter.On("Job")
		goodOutput := netStatus("public", "2000::1") +
			"\n" + separator() + "\n" +
			ipAddrOutput("public", "2000::1/112")
		cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
		newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

		ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
		assert.NoError(t, err)
		assert.Equal(t, []string{"2000::/112"}, ranges)
		assert.Equal(t, tolerations, cmdReporter.Job().Spec.Template.Spec.Tolerations)
	})

	t.Run("cluster config with osd placement", func(t *testing.T) {
		tolerations := []v1.Toleration{
			{
				Key:      "testkey",
				Operator: v1.TolerationOpExists,
			},
		}
		clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
			cephv1.NetworkSpec{
				Provider: cephv1.NetworkProviderMultus,
				Selectors: map[cephv1.CephNetworkType]string{
					cephv1.CephNetworkPublic:  "macvlan-public",
					cephv1.CephNetworkCluster: "macvlan-cluster",
				},
				AddressRanges: nil, // should auto-detect
			})
		clusterSpec.Placement = map[cephv1.KeyType]cephv1.Placement{
			cephv1.KeyOSD: {Tolerations: tolerations},
		}

		cmdReporter := new(mockCmdReporter)
		cmdReporter.On("Job")
		goodOutput := netStatus("public", "2000::1") +
			"\n" + separator() + "\n" +
			ipAddrOutput("public", "2000::1/112")
		cmdReporter.On("Run", mock.Anything, mock.Anything).Return(goodOutput, "", 0, nil)
		newCmdReporter = mockNewCmdReporter(cmdReporter, nil)

		ranges, err := discoverAddressRanges(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
		assert.NoError(t, err)
		assert.Equal(t, []string{"2000::/112"}, ranges)
		assert.Equal(t, tolerations, cmdReporter.Job().Spec.Template.Spec.Tolerations)
	})
}

func mockDiscoverAddressRangesFunc(publicReturn []string, clusterReturn []string, panicIfDiscoverPublic, panicIfDiscoverCluster bool, shouldErr error) func(ctx context.Context, rookImage string, clusterdContext *clusterd.Context, clusterSpec *cephv1.ClusterSpec, clusterInfo *client.ClusterInfo, discoverPublic bool, discoverCluster bool) (publicRanges []cephv1.CIDR, clusterRanges []cephv1.CIDR, err error) {
	return func(ctx context.Context, rookImage string, clusterdContext *clusterd.Context, clusterSpec *cephv1.ClusterSpec, clusterInfo *client.ClusterInfo, discoverPublic bool, discoverCluster bool) (publicRanges []cephv1.CIDR, clusterRanges []cephv1.CIDR, err error) {
		// panic is fine for unit testing
		if panicIfDiscoverPublic && discoverPublic {
			panic("should not discover public net")
		}
		if panicIfDiscoverCluster && discoverCluster {
			panic("should not discover cluster net")
		}
		return toCephCIDRs(publicReturn), toCephCIDRs(clusterReturn), shouldErr
	}
}

type mockMonStore struct {
	mock.Mock
}

func (m *mockMonStore) SetIfChanged(who string, option string, value string) (bool, error) {
	args := m.Called(who, option, value)
	return args.Bool(0), args.Error(1)
}

func mockGetMonStoreFunc(m *mockMonStore) func(context *clusterd.Context, clusterInfo *client.ClusterInfo) monStoreInterface {
	return func(context *clusterd.Context, clusterInfo *client.ClusterInfo) monStoreInterface {
		return m
	}
}

func TestApplyCephNetworkSettings(t *testing.T) {
	oldDiscoverCephAddressRangesFunc := discoverCephAddressRangesFunc
	defer func() { discoverCephAddressRangesFunc = oldDiscoverCephAddressRangesFunc }()

	oldGetMonStoreFunc := getMonStoreFunc
	defer func() { getMonStoreFunc = oldGetMonStoreFunc }()

	t.Run("no net defined", func(t *testing.T) {
		clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(cephv1.NetworkSpec{})

		discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, false, false, errors.New("should not be called"))

		monStore := new(mockMonStore)
		// implicit: mon store should not have SetIfChanged called on it
		getMonStoreFunc = mockGetMonStoreFunc(monStore)

		err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
		assert.NoError(t, err)
	})
	t.Run("multus", func(t *testing.T) {
		t.Run("public selected", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkPublic: "macvlan-public",
					},
					AddressRanges: nil, // should auto-detect
				})

			t.Run("single public cidr", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/16"}, []string{}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/16").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.NoError(t, err)
			})
			t.Run("multiple public cidrs", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/16", "feab::/112"}, []string{}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/16, feab::/112").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.NoError(t, err)
			})
		})
		t.Run("cluster selected", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkCluster: "macvlan-cluster",
					},
					AddressRanges: nil, // should auto-detect
				})

			t.Run("single cluster cidr", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{"192.168.0.0/16"}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "cluster_network", "192.168.0.0/16").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.NoError(t, err)
			})
			t.Run("multiple cluster cidrs", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{"192.168.0.0/16", "feab::/112"}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "cluster_network", "192.168.0.0/16, feab::/112").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.NoError(t, err)
			})
		})
		t.Run("public and cluster selected", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkPublic:  "macvlan-public",
						cephv1.CephNetworkCluster: "macvlan-cluster",
					},
					AddressRanges: nil, // should auto-detect
				})

			t.Run("single cidr", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/16"}, []string{"192.168.10.0/16"}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/16").Return(true, nil)
				monStore.On("SetIfChanged", "global", "cluster_network", "192.168.10.0/16").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.NoError(t, err)
			})
			t.Run("multiple cidrs", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/16", "feab::/112"}, []string{"192.168.10.0/16", "feab:10::/112"}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/16, feab::/112").Return(true, nil)
				monStore.On("SetIfChanged", "global", "cluster_network", "192.168.10.0/16, feab:10::/112").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.NoError(t, err)
			})
			t.Run("single and multiple cidrs", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/16"}, []string{"192.168.10.0/16", "feab:10::/112"}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/16").Return(true, nil)
				monStore.On("SetIfChanged", "global", "cluster_network", "192.168.10.0/16, feab:10::/112").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.NoError(t, err)
			})
			t.Run("multiple and single cidrs", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/16", "feab::/112"}, []string{"192.168.10.0/16"}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/16, feab::/112").Return(true, nil)
				monStore.On("SetIfChanged", "global", "cluster_network", "192.168.10.0/16").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.NoError(t, err)
			})
			t.Run("discover err", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, false, false, errors.New("fake err"))

				monStore := new(mockMonStore)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.ErrorContains(t, err, "fake err")
			})
			t.Run("public apply err", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/16"}, []string{"192.168.10.0/16"}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/16").Return(true, errors.New("fake err"))
				monStore.On("SetIfChanged", "global", "cluster_network", "192.168.10.0/16").Return(true, nil)
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.ErrorContains(t, err, "fake err")
			})
			t.Run("cluster apply err", func(t *testing.T) {
				discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/16"}, []string{"192.168.10.0/16"}, false, false, nil)

				monStore := new(mockMonStore)
				monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/16").Return(true, nil)
				monStore.On("SetIfChanged", "global", "cluster_network", "192.168.10.0/16").Return(true, errors.New("fake err"))
				getMonStoreFunc = mockGetMonStoreFunc(monStore)

				err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
				assert.ErrorContains(t, err, "fake err")
			})
		})
		t.Run("public selected with address ranges", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkPublic: "macvlan-public",
					},
					AddressRanges: &cephv1.AddressRangesSpec{
						Public: []cephv1.CIDR{
							"192.168.0.0/24",
						},
					},
				})

			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, true, false, nil)

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/24").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
		t.Run("cluster selected with address ranges", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkCluster: "macvlan-cluster",
					},
					AddressRanges: &cephv1.AddressRangesSpec{
						Cluster: []cephv1.CIDR{
							"192.168.0.0/24",
						},
					},
				})

			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, false, true, nil)

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "cluster_network", "192.168.0.0/24").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
		t.Run("public and cluster selected with address ranges", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkPublic:  "macvlan-public",
						cephv1.CephNetworkCluster: "macvlan-cluster",
					},
					AddressRanges: &cephv1.AddressRangesSpec{
						Public: []cephv1.CIDR{
							"192.168.0.0/24",
						},
						Cluster: []cephv1.CIDR{
							"192.168.50.0/24",
						},
					},
				})

			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, true, true, errors.New("should not be called"))

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/24").Return(true, nil)
			monStore.On("SetIfChanged", "global", "cluster_network", "192.168.50.0/24").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
		t.Run("public and cluster selected with multi-cidr address ranges", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkPublic:  "macvlan-public",
						cephv1.CephNetworkCluster: "macvlan-cluster",
					},
					AddressRanges: &cephv1.AddressRangesSpec{
						Public: []cephv1.CIDR{
							"192.168.0.0/24",
							"fabc::/112",
						},
						Cluster: []cephv1.CIDR{
							"192.168.50.0/24",
							"fabc:50::/112",
						},
					},
				})

			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, true, true, errors.New("should not be called"))

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/24, fabc::/112").Return(true, nil)
			monStore.On("SetIfChanged", "global", "cluster_network", "192.168.50.0/24, fabc:50::/112").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
		t.Run("public and cluster selected, only cluster addr ranges specified", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkPublic:  "macvlan-public",
						cephv1.CephNetworkCluster: "macvlan-cluster",
					},
					AddressRanges: &cephv1.AddressRangesSpec{
						Cluster: []cephv1.CIDR{
							"192.168.50.0/24",
						},
					},
				})

			clusterAddrsShouldNotBeDiscovered := true
			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{"192.168.0.0/24"}, []string{}, false, clusterAddrsShouldNotBeDiscovered, nil)

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/24").Return(true, nil)
			monStore.On("SetIfChanged", "global", "cluster_network", "192.168.50.0/24").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
		t.Run("public and cluster selected, only cluster public ranges specified", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider: cephv1.NetworkProviderMultus,
					Selectors: map[cephv1.CephNetworkType]string{
						cephv1.CephNetworkPublic:  "macvlan-public",
						cephv1.CephNetworkCluster: "macvlan-cluster",
					},
					AddressRanges: &cephv1.AddressRangesSpec{
						Public: []cephv1.CIDR{
							"192.168.0.0/24",
						},
					},
				})

			publicAddrsShouldNotBeDiscovered := true
			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{"192.168.50.0/24"}, publicAddrsShouldNotBeDiscovered, false, nil)

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/24").Return(true, nil)
			monStore.On("SetIfChanged", "global", "cluster_network", "192.168.50.0/24").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
	})
	t.Run("hostnet", func(t *testing.T) {
		t.Run("no ranges specified", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider:      cephv1.NetworkProviderHost,
					Selectors:     map[cephv1.CephNetworkType]string{},
					AddressRanges: nil,
				})

			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, false, false, errors.New("should not be called"))

			monStore := new(mockMonStore)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
		t.Run("public ranges specified", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider:  cephv1.NetworkProviderHost,
					Selectors: map[cephv1.CephNetworkType]string{},
					AddressRanges: &cephv1.AddressRangesSpec{
						Public: []cephv1.CIDR{
							"192.168.0.0/24",
							"feab::/112",
						},
					},
				})

			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, false, false, errors.New("should not be called"))

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/24, feab::/112").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
		t.Run("cluster ranges specified", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider:  cephv1.NetworkProviderHost,
					Selectors: map[cephv1.CephNetworkType]string{},
					AddressRanges: &cephv1.AddressRangesSpec{
						Cluster: []cephv1.CIDR{
							"192.168.0.0/24",
							"feab::/112",
						},
					},
				})

			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, false, false, errors.New("should not be called"))

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "cluster_network", "192.168.0.0/24, feab::/112").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
		t.Run("public and cluster ranges specified", func(t *testing.T) {
			clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(
				cephv1.NetworkSpec{
					Provider:  cephv1.NetworkProviderHost,
					Selectors: map[cephv1.CephNetworkType]string{},
					AddressRanges: &cephv1.AddressRangesSpec{
						Public: []cephv1.CIDR{
							"192.168.0.0/24",
							"feab::/112",
						},
						Cluster: []cephv1.CIDR{
							"192.168.100.0/24",
							"feab:100::/112",
						},
					},
				})

			discoverCephAddressRangesFunc = mockDiscoverAddressRangesFunc([]string{}, []string{}, false, false, errors.New("should not be called"))

			monStore := new(mockMonStore)
			monStore.On("SetIfChanged", "global", "public_network", "192.168.0.0/24, feab::/112").Return(true, nil)
			monStore.On("SetIfChanged", "global", "cluster_network", "192.168.100.0/24, feab:100::/112").Return(true, nil)
			getMonStoreFunc = mockGetMonStoreFunc(monStore)

			err := ApplyCephNetworkSettings(context.Background(), "rook/ceph:master", clusterdCtx, clusterSpec, clusterInfo)
			assert.NoError(t, err)
		})
	})
}

// generate a net status for interface (expected to be "public" or "cluster") with some number of IPs
func netStatus(iface string, ips ...string) string {
	ipsInJson := ""
	for _, ip := range ips {
		if ipsInJson != "" {
			ipsInJson += ","
		}
		ipsInJson = ipsInJson + "\n" + `        "` + ip + `"`
	}
	out := `[{
    "name": "bridge",
    "interface": "eth0",
    "ips": [
        "10.244.1.152"
    ],
    "mac": "1a:15:5d:61:18:50",
    "default": true,
    "dns": {},
    "gateway": [
        "10.244.0.1"
    ]
},{
    "name": "dont/care",
    "interface": "` + iface + `",
    "ips": [` + ipsInJson + `
    ],
    "mac": "8e:ec:4a:e3:1b:96",
    "dns": {}
}]`
	return out
}

// generate 'ip --json address show dev <iface>' output with some number of <ip>/<prefix>-es
// also contains extra IPv6 SLAAC addrs that are often present in containers
func ipAddrOutput(iface string, ipsWithPrefixLens ...string) string {
	ipsInJson := ""
	for _, ipWithPrefix := range ipsWithPrefixLens {
		s := strings.Split(ipWithPrefix, "/")
		ip := s[0]
		prefix := s[1]
		// specifically omit "family": "inet" from the output so it's certain that field isn't used
		// when parsing IPv4 vs IPv6 addrs
		ipsInJson = ipsInJson + `
            {
                "local": "` + ip + `",
                "prefixlen": ` + prefix + `,
                "scope": "global",
                "valid_life_time": 4294967295,
                "preferred_life_time": 4294967295
            },`
	}
	out := `[
    {
        "ifindex": 3,
        "link_index": 2,
        "ifname": "` + iface + `",
        "flags": [
            "BROADCAST",
            "MULTICAST",
            "UP",
            "LOWER_UP"
        ],
        "mtu": 1500,
        "qdisc": "noqueue",
        "operstate": "UP",
        "group": "default",
        "link_type": "ether",
        "address": "8e:ec:4a:e3:1b:96",
        "broadcast": "ff:ff:ff:ff:ff:ff",
        "link_netnsid": 0,
        "addr_info": [
            {
                "family": "inet6",
                "local": "fd4e:7658:764f:15c4:8cec:4aff:fee3:1b96",
                "prefixlen": 64,
                "scope": "global",
                "dynamic": true,
                "mngtmpaddr": true,
                "valid_life_time": 2591998,
                "preferred_life_time": 604798
            },` + ipsInJson + `
            {
                "family": "inet6",
                "local": "fe80::8cec:4aff:fee3:1b96",
                "prefixlen": 64,
                "scope": "link",
                "valid_life_time": 4294967295,
                "preferred_life_time": 4294967295
            }
        ]
    }
]`
	return out
}
