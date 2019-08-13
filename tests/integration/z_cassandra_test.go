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

package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ************************************************
// *** Major scenarios tested by the CassandraSuite ***
// Setup
// - via the cluster CRD with very simple properties
//   - 1 replica
//     - 1 CPU
//     - 2GB memory
//     - 5Gi volume from default provider
// ************************************************

type CassandraSuite struct {
	suite.Suite
	k8sHelper       *utils.K8sHelper
	installer       *installer.CassandraInstaller
	namespace       string
	systemNamespace string
	instanceCount   int
}

// TestCassandraSuite initiates the CassandraSuite
func TestCassandraSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CassandraTestSuite) {
		t.Skip()
	}

	s := new(CassandraSuite)
	defer func(s *CassandraSuite) {
		r := recover()
		if r != nil {
			logger.Infof("unexpected panic occurred during test %s, --> %v", t.Name(), r)
			t.Fail()
			s.Teardown()
			t.FailNow()
		}
	}(s)
	suite.Run(t, s)
}

// SetupSuite runs once at the beginning of the suite,
// before any tests are run.
func (s *CassandraSuite) SetupSuite() {

	s.namespace = "cassandra-ns"
	s.systemNamespace = installer.SystemNamespace(s.namespace)
	s.instanceCount = 1

	k8sHelper, err := utils.CreateK8sHelper(s.T)
	require.NoError(s.T(), err)
	s.k8sHelper = k8sHelper

	k8sVersion := s.k8sHelper.GetK8sServerVersion()
	logger.Infof("Installing Cassandra on K8s %s", k8sVersion)

	s.installer = installer.NewCassandraInstaller(s.k8sHelper, s.T)

	if err = s.installer.InstallCassandra(s.systemNamespace, s.namespace, s.instanceCount, cassandrav1alpha1.ClusterModeCassandra); err != nil {
		logger.Errorf("Cassandra was not installed successfully: %s", err.Error())
		s.T().Fail()
		s.Teardown()
		s.T().FailNow()
	}
}

// BeforeTest runs before every test in the CassandraSuite.
func (s *CassandraSuite) TeardownSuite() {
	s.Teardown()
}

///////////
// Tests //
///////////

// TestCassandraClusterCreation tests the creation of a Cassandra cluster.
func (s *CassandraSuite) TestCassandraClusterCreation() {
	s.CheckClusterHealth()
}

// TestScyllaClusterCreation tests the creation of a Scylla cluster.
// func (s *CassandraSuite) TestScyllaClusterCreation() {
// 	s.CheckClusterHealth()
// }

//////////////////////
// Helper Functions //
//////////////////////

// Teardown gathers logs and other helping info and then uninstalls
// everything installed by the CassandraSuite
func (s *CassandraSuite) Teardown() {
	s.installer.GatherAllCassandraLogs(s.systemNamespace, s.namespace, s.T().Name())
	s.installer.UninstallCassandra(s.systemNamespace, s.namespace)
}

// CheckClusterHealth checks if all Pods in the cluster are ready
// and CQL is working.
func (s *CassandraSuite) CheckClusterHealth() {
	// Verify that cassandra-operator is running
	logger.Infof("Verifying that all expected pods in cassandra cluster %s are running", s.namespace)
	assert.True(s.T(), s.k8sHelper.CheckPodCountAndState("rook-cassandra-operator", s.systemNamespace, 1, "Running"), "rook-cassandra-operator must be in Running state")

	// Give the StatefulSet a head start
	// CheckPodCountAndState timeout might be too fast and the test may fail
	time.Sleep(30 * time.Second)
	// Verify cassandra cluster instances are running OK
	assert.True(s.T(), s.k8sHelper.CheckPodCountAndState("rook-cassandra", s.namespace, s.instanceCount, "Running"), fmt.Sprintf("%d rook-cassandra pods must be in running state", s.instanceCount))

	// Determine a pod name for the cluster
	podName := "cassandra-ns-us-east-1-us-east-1a-0"

	// Get the Pod's IP address
	command := "hostname"
	commandArgs := []string{"-i"}
	podIP, err := s.k8sHelper.Exec(s.namespace, podName, command, commandArgs)
	assert.NoError(s.T(), err)

	command = "cqlsh"
	commandArgs = []string{
		"-e",
		`
CREATE KEYSPACE IF NOT EXISTS test WITH REPLICATION = {
'class': 'SimpleStrategy',
'replication_factor': 1
};
USE test;
CREATE TABLE IF NOT EXISTS map (key text, value text, PRIMARY KEY(key));
INSERT INTO map (key, value) VALUES('test_key', 'test_value');
SELECT key,value FROM map WHERE key='test_key';`,
		podIP,
	}

	time.Sleep(30 * time.Second)
	var result string
	for inc := 0; inc < utils.RetryLoop; inc++ {
		result, err = s.k8sHelper.Exec(s.namespace, podName, command, commandArgs)
		logger.Infof("cassandra cql command exited, err: %v. result: %s", err, result)
		if err == nil {
			break
		}
		logger.Warning("cassandra cql command failed, will try again")
		time.Sleep(utils.RetryInterval * time.Second)
	}

	assert.NoError(s.T(), err)
	assert.True(s.T(), strings.Contains(result, "test_key"))
	assert.True(s.T(), strings.Contains(result, "test_value"))
}
