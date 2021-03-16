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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// **************************************************
// *** Mgr operations covered by TestMgrSmokeSuite ***
//
// Ceph orchestrator device ls
// Ceph orchestrator status
// Ceph orchestrator host ls
// Ceph orchestrator create OSD
// Ceph orchestrator ls
// **************************************************
func TestCephMgrSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}
	// Skip this test suite in master and release builds. If there is an issue
	// running against Ceph master we don't want to block the official builds.
	if installer.TestIsOfficialBuild() {
		t.Skip()
	}

	logger.Info("TEMPORARILY disable the mgr test suite until https://github.com/rook/rook/issues/5877 is resolved")
	t.Skip()

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
		UseCSI:            true,
		SkipOSDCreation:   true,
		RookVersion:       installer.VersionMaster,
		CephVersion:       installer.MasterVersion,
	}
	s.settings.ApplyEnvVars()
	s.installer, s.k8sh = StartTestCluster(s.T, s.settings, cephMasterSuiteMinimalTestVersion)
	s.waitForOrchestrationModule()
}

func (s *CephMgrSuite) AfterTest(suiteName, testName string) {
	s.installer.CollectOperatorLog(suiteName, testName)
}

func (s *CephMgrSuite) TearDownSuite() {
	s.installer.UninstallRook()
}

func (s *CephMgrSuite) execute(command []string) (error, string) {
	orchCommand := append([]string{"orch"}, command...)
	return s.installer.Execute("ceph", orchCommand, s.namespace)
}

func (s *CephMgrSuite) waitForOrchestrationModule() {
	var err error
	for timeout := 0; timeout < 30; timeout++ {
		err, output := s.execute([]string{"status"})
		logger.Infof("%s", output)
		if err == nil {
			logger.Info("Rook Toolbox ready to execute commands")
			return
		}
		time.Sleep(2 * time.Second)
	}
	logger.Error("Giving up waiting for Rook Toolbox to be ready")
	assert.Nil(s.T(), err)
}
func (s *CephMgrSuite) TestDeviceLs() {
	logger.Info("Testing .... <ceph orch device ls>")
	err, device_list := s.execute([]string{"device", "ls"})
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", device_list)
}

func (s *CephMgrSuite) TestStatus() {
	logger.Info("Testing .... <ceph orch status>")
	err, status := s.execute([]string{"status"})
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", status)

	assert.Equal(s.T(), status, "Backend: rook\nAvailable: True")
}

func (s *CephMgrSuite) TestHostLs() {
	logger.Info("Testing .... <ceph orch host ls>")

	// Get the orchestrator hosts
	err, output := s.execute([]string{"host", "ls", "json"})
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", output)

	hosts := []byte(output)
	var hostsList []host

	err = json.Unmarshal(hosts, &hostsList)
	if err != nil {
		assert.Nil(s.T(), err)
	}

	var hostOutput []string
	for _, hostItem := range hostsList {
		hostOutput = append(hostOutput, hostItem.Addr)
	}
	sort.Strings(hostOutput)

	// get the k8s nodes
	nodes, err := k8sutil.GetNodeHostNames(s.k8sh.Clientset)
	assert.Nil(s.T(), err)

	k8sNodes := make([]string, 0, len(nodes))
	for k := range nodes {
		k8sNodes = append(k8sNodes, k)
	}
	sort.Strings(k8sNodes)

	// nodes and hosts must be the same
	assert.Equal(s.T(), hostOutput, k8sNodes)
}

func (s *CephMgrSuite) TestCreateOSD() {
	logger.Info("Testing .... <ceph orch create OSD>")

	// Get the first available device
	err, deviceList := s.execute([]string{"device", "ls", "--format", "json"})
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", deviceList)

	inventory := make([]map[string]interface{}, 0)

	err = json.Unmarshal([]byte(deviceList), &inventory)
	assert.Nil(s.T(), err)

	selectedNode := ""
	selectedDevice := ""
	for _, node := range inventory {
		for _, device := range node["devices"].([]interface{}) {
			if device.(map[string]interface{})["available"].(bool) {
				selectedNode = node["name"].(string)
				selectedDevice = strings.TrimPrefix(device.(map[string]interface{})["path"].(string), "/dev/")
				break
			}
		}
		if selectedDevice != "" {
			break
		}
	}
	assert.NotEqual(s.T(), "", selectedDevice, "No devices available to create test OSD")
	assert.NotEqual(s.T(), "", selectedNode, "No nodes available to create test OSD")

	if selectedDevice == "" || selectedNode == "" {
		return
	}
	// Create the OSD
	err, output := s.execute([]string{"daemon", "add", "osd", fmt.Sprintf("%s:%s", selectedNode, selectedDevice)})

	assert.Nil(s.T(), err)
	logger.Infof("output = %s", output)

	err = s.k8sh.WaitForPodCount("app=rook-ceph-osd", s.namespace, 1)
	assert.Nil(s.T(), err)
}

func (s *CephMgrSuite) TestServiceLs() {
	logger.Info("Testing .... <ceph orch ls>")
	err, output := s.execute([]string{"ls", "--format", "json"})
	assert.Nil(s.T(), err)
	logger.Infof("output = %s", output)

	services := []byte(output)
	var servicesList []service

	err = json.Unmarshal(services, &servicesList)
	assert.Nil(s.T(), err)

	for _, svc := range servicesList {
		labelFilter := fmt.Sprintf("app=rook-ceph-%s", svc.ServiceName)
		k8sPods, err := k8sutil.PodsRunningWithLabel(s.k8sh.Clientset, s.namespace, labelFilter)
		logger.Infof("Service: %+v", svc)
		logger.Infof("k8s pods for svc %q: %d", svc.ServiceName, k8sPods)
		assert.Nil(s.T(), err)
		assert.Equal(s.T(), svc.Status.Running, k8sPods, fmt.Sprintf("Wrong number of pods for kind of service <%s>", svc.ServiceName))
	}
}
