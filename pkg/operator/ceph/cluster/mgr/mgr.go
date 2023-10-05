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
	"strconv"

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
	PrometheusModuleName   = "prometheus"
	crashModuleName        = "crash"
	PgautoscalerModuleName = "pg_autoscaler"
	balancerModuleName     = "balancer"
	balancerModuleMode     = "upmap"
	mgrRoleLabelName       = "mgr_role"
	activeMgrStatus        = "active"
	standbyMgrStatus       = "standby"
	monitoringPath         = "/etc/ceph-monitoring/"
	serviceMonitorFile     = "service-monitor.yaml"
	serviceMonitorPort     = "http-metrics"
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
	}

	// If the mgr is newly created, wait for it to start before continuing with the service and
	// module configuration
	for _, d := range deploymentsToWaitFor {
		if err := waitForDeploymentToStart(c.clusterInfo.Context, c.context, d); err != nil {
			return errors.Wrapf(err, "failed to wait for mgr %q to start", d.Name)
		}
	}

	// check if any extra mgrs need to be removed
	c.removeExtraMgrs(daemonIDs)

	if len(daemonIDs) > 1 {
		// reconcile mgr PDB
		if err := c.reconcileMgrPDB(); err != nil {
			return errors.Wrap(err, "failed to reconcile mgr PDB")
		}
	} else {
		// delete MGR PDB as the count is less than 2
		c.deleteMgrPDB()
	}

	if err := c.reconcileServices(); err != nil {
		return errors.Wrap(err, "failed to enable mgr services")
	}

	// configure the mgr modules
	c.configureModules(daemonIDs)
	return nil
}

func (c *Cluster) removeExtraMgrs(daemonIDs []string) {
	options := metav1.ListOptions{LabelSelector: "app=" + AppName}
	mgrDeployments, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).List(c.clusterInfo.Context, options)
	if err != nil {
		logger.Warningf("failed to check for extra mgrs. %v", err)
		return
	}
	if len(mgrDeployments.Items) == len(daemonIDs) {
		logger.Debugf("expected number %d of mgrs found", len(daemonIDs))
		return
	}

	// In case the mgr count was reduced, delete the extra mgrs
	for _, mgrDeployment := range mgrDeployments.Items {
		id, ok := mgrDeployment.Labels[controller.DaemonIDLabel]
		if !ok {
			// skipping evaluation of non-mgr daemon that mistakenly matched the mgr labels
			continue
		}
		found := false
		for _, daemonID := range daemonIDs {
			if id == daemonID {
				// mark the mgr as found if the ID matches
				found = true
				break
			}
		}
		if !found {
			err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Delete(c.clusterInfo.Context, mgrDeployment.Name, metav1.DeleteOptions{})
			if err == nil {
				logger.Infof("removed extra mgr %q", mgrDeployment.Name)
			} else {
				logger.Warningf("failed to remove extra mgr %q. %v", mgrDeployment.Name, err)
			}
		}
	}
}

// UpdateActiveMgrLabel updates the mgr_role label value to either
// active or standby depending on the status of the ceph mgr running
// in the pod
func (c *Cluster) UpdateActiveMgrLabel(daemonNameToUpdate string, prevActiveMgr string) (string, error) {

	logger.Infof("Checking mgr_role label value of daemon %s (prev active mgr was %s)", daemonNameToUpdate, prevActiveMgr)
	currActiveMgr, err := c.getActiveMgr()
	if err != nil || currActiveMgr == "" {
		logger.Infof("cannot update active mgr, no active mgr found. err=%v", err)
		return "", err
	} else if prevActiveMgr == currActiveMgr {
		logger.Infof("active mgr is still the same (%s). No need to update mgr_role label on daemon %s.", currActiveMgr, daemonNameToUpdate)
		return currActiveMgr, err
	}

	pods, err := c.context.Clientset.CoreV1().Pods(c.clusterInfo.Namespace).List(c.clusterInfo.Context, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("mgr=%s", daemonNameToUpdate),
	})
	if err != nil {
		logger.Infof("cannot get pod for mgr daemon %s", daemonNameToUpdate)
		return "", err // force mrg_role update in the next call
	}

	for i, pod := range pods.Items {

		labels := pod.GetLabels()
		cephDaemonId := labels[controller.DaemonIDLabel]
		newMgrRole := standbyMgrStatus
		if currActiveMgr == cephDaemonId {
			newMgrRole = activeMgrStatus
		}

		currMgrRole, mgrHasLabel := labels[mgrRoleLabelName]
		if !mgrHasLabel || currMgrRole != newMgrRole {

			logger.Infof("updating mgr_role label value of daemon %s to '%s'. New active mgr is %s.", daemonNameToUpdate, newMgrRole, currActiveMgr)
			labels[mgrRoleLabelName] = newMgrRole
			pod.SetLabels(labels)
			_, err = c.context.Clientset.CoreV1().Pods(c.clusterInfo.Namespace).Update(c.clusterInfo.Context, &pods.Items[i], metav1.UpdateOptions{})
			if err != nil {
				logger.Infof("cannot update the active mgr pod %q. err=%v", pods.Items[i].Name, err)
			}
		}
	}

	return currActiveMgr, c.reconcileServices()
}

func (c *Cluster) getActiveMgr() (string, error) {
	mgrStat, err := cephclient.CephMgrStat(c.context, c.clusterInfo)
	if err != nil {
		return "", errors.Wrap(err, "failed to get mgr stat for the active mgr")
	}
	return mgrStat.ActiveName, nil
}

// reconcile the services,
func (c *Cluster) reconcileServices() error {

	if err := c.configureDashboardService(); err != nil {
		return errors.Wrap(err, "failed to configure dashboard svc")
	}

	// create the metrics service
	service, err := c.MakeMetricsService(AppName, serviceMetricName)
	if err != nil {
		return err
	}
	if _, err := k8sutil.CreateOrUpdateService(c.clusterInfo.Context, c.context.Clientset, c.clusterInfo.Namespace, service); err != nil {
		return errors.Wrap(err, "failed to create mgr metrics service")
	}

	// enable monitoring if `monitoring: enabled: true`
	if c.spec.Monitoring.Enabled {
		if err := c.EnableServiceMonitor(); err != nil {
			return errors.Wrap(err, "failed to enable service monitor")
		}
	}

	c.updateServiceSelectors()
	return nil
}

// For the upgrade scenario: we remove any selector DaemonIDLabel from the all
// the services since new mgr HA doesn't rely on this label anymore and we add
// the new "mgr_role=active" instead.
func (c *Cluster) updateServiceSelectors() {
	selector := metav1.ListOptions{LabelSelector: "app=rook-ceph-mgr"}
	services, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).List(c.clusterInfo.Context, selector)
	if err != nil {
		logger.Errorf("failed to query mgr services to update labels: %v", err)
		return
	}
	for i, service := range services.Items {
		if service.Spec.Selector == nil {
			continue
		}

		updateService := false

		// Check if the service has a DaemonIDLabel (legacy mgr HA implementation) and remove it
		_, hasDaemonLabel := service.Spec.Selector[controller.DaemonIDLabel]
		if hasDaemonLabel {
			logger.Infof("removing %s selector label on mgr service %q", controller.DaemonIDLabel, service.Name)
			delete(service.Spec.Selector, controller.DaemonIDLabel)
			updateService = true
		}

		// In case the service doesn't have the new mgr_role DaemonIDLabel we add it
		_, hasMgrRoleLabel := service.Spec.Selector[mgrRoleLabelName]
		if !hasMgrRoleLabel {
			logger.Infof("adding %s selector label on mgr service %q", mgrRoleLabelName, service.Name)
			service.Spec.Selector[mgrRoleLabelName] = activeMgrStatus
			updateService = true
		}

		if updateService {
			if _, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Update(c.clusterInfo.Context, &services.Items[i], metav1.UpdateOptions{}); err != nil {
				logger.Errorf("failed to update service %q. %v", service.Name, err)
			} else {
				logger.Infof("service %q successfully updated", service.Name)
			}
		}
	}
}

func (c *Cluster) configureModules(daemonIDs []string) {
	// Configure the modules asynchronously so we can complete all the configuration much sooner.
	startModuleConfiguration("prometheus", c.configurePrometheusModule)
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
func (c *Cluster) configurePrometheusModule() error {
	if !c.spec.Monitoring.MetricsDisabled {
		if err := cephclient.MgrEnableModule(c.context, c.clusterInfo, PrometheusModuleName, true); err != nil {
			return errors.Wrap(err, "failed to enable mgr prometheus module")
		}
	} else {
		if err := cephclient.MgrDisableModule(c.context, c.clusterInfo, PrometheusModuleName); err != nil {
			logger.Errorf("failed to disable mgr prometheus module. %v", err)
		}
		return nil
	}

	var (
		err                error
		portHasChanged     bool
		intervalHasChanged bool
		daemonID           = "mgr"
	)
	monStore := config.GetMonStore(c.context, c.clusterInfo)
	// port
	if c.spec.Monitoring.Port != 0 {
		port := strconv.Itoa(c.spec.Monitoring.Port)
		portHasChanged, err = monStore.SetIfChanged(daemonID, "mgr/prometheus/server_port", port)
		if err != nil {
			return err
		}
		logger.Infof("prometheus config will change, port: %s", port)
	}
	// scrape interval
	if c.spec.Monitoring.Interval != nil {
		interval := c.spec.Monitoring.Interval.Duration.Seconds()
		intervalHasChanged, err = monStore.SetIfChanged(daemonID, "mgr/prometheus/scrape_interval", fmt.Sprintf("%v", interval))
		if err != nil {
			return err
		}
		logger.Infof("prometheus config will change, interval: %v", interval)
	}

	if portHasChanged || intervalHasChanged {
		logger.Info("prometheus config has changed. restarting the prometheus module")
		return c.restartMgrModule(PrometheusModuleName)
	}
	return nil
}

func (c *Cluster) restartMgrModule(name string) error {
	logger.Infof("restarting the mgr module: %s", name)
	if err := cephclient.MgrDisableModule(c.context, c.clusterInfo, name); err != nil {
		return errors.Wrapf(err, "failed to disable mgr module %q.", name)
	}
	if err := cephclient.MgrEnableModule(c.context, c.clusterInfo, name, true); err != nil {
		return errors.Wrapf(err, "failed to enable mgr module %q.", name)
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
func (c *Cluster) EnableServiceMonitor() error {
	serviceMonitor := k8sutil.GetServiceMonitor(AppName, c.clusterInfo.Namespace, serviceMonitorPort)
	cephv1.GetMonitoringLabels(c.spec.Labels).OverwriteApplyToObjectMeta(&serviceMonitor.ObjectMeta)

	if c.spec.External.Enable {
		serviceMonitor.Spec.Endpoints[0].Port = controller.ServiceExternalMetricName
	}
	err := c.clusterInfo.OwnerInfo.SetControllerReference(serviceMonitor)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to service monitor %q", serviceMonitor.Name)
	}

	applyMonitoringLabels(c, serviceMonitor)

	if _, err = k8sutil.CreateOrUpdateServiceMonitor(c.context, c.clusterInfo.Context, serviceMonitor); err != nil {
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
			logger.Debug("monitoring labels not specified")
		}
	}
}
