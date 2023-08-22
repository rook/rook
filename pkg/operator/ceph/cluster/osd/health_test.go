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
	"reflect"
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

	var execCount = 0
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
	osdMon := NewOSDHealthMonitor(context, clusterInfo, true, cephv1.CephClusterHealthCheckSpec{})

	// Run OSD monitoring routine
	err := osdMon.checkOSDDump()
	assert.Nil(t, err)
	// After creating an OSD, the dump has 1 mocked cmd and safe to destroy has 1 mocked cmd
	assert.Equal(t, 2, execCount)

	// Check if the osd deployment was deleted
	dp, _ = context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%d", OsdIdLabelKey, 0)})
	assert.Equal(t, 0, len(dp.Items))
}

func TestMonitorStart(t *testing.T) {
	context, cancel := context.WithCancel(context.TODO())
	monitoringRoutines := make(map[string]*opcontroller.ClusterHealth)
	monitoringRoutines["osd"] = &opcontroller.ClusterHealth{
		InternalCtx:    context,
		InternalCancel: cancel,
	}

	osdMon := NewOSDHealthMonitor(&clusterd.Context{}, client.AdminTestClusterInfo("ns"), true, cephv1.CephClusterHealthCheckSpec{})
	logger.Infof("starting osd monitor")
	go osdMon.Start(monitoringRoutines, "osd")
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
		name string
		args args
		want *OSDHealthMonitor
	}{
		{"default-interval", args{c, false, cephv1.CephClusterHealthCheckSpec{}}, &OSDHealthMonitor{c, clusterInfo, false, &defaultHealthCheckInterval}},
		{"10s-interval", args{c, false, cephv1.CephClusterHealthCheckSpec{DaemonHealth: cephv1.DaemonHealthSpec{ObjectStorageDaemon: cephv1.HealthCheckSpec{Interval: &metav1.Duration{Duration: time10s}}}}}, &OSDHealthMonitor{c, clusterInfo, false, &time10s}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewOSDHealthMonitor(tt.args.context, clusterInfo, tt.args.removeOSDsIfOUTAndSafeToRemove, tt.args.healthCheck); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewOSDHealthMonitor() = %v, want %v", got, tt.want)
			}
		})
	}
}
