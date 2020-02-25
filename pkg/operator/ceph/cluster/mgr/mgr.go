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
	"strconv"
	"strings"
	"sync"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mgr")

var prometheusRuleName = "prometheus-ceph-vVERSION-rules"

const (
	// AppName is the ceph mgr application name
	AppName                = "rook-ceph-mgr"
	serviceAccountName     = "rook-ceph-mgr"
	prometheusModuleName   = "prometheus"
	crashModuleName        = "crash"
	pgautoscalerModuleName = "pg_autoscaler"
	balancerModuleName     = "balancer"
	balancerModuleMode     = "upmap"
	metricsPort            = 9283
	monitoringPath         = "/etc/ceph-monitoring/"
	serviceMonitorFile     = "service-monitor.yaml"
	// minimum amount of memory in MB to run the pod
	cephMgrPodMinimumMemory uint64 = 512
)

// Cluster represents the Rook and environment configuration settings needed to set up Ceph mgrs.
type Cluster struct {
	clusterInfo       *cephconfig.ClusterInfo
	Namespace         string
	Replicas          int
	placement         rookv1.Placement
	annotations       rookv1.Annotations
	context           *clusterd.Context
	dataDir           string
	Network           cephv1.NetworkSpec
	resources         v1.ResourceRequirements
	priorityClassName string
	ownerRef          metav1.OwnerReference
	dashboard         cephv1.DashboardSpec
	monitoringSpec    cephv1.MonitoringSpec
	mgrSpec           cephv1.MgrSpec
	cephVersion       cephv1.CephVersionSpec
	rookVersion       string
	exitCode          func(err error) (int, bool)
	dataDirHostPath   string
	skipUpgradeChecks bool
	appliedHttpBind   bool
}

// New creates an instance of the mgr
func New(
	clusterInfo *cephconfig.ClusterInfo,
	context *clusterd.Context,
	namespace, rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	placement rookv1.Placement,
	annotations rookv1.Annotations,
	network cephv1.NetworkSpec,
	dashboard cephv1.DashboardSpec,
	monitoringSpec cephv1.MonitoringSpec,
	mgrSpec cephv1.MgrSpec,
	resources v1.ResourceRequirements,
	priorityClassName string,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
	skipUpgradeChecks bool,
) *Cluster {
	return &Cluster{
		clusterInfo:       clusterInfo,
		context:           context,
		Namespace:         namespace,
		placement:         placement,
		annotations:       annotations,
		rookVersion:       rookVersion,
		cephVersion:       cephVersion,
		Replicas:          1,
		dataDir:           k8sutil.DataDir,
		dashboard:         dashboard,
		monitoringSpec:    monitoringSpec,
		mgrSpec:           mgrSpec,
		Network:           network,
		resources:         resources,
		priorityClassName: priorityClassName,
		ownerRef:          ownerRef,
		exitCode:          getExitCode,
		dataDirHostPath:   dataDirHostPath,
		skipUpgradeChecks: skipUpgradeChecks,
	}
}

var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

func (c *Cluster) getDaemonIDs() []string {
	var daemonIDs []string
	for i := 0; i < c.Replicas; i++ {
		if i >= 2 {
			logger.Errorf("cannot have more than 2 mgrs")
			break
		}
		daemonIDs = append(daemonIDs, k8sutil.IndexToName(i))
	}
	return daemonIDs
}

// Start begins the process of running a cluster of Ceph mgrs.
func (c *Cluster) Start() error {
	// Validate pod's memory if specified
	err := opspec.CheckPodMemory(c.resources, cephMgrPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	logger.Infof("start running mgr")
	daemonIDs := c.getDaemonIDs()
	for _, daemonID := range daemonIDs {
		resourceName := fmt.Sprintf("%s-%s", AppName, daemonID)
		mgrConfig := &mgrConfig{
			DaemonID:     daemonID,
			ResourceName: resourceName,
			DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, daemonID, c.Namespace, c.dataDirHostPath),
		}

		// generate keyring specific to this mgr daemon saved to k8s secret
		keyring, err := c.generateKeyring(mgrConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to generate keyring for %q", resourceName)
		}

		// start the deployment
		d := c.makeDeployment(mgrConfig)
		logger.Debugf("starting mgr deployment: %+v", d)

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(d)
		if err != nil {
			return errors.Wrapf(err, "failed to set annotation for deployment %q", d.Name)
		}

		_, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(d)
		if err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create mgr deployment %s", resourceName)
			}
			logger.Infof("deployment for mgr %s already exists. updating if needed", resourceName)

			// Always invoke ceph version before an upgrade so we are sure to be up-to-date
			daemon := string(config.MgrType)
			var cephVersionToUse cephver.CephVersion

			currentCephVersion, err := client.LeastUptodateDaemonVersion(c.context, c.clusterInfo.Name, daemon)
			if err != nil {
				logger.Warningf("failed to retrieve current ceph %q version. %+v", daemon, err)
				logger.Debug("could not detect ceph version during update, this is likely an initial bootstrap, proceeding with %+v", c.clusterInfo.CephVersion)
				cephVersionToUse = c.clusterInfo.CephVersion
			} else {
				logger.Debugf("current cluster version for mgrs before upgrading is: %+v", currentCephVersion)
				cephVersionToUse = currentCephVersion
			}

			if err := updateDeploymentAndWait(c.context, d, c.Namespace, daemon, mgrConfig.DaemonID, cephVersionToUse, c.skipUpgradeChecks, false); err != nil {
				logger.Errorf("failed to update mgr deployment %q. %+v", resourceName, err)
			}
		}
		if existingDeployment, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(d.GetName(), metav1.GetOptions{}); err != nil {
			logger.Warningf("failed to find mgr deployment %q for keyring association. %v", resourceName, err)
		} else {
			if err = c.associateKeyring(keyring, existingDeployment); err != nil {
				logger.Warningf("failed to associate keyring with mgr deployment %q. %v", resourceName, err)
			}
		}
	}

	if err := c.configureDashboardService(); err != nil {
		logger.Errorf("failed to enable dashboard. %v", err)
	}

	// configure the mgr modules
	c.configureModules(daemonIDs)

	// create the metrics service
	service := c.makeMetricsService(AppName)
	if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(service); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create mgr service")
		}
		logger.Infof("mgr metrics service already exists")
	} else {
		logger.Infof("mgr metrics service started")
	}

	// enable monitoring if `monitoring: enabled: true`
	if c.monitoringSpec.Enabled {
		logger.Infof("starting monitoring deployment")
		// servicemonitor takes some metadata from the service for easy mapping
		if err := c.enableServiceMonitor(service); err != nil {
			logger.Errorf("failed to enable service monitor. %v", err)
		} else {
			logger.Infof("servicemonitor enabled")
		}
		// namespace in which the prometheusRule should be deployed
		// if left empty, it will be deployed in current namespace
		namespace := c.monitoringSpec.RulesNamespace
		if namespace == "" {
			namespace = c.Namespace
		}
		if err := c.deployPrometheusRule(prometheusRuleName, namespace); err != nil {
			logger.Errorf("failed to deploy prometheus rule. %v", err)
		} else {
			logger.Infof("prometheusRule deployed")
		}
		logger.Debugf("ended monitoring deployment")
	}
	return nil
}

func (c *Cluster) configureModules(daemonIDs []string) {
	// Configure the modules asynchronously so we can complete all the configuration much sooner.
	var wg sync.WaitGroup
	startModuleConfiguration(&wg, "http bind settings", c.clearHTTPBindFix)
	startModuleConfiguration(&wg, "orchestrator modules", c.configureOrchestratorModules)
	startModuleConfiguration(&wg, "prometheus", c.enablePrometheusModule)
	startModuleConfiguration(&wg, "crash", c.enableCrashModule)
	startModuleConfiguration(&wg, "mgr module(s) from the spec", c.configureMgrModules)
	startModuleConfiguration(&wg, "dashboard", c.configureDashboardModules)

	// Wait for the goroutines to complete before continuing
	wg.Wait()
}

func startModuleConfiguration(wg *sync.WaitGroup, description string, configureModules func() error) {
	wg.Add(1)
	go func() {
		err := configureModules()
		if err != nil {
			logger.Errorf("failed modules: %q. %v", description, err)
		} else {
			logger.Infof("successful modules: %s", description)
		}
		wg.Done()
	}()
}

// Ceph docs about the prometheus module: http://docs.ceph.com/docs/master/mgr/prometheus/
func (c *Cluster) enablePrometheusModule() error {
	if err := client.MgrEnableModule(c.context, c.Namespace, prometheusModuleName, true); err != nil {
		return errors.Wrapf(err, "failed to enable mgr prometheus module")
	}
	return nil
}

// Ceph docs about the crash module: https://docs.ceph.com/docs/master/mgr/crash/
func (c *Cluster) enableCrashModule() error {
	if err := client.MgrEnableModule(c.context, c.Namespace, crashModuleName, true); err != nil {
		return errors.Wrapf(err, "failed to enable mgr crash module")
	}
	return nil
}

func (c *Cluster) configureMgrModules() error {
	// Enable mgr modules from the spec
	for _, module := range c.mgrSpec.Modules {
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
				// Set min compat client to luminous before enabling the balancer mode "upmap"
				err := client.SetMinCompatClientLuminous(c.context, c.Namespace)
				if err != nil {
					return errors.Wrap(err, "failed to set minimum compatibility client")
				}
				// Set balancer module mode
				err = client.MgrSetBalancerMode(c.context, c.Namespace, balancerModuleMode)
				if err != nil {
					return errors.Wrapf(err, "failed to set module %q mode to %q", module.Name, balancerModuleMode)
				}
			}

			if err := client.MgrEnableModule(c.context, c.Namespace, module.Name, false); err != nil {
				return errors.Wrapf(err, "failed to enable mgr module %q", module.Name)
			}

			// Configure special settings for individual modules that are enabled
			if module.Name == pgautoscalerModuleName {
				monStore := config.GetMonStore(c.context, c.Namespace)
				// Ceph Octopus will have that option enabled
				err := monStore.Set("global", "osd_pool_default_pg_autoscale_mode", "on")
				if err != nil {
					return errors.Wrapf(err, "failed to enable pg autoscale mode for newly created pools")
				}
				err = monStore.Set("global", "mon_pg_warn_min_per_osd", "0")
				if err != nil {
					return errors.Wrapf(err, "failed to set minimal number PGs per (in) osd before we warn the admin to")
				}
			}

		} else {
			if err := client.MgrDisableModule(c.context, c.Namespace, module.Name); err != nil {
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
	knownModules := []string{rookModuleName, dashboardModuleName, prometheusModuleName, crashModuleName}
	for _, known := range knownModules {
		if name == known {
			return true
		}
	}
	return false
}

// add a servicemonitor that allows prometheus to scrape from the monitoring endpoint of the cluster
func (c *Cluster) enableServiceMonitor(service *v1.Service) error {
	name := service.GetName()
	namespace := service.GetNamespace()
	serviceMonitor, err := k8sutil.GetServiceMonitor(path.Join(monitoringPath, serviceMonitorFile))
	if err != nil {
		return errors.Wrapf(err, "service monitor could not be enabled")
	}
	serviceMonitor.SetName(name)
	serviceMonitor.SetNamespace(namespace)
	k8sutil.SetOwnerRef(&serviceMonitor.ObjectMeta, &c.ownerRef)
	serviceMonitor.Spec.NamespaceSelector.MatchNames = []string{namespace}
	serviceMonitor.Spec.Selector.MatchLabels = service.GetLabels()
	if _, err := k8sutil.CreateOrUpdateServiceMonitor(serviceMonitor); err != nil {
		return errors.Wrapf(err, "service monitor could not be enabled")
	}
	return nil
}

// deploy prometheusRule that adds alerting and/or recording rules to the cluster
func (c *Cluster) deployPrometheusRule(name, namespace string) error {
	version := strconv.Itoa(c.clusterInfo.CephVersion.Major)
	name = strings.Replace(name, "VERSION", version, 1)
	prometheusRuleFile := name + ".yaml"
	prometheusRuleFile = path.Join(monitoringPath, prometheusRuleFile)
	prometheusRule, err := k8sutil.GetPrometheusRule(prometheusRuleFile)
	if err != nil {
		return errors.Wrapf(err, "prometheus rule could not be deployed")
	}
	prometheusRule.SetName(name)
	prometheusRule.SetNamespace(namespace)
	owners := append(prometheusRule.GetOwnerReferences(), c.ownerRef)
	k8sutil.SetOwnerRefs(&prometheusRule.ObjectMeta, owners)
	if _, err := k8sutil.CreateOrUpdatePrometheusRule(prometheusRule); err != nil {
		return errors.Wrapf(err, "prometheus rule could not be deployed")
	}
	return nil
}
