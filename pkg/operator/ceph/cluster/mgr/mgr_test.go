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

package mgr

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testopk8s "github.com/rook/rook/pkg/operator/k8sutil/test"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createNewCluster(t *testing.T) *Cluster {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Execute: %s %v", command, args)
			if args[0] == "mgr" && args[1] == "stat" {
				return `{"active_name": "a"}`, nil
			}
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}
	waitForDeploymentToStart = func(ctx context.Context, clusterdContext *clusterd.Context, deployment *apps.Deployment) error {
		logger.Infof("simulated mgr deployment starting")
		return nil
	}

	clientset := testop.New(t, 3)
	configDir := t.TempDir()
	scheme := scheme.Scheme
	err := policyv1.AddToScheme(scheme)
	assert.NoError(t, err)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()

	ctx := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
		Client:    cl,
	}
	ownerInfo := cephclient.NewMinimumOwnerInfo(t)
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns", FSID: "myfsid", OwnerInfo: ownerInfo, CephVersion: cephver.CephVersion{Major: 17, Minor: 2, Build: 0}, Context: context.TODO()}
	clusterInfo.SetName("test")
	clusterSpec := cephv1.ClusterSpec{
		Annotations:        map[cephv1.KeyType]cephv1.Annotations{cephv1.KeyMgr: {"my": "annotation"}},
		Labels:             map[cephv1.KeyType]cephv1.Labels{cephv1.KeyMgr: {"my-label-key": "value"}},
		Dashboard:          cephv1.DashboardSpec{Enabled: true, SSL: true},
		Mgr:                cephv1.MgrSpec{Count: 1},
		PriorityClassNames: map[cephv1.KeyType]string{cephv1.KeyMgr: "my-priority-class"},
		DataDirHostPath:    "/var/lib/rook/",
	}
	c := New(ctx, clusterInfo, clusterSpec, "myversion")
	defer os.RemoveAll(c.spec.DataDirHostPath)

	return c
}

func TestStartMgr(t *testing.T) {
	var deploymentsUpdated *[]*apps.Deployment
	updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

	c := createNewCluster(t)

	// start a basic service
	err := c.Start()
	assert.NoError(t, err)
	validateStart(t, c)
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	c.spec.Dashboard.URLPrefix = "/test"
	c.spec.Dashboard.Port = 12345
	err = c.Start()
	assert.NoError(t, err)
	validateStart(t, c)
	assert.ElementsMatch(t, []string{"rook-ceph-mgr-a"}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// starting with more replicas
	c.spec.Mgr.Count = 2
	c.spec.Dashboard.Enabled = false
	// delete the previous mgr since the mocked test won't update the existing one
	err = c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Delete(context.TODO(), "rook-ceph-mgr-a", metav1.DeleteOptions{})
	assert.NoError(t, err)
	err = c.Start()
	assert.NoError(t, err)
	validateStart(t, c)

	c.spec.Mgr.Count = 1
	c.spec.Dashboard.Enabled = false
	// clean the previous deployments
	err = c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Delete(context.TODO(), "rook-ceph-mgr-a", metav1.DeleteOptions{})
	assert.NoError(t, err)
	assert.NoError(t, err)
	err = c.Start()
	assert.NoError(t, err)
	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {
	mgrNames := []string{"a", "b"}
	for i := 0; i < c.spec.Mgr.Count; i++ {
		logger.Infof("Looking for cephmgr replica %d", i)
		daemonName := mgrNames[i]
		d, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Get(context.TODO(), fmt.Sprintf("rook-ceph-mgr-%s", daemonName), metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"my": "annotation"}, d.Spec.Template.Annotations)
		assert.Contains(t, d.Spec.Template.Labels, "my-label-key")
		assert.Equal(t, "my-priority-class", d.Spec.Template.Spec.PriorityClassName)
		if c.spec.Mgr.Count == 1 {
			assert.Equal(t, 1, len(d.Spec.Template.Spec.Containers))
		} else {
			// The sidecar container is only there when multiple mgrs are enabled
			assert.Equal(t, 2, len(d.Spec.Template.Spec.Containers))
			assert.Equal(t, "watch-active", d.Spec.Template.Spec.Containers[1].Name)
		}
	}

	// verify we have exactly the expected number of deployments and not extra
	// the expected deployments were already retrieved above, but now we check for no extra deployments
	options := metav1.ListOptions{LabelSelector: "app=rook-ceph-mgr"}
	deployments, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).List(context.TODO(), options)
	assert.NoError(t, err)
	assert.Equal(t, c.spec.Mgr.Count, len(deployments.Items))

	validateServices(t, c)
}

func validateServices(t *testing.T, c *Cluster) {
	_, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Get(context.TODO(), "rook-ceph-mgr", metav1.GetOptions{})
	assert.NoError(t, err)

	ds, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Get(context.TODO(), "rook-ceph-mgr-dashboard", metav1.GetOptions{})
	if c.spec.Dashboard.Enabled {
		assert.NoError(t, err)
		if c.spec.Dashboard.Port == 0 {
			// port=0 -> default port
			assert.Equal(t, ds.Spec.Ports[0].Port, int32(dashboardPortHTTPS))
		} else {
			// non-zero ports are configured as-is
			assert.Equal(t, ds.Spec.Ports[0].Port, int32(c.spec.Dashboard.Port))
		}
	} else {
		assert.True(t, kerrors.IsNotFound(err))
	}
}

func TestActiveMgrLabels(t *testing.T) {
	var deploymentsUpdated *[]*apps.Deployment
	updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

	c := createNewCluster(t)
	err := c.Start()
	assert.NoError(t, err)
	validateStart(t, c)

	mgrNames := []string{"a", "b"}
	for i := 0; i < c.spec.Mgr.Count; i++ {
		// Simulate the Mgr pod having been created
		daemonName := mgrNames[i]
		mgrPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("rook-ceph-mgr-%s", daemonName),
			Labels: map[string]string{k8sutil.AppAttr: "rook-ceph-mgr",
				"instance":       daemonName,
				"ceph_daemon_id": daemonName,
				"mgr":            daemonName,
				"rook_cluster":   "ns"}}}
		_, err = c.context.Clientset.CoreV1().Pods(c.clusterInfo.Namespace).Create(c.clusterInfo.Context, mgrPod, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	for i := 0; i < c.spec.Mgr.Count; i++ {
		daemonName := mgrNames[i]
		_, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Get(context.TODO(), fmt.Sprintf("rook-ceph-mgr-%s", daemonName), metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = c.UpdateActiveMgrLabel(daemonName, "")
		assert.NoError(t, err)
		pods, err := c.context.Clientset.CoreV1().Pods(c.clusterInfo.Namespace).List(c.clusterInfo.Context, metav1.ListOptions{LabelSelector: "app=rook-ceph-mgr"})
		assert.NoError(t, err)
		assert.Contains(t, pods.Items[i].Labels, "mgr_role")
		if daemonName == "a" {
			assert.Equal(t, pods.Items[i].Labels["mgr_role"], "active")
		} else if daemonName == "b" {
			assert.Equal(t, pods.Items[i].Labels["mgr_role"], "standby")
		}
	}

	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)
}

func TestUpdateServiceSelectors(t *testing.T) {
	clientset := testop.New(t, 3)
	ctx := &clusterd.Context{Clientset: clientset}
	clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
	spec := cephv1.ClusterSpec{
		Dashboard: cephv1.DashboardSpec{
			Enabled: true,
			Port:    7000,
		},
	}
	c := &Cluster{spec: spec, context: ctx, clusterInfo: clusterInfo}

	// Make sure we remove the daemon_id label from the selector
	// of all services with a label "app=rook-ceph-mgr"
	t.Run("remove daemon_id from mgr services", func(t *testing.T) {
		svc1 := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "svc1",
				Labels: map[string]string{
					"app":                    "rook-ceph-mgr",
					"svc":                    "rook-ceph-mgr",
					controller.DaemonIDLabel: "a",
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"app":                    "rook-ceph-mgr",
					controller.DaemonIDLabel: "a",
				}},
		}
		svc2 := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc2"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"app":                    "rook-ceph-mgr",
					controller.DaemonIDLabel: "a",
				}},
		}
		_, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Create(clusterInfo.Context, &svc1, metav1.CreateOptions{})
		assert.NoError(t, err)
		_, err2 := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Create(clusterInfo.Context, &svc2, metav1.CreateOptions{})
		assert.NoError(t, err2)

		// Check the the label has been only removed from svc1
		c.updateServiceSelectors()

		// Make sure we remove controller.DaemonIDLabel label from all services tagged as 'app: rook-ceph-mgr'
		// and we add the selector label "mgr_role" set to "active"
		updatedService1, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Get(clusterInfo.Context, "svc1", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotContains(t, updatedService1.Spec.Selector, controller.DaemonIDLabel)
		assert.Contains(t, updatedService1.Spec.Selector, "mgr_role")
		assert.Equal(t, updatedService1.Spec.Selector["mgr_role"], "active")

		// Make sure services not tagged as 'app: rook-ceph-mgr' are not modified
		updatedService2, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Get(clusterInfo.Context, "svc2", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Contains(t, updatedService2.Spec.Selector, controller.DaemonIDLabel)
		assert.NotContains(t, updatedService2.Spec.Selector, "mgr_role")
	})
}

func TestConfigureModules(t *testing.T) {
	modulesEnabled := 0
	modulesDisabled := 0
	configSettings := map[string]string{}
	lastModuleConfigured := ""
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" && len(args) > 3 {
				if args[0] == "mgr" && args[1] == "module" {
					if args[2] == "enable" {
						modulesEnabled++
					}
					if args[2] == "disable" {
						modulesDisabled++
					}
					lastModuleConfigured = args[3]
				}
			}
			return "", nil //return "{\"key\":\"mysecurekey\"}", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "config" && args[1] == "set" && args[2] == "global" {
				configSettings[args[3]] = args[4]
			}
			return "", nil
		},
	}

	clientset := testop.New(t, 3)
	context := &clusterd.Context{Executor: executor, Clientset: clientset}
	clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
	c := &Cluster{
		context:     context,
		clusterInfo: clusterInfo,
	}

	// one module without any special configuration
	c.spec.Mgr.Modules = []cephv1.Module{
		{Name: "mymodule", Enabled: true},
	}
	assert.NoError(t, c.configureMgrModules())
	assert.Equal(t, 1, modulesEnabled)
	assert.Equal(t, 0, modulesDisabled)
	assert.Equal(t, "mymodule", lastModuleConfigured)

	// disable the module
	modulesEnabled = 0
	lastModuleConfigured = ""
	configSettings = map[string]string{}
	c.spec.Mgr.Modules[0].Enabled = false
	assert.NoError(t, c.configureMgrModules())
	assert.Equal(t, 0, modulesEnabled)
	assert.Equal(t, 1, modulesDisabled)
	assert.Equal(t, "mymodule", lastModuleConfigured)
	assert.Equal(t, 0, len(configSettings))
}

func TestMgrDaemons(t *testing.T) {
	spec := cephv1.ClusterSpec{
		Mgr: cephv1.MgrSpec{Count: 1},
	}
	c := &Cluster{spec: spec}
	daemons := c.getDaemonIDs()
	require.Equal(t, 1, len(daemons))
	assert.Equal(t, "a", daemons[0])

	c.spec.Mgr.Count = 2
	daemons = c.getDaemonIDs()
	require.Equal(t, 2, len(daemons))
	assert.Equal(t, "a", daemons[0])
	assert.Equal(t, "b", daemons[1])
}

func TestApplyMonitoringLabels(t *testing.T) {
	clusterSpec := cephv1.ClusterSpec{
		Labels: cephv1.LabelsSpec{},
	}
	c := &Cluster{spec: clusterSpec}
	sm := &monitoringv1.ServiceMonitor{Spec: monitoringv1.ServiceMonitorSpec{
		Endpoints: []monitoringv1.Endpoint{{}}}}

	// Service Monitor RelabelConfigs updated when 'rook.io/managedBy' monitoring label is found
	monitoringLabels := cephv1.LabelsSpec{
		cephv1.KeyMonitoring: map[string]string{
			"rook.io/managedBy": "storagecluster"},
	}
	c.spec.Labels = monitoringLabels
	applyMonitoringLabels(c, sm)
	fmt.Printf("Hello1")
	assert.Equal(t, "managedBy", sm.Spec.Endpoints[0].RelabelConfigs[0].TargetLabel)
	assert.Equal(t, "storagecluster", sm.Spec.Endpoints[0].RelabelConfigs[0].Replacement)

	// Service Monitor RelabelConfigs not updated when the required monitoring label is not found
	monitoringLabels = cephv1.LabelsSpec{
		cephv1.KeyMonitoring: map[string]string{
			"wrongLabelKey": "storagecluster"},
	}
	c.spec.Labels = monitoringLabels
	sm.Spec.Endpoints[0].RelabelConfigs = nil
	applyMonitoringLabels(c, sm)
	assert.Nil(t, sm.Spec.Endpoints[0].RelabelConfigs)

	// Service Monitor RelabelConfigs not updated when no monitoring labels are found
	c.spec.Labels = cephv1.LabelsSpec{}
	sm.Spec.Endpoints[0].RelabelConfigs = nil
	applyMonitoringLabels(c, sm)
	assert.Nil(t, sm.Spec.Endpoints[0].RelabelConfigs)
}

func TestCluster_configurePrometheusModule(t *testing.T) {
	modulesEnabled := 0
	modulesDisabled := 0
	configSettings := map[string]string{}
	lastModuleConfigured := ""
	modulesGetOutput := ""
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" && len(args) > 3 {
				if args[0] == "mgr" && args[1] == "module" {
					if args[2] == "enable" {
						modulesEnabled++
					}
					if args[2] == "disable" {
						modulesDisabled++
					}
					lastModuleConfigured = args[3]
				}
			}
			return "", nil //return "{\"key\":\"mysecurekey\"}", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "config" && args[1] == "set" && args[2] == "mgr" {
				configSettings[args[3]] = args[4]
			}
			if args[0] == "config" && args[1] == "get" && args[2] == "mgr" {
				return modulesGetOutput, nil
			}
			return "", nil
		},
	}

	c := &Cluster{
		context:     &clusterd.Context{Executor: executor, Clientset: testop.New(t, 3)},
		clusterInfo: cephclient.AdminTestClusterInfo("mycluster"),
		spec: cephv1.ClusterSpec{
			Monitoring: cephv1.MonitoringSpec{
				Enabled:         false,
				MetricsDisabled: true,
			},
		},
	}
	// Disable prometheus module
	err := c.configurePrometheusModule()
	assert.NoError(t, err)
	assert.Equal(t, 0, modulesEnabled)
	assert.Equal(t, 1, modulesDisabled)
	assert.Equal(t, PrometheusModuleName, lastModuleConfigured)

	// Enable prometheus module, no changed
	modulesDisabled = 0
	c.spec.Monitoring.MetricsDisabled = false
	err = c.configurePrometheusModule()
	assert.NoError(t, err)
	assert.Equal(t, 1, modulesEnabled)
	assert.Equal(t, 0, modulesDisabled)

	// Enable prometheus module, port changed
	modulesEnabled = 0
	c.spec.Monitoring.Port = 30001
	err = c.configurePrometheusModule()
	assert.NoError(t, err)
	assert.Equal(t, 2, modulesEnabled)
	assert.Equal(t, 1, modulesDisabled)
	assert.Equal(t, "30001", configSettings["mgr/prometheus/server_port"])
	assert.Equal(t, 1, len(configSettings))

	modulesEnabled = 0
	modulesDisabled = 0
	c.spec.Monitoring.Interval = &metav1.Duration{
		Duration: 30 * time.Second,
	}
	modulesGetOutput = "30"
	configSettings = make(map[string]string)
	err = c.configurePrometheusModule()
	assert.NoError(t, err)
	assert.Equal(t, 2, modulesEnabled)
	assert.Equal(t, 1, modulesDisabled)
	assert.Equal(t, "30001", configSettings["mgr/prometheus/server_port"])
	assert.Equal(t, 1, len(configSettings))

	// Enable prometheus module, port and interval changed
	modulesEnabled = 0
	modulesDisabled = 0
	c.spec.Monitoring.Port = 30002
	c.spec.Monitoring.Interval = &metav1.Duration{
		Duration: 60 * time.Second,
	}
	err = c.configurePrometheusModule()
	assert.NoError(t, err)
	assert.Equal(t, 2, modulesEnabled)
	assert.Equal(t, 1, modulesDisabled)
	assert.Equal(t, "30002", configSettings["mgr/prometheus/server_port"])
	assert.Equal(t, "60", configSettings["mgr/prometheus/scrape_interval"])

}
