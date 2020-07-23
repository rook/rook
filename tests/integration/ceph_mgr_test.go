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

	s := new(CephMgrSuite)
	defer func(s *CephMgrSuite) {
		HandlePanics(recover(), s.cluster, s.T)
	}(s)
	suite.Run(t, s)
}

type CephMgrSuite struct {
	suite.Suite
	cluster   *TestCluster
	k8sh      *utils.K8sHelper
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
	placement   map[string]string
	ServiceName string `json:"Service_name"`
	ServiceType string `json:"Service_type"`
	Status      serviceStatus
}

func (suite *CephMgrSuite) SetupSuite() {
	suite.namespace = "mgr-ns"

	mgrTestCluster := TestCluster{
		clusterName:             suite.namespace,
		namespace:               suite.namespace,
		storeType:               "bluestore",
		storageClassName:        "",
		useHelm:                 false,
		usePVC:                  false,
		mons:                    1,
		rbdMirrorWorkers:        0,
		rookCephCleanup:         true,
		skipOSDCreation:         true,
		minimalMatrixK8sVersion: cephMasterSuiteMinimalTestVersion,
		rookVersion:             installer.VersionMaster,
		cephVersion:             installer.MasterVersion,
	}

	suite.cluster, suite.k8sh = StartTestCluster(suite.T, &mgrTestCluster)
	suite.waitForOrchestrationModule()
}

func (suite *CephMgrSuite) AfterTest(suiteName, testName string) {
	suite.cluster.installer.CollectOperatorLog(suiteName, testName, installer.SystemNamespace(suite.namespace))
}

func (suite *CephMgrSuite) TearDownSuite() {
	suite.cluster.Teardown()
}

func (suite *CephMgrSuite) execute(command []string) (error, string) {
	orchCommand := append([]string{"orch"}, command...)
	return suite.cluster.installer.Execute("ceph", orchCommand, suite.namespace)
}

func (suite *CephMgrSuite) enableRookOrchestrator() error {
	logger.Info("Enabling Rook orchestrator module: <ceph mgr module enable rook>")
	err, output := suite.cluster.installer.Execute("ceph", []string{"mgr", "module", "enable", "rook"}, suite.namespace)
	logger.Infof("output: %s", output)
	if err != nil {
		logger.Infof("Not possible to enable rook orchestrator module: %q", err)
		return err
	}
	logger.Info("Setting orchestrator backend to Rook .... <ceph orch set backend rook>")
	err, output = suite.execute([]string{"set", "backend", "rook"})
	logger.Infof("output: %s", output)
	if err != nil {
		logger.Infof("Not possible to set rook as backend orchestrator module: %q", err)
	}
	return err
}

func (suite *CephMgrSuite) waitForOrchestrationModule() {
	var err error
	var orchStatus map[string]string

	err = suite.enableRookOrchestrator()
	if err != nil {
		logger.Error("First attemp: Error trying to set Rook orchestrator module")
	}

	for timeout := 0; timeout < 30; timeout++ {
		logger.Info("Waiting for rook orchestrator module enabled and ready ...")
		err, output := suite.execute([]string{"status"})
		logger.Infof("output: %s", output)
		if err == nil {
			logger.Info("Rook Toolbox ready to execute commands")
			// Convert string returned to map
			outputLines := strings.Split(output, "\n")
			orchStatus = make(map[string]string)
			for _, setting := range outputLines {
				s := strings.Split(setting, ":")
				orchStatus[strings.TrimSpace(strings.ToLower(s[0]))] = strings.TrimSpace(strings.ToLower(s[1]))
			}
			if orchStatus["backend"] != "rook" {
				err = suite.enableRookOrchestrator()
				if err != nil {
					continue
				}
			}
			break
		} else {
			logger.Info("Rook orchestrator not ready. Enabling again ... ")
			err = suite.enableRookOrchestrator()
		}
		time.Sleep(2 * time.Second)
	}
}

func (suite *CephMgrSuite) TestDeviceLs() {
	logger.Info("Testing .... <ceph orch device ls>")
	err, device_list := suite.execute([]string{"device", "ls"})
	assert.Nil(suite.T(), err)
	logger.Infof("output = %s", device_list)
}

func (suite *CephMgrSuite) TestStatus() {
	logger.Info("Testing .... <ceph orch status>")
	err, status := suite.execute([]string{"status"})
	assert.Nil(suite.T(), err)
	logger.Infof("output = %s", status)

	assert.Equal(suite.T(), status, "Backend: rook\nAvailable: True")
}

func (suite *CephMgrSuite) TestHostLs() {
	logger.Info("Testing .... <ceph orch host ls>")

	// Get the orchestrator hosts
	err, output := suite.execute([]string{"host", "ls", "json"})
	assert.Nil(suite.T(), err)
	logger.Infof("output = %s", output)

	hosts := []byte(output)
	var hostsList []host

	err = json.Unmarshal(hosts, &hostsList)
	if err != nil {
		assert.Nil(suite.T(), err)
	}

	var hostOutput []string
	for _, hostItem := range hostsList {
		hostOutput = append(hostOutput, hostItem.Addr)
	}
	sort.Strings(hostOutput)

	// get the k8s nodes
	nodes, err := k8sutil.GetNodeHostNames(suite.k8sh.Clientset)
	assert.Nil(suite.T(), err)

	k8sNodes := make([]string, 0, len(nodes))
	for k := range nodes {
		k8sNodes = append(k8sNodes, k)
	}
	sort.Strings(k8sNodes)

	// nodes and hosts must be the same
	assert.Equal(suite.T(), hostOutput, k8sNodes)
}

func (suite *CephMgrSuite) TestCreateOSD() {
	logger.Info("Testing .... <ceph orch create OSD>")

	// Get the first available device
	err, deviceList := suite.execute([]string{"device", "ls", "--format", "json"})
	assert.Nil(suite.T(), err)
	logger.Infof("output = %s", deviceList)

	inventory := make([]map[string]interface{}, 0)

	err = json.Unmarshal([]byte(deviceList), &inventory)
	assert.Nil(suite.T(), err)

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
	assert.NotEqual(suite.T(), "", selectedDevice, "No devices available to create test OSD")
	assert.NotEqual(suite.T(), "", selectedNode, "No nodes available to create test OSD")

	if selectedDevice == "" || selectedNode == "" {
		return
	}
	// Create the OSD
	err, output := suite.execute([]string{"daemon", "add", "osd", fmt.Sprintf("%s:%s", selectedNode, selectedDevice)})

	assert.Nil(suite.T(), err)
	logger.Infof("output = %s", output)

	err = suite.k8sh.WaitForPodCount("app=rook-ceph-osd", suite.namespace, 1)
	assert.Nil(suite.T(), err)
}

func (suite *CephMgrSuite) TestServiceLs() {
	logger.Info("Testing .... <ceph orch ls>")
	err, output := suite.execute([]string{"ls", "--format", "json"})
	assert.Nil(suite.T(), err)
	logger.Infof("output = %s", output)

	services := []byte(output)
	var servicesList []service

	err = json.Unmarshal(services, &servicesList)
	assert.Nil(suite.T(), err)

	for _, svc := range servicesList {
		labelFilter := fmt.Sprintf("app=rook-ceph-%s", svc.ServiceName)
		k8sPods, err := k8sutil.PodsRunningWithLabel(suite.k8sh.Clientset, suite.namespace, labelFilter)
		logger.Infof("Service: %+v", svc)
		logger.Infof("k8s pods for svc %q: %d", svc.ServiceName, k8sPods)
		assert.Nil(suite.T(), err)
		assert.Equal(suite.T(), svc.Status.Running, k8sPods, fmt.Sprintf("Wrong number of pods for kind of service <%s>", svc.ServiceName))
	}
}
