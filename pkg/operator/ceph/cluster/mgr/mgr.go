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

// Package mgr for the Ceph manager.
package mgr

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mgr")

var prometheusRuleName = "prometheus-ceph-vVERSION-rules"

// PrometheusExternalRuleName is the name of the prometheus external rule
var PrometheusExternalRuleName = "prometheus-ceph-vVERSION-rules-external"

const (
	// AppName is the ceph mgr application name
	AppName                = "rook-ceph-mgr"
	serviceAccountName     = "rook-ceph-mgr"
	PrometheusModuleName   = "prometheus"
	crashModuleName        = "crash"
	PgautoscalerModuleName = "pg_autoscaler"
	balancerModuleName     = "balancer"
	balancerModuleMode     = "upmap"
	monitoringPath         = "/etc/ceph-monitoring/"
	serviceMonitorFile     = "service-monitor.yaml"
	// minimum amount of memory in MB to run the pod
	cephMgrPodMinimumMemory uint64 = 512
	// DefaultMetricsPort prometheus exporter port
	DefaultMetricsPort uint16 = 9283
)

// Cluster represents the Rook and environment configuration settings needed to set up Ceph mgrs.
type Cluster struct {
	context         *clusterd.Context
	clusterInfo     *cephclient.ClusterInfo
	Replicas        int
	rookVersion     string
	exitCode        func(err error) (int, bool)
	appliedHttpBind bool
	spec            cephv1.ClusterSpec
}

// New creates an instance of the mgr
func New(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, spec cephv1.ClusterSpec, rookVersion string) *Cluster {
	return &Cluster{
		context:     context,
		clusterInfo: clusterInfo,
		spec:        spec,
		rookVersion: rookVersion,
		Replicas:    spec.Mgr.Count,
		exitCode:    exec.ExitStatus,
	}
}

var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

// for backward compatibility, default to 1 mgr
func (c *Cluster) getReplicas() int {
	replicas := c.Replicas
	if replicas == 0 {
		replicas = 1
	}
	return replicas
}

func (c *Cluster) getDaemonIDs() []string {
	var daemonIDs []string
	for i := 0; i < c.getReplicas(); i++ {
		daemonIDs = append(daemonIDs, k8sutil.IndexToName(i))
	}
	return daemonIDs
}

// Start begins the process of running a cluster of Ceph mgrs.
func (c *Cluster) Start() error {
	ctx := context.TODO()
	// Validate pod's memory if specified
	err := controller.CheckPodMemory(cephv1.ResourcesKeyMgr, cephv1.GetMgrResources(c.spec.Resources), cephMgrPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	logger.Infof("start running mgr")
	daemonIDs := c.getDaemonIDs()
	for _, daemonID := range daemonIDs {
		// Check whether we need to cancel the orchestration
		if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
			return err
		}

		resourceName := fmt.Sprintf("%s-%s", AppName, daemonID)
		mgrConfig := &mgrConfig{
			DaemonID:     daemonID,
			ResourceName: resourceName,
			DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, daemonID, c.clusterInfo.Namespace, c.spec.DataDirHostPath),
		}

		// We set the owner reference of the Secret to the Object controller instead of the replicaset
		// because we watch for that resource and reconcile if anything happens to it
		_, err := c.generateKeyring(mgrConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to generate keyring for %q", resourceName)
		}

		// start the deployment
		d, err := c.makeDeployment(mgrConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to create deployment")
		}

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(d)
		if err != nil {
			return errors.Wrapf(err, "failed to set annotation for deployment %q", d.Name)
		}

		_, err = c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Create(ctx, d, metav1.CreateOptions{})
		if err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create mgr deployment %s", resourceName)
			}
			logger.Infof("deployment for mgr %s already exists. updating if needed", resourceName)

			if err := updateDeploymentAndWait(c.context, c.clusterInfo, d, config.MgrType, mgrConfig.DaemonID, c.spec.SkipUpgradeChecks, false); err != nil {
				logger.Errorf("failed to update mgr deployment %q. %v", resourceName, err)
			}
		}
	}

	if err := c.reconcileServices(); err != nil {
		return errors.Wrap(err, "failed to enable mgr services")
	}

	// configure the mgr modules
	c.configureModules(daemonIDs)

	// enable monitoring if `monitoring: enabled: true`
	if c.spec.Monitoring.Enabled {
		// namespace in which the prometheusRule should be deployed
		// if left empty, it will be deployed in current namespace
		namespace := c.spec.Monitoring.RulesNamespace
		if namespace == "" {
			namespace = c.clusterInfo.Namespace
		}
		if err := c.DeployPrometheusRule(prometheusRuleName, namespace); err != nil {
			logger.Errorf("failed to deploy prometheus rule. %v", err)
		} else {
			logger.Infof("prometheusRule deployed")
		}
		logger.Debugf("ended monitoring deployment")
	}
	return nil
}

func (c *Cluster) reconcileServices() error {

	mgrMap, err := cephclient.CephMgrMap(c.context, c.clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get ceph status for the active mgr")
	}
	activeDaemon := mgrMap.ActiveName
	if activeDaemon == "" {
		return errors.Errorf("active mgr not found")
	}
	logger.Infof("Active mgr is %q", activeDaemon)

	if err := c.configureDashboardService(activeDaemon); err != nil {
		return errors.Wrap(err, "failed to configure dashboard svc")
	}

	// create the metrics service
	service := c.MakeMetricsService(AppName, activeDaemon, serviceMetricName)
	if _, err := k8sutil.CreateOrUpdateService(c.context.Clientset, c.clusterInfo.Namespace, service); err != nil {
		return errors.Wrap(err, "failed to create mgr metrics service")
	}

	// enable monitoring if `monitoring: enabled: true`
	if c.spec.Monitoring.Enabled {
		if err := c.EnableServiceMonitor(activeDaemon); err != nil {
			return errors.Wrap(err, "failed to enable service monitor")
		}
	}

	return nil
}

func (c *Cluster) configureModules(daemonIDs []string) {
	// Configure the modules asynchronously so we can complete all the configuration much sooner.
	startModuleConfiguration("http bind settings", c.clearHTTPBindFix)
	startModuleConfiguration("prometheus", c.enablePrometheusModule)
	startModuleConfiguration("dashboard", c.configureDashboardModules)
	// "crash" is part of the "always_on_modules" list as of Octopus
	if !c.clusterInfo.CephVersion.IsAtLeastOctopus() {
		startModuleConfiguration("crash", c.enableCrashModule)
	} else {
		// The balancer module must be configured on Octopus
		// It is a bit confusing but as of Octopus modules that are in the "always_on_modules" list
		// are "just" enabled, but still they must be configured to work properly
		startModuleConfiguration("balancer", c.enableBalancerModule)
	}
	startModuleConfiguration("mgr module(s) from the spec", c.configureMgrModules)
}

func startModuleConfiguration(description string, configureModules func() error) {
	go func() {
		err := configureModules()
		if err != nil {
			logger.Errorf("failed modules: %q. %v", description, err)
		} else {
			logger.Infof("successful modules: %s", description)
		}
	}()
}

// Ceph docs about the prometheus module: http://docs.ceph.com/docs/master/mgr/prometheus/
func (c *Cluster) enablePrometheusModule() error {
	if err := client.MgrEnableModule(c.context, c.clusterInfo, PrometheusModuleName, true); err != nil {
		return errors.Wrap(err, "failed to enable mgr prometheus module")
	}
	return nil
}

// Ceph docs about the crash module: https://docs.ceph.com/docs/master/mgr/crash/
func (c *Cluster) enableCrashModule() error {
	if err := client.MgrEnableModule(c.context, c.clusterInfo, crashModuleName, true); err != nil {
		return errors.Wrap(err, "failed to enable mgr crash module")
	}
	return nil
}

func (c *Cluster) enableBalancerModule() error {
	// The order MATTERS, always configure this module first, then turn it on

	// This sets min compat client to luminous and the balancer module mode
	err := client.ConfigureBalancerModule(c.context, c.clusterInfo, balancerModuleMode)
	if err != nil {
		return errors.Wrapf(err, "failed to configure module %q", balancerModuleName)
	}

	// This turns "on" the balancer
	err = client.MgrEnableModule(c.context, c.clusterInfo, balancerModuleName, false)
	if err != nil {
		return errors.Wrapf(err, "failed to turn on mgr %q module", balancerModuleName)
	}

	return nil
}

func (c *Cluster) configureMgrModules() error {
	// Enable mgr modules from the spec
	for _, module := range c.spec.Mgr.Modules {
		if module.Name == "" {
			return errors.New("name not specified for the mgr module configuration")
		}
		if wellKnownModule(module.Name) {
			return errors.Errorf("cannot configure mgr module %q that is configured with other cluster settings", module.Name)
		}
		minVersion, versionOK := c.moduleMeetsMinVersion(module.Name)
		if !versionOK {
			return errors.Errorf("module %q cannot be configured because it requires at least Ceph version %q", module.Name, minVersion.String())
		}

		if module.Enabled {
			if module.Name == balancerModuleName {
				// Configure balancer module mode
				err := client.ConfigureBalancerModule(c.context, c.clusterInfo, balancerModuleMode)
				if err != nil {
					return errors.Wrapf(err, "failed to configure module %q", module.Name)
				}
			}

			if err := client.MgrEnableModule(c.context, c.clusterInfo, module.Name, false); err != nil {
				return errors.Wrapf(err, "failed to enable mgr module %q", module.Name)
			}

			// Configure special settings for individual modules that are enabled
			switch module.Name {
			case PgautoscalerModuleName:
				monStore := config.GetMonStore(c.context, c.clusterInfo)
				// Ceph Octopus will have that option enabled
				err := monStore.Set("global", "osd_pool_default_pg_autoscale_mode", "on")
				if err != nil {
					return errors.Wrap(err, "failed to enable pg autoscale mode for newly created pools")
				}
				err = monStore.Set("global", "mon_pg_warn_min_per_osd", "0")
				if err != nil {
					return errors.Wrap(err, "failed to set minimal number PGs per (in) osd before we warn the admin to")
				}
			case rookModuleName:
				startModuleConfiguration("orchestrator modules", c.configureOrchestratorModules)
			}

		} else {
			if err := client.MgrDisableModule(c.context, c.clusterInfo, module.Name); err != nil {
				return errors.Wrapf(err, "failed to disable mgr module %q", module.Name)
			}
		}
	}

	return nil
}

func (c *Cluster) moduleMeetsMinVersion(name string) (*cephver.CephVersion, bool) {
	minVersions := map[string]cephver.CephVersion{
		// Put the modules here, example:
		// pgautoscalerModuleName: {Major: 14},
	}
	if ver, ok := minVersions[name]; ok {
		// Check if the required min version is met
		return &ver, c.clusterInfo.CephVersion.IsAtLeast(ver)
	}
	// no min version was required
	return nil, true
}

func wellKnownModule(name string) bool {
	knownModules := []string{dashboardModuleName, PrometheusModuleName, crashModuleName}
	for _, known := range knownModules {
		if name == known {
			return true
		}
	}
	return false
}

// EnableServiceMonitor add a servicemonitor that allows prometheus to scrape from the monitoring endpoint of the cluster
func (c *Cluster) EnableServiceMonitor(activeDaemon string) error {
	serviceMonitor, err := k8sutil.GetServiceMonitor(path.Join(monitoringPath, serviceMonitorFile))
	if err != nil {
		return errors.Wrap(err, "service monitor could not be enabled")
	}
	serviceMonitor.SetName(AppName)
	serviceMonitor.SetNamespace(c.clusterInfo.Namespace)
	if c.spec.External.Enable {
		serviceMonitor.Spec.Endpoints[0].Port = ServiceExternalMetricName
	}
	k8sutil.SetOwnerRef(&serviceMonitor.ObjectMeta, &c.clusterInfo.OwnerRef)
	serviceMonitor.Spec.NamespaceSelector.MatchNames = []string{c.clusterInfo.Namespace}
	serviceMonitor.Spec.Selector.MatchLabels = c.selectorLabels(activeDaemon)
	if _, err := k8sutil.CreateOrUpdateServiceMonitor(serviceMonitor); err != nil {
		return errors.Wrap(err, "service monitor could not be enabled")
	}
	return nil
}

// DeployPrometheusRule deploy prometheusRule that adds alerting and/or recording rules to the cluster
func (c *Cluster) DeployPrometheusRule(name, namespace string) error {
	version := strconv.Itoa(c.clusterInfo.CephVersion.Major)
	name = strings.Replace(name, "VERSION", version, 1)
	prometheusRuleFile := name + ".yaml"
	prometheusRuleFile = path.Join(monitoringPath, prometheusRuleFile)
	prometheusRule, err := k8sutil.GetPrometheusRule(prometheusRuleFile)
	if err != nil {
		return errors.Wrap(err, "prometheus rule could not be deployed")
	}
	prometheusRule.SetName(name)
	prometheusRule.SetNamespace(namespace)
	owners := append(prometheusRule.GetOwnerReferences(), c.clusterInfo.OwnerRef)
	k8sutil.SetOwnerRefs(&prometheusRule.ObjectMeta, owners)
	if _, err := k8sutil.CreateOrUpdatePrometheusRule(prometheusRule); err != nil {
		return errors.Wrap(err, "prometheus rule could not be deployed")
	}
	return nil
}

// IsModuleInSpec returns whether a module is present in the CephCluster manager spec
func IsModuleInSpec(modules []cephv1.Module, moduleName string) bool {
	for _, v := range modules {
		if v.Name == moduleName {
			return true
		}
	}

	return false
}
