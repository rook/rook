/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package integration

import (
	"fmt"
	"time"

	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	defaultNamespace = "default"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "integrationTest")
)

// Test to make sure all rook components are installed and Running
func checkIfRookClusterIsInstalled(s suite.Suite, k8sh *utils.K8sHelper, opNamespace, clusterNamespace string, mons int) {
	logger.Infof("Make sure all Pods in Rook Cluster %s are running", clusterNamespace)
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-operator", opNamespace, 1, "Running"),
		"Make sure there is 1 rook-operator present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mgr", clusterNamespace, 1, "Running"),
		"Make sure there is 1 rook-ceph-mgr present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-osd", clusterNamespace, 1, "Running"),
		"Make sure there is at lest 1 rook-ceph-osd present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mon", clusterNamespace, mons, "Running"),
		fmt.Sprintf("Make sure there are %d rook-ceph-mon present in Running state", mons))
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-crashcollector", clusterNamespace, 1, "Running"),
		"Make sure there is at lest 1 rook-ceph-crash present in Running state")
}

func checkIfRookClusterIsHealthy(s suite.Suite, testClient *clients.TestClient, clusterNamespace string) {
	logger.Infof("Testing cluster %s health", clusterNamespace)
	var err error

	retryCount := 0
	for retryCount < utils.RetryLoop {
		healthy, err := clients.IsClusterHealthy(testClient, clusterNamespace)
		if healthy {
			logger.Infof("cluster %s is healthy", clusterNamespace)
			return
		}

		retryCount++
		logger.Infof("waiting for cluster %s to become healthy. err: %+v", clusterNamespace, err)
		<-time.After(time.Duration(utils.RetryInterval) * time.Second)
	}

	require.Nil(s.T(), err)
}

func HandlePanics(r interface{}, uninstaller func(), t func() *testing.T) {
	if r != nil {
		logger.Infof("unexpected panic occurred during test %s, --> %v", t().Name(), r)
		t().Fail()
		uninstaller()
		t().FailNow()
	}
}

// StartTestCluster creates new instance of TestCephSettings struct
func StartTestCluster(t func() *testing.T, settings *installer.TestCephSettings) (*installer.CephInstaller, *utils.K8sHelper) {
	k8shelper, err := utils.CreateK8sHelper(t)
	require.NoError(t(), err)

	// Turn on DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)

	installer := installer.NewCephInstaller(t, k8shelper.Clientset, settings)
	isRookInstalled, err := installer.InstallRook()

	if !isRookInstalled || err != nil {
		logger.Errorf("Rook was not installed successfully: %v", err)
		if !installer.T().Failed() {
			installer.GatherAllRookLogs(t().Name(), settings.Namespace, settings.OperatorNamespace)
		}
		t().Fail()
		installer.UninstallRook()
		t().FailNow()
	}

	return installer, k8shelper
}
