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

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	defaultTries = 3
)

// **************************************************
// *** Mgr operations covered by TestMgrSmokeSuite ***
//
// Ceph orchestrator device ls
// Ceph orchestrator status
// Ceph orchestrator host ls
// Ceph orchestrator ls
// **************************************************
func TestCephMgrSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

	s := new(CephMgrSuite)
	defer func(s *CephMgrSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type CephMgrSuite struct {
	suite.Suite
	settings  *installer.TestCephSettings
	k8sh      *utils.K8sHelper
	installer *installer.CephInstaller
	namespace string
}

type host struct {
	Addr     string
	Hostname string
	Labels   []string
	Status   string
}

type serviceStatus struct {
	ContainerImageName string `json:"Container_image_name"`
	LastRefresh        string `json:"Last_refresh"`
	Running            int
	Size               int
}

type service struct {
	ServiceName string `json:"Service_name"`
	ServiceType string `json:"Service_type"`
	Status      serviceStatus
}

func (s *CephMgrSuite) SetupSuite() {
	s.namespace = "mgr-ns"

	s.settings = &installer.TestCephSettings{
		ClusterName:       s.namespace,
		OperatorNamespace: installer.SystemNamespace(s.namespace),
		Namespace:         s.namespace,
		StorageClassName:  "",
		UseHelm:           false,
		UsePVC:            false,
		Mons:              1,
		SkipOSDCreation:   true,
		EnableDiscovery:   false,
		RookVersion:       installer.LocalBuildTag,
		CephVersion:       installer.MasterVersion,
	}
	s.settings.ApplyEnvVars()
	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	s.waitForOrchestrationModule()
	s.prepareLocalStorageClass("local-storage")
}

func (s *CephMgrSuite) AfterTest(suiteName, testName string) {
	s.installer.CollectOperatorLog(suiteName, testName)
}

func (s *CephMgrSuite) TearDownSuite() {
	_ = s.k8sh.DeleteResource("sc", "local-storage")
	s.installer.UninstallRook()
}

func (s *CephMgrSuite) executeWithRetry(command []string, maxRetries int) (string, error) {
	tries := 0
	orchestratorCommand := append([]string{"orch"}, command...)
	for {
		err, output := s.installer.Execute("ceph", orchestratorCommand, s.namespace)
		tries++
		if err != nil  {
			if maxRetries == 1 {
				return output, err
			}
			if tries == maxRetries {
				return "", fmt.Errorf("max retries(%d) reached, last err: %v", tries, err)
			}
			logger.Infof("retrying command <<ceph %s>>: last error: %v", command, err)
			continue
		}
		return output, nil
	}
}

func (s *CephMgrSuite) execute(command []string) (string, error) {
	return s.executeWithRetry(command, 1)
}

func (s *CephMgrSuite) prepareLocalStorageClass(storageClassName string) {
	// Rook orchestrator use PVs based in this storage class to create OSDs
	// It is also needed to list "devices"
	localStorageClass := `
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: ` + storageClassName + `
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
`
	err := s.k8sh.ResourceOperation("apply", localStorageClass)
	if err == nil {
		err, _ = s.installer.Execute("ceph", []string{"config", "set", "mgr", "mgr/rook/storage_class", storageClassName}, s.namespace)
		if err == nil {
			logger.Infof("Storage class %q set in manager config", storageClassName)
		} else {
			assert.Fail(s.T(), fmt.Sprintf("Error configuring local storage class in manager config: %q", err))
		}
	} else {
		assert.Fail(s.T(), fmt.Sprintf("Error creating local storage class: %q ", err))
	}
}

func (s *CephMgrSuite) enableOrchestratorModule() {
	logger.Info("Enabling Rook orchestrator module: <ceph mgr module enable rook --force>")
	err, output := s.installer.Execute("ceph", []string{"mgr", "module", "enable", "rook", "--force"}, s.namespace)
	logger.Infof("output: %s", output)
	if err != nil {
		logger.Infof("Failed to enable rook orchestrator module: %q", err)
		return
	}

	logger.Info("Setting orchestrator backend to Rook .... <ceph orch set backend rook>")
	output, err = s.execute([]string{"set", "backend", "rook"})
	logger.Infof("output: %s", output)
	if err != nil {
		logger.Infof("Not possible to set rook as backend orchestrator module: %q", err)
	}
}

func (s *CephMgrSuite) waitForOrchestrationModule() {
	var err error

	// Status struct
	type orchStatus struct {
		Available bool   `json:"available"`
		Backend   string `json:"backend"`
	}

	for timeout := 0; timeout < 30; timeout++ {
		logger.Info("Waiting for rook orchestrator module enabled and ready ...")
		output, err := s.execute([]string{"status", "--format", "json"})
		logger.Infof("%s", output)
		if err == nil {
			logger.Info("Ceph orchestrator ready to execute commands")

			// Get status information
			bytes := []byte(output)
			logBytesInfo(bytes)

			var status orchStatus
			err := json.Unmarshal(bytes[:len(output)], &status)
			if err != nil {
				logger.Error("Error getting ceph orch status")
				continue
			}

			if status.Backend != "rook" {
				assert.Fail(s.T(), fmt.Sprintf("Orchestrator backend is <%q>. Setting it to <Rook>", status.Backend))
				s.enableOrchestratorModule()
			} else {
				logger.Info("Orchestrator backend is <Rook>")
				return
			}
		} else {
			exitError, _ := err.(*exec.ExitError)
			if exitError.ExitCode() == 22 { // The <ceph orch> commands are still not recognized
				logger.Info("Ceph manager modules still not ready ... ")
			} else if exitError.ExitCode() == 2 { // The rook orchestrator is not the orchestrator backend
				s.enableOrchestratorModule()
			}
		}
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		logger.Error("Giving up waiting for manager module to be ready")
	}
	require.Nil(s.T(), err)
}
func (s *CephMgrSuite) TestDeviceLs() {
	logger.Info("Testing .... <ceph orch device ls>")
	deviceList, err := s.executeWithRetry([]string{"device", "ls"}, defaultTries)
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", deviceList)
}

func (s *CephMgrSuite) TestStatus() {
	logger.Info("Testing .... <ceph orch status>")
	status, err := s.executeWithRetry([]string{"status"}, defaultTries)
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", status)

	assert.Equal(s.T(), status, "Backend: rook\nAvailable: Yes")
}

func logBytesInfo(bytesSlice []byte) {
	logger.Infof("---- bytes slice info ---")
	logger.Infof("bytes: %v\n", bytesSlice)
	logger.Infof("length: %d\n", len(bytesSlice))
	logger.Infof("string: -->%s<--\n", string(bytesSlice))
	logger.Infof("-------------------------")
}

func (s *CephMgrSuite) TestHostLs() {
	logger.Info("Testing .... <ceph orch host ls>")

	// Get the orchestrator hosts
	output, err := s.executeWithRetry([]string{"host", "ls", "json"}, defaultTries)
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", output)

	hosts := []byte(output)
	logBytesInfo(hosts)

	var hostsList []host
	err = json.Unmarshal(hosts[:len(output)], &hostsList)
	if err != nil {
		assert.Nil(s.T(), err)
	}

	var hostOutput []string
	for _, hostItem := range hostsList {
		hostOutput = append(hostOutput, hostItem.Hostname)
	}
	sort.Strings(hostOutput)

	// get the k8s nodes
	nodes, err := k8sutil.GetNodeHostNames(context.TODO(), s.k8sh.Clientset)
	assert.Nil(s.T(), err)

	k8sNodes := make([]string, 0, len(nodes))
	for k := range nodes {
		k8sNodes = append(k8sNodes, k)
	}
	sort.Strings(k8sNodes)

	// nodes and hosts must be the same
	assert.Equal(s.T(), hostOutput, k8sNodes)
}

func (s *CephMgrSuite) TestServiceLs() {
	logger.Info("Testing .... <ceph orch ls --format json>")
	output, err := s.executeWithRetry([]string{"ls", "--format", "json"}, defaultTries)
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", output)

	services := []byte(output)
	logBytesInfo(services)

	var servicesList []service
	err = json.Unmarshal(services[:len(output)], &servicesList)
	assert.Nil(s.T(), err)

	labelFilter := ""
	for _, svc := range servicesList {
		if svc.ServiceName != "crash" {
			labelFilter = fmt.Sprintf("app=rook-ceph-%s", svc.ServiceName)
		} else {
			labelFilter = "app=rook-ceph-crashcollector"
		}
		k8sPods, err := k8sutil.PodsRunningWithLabel(context.TODO(), s.k8sh.Clientset, s.namespace, labelFilter)
		logger.Infof("Service: %+v", svc)
		logger.Infof("k8s pods for svc %q using label <%q>: %d", svc.ServiceName, labelFilter, k8sPods)
		assert.Nil(s.T(), err)
		assert.Equal(s.T(), svc.Status.Running, k8sPods, fmt.Sprintf("Wrong number of pods for kind of service <%s>", svc.ServiceType))
	}
}
