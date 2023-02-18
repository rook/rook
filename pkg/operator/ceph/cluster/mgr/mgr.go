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
	"fmt"
	"path"

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

var waitForPodsWithLabelToRun = k8sutil.WaitForPodsWithLabelToRun
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
	// Validate pod's memory if specified
	err := controller.CheckPodMemory(cephv1.ResourcesKeyMgr, cephv1.GetMgrResources(c.spec.Resources), cephMgrPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	logger.Infof("start running mgr")
	daemonIDs := c.getDaemonIDs()
	var deploymentsToWaitFor []*v1.Deployment

	for _, daemonID := range daemonIDs {
		if c.clusterInfo.Context.Err() != nil {
			return c.clusterInfo.Context.Err()
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

		newDeployment, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Create(c.clusterInfo.Context, d, metav1.CreateOptions{})
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

	// Insecure global IDs should be disabled for new clusters immediately.
	// If we're waiting for the mgr deployments to start, it is a clean deployment
	if len(deploymentsToWaitFor) > 0 {
		config.DisableInsecureGlobalID(c.context, c.clusterInfo)
		mgrLabel := fmt.Sprintf("%s=%s", controller.DaemonTypeLabel, cephv1.KeyMgr)
		if err := waitForPodsWithLabelToRun(c.clusterInfo.Context, c.context.Clientset, c.clusterInfo.Namespace, mgrLabel); err != nil {
			return errors.Wrap(err, "failed to wait for mgr pods to start")
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
	return nil
}

func (c *Cluster) removeExtraMgrs(daemonIDs []string) {
	// In case the mgr count was reduced, delete the extra mgrs
	for i := maxMgrCount - 1; i >= len(daemonIDs); i-- {
		mgrName := fmt.Sprintf("%s-%s", AppName, k8sutil.IndexToName(i))
		err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Delete(c.clusterInfo.Context, mgrName, metav1.DeleteOptions{})
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
	svc, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Get(c.clusterInfo.Context, AppName, metav1.GetOptions{})
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
	mgrStat, err := cephclient.CephMgrStat(c.context, c.clusterInfo)
	if err != nil {
		return "", errors.Wrap(err, "failed to get mgr stat for the active mgr")
	}
	return mgrStat.ActiveName, nil
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
	if _, err := k8sutil.CreateOrUpdateService(c.clusterInfo.Context, c.context.Clientset, c.clusterInfo.Namespace, service); err != nil {
		return errors.Wrap(err, "failed to create mgr metrics service")
	}

	// enable monitoring if `monitoring: enabled: true`
	if c.spec.Monitoring.Enabled {
		if err := c.EnableServiceMonitor(activeDaemon); err != nil {
			return errors.Wrap(err, "failed to enable service monitor")
		}
	}

	return c.updateServiceSelectors(activeDaemon)
}

// Make a best effort to update the services that have been labeled for being updated
// when the mgr has changed. They might be services for node ports, ingress, etc
func (c *Cluster) updateServiceSelectors(activeDaemon string) error {
	selector := metav1.ListOptions{LabelSelector: "app=rook-ceph-mgr"}
	services, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).List(c.clusterInfo.Context, selector)
	if err != nil {
		return errors.Wrap(err, "failed to query mgr services to update")
	}
	for i, service := range services.Items {
		if service.Spec.Selector == nil {
			service.Spec.Selector = map[string]string{}
		}
		// Update the selector on the service to point to the active mgr
		if service.Spec.Selector[controller.DaemonIDLabel] == activeDaemon {
			logger.Infof("no need to update service %q", service.Name)
			continue
		}
		// Update the service to point to the new active mgr
		service.Spec.Selector[controller.DaemonIDLabel] = activeDaemon
		logger.Infof("updating selector on mgr service %q to active mgr %q", service.Name, activeDaemon)
		if _, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Update(c.clusterInfo.Context, &services.Items[i], metav1.UpdateOptions{}); err != nil {
			logger.Errorf("failed to update service %q. %v", service.Name, err)
		} else {
			logger.Infof("service %q successfully updated to active mgr %q", service.Name, activeDaemon)
		}
	}
	return nil
}

func (c *Cluster) configureModules(daemonIDs []string) {
	// Configure the modules asynchronously so we can complete all the configuration much sooner.
	startModuleConfiguration("prometheus", c.enablePrometheusModule)
	startModuleConfiguration("dashboard", c.configureDashboardModules)

	// It is a bit confusing but modules that are in the "always_on_modules" list
	// are "just" enabled, but still they must be configured to work properly
	startModuleConfiguration("balancer", c.enableBalancerModule)
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

func (c *Cluster) enableBalancerModule() error {

	// This turns "on" the balancer
	err := cephclient.MgrEnableModule(c.context, c.clusterInfo, balancerModuleName, false)
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
				err := monStore.Set("global", "mon_pg_warn_min_per_osd", "0")
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
		// pgautoscalerModuleName: {Major: 15},
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
	cephv1.GetMonitoringLabels(c.spec.Labels).OverwriteApplyToObjectMeta(&serviceMonitor.ObjectMeta)

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

	if _, err = k8sutil.CreateOrUpdateServiceMonitor(c.clusterInfo.Context, serviceMonitor); err != nil {
		return errors.Wrap(err, "service monitor could not be enabled")
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
