/*
Copyright 2017 The Rook Authors. All rights reserved.

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
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testexec "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOSDHealthCheck(t *testing.T) {
	ctx := context.TODO()
	clientset := testexec.New(t, 2)
	clusterInfo := client.AdminTestClusterInfo("fake")

	execCount := 0
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
			execCount++
			if args[1] == "dump" {
				// Mock executor for OSD Dump command, returning an osd in Down state
				return `{"OSDs": [{"OSD": 0, "Up": 0, "In": 0}]}`, nil
			} else if args[1] == "safe-to-destroy" {
				// Mock executor for OSD Dump command, returning an osd in Down state
				return `{"safe_to_destroy":[0],"active":[],"missing_stats":[],"stored_pgs":[]}`, nil
			} else if args[0] == "auth" && args[1] == "get-or-create-key" {
				return "{\"key\":\"mysecurekey\", \"osdid\":3.0}", nil
			}
			return "", nil
		},
	}

	// Setting up objects needed to create OSD
	context := &clusterd.Context{
		Executor:  executor,
		Clientset: clientset,
	}

	labels := map[string]string{
		k8sutil.AppAttr:     AppName,
		k8sutil.ClusterAttr: clusterInfo.Namespace,
		OsdIdLabelKey:       "0",
	}

	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "osd0",
			Namespace: clusterInfo.Namespace,
			Labels:    labels,
		},
	}
	if _, err := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		logger.Errorf("Error creating fake deployment: %v", err)
	}

	// Check if the osd deployment is created
	dp, _ := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%d", OsdIdLabelKey, 0)})
	assert.Equal(t, 1, len(dp.Items))

	// Initializing an OSD monitoring
	osdMon := NewOSDHealthMonitor(context, clusterInfo, true, cephv1.CephClusterHealthCheckSpec{}, cephv1.ClusterSpec{}, "rook/ceph:test")

	// Run OSD monitoring routine
	err := osdMon.checkOSDDump(nil)
	assert.Nil(t, err)
	// After creating an OSD, the dump has 1 mocked cmd and safe to destroy has 1 mocked cmd
	assert.Equal(t, 2, execCount)

	// Check if the osd deployment was deleted
	dp, _ = context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%d", OsdIdLabelKey, 0)})
	assert.Equal(t, 0, len(dp.Items))
}

func TestMonitorStart(t *testing.T) {
	context, cancel := context.WithCancel(context.TODO())
	var monitoringRoutines sync.Map
	monitoringRoutines.Store("osd", &opcontroller.ClusterHealth{
		InternalCtx:    context,
		InternalCancel: cancel,
	})

	osdMon := NewOSDHealthMonitor(&clusterd.Context{}, client.AdminTestClusterInfo("ns"), true, cephv1.CephClusterHealthCheckSpec{}, cephv1.ClusterSpec{}, "rook/ceph:test")
	logger.Infof("starting osd monitor")
	go osdMon.Start(&monitoringRoutines, "osd")
	cancel()
}

func TestNewOSDHealthMonitor(t *testing.T) {
	clusterInfo := client.AdminTestClusterInfo("test")
	c := &clusterd.Context{}
	time10s, _ := time.ParseDuration("10s")
	type args struct {
		context                        *clusterd.Context
		removeOSDsIfOUTAndSafeToRemove bool
		healthCheck                    cephv1.CephClusterHealthCheckSpec
	}
	tests := []struct {
		name         string
		args         args
		wantInterval *time.Duration
	}{
		{"default-interval", args{c, false, cephv1.CephClusterHealthCheckSpec{}}, &defaultHealthCheckInterval},
		{"10s-interval", args{c, false, cephv1.CephClusterHealthCheckSpec{DaemonHealth: cephv1.DaemonHealthSpec{ObjectStorageDaemon: cephv1.HealthCheckSpec{Interval: &metav1.Duration{Duration: time10s}}}}}, &time10s},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewOSDHealthMonitor(tt.args.context, clusterInfo, tt.args.removeOSDsIfOUTAndSafeToRemove, tt.args.healthCheck, cephv1.ClusterSpec{}, "rook/ceph:test")
			assert.Equal(t, tt.args.context, got.context)
			assert.Equal(t, clusterInfo, got.clusterInfo)
			assert.Equal(t, tt.args.removeOSDsIfOUTAndSafeToRemove, got.removeOSDsIfOUTAndSafeToRemove)
			assert.Equal(t, tt.wantInterval, got.interval)
			assert.Equal(t, "", got.lastRequireOSDRelease)
			// The monitor must build a Cluster so it can drive the replacement state machine.
			assert.NotNil(t, got.cluster)
		})
	}
}

// TestCheckRequireOSDRelease validates checkRequireOSDRelease() which runs on every
// health-monitor tick. The method:
//  1. Queries "ceph versions" to get all OSD daemon versions
//  2. If exactly one OSD version exists (all converged), extracts the release name
//  3. Skips if the release name matches the cached value (avoids redundant ceph calls)
//  4. Calls "ceph osd require-osd-release <name>" and caches on success
//
// Each subtest targets one branch in this flow.
func TestCheckRequireOSDRelease(t *testing.T) {
	clusterInfo := client.AdminTestClusterInfo("test")

	// HAPPY PATH: All OSDs upgraded to squid, cache is empty → should set the flag and cache it.
	t.Run("single converged version sets lastRequireOSDRelease", func(t *testing.T) {
		enableCalls := 0
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				// handles: ceph versions
				if args[0] == "versions" {
					// All 3 OSDs report the same version (single map entry)
					return `{"osd":{"ceph version 19.2.0 (abc) squid (stable)":3}}`, nil
				}
				// handles: ceph osd require-osd-release squid
				if args[0] == "osd" && args[1] == "require-osd-release" {
					assert.Equal(t, "squid", args[2])
					enableCalls++
					return "", nil
				}
				return "", nil
			},
		}
		ctx := &clusterd.Context{Executor: executor}
		mon := &OSDHealthMonitor{
			context:     ctx,
			clusterInfo: clusterInfo,
			// lastRequireOSDRelease is "" (zero value) — nothing cached yet
		}

		mon.checkRequireOSDRelease()

		assert.Equal(t, "squid", mon.lastRequireOSDRelease) // cached after success
		assert.Equal(t, 1, enableCalls)                     // called exactly once
	})

	// CACHING: Release already cached as "squid" and OSDs still report squid →
	// should NOT call EnableReleaseOSDFunctionality again (avoids redundant work every 60s).
	t.Run("cached release skips EnableReleaseOSDFunctionality", func(t *testing.T) {
		enableCalls := 0
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				// handles: ceph versions
				if args[0] == "versions" {
					return `{"osd":{"ceph version 19.2.0 (abc) squid (stable)":3}}`, nil
				}
				// handles: ceph osd require-osd-release (should NOT be reached)
				if args[0] == "osd" && args[1] == "require-osd-release" {
					enableCalls++
					return "", nil
				}
				return "", nil
			},
		}
		ctx := &clusterd.Context{Executor: executor}
		mon := &OSDHealthMonitor{
			context:               ctx,
			clusterInfo:           clusterInfo,
			lastRequireOSDRelease: "squid", // already cached from a previous tick
		}

		mon.checkRequireOSDRelease()

		assert.Equal(t, "squid", mon.lastRequireOSDRelease) // unchanged
		assert.Equal(t, 0, enableCalls)                     // skipped — no ceph command issued
	})

	// MIXED VERSIONS: Upgrade in progress — some OSDs on squid, some on tentacle.
	// Should bail early without setting anything.
	t.Run("multiple OSD versions does not set release", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				// handles: ceph versions
				if args[0] == "versions" {
					// Two map entries → versions have not converged
					return `{"osd":{"ceph version 19.2.0 (abc) squid (stable)":2,"ceph version 20.1.0 (def) tentacle (stable)":1}}`, nil
				}
				return "", nil
			},
		}
		ctx := &clusterd.Context{Executor: executor}
		mon := &OSDHealthMonitor{
			context:     ctx,
			clusterInfo: clusterInfo,
		}

		mon.checkRequireOSDRelease()

		assert.Equal(t, "", mon.lastRequireOSDRelease) // still empty — no action taken
	})

	// ERROR QUERYING VERSIONS: "ceph versions" fails (e.g. mons unreachable).
	// Should bail early, cache stays empty so it retries on the next tick.
	t.Run("error from GetAllCephDaemonVersions does not set release", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				// handles: ceph versions — returns an error
				if args[0] == "versions" {
					return "", fmt.Errorf("simulated error")
				}
				return "", nil
			},
		}
		ctx := &clusterd.Context{Executor: executor}
		mon := &OSDHealthMonitor{
			context:     ctx,
			clusterInfo: clusterInfo,
		}

		mon.checkRequireOSDRelease()

		assert.Equal(t, "", mon.lastRequireOSDRelease) // no change — will retry next tick
	})

	// ERROR SETTING FLAG: Versions converged but "ceph osd require-osd-release" fails.
	// Should NOT cache the release name, so the next tick retries the operation.
	t.Run("error from EnableReleaseOSDFunctionality does not cache", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				// handles: ceph versions — returns converged
				if args[0] == "versions" {
					return `{"osd":{"ceph version 19.2.0 (abc) squid (stable)":3}}`, nil
				}
				// handles: ceph osd require-osd-release — returns an error
				if args[0] == "osd" && args[1] == "require-osd-release" {
					return "", fmt.Errorf("simulated enable error")
				}
				return "", nil
			},
		}
		ctx := &clusterd.Context{Executor: executor}
		mon := &OSDHealthMonitor{
			context:     ctx,
			clusterInfo: clusterInfo,
		}

		mon.checkRequireOSDRelease()

		assert.Equal(t, "", mon.lastRequireOSDRelease) // NOT cached — ensures retry
	})
}
