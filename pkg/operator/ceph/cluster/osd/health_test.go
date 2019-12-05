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
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testexec "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestOSDStatus(t *testing.T) {
	cluster := "fake"

	var execCount = 0
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\", \"osdid\":3.0}", nil
		},
	}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
		execCount++
		if args[1] == "dump" {
			// Mock executor for OSD Dump command, returning an osd in Down state
			return `{"OSDs": [{"OSD": 0, "Up": 1, "In": 0}]}`, nil
		} else if args[1] == "safe-to-destroy" {
			// Mock executor for OSD Dump command, returning an osd in Down state
			return `{"safe_to_destroy":[0],"active":[],"missing_stats":[],"stored_pgs":[]}`, nil
		}
		return "", nil
	}

	executor.MockExecuteCommandWithOutput = func(debug bool, actionName string, command string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutput: %s %v", command, args)
		return "", nil
	}

	// Setting up objects needed to create OSD
	context := &clusterd.Context{
		Executor:  executor,
		Clientset: testexec.New(2),
	}

	labels := map[string]string{
		k8sutil.AppAttr:     AppName,
		k8sutil.ClusterAttr: cluster,
		OsdIdLabelKey:       "0",
	}

	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "osd0",
			Namespace: cluster,
			Labels:    labels,
		},
	}
	if _, err := context.Clientset.AppsV1().Deployments(cluster).Create(deployment); err != nil {
		logger.Errorf("Error creating fake deployment: %v", err)
	}

	// Check if the osd deployment is created
	dp, _ := context.Clientset.AppsV1().Deployments(cluster).List(metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%d", OsdIdLabelKey, 0)})
	assert.Equal(t, 1, len(dp.Items))

	cephVersion := cephver.CephVersion{
		Major: 14,
	}

	// Initializing an OSD monitoring
	osdMon := NewMonitor(context, cluster, true, cephVersion)

	// Run OSD monitoring routine
	err := osdMon.osdStatus()
	assert.Nil(t, err)
	// After creating an OSD, the dump has 1 mocked cmd and safe to destroy has 1 mocked cmd
	assert.Equal(t, 2, execCount)

	// Check if the osd deployment was deleted
	dp, _ = context.Clientset.AppsV1().Deployments(cluster).List(metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%d", OsdIdLabelKey, 0)})
	assert.Equal(t, 0, len(dp.Items))
}

func TestMonitorStart(t *testing.T) {
	cephVersion := cephver.CephVersion{
		Major: 14,
	}

	stopCh := make(chan struct{})
	osdMon := NewMonitor(&clusterd.Context{}, "cluster", true, cephVersion)
	logger.Infof("starting osd monitor")
	go osdMon.Start(stopCh)
	close(stopCh)
}
