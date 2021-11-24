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
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// **************************************************
// *** Monitoring stack deployment and integration with Ceph dashboard***
//
// Install prometheus
// Install Grafana
// Install alert manager
// Install node exporter
// Verify monitoring stack using Ceph dashboard
// **************************************************
const (
	prometheusOperatorBundle = "https://raw.githubusercontent.com/coreos/prometheus-operator/v0.40.0/bundle.yaml"
	manifestFolder = "../../deploy/examples/"
)

func httpGet(url string, user string, password string) (string, int){
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{Transport: customTransport, Timeout: time.Second * 10}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Error(err)
		return "", -1
	}

	if user != "" {
		basicAuth := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
		req.Header.Add("Authorization","Basic " + basicAuth)
	}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error(err)
		return "", -1
	}
	defer resp.Body.Close()

	response, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err)
		return "", -1
	}
	return string(response), resp.StatusCode
}

func TestCephMonStackSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

	s := new(CephMonStackSuite)
	defer func(s *CephMonStackSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type CephMonStackSuite struct {
	suite.Suite
	settings  *installer.TestCephSettings
	k8sh      *utils.K8sHelper
	installer *installer.CephInstaller
	dashboardService string
	prometheusService string
	alertManagerService string
	grafanaService string
	nodeExporterService  string
}

func (s *CephMonStackSuite) SetupSuite() {
	namespace := "mgr-monstack"
	s.settings = &installer.TestCephSettings{ //nolint:exhaustivestruct
		ClusterName:       "monstack-cluster",
		OperatorNamespace: installer.SystemNamespace(namespace),
		Namespace:         namespace,
		StorageClassName:  installer.StorageClassName(),
		UseHelm:           false,
		UsePVC:            installer.UsePVC(),
		Mons:              1,
		SkipOSDCreation:   false,
		EnableAdmissionController: true,
		UseCrashPruner:            true,
		EnableVolumeReplication:   false,
		RookVersion:       installer.LocalBuildTag,
		CephVersion:       installer.ReturnCephVersion(),
	}
	s.settings.ApplyEnvVars()
	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
}

func (s *CephMonStackSuite) AfterTest(suiteName, testName string) {
	s.installer.CollectOperatorLog(suiteName, testName)
}

func (s *CephMonStackSuite) TearDownSuite() {
	_ = s.k8sh.DeleteResource("sc", "local-storage")
	s.installer.UninstallRook()
}

func (s *CephMonStackSuite) getManifestFile(manifestFileName string) string {
	var manifestFile = manifestFolder + manifestFileName

	base, err := os.Getwd()
	if err != nil {
		logger.Errorf("Cannot get current working dir: %v", err)
		return ""
	}
	absoluteFilePath := filepath.Join(base, manifestFile)
	fileContent, err := ioutil.ReadFile(absoluteFilePath)
	if err != nil {
		logger.Errorf("Error reading %s manifest file in %q: %v", manifestFileName, absoluteFilePath, err)
		return ""
	}

	return strings.ReplaceAll(string(fileContent), " rook-ceph", fmt.Sprintf(" %s", s.settings.Namespace))
}

func (s *CephMonStackSuite) deployPrometheus() {
	resp, err := http.Get(prometheusOperatorBundle)
	if err != nil {
		logger.Errorf("Error trying to get Prometheus operator manifests: %v", err)
	}
	defer resp.Body.Close()
	operatorManifest, err := io.ReadAll(resp.Body)
	assert.Nil(s.T(), err)
	err = s.k8sh.ResourceOperation("apply", string(operatorManifest))
	assert.Nil(s.T(), err)

	err = s.k8sh.WaitForPodCount("app.kubernetes.io/name=prometheus-operator", "", 1)
	assert.Nil(s.T(), err)

	err = s.k8sh.ResourceOperation("apply", s.getManifestFile("monitoring/service-monitor.yaml"))
	assert.Nil(s.T(), err)

	err = s.k8sh.ResourceOperation("apply", s.getManifestFile("monitoring/prometheus.yaml"))
	assert.Nil(s.T(), err)
	err = s.k8sh.WaitForPodCount("prometheus=rook-prometheus", s.settings.Namespace, 1)
	assert.Nil(s.T(), err)

	err = s.k8sh.ResourceOperation("apply", s.getManifestFile("monitoring/prometheus-service.yaml"))
	assert.Nil(s.T(), err)

	assert.True(s.T(), s.k8sh.IsServiceUp("rook-prometheus", s.settings.Namespace))

	//Get Service
	prometheusIP, err := s.k8sh.GetPodHostID( "rook-ceph-mgr", s.settings.Namespace)
	assert.Nil(s.T(), err)
	prometheusPort, err := s.k8sh.GetServiceNodePort("rook-prometheus", s.settings.Namespace)
	assert.Nil(s.T(), err)
	s.prometheusService = fmt.Sprintf("http://%s:%s", prometheusIP, prometheusPort)
	logger.Infof("Prometheus service in: %s", s.prometheusService)
}

func (s *CephMonStackSuite) deployGrafana() {
	grafanaManifests := strings.ReplaceAll(string(s.getManifestFile("monitoring/grafana.yaml")), "PROMETHEUS-SVC", s.prometheusService)
	err := s.k8sh.ResourceOperation("apply", grafanaManifests)
	assert.Nil(s.T(), err)

	assert.True(s.T(), s.k8sh.IsServiceUp("grafana", s.settings.Namespace))

	//Create external service
	grafanaExtSvc := s.getManifestFile("monitoring/grafana-external-https.yaml")
	err = s.k8sh.ResourceOperation("apply", grafanaExtSvc)
	assert.Nil(s.T(), err)

	//Get Service (nodeport)
	grafanaIP, err := s.k8sh.GetPodHostID( "rook-ceph-mgr", s.settings.Namespace)
	assert.Nil(s.T(), err)
	grafanaPort, err := s.k8sh.GetServiceNodePort("grafana-external-https", s.settings.Namespace)
	assert.Nil(s.T(), err)
	s.grafanaService = fmt.Sprintf("https://%s:%s", grafanaIP, grafanaPort)
	logger.Infof("Grafana service in: %s", s.grafanaService)

}

func (s *CephMonStackSuite) deployNodeExporter() {
	err := s.k8sh.ResourceOperation("apply", s.getManifestFile("monitoring/node-exporter.yaml"))
	assert.Nil(s.T(), err)

	assert.True(s.T(), s.k8sh.IsServiceUp("node-exporter", s.settings.Namespace))

	//Get Service (clusterip)
	nodeExporterSvc, err := s.k8sh.GetService("node-exporter", s.settings.Namespace)
	assert.Nil(s.T(), err)
	nodeExporterIP := nodeExporterSvc.Spec.ClusterIP
	nodeExporterPort := strconv.FormatInt(int64(nodeExporterSvc.Spec.Ports[0].Port), 10)
	s.nodeExporterService = fmt.Sprintf("http://%s:%s", nodeExporterIP, nodeExporterPort)
	logger.Infof("Node exporter service in: %s", s.nodeExporterService)

	// Add node scrape data to Prometheus
	prometheusAdditionalScrapeFile := manifestFolder + "monitoring/prometheus-additional.yaml"
	_, err = s.k8sh.Kubectl("-n", s.settings.Namespace, "create", "secret", "generic", "additional-scrape-configs", "--from-file=" + prometheusAdditionalScrapeFile, "-oyaml")
	assert.Nil(s.T(), err)

	_, err = s.k8sh.Kubectl("-n", s.settings.Namespace, "patch", "prometheus", "rook-prometheus", "--type=merge",
		"-p", `{"spec": {"additionalScrapeConfigs": {"name": "additional-scrape-configs", "key": "prometheus-additional.yaml"}}}`)
	assert.Nil(s.T(), err)
}

func (s *CephMonStackSuite) deployAlertManager() {
	err := s.k8sh.ResourceOperation("apply", s.getManifestFile("monitoring/alert-manager.yaml"))
	assert.Nil(s.T(), err)

	assert.True(s.T(), s.k8sh.IsServiceUp("alertmanager", s.settings.Namespace))

	//Get Service (nodeport)
	alertIP, err := s.k8sh.GetPodHostID( "rook-ceph-mgr", s.settings.Namespace)
	assert.Nil(s.T(), err)
	alertPort, err := s.k8sh.GetServiceNodePort("alertmanager", s.settings.Namespace)
	assert.Nil(s.T(), err)
	s.alertManagerService = fmt.Sprintf("http://%s:%s", alertIP, alertPort)
	logger.Infof("Alert manager service in: %s", s.alertManagerService)

	//Provide alert manager configuration to Prometheus
	_, err = s.k8sh.Kubectl("-n", s.settings.Namespace, "patch",  "prometheus", "rook-prometheus", "--type=merge",
		"-p", `{"spec": {"alerting": {"alertmanagers": [{"namespace": "` + s.settings.Namespace + `", "name": "alertmanager", "port": ` +
		alertPort + `}]}}}`)
	assert.Nil(s.T(), err)
}

func (s *CephMonStackSuite) configCephDashboard() {
	assert.True(s.T(), s.k8sh.IsServiceUp("rook-ceph-mgr-dashboard", s.settings.Namespace))

	//Create external service
	dashboardExtSvc := s.getManifestFile("dashboard-external-http.yaml")
	err := s.k8sh.ResourceOperation("apply", dashboardExtSvc)
	assert.Nil(s.T(), err)

	//Get Service (NodePort)
	dashboardIP, err := s.k8sh.GetPodHostID( "rook-ceph-mgr", s.settings.Namespace)
	assert.Nil(s.T(), err)
	dashboardPort, err := s.k8sh.GetServiceNodePort(fmt.Sprintf("%s-mgr-dashboard-external-http", s.settings.Namespace), s.settings.Namespace)
	assert.Nil(s.T(), err)

	//dashboardIP := dashboardSvc.Spec.ClusterIP
	//dashboardPort := strconv.FormatInt(int64(dashboardSvc.Spec.Ports[0].Port), 10)
	s.dashboardService = fmt.Sprintf("https://%s:%s", dashboardIP, dashboardPort)
	logger.Infof("Ceph dashboard external service in: %s", s.dashboardService)

	// Configure dashboard
	//err, output := s.installer.ExecuteWithRetry("ceph", []string{"dashboard", "create-self-signed-cert"} , s.settings.Namespace , 10)
	//assert.Nil(s.T(), err)
	//logger.Info(output)
	err, output := s.installer.ExecuteWithRetry("ceph", []string{"dashboard", "set-grafana-api-url", s.grafanaService} , s.settings.Namespace , 10)
	assert.Nil(s.T(), err)
	logger.Info(output)
	err, output = s.installer.ExecuteWithRetry("ceph", []string{"dashboard", "set-grafana-api-ssl-verify", "False"} , s.settings.Namespace , 10)
	assert.Nil(s.T(), err)
	logger.Info(output)
	err, output = s.installer.ExecuteWithRetry("ceph", []string{"dashboard", "set-alertmanager-api-host", s.alertManagerService}, s.settings.Namespace, 10)
	assert.Nil(s.T(), err)
	logger.Info(output)
	err, output = s.installer.ExecuteWithRetry("ceph", []string{"dashboard", "set-alertmanager-api-ssl-verify", "False"}, s.settings.Namespace, 10)
	assert.Nil(s.T(), err)
	logger.Info(output)

}

func (s *CephMonStackSuite) verifyGrafana() {
	//There are grafana ceph dashboards
	cephDashboards, rCode := httpGet(fmt.Sprintf("%s/api/search?query=%s", s.grafanaService, "%"), "", "")
	assert.True(s.T(), rCode == 200)

	// The OSD details grafana dashboard is in the list
	dashboardFound := strings.Contains(cephDashboards, "\"id\":2,\"uid\":\"CrAHE0iZz\",\"title\":\"OSD device details\"")
	assert.True(s.T(), dashboardFound)

	// And it is possible to retrieve them
	_, rCode = httpGet(fmt.Sprintf("%s/api/dashboards/uid/CrAHE0iZz", s.grafanaService), "", "")
	assert.True(s.T(), rCode == 200)

	// We have configured a Prometheus grafana datasource pointing the right place
	datasources, rCode := httpGet(fmt.Sprintf("%s/api/datasources", s.grafanaService), "admin", "admin")
	assert.True(s.T(), rCode == 200)
	DatasourcesOK := strings.Contains(datasources, s.prometheusService)
	assert.True(s.T(), DatasourcesOK)
}

func (s *CephMonStackSuite) verifyPrometheus() {
	prometheusConfig, rCode := httpGet(fmt.Sprintf("%s/api/v1/status/config", s.prometheusService),"", "")
	assert.True(s.T(), rCode == 200)

	scrapeConfig := "scrape_configs:\\n- job_name: rook-ceph/rook-ceph-mgr/0\\n"
	scrapeConfigOk := strings.Contains(prometheusConfig, scrapeConfig)
	assert.True(s.T(), scrapeConfigOk)

	alertManagerConfig := "alertmanagers:\\n  - kubernetes_sd_configs:\\n    - role: endpoints\\n      namespaces:\\n        names:\\n        - rook-ceph\\n"
	alertManagerConfigOk := strings.Contains(prometheusConfig, alertManagerConfig)
	assert.True(s.T(), alertManagerConfigOk)

	targetData, rCode := httpGet(fmt.Sprintf("%s/api/v1/targets?state=active", s.prometheusService), "", "")
	assert.True(s.T(), rCode == 200)

	nodeExporterTargets := "\"__meta_kubernetes_namespace\":\"rook-ceph\",\"__meta_kubernetes_pod_container_name\":\"node-exporter\""
	nodeExporterTargetsOk := strings.Contains(targetData, nodeExporterTargets)
	assert.True(s.T(), nodeExporterTargetsOk)
}

func (s *CephMonStackSuite) TestMonitoringStack() {
	s.deployPrometheus()
	s.deployGrafana()
	s.deployNodeExporter()
	s.deployAlertManager()
	s.configCephDashboard()

	logger.Infof("Dashboard service in: %s", s.dashboardService)
	logger.Infof("Prometheus service in: %s", s.prometheusService)
	logger.Infof("Grafana service in: %s", s.grafanaService)
	logger.Infof("Node Exporter service in: %s", s.nodeExporterService)
	logger.Infof("Alert manager service in: %s", s.alertManagerService)

	s.verifyGrafana()
	s.verifyPrometheus()
}

