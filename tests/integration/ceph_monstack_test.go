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
	"encoding/base64"
	"fmt"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
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
	manifestFolder = "../../cluster/examples/kubernetes/ceph/monitoring/"
)

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
	namespace string
}

func (s *CephMonStackSuite) SetupSuite() {
	s.namespace = "mgr-ns"

	s.settings = &installer.TestCephSettings{ //nolint:exhaustivestruct
		ClusterName:       s.namespace,
		OperatorNamespace: installer.SystemNamespace(s.namespace),
		Namespace:         s.namespace,
		StorageClassName:  "",
		UseHelm:           false,
		UsePVC:            false,
		Mons:              1,
		SkipOSDCreation:   false,
		EnableDiscovery:   false,
		RookVersion:       installer.LocalBuildTag,
		CephVersion:       installer.MasterVersion,
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

	return strings.ReplaceAll(string(fileContent), " rook-ceph", fmt.Sprintf(" %s", s.namespace))
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

	err = s.k8sh.ResourceOperation("apply", s.getManifestFile("service-monitor.yaml"))
	assert.Nil(s.T(), err)

	err = s.k8sh.ResourceOperation("apply", s.getManifestFile("prometheus.yaml"))
	assert.Nil(s.T(), err)
	err = s.k8sh.WaitForPodCount("prometheus=rook-prometheus", s.namespace, 1)
	assert.Nil(s.T(), err)

	err = s.k8sh.ResourceOperation("apply", s.getManifestFile("prometheus-service.yaml"))
	assert.Nil(s.T(), err)

	assert.True(s.T(), s.k8sh.IsServiceUp("rook-prometheus", s.namespace) )
}

func (s *CephMonStackSuite) deployGrafana() {
	prometheusIP, err := s.k8sh.Kubectl("-n", s.namespace, "-o", `jsonpath="{.status.hostIP}"`, "get", "pod", "prometheus-rook-prometheus-0")
	assert.Nil(s.T(), err)
	if len(prometheusIP) > 0 && prometheusIP[0] == '"' && prometheusIP[len(prometheusIP)-1] == '"' {
		prometheusIP = prometheusIP[1 : len(prometheusIP)-1]
	}
	prometheusService := fmt.Sprintf("http://%s:30900", prometheusIP)

	grafanaManifests := strings.ReplaceAll(string(s.getManifestFile("grafana.yaml")), "PROMETHEUS-SVC", prometheusService)
	err = s.k8sh.ResourceOperation("apply", grafanaManifests)
	assert.Nil(s.T(), err)

	assert.True(s.T(), s.k8sh.IsServiceUp("grafana", s.namespace) )
}

func (s *CephMonStackSuite) deployNodeExporter() {
	err := s.k8sh.ResourceOperation("apply", s.getManifestFile("node-exporter.yaml"))
	assert.Nil(s.T(), err)

	assert.True(s.T(), s.k8sh.IsServiceUp("node-exporter", s.namespace) )

	// Add node scrape data to Prometheus
	prometheusAdditionalScrapeFile := manifestFolder + "prometheus-additional.yaml"
	_, err = s.k8sh.Kubectl("-n", s.namespace, "create", "secret", "generic", "additional-scrape-configs", "--from-file=" + prometheusAdditionalScrapeFile, "-oyaml")
	assert.Nil(s.T(), err)

	_, err = s.k8sh.Kubectl("-n", s.namespace, "patch", "prometheus", "rook-prometheus", "--type", "merge", "-p", `$'` + s.getManifestFile("scrape-config-patch.yaml") + `'`)
	assert.Nil(s.T(), err)
}

func (s *CephMonStackSuite) deployAlertManager() {
	err := s.k8sh.ResourceOperation("apply", s.getManifestFile("alert-manager.yaml"))
	assert.Nil(s.T(), err)

	assert.True(s.T(), s.k8sh.IsServiceUp("alertmanager", s.namespace) )

	//Provide alert manager configuration to Prometheus
	_, err = s.k8sh.Kubectl("-n", s.namespace, "patch",  "prometheus", "rook-prometheus", "--type", "merge", "-p", `$'`  + s.getManifestFile("alert-manager-patch.yaml") + `'`)
	assert.Nil(s.T(), err)
}

func (s *CephMonStackSuite) configCephDashboard() {
	dashboardManifest := `
apiVersion: v1
kind: Service
metadata:
  name: rook-ceph-mgr-dashboard-external-https
  namespace: ` + s.namespace + `
  labels:
    app: rook-ceph-mgr
    rook_cluster: ` + s.namespace + `
spec:
  ports:
    - name: dashboard
      port: 8443
      protocol: TCP
      targetPort: 8443
  selector:
    app: rook-ceph-mgr
    rook_cluster: rook-ceph
  sessionAffinity: None
  type: NodePort
`
	//Make ssl works for ceph dashboard
	err, output := s.installer.Execute("ceph", []string{"dashboard", "create-self-signed-cert"} , s.namespace )
	assert.Nil(s.T(), err)
	logger.Info(output)

	//Create dashboard external service
	err = s.k8sh.ResourceOperation("apply", dashboardManifest)
	assert.Nil(s.T(), err)
	dashboardIP, err := s.k8sh.GetPodHostID("rook-ceph-mgr", s.namespace)
	assert.Nil(s.T(), err)
	dashboardPort, err := s.k8sh.GetServiceNodePort("rook-ceph-mgr-dashboard-external-https", s.namespace)
	assert.Nil(s.T(), err)
	DashboardService := fmt.Sprintf("https://%s:%s", dashboardIP, dashboardPort)
	logger.Infof("Dashboard in: %s", DashboardService)

	//Grafana
	grafanaIP, err := s.k8sh.GetPodHostID("grafana", s.namespace)
	assert.Nil(s.T(), err)
	grafanaService := fmt.Sprintf("https://%s:32000", grafanaIP)
	logger.Infof("Grafana service in: %s", grafanaService)

	//Alert manager
	alertManagerIP, err := s.k8sh.GetPodHostID("alertmanager", s.namespace)
	assert.Nil(s.T(), err)
	alertManagerPort, err := s.k8sh.GetServiceNodePort("alertmanager", s.namespace)
	assert.Nil(s.T(), err)
	alertManagerURL := fmt.Sprintf("http://%s:%s", alertManagerIP, alertManagerPort)
	logger.Infof("Alert manager service in: %s", alertManagerURL)

	// Configure dashboard
	err, output = s.installer.Execute("ceph", []string{"dashboard", "set-grafana-api-url", grafanaService} , s.namespace )
	assert.Nil(s.T(), err)
	logger.Info(output)
	err, output = s.installer.Execute("ceph", []string{"dashboard", "set-grafana-api-ssl-verify", "False"} , s.namespace )
	assert.Nil(s.T(), err)
	logger.Info(output)

	err, output = s.installer.Execute("ceph", []string{"dashboard", "set-alertmanager-api-host", alertManagerURL}, s.namespace)
	assert.Nil(s.T(), err)
	logger.Info(output)
	err, output = s.installer.Execute("ceph", []string{"dashboard", "set-alertmanager-api-ssl-verify", "False"}, s.namespace)
	assert.Nil(s.T(), err)
	logger.Info(output)

}

func (s *CephMonStackSuite) verifyMonitoringStack() {
	dashboardIP, err := s.k8sh.GetPodHostID("rook-ceph-mgr", s.namespace)
	assert.Nil(s.T(), err)
	dashboardPort, err := s.k8sh.GetServiceNodePort("rook-ceph-mgr-dashboard-external-https", s.namespace)
	assert.Nil(s.T(), err)

	//kubectl -n rook-ceph get secret rook-ceph-dashboard-password -o jsonpath="{['data']['password']}" | base64 --decode && echo
	passwordEncoded, err := s.k8sh.Kubectl("-n", s.namespace, "get", "secret", "rook-ceph-dashboard-password", "-o", "jsonpath='{['data']['password']}'")
	assert.Nil(s.T(), err)
	p, err := base64.StdEncoding.DecodeString(passwordEncoded[1 : len(passwordEncoded)-1])
	assert.Nil(s.T(), err)
	password := string(p)

	// Ceph dashboard Authentication
	resource := "/auth/"
	data := url.Values{}
	data.Set("username", "admin")
	data.Set("password", password)
	logger.Info(fmt.Sprintf("https://%s:%s/api", dashboardIP, dashboardPort))

	u, _ := url.ParseRequestURI(fmt.Sprintf("https://%s:%s/api", dashboardIP, dashboardPort))
	u.Path = resource
	urlStr := u.String()

	client := &http.Client{}
	r, _ := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(data.Encode()))
	r.Header.Add("Accept", "application/vnd.ceph.api.v1.0+json")
	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

	resp, _ := client.Do(r)
	logger.Info(resp)
	logger.Info(resp.Status)
}

func (s *CephMonStackSuite) TestMonitoringStack() {
	s.deployPrometheus()
	s.deployGrafana()
	s.deployNodeExporter()
	s.deployAlertManager()
	time.Sleep(10 * time.Second)
	s.configCephDashboard()
	s.verifyMonitoringStack()
}