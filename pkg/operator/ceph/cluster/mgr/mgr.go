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
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	v1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mgr")

var prometheusRuleName = "prometheus-ceph-vVERSION-rules"

// PrometheusExternalRuleName is the name of the prometheus external rule
var PrometheusExternalRuleName = "prometheus-ceph-vVERSION-rules-external"

const (
	AppName                = "rook-ceph-mgr"
	serviceAccountName     = "rook-ceph-mgr"
	maxMgrCount            = 2
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
	context     *clusterd.Context
	clusterInfo *cephclient.ClusterInfo
	rookVersion string
	exitCode    func(err error) (int, bool)
	spec        cephv1.ClusterSpec
}

// New creates an instance of the mgr
func New(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, spec cephv1.ClusterSpec, rookVersion string) *Cluster {
	return &Cluster{
		context:     context,
		clusterInfo: clusterInfo,
		spec:        spec,
		rookVersion: rookVersion,
		exitCode:    exec.ExitStatus,
	}
}

var waitForDeploymentToStart = k8sutil.WaitForDeploymentToStart
var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

// for backward compatibility, default to 1 mgr
func (c *Cluster) getReplicas() int {
	replicas := c.spec.Mgr.Count
	if replicas == 0 {
		replicas = 1
	}
	return replicas
}

func (c *Cluster) getDaemonIDs() []string {
	var daemonIDs []string
	replicas := c.getReplicas()
	if replicas > maxMgrCount {
		replicas = maxMgrCount
	}
	for i := 0; i < replicas; i++ {
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
	var deploymentsToWaitFor []*v1.Deployment

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

		newDeployment, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Create(ctx, d, metav1.CreateOptions{})
		if err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create mgr deployment %s", resourceName)
			}
			logger.Infof("deployment for mgr %s already exists. updating if needed", resourceName)

			if err := updateDeploymentAndWait(c.context, c.clusterInfo, d, config.MgrType, mgrConfig.DaemonID, c.spec.SkipUpgradeChecks, false); err != nil {
				logger.Errorf("failed to update mgr deployment %q. %v", resourceName, err)
			}
		} else {
			// wait for the new deployment
			deploymentsToWaitFor = append(deploymentsToWaitFor, newDeployment)
		}
	}

	// If the mgr is newly created, wait for it to start before continuing with the service and
	// module configuration
	for _, d := range deploymentsToWaitFor {
		if err := waitForDeploymentToStart(c.context, d); err != nil {
			return errors.Wrapf(err, "failed to wait for mgr %q to start", d.Name)
		}
	}

	// check if any extra mgrs need to be removed
	c.removeExtraMgrs(daemonIDs)

	activeMgr := daemonIDs[0]
	if len(daemonIDs) > 1 {
		// When multiple mgrs are running, the mgr sidecar for the active mgr
		// will create the services. However, the sidecar will only reconcile all
		// the services when the active mgr changes. Here as part of the regular reconcile
		// we trigger reconciling all the services to ensure they are current.
		activeMgr, err = c.getActiveMgr()
		if err != nil || activeMgr == "" {
			activeMgr = ""
			logger.Infof("cannot reconcile mgr services, no active mgr found. err=%v", err)
		}

		// reconcile mgr PDB
		if err := c.reconcileMgrPDB(); err != nil {
			return errors.Wrap(err, "failed to reconcile mgr PDB")
		}
	} else {
		// delete MGR PDB as the count is less than 2
		c.deleteMgrPDB()
	}
	if activeMgr != "" {
		if err := c.reconcileServices(activeMgr); err != nil {
			return errors.Wrap(err, "failed to enable mgr services")
		}
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

func (c *Cluster) removeExtraMgrs(daemonIDs []string) {
	// In case the mgr count was reduced, delete the extra mgrs
	for i := maxMgrCount - 1; i >= len(daemonIDs); i-- {
		mgrName := fmt.Sprintf("%s-%s", AppName, k8sutil.IndexToName(i))
		err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Delete(context.TODO(), mgrName, metav1.DeleteOptions{})
		if err == nil {
			logger.Infof("removed extra mgr %q", mgrName)
		} else if !kerrors.IsNotFound(err) {
			logger.Warningf("failed to remove extra mgr %q. %v", mgrName, err)
		}
	}
}

// ReconcileActiveMgrServices reconciles the services if the active mgr is the one running
// in the sidecar
func (c *Cluster) ReconcileActiveMgrServices(daemonNameToUpdate string) error {
	// If the services are already set to this daemon, no need to attempt to update
	svc, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Get(context.TODO(), AppName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to check current mgr service, proceeding to update. %v", err)
	} else {
		currentDaemon := svc.Spec.Selector[controller.DaemonIDLabel]
		if currentDaemon == daemonNameToUpdate {
			logger.Infof("mgr services already set to daemon %q, no need to update", daemonNameToUpdate)
			return nil
		}
		logger.Infof("mgr service currently set to %q, checking if need to update to %q", currentDaemon, daemonNameToUpdate)
	}

	activeName, err := c.getActiveMgr()
	if err != nil {
		return err
	}
	if activeName == "" {
		return errors.New("active mgr not found")
	}
	if daemonNameToUpdate != activeName {
		logger.Infof("no need for the mgr update since the active mgr is %q, rather than the local mgr %q", activeName, daemonNameToUpdate)
		return nil
	}

	return c.reconcileServices(activeName)
}

func (c *Cluster) getActiveMgr() (string, error) {
	// The preferred way to query the active mgr is "ceph mgr stat", which is only available in pacific or newer
	if c.clusterInfo.CephVersion.IsAtLeastPacific() {
		mgrStat, err := cephclient.CephMgrStat(c.context, c.clusterInfo)
		if err != nil {
			return "", errors.Wrap(err, "failed to get mgr stat for the active mgr")
		}
		return mgrStat.ActiveName, nil
	}

	// The legacy way to query the active mgr is with the verbose "ceph mgr dump"
	mgrMap, err := cephclient.CephMgrMap(c.context, c.clusterInfo)
	if err != nil {
		return "", errors.Wrap(err, "failed to get mgr map for the active mgr")
	}

	return mgrMap.ActiveName, nil
}

// reconcile the services, if the active mgr is not detected, use the default mgr
func (c *Cluster) reconcileServices(activeDaemon string) error {
	logger.Infof("setting services to point to mgr %q", activeDaemon)

	if err := c.configureDashboardService(activeDaemon); err != nil {
		return errors.Wrap(err, "failed to configure dashboard svc")
	}

	// create the metrics service
	service, err := c.MakeMetricsService(AppName, activeDaemon, serviceMetricName)
	if err != nil {
		return err
	}
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
	if err := cephclient.MgrEnableModule(c.context, c.clusterInfo, PrometheusModuleName, true); err != nil {
		return errors.Wrap(err, "failed to enable mgr prometheus module")
	}
	return nil
}

// Ceph docs about the crash module: https://docs.ceph.com/docs/master/mgr/crash/
func (c *Cluster) enableCrashModule() error {
	if err := cephclient.MgrEnableModule(c.context, c.clusterInfo, crashModuleName, true); err != nil {
		return errors.Wrap(err, "failed to enable mgr crash module")
	}
	return nil
}

func (c *Cluster) enableBalancerModule() error {
	// The order MATTERS, always configure this module first, then turn it on

	// This sets min compat client to luminous and the balancer module mode
	err := cephclient.ConfigureBalancerModule(c.context, c.clusterInfo, balancerModuleMode)
	if err != nil {
		return errors.Wrapf(err, "failed to configure module %q", balancerModuleName)
	}

	// This turns "on" the balancer
	err = cephclient.MgrEnableModule(c.context, c.clusterInfo, balancerModuleName, false)
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
				err := cephclient.ConfigureBalancerModule(c.context, c.clusterInfo, balancerModuleMode)
				if err != nil {
					return errors.Wrapf(err, "failed to configure module %q", module.Name)
				}
			}

			if err := cephclient.MgrEnableModule(c.context, c.clusterInfo, module.Name, false); err != nil {
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
			if err := cephclient.MgrDisableModule(c.context, c.clusterInfo, module.Name); err != nil {
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
	cephv1.GetMonitoringLabels(c.spec.Labels).ApplyToObjectMeta(&serviceMonitor.ObjectMeta)

	if c.spec.External.Enable {
		serviceMonitor.Spec.Endpoints[0].Port = controller.ServiceExternalMetricName
	}
	err = c.clusterInfo.OwnerInfo.SetControllerReference(serviceMonitor)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to service monitor %q", serviceMonitor.Name)
	}
	serviceMonitor.Spec.NamespaceSelector.MatchNames = []string{c.clusterInfo.Namespace}
	serviceMonitor.Spec.Selector.MatchLabels = c.selectorLabels(activeDaemon)

	applyMonitoringLabels(c, serviceMonitor)

	if _, err = k8sutil.CreateOrUpdateServiceMonitor(serviceMonitor); err != nil {
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
	err = c.clusterInfo.OwnerInfo.SetControllerReference(prometheusRule)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to prometheus rule %q", prometheusRule.Name)
	}
	cephv1.GetMonitoringLabels(c.spec.Labels).ApplyToObjectMeta(&prometheusRule.ObjectMeta)
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

// ApplyMonitoringLabels function adds the name of the resource that manages
// cephcluster, as a label on the ceph metrics
func applyMonitoringLabels(c *Cluster, serviceMonitor *monitoringv1.ServiceMonitor) {
	if c.spec.Labels != nil {
		if monitoringLabels, ok := c.spec.Labels["monitoring"]; ok {
			if managedBy, ok := monitoringLabels["rook.io/managedBy"]; ok {
				relabelConfig := monitoringv1.RelabelConfig{
					TargetLabel: "managedBy",
					Replacement: managedBy,
				}
				serviceMonitor.Spec.Endpoints[0].RelabelConfigs = append(
					serviceMonitor.Spec.Endpoints[0].RelabelConfigs, &relabelConfig)
			} else {
				logger.Info("rook.io/managedBy not specified in monitoring labels")
			}
		} else {
			logger.Info("monitoring labels not specified")
		}
	}
}
