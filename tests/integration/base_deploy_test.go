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
	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "integrationTest")

	defaultNamespace     = "default"
	defaultRookNamespace = "test-rook"
	clusterNamespace1    = "test-cluster-1"
	clusterNamespace2    = "test-cluster-2"
)

//Test to make sure all rook components are installed and Running
func checkIfRookClusterIsInstalled(k8sh *utils.K8sHelper, s suite.Suite, oNamespace string, cNamespace string) {
	logger.Infof("Make sure all Pods in Rook Cluster %s are running", cNamespace)
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-operator", oNamespace, 1, "Running"),
		"Make sure there is 1 rook-operator present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-api", cNamespace, 1, "Running"),
		"Make sure there is 1 rook-api present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mgr", cNamespace, 1, "Running"),
		"Make sure there is 1 rook-ceph-mgr present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-osd", cNamespace, 1, "Running"),
		"Make sure there is at lest 1 rook-ceph-osd present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mon", cNamespace, 3, "Running"),
		"Make sure there are 3 rook-ceph-mon present in Running state")
}

func gatherAllRookLogs(k8sh *utils.K8sHelper, s suite.Suite, oNamespace string, cNamespace string) {
	logger.Infof("Gathering all logs from Rook Cluster %s", cNamespace)
	k8sh.GetRookLogs("rook-operator", oNamespace, s.T().Name())
	k8sh.GetRookLogs("rook-api", cNamespace, s.T().Name())
	k8sh.GetRookLogs("rook-ceph-mgr", cNamespace, s.T().Name())
	k8sh.GetRookLogs("rook-ceph-mon", cNamespace, s.T().Name())
	k8sh.GetRookLogs("rook-ceph-osd", cNamespace, s.T().Name())
	k8sh.GetRookLogs("rook-ceph-rgw", cNamespace, s.T().Name())
	k8sh.GetRookLogs("rook-ceph-mds", cNamespace, s.T().Name())
}
