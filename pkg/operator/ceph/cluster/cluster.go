/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"

	"github.com/google/go-cmp/cmp"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/crash"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	cephspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/ceph/version"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	detectVersionName = "rook-ceph-detect-version"
)

type cluster struct {
	Info                 *cephconfig.ClusterInfo
	context              *clusterd.Context
	Namespace            string
	Spec                 *cephv1.ClusterSpec
	crdName              string
	mons                 *mon.Cluster
	initCompleted        bool
	stopCh               chan struct{}
	ownerRef             metav1.OwnerReference
	orchestrationRunning bool
	orchestrationNeeded  bool
	orchMux              sync.Mutex
	childControllers     []childController
	isUpgrade            bool
}

// ChildController is implemented by CRs that are owned by the CephCluster
type childController interface {
	// ParentClusterChanged is called when the CephCluster CR is updated, for example for a newer ceph version
	ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo, isUpgrade bool)
}

func newCluster(c *cephv1.CephCluster, context *clusterd.Context, csiMutex *sync.Mutex) *cluster {
	ownerRef := ClusterOwnerRef(c.Name, string(c.UID))
	return &cluster{
		// at this phase of the cluster creation process, the identity components of the cluster are
		// not yet established. we reserve this struct which is filled in as soon as the cluster's
		// identity can be established.
		Info:      nil,
		Namespace: c.Namespace,
		Spec:      &c.Spec,
		context:   context,
		crdName:   c.Name,
		stopCh:    make(chan struct{}),
		ownerRef:  ownerRef,
		// we set isUpgrade to false since it's a new cluster
		mons: mon.New(context, c.Namespace, c.Spec.DataDirHostPath, c.Spec.Network, ownerRef, csiMutex, false),
	}
}

// detectCephVersion loads the ceph version from the image and checks that it meets the version requirements to
// run in the cluster
func (c *cluster) detectCephVersion(rookImage, cephImage string, timeout time.Duration) (*cephver.CephVersion, error) {
	logger.Infof("detecting the ceph image version for image %s...", cephImage)
	versionReporter, err := cmdreporter.New(
		c.context.Clientset, &c.ownerRef,
		detectVersionName, detectVersionName, c.Namespace,
		[]string{"ceph"}, []string{"--version"},
		rookImage, cephImage)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set up ceph version job")
	}

	job := versionReporter.Job()
	job.Spec.Template.Spec.ServiceAccountName = "rook-ceph-cmd-reporter"

	// Apply the same node selector and tolerations for the ceph version detection as the mon daemons
	cephv1.GetMonPlacement(c.Spec.Placement).ApplyToPodSpec(&job.Spec.Template.Spec)

	stdout, stderr, retcode, err := versionReporter.Run(timeout)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to complete ceph version job")
	}
	if retcode != 0 {
		return nil, errors.Errorf(`ceph version job returned failure with retcode %d.
  stdout: %s
  stderr: %s`, retcode, stdout, stderr)
	}

	version, err := cephver.ExtractCephVersion(stdout)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract ceph version")
	}
	logger.Infof("Detected ceph image version: %q", version)
	return version, nil
}

func (c *cluster) validateCephVersion(version *cephver.CephVersion) error {
	if !c.Spec.External.Enable {
		if !version.IsAtLeast(cephver.Minimum) {
			return errors.Errorf("the version does not meet the minimum version: %q", cephver.Minimum.String())
		}

		if !version.Supported() {
			if !c.Spec.CephVersion.AllowUnsupported {
				return errors.Errorf("allowUnsupported must be set to true to run with this version: %+v", version)
			}
			logger.Warningf("unsupported ceph version detected: %q, pursuing", version)
		}
	}

	// The following tries to determine if the operator can proceed with an upgrade because we come from an OnAdd() call
	// If the cluster was unhealthy and someone injected a new image version, an upgrade was triggered but failed because the cluster is not healthy
	// Then after this, if the operator gets restarted we are not able to fail if the cluster is not healthy, the following tries to determine the
	// state we are in and if we should upgrade or not

	// Try to load clusterInfo so we can compare the running version with the one from the spec image
	clusterInfo, _, _, err := mon.LoadClusterInfo(c.context, c.Namespace)
	if err == nil {
		// Write connection info (ceph config file and keyring) for ceph commands
		err = mon.WriteConnectionConfig(c.context, clusterInfo)
		if err != nil {
			logger.Errorf("failed to write config. attempting to continue. %v", err)
		}
	}

	if !clusterInfo.IsInitialized() {
		// If not initialized, this is likely a new cluster so there is nothing to do
		logger.Debug("cluster not initialized, nothing to validate")
		return nil
	}

	if c.Spec.External.Enable && c.Spec.CephVersion.Image != "" {
		c.Info.CephVersion, err = cephspec.ValidateCephVersionsBetweenLocalAndExternalClusters(c.context, c.Namespace, *version)
		if err != nil {
			return errors.Wrapf(err, "failed to validate ceph version between external and local")
		}
	}

	// On external cluster setup, if we don't bootstrap any resources in the Kubernetes cluster then
	// there is no need to validate the Ceph image further
	if c.Spec.External.Enable && c.Spec.CephVersion.Image == "" {
		logger.Debug("no spec image specified on external cluster, not validating Ceph version.")
		return nil
	}

	// Get cluster running versions
	versions, err := client.GetAllCephDaemonVersions(c.context, c.Namespace)
	if err != nil {
		logger.Errorf("failed to get ceph daemons versions. %v", err)
		return nil
	}

	runningVersions := *versions
	differentImages, err := diffImageSpecAndClusterRunningVersion(*version, runningVersions)
	if err != nil {
		logger.Errorf("failed to determine if we should upgrade or not. %v", err)
		// we shouldn't block the orchestration if we can't determine the version of the image spec, we proceed anyway in best effort
		// we won't be able to check if there is an update or not and what to do, so we don't check the cluster status either
		// This will happen if someone uses ceph/daemon:latest-master for instance
		return nil
	}

	if differentImages {
		// If the image version changed let's make sure we can safely upgrade
		// check ceph's status, if not healthy we fail
		cephHealthy := client.IsCephHealthy(c.context, c.Namespace)
		if !cephHealthy {
			if c.Spec.SkipUpgradeChecks {
				logger.Warning("ceph is not healthy but SkipUpgradeChecks is set, forcing upgrade.")
			} else {
				return errors.Errorf("ceph status in namespace %s is not healthy, refusing to upgrade. fix the cluster and re-edit the cluster CR to trigger a new orchestation update", c.Namespace)
			}
		}
		c.isUpgrade = true
	}

	return nil
}

// initialized checks if the cluster has ever completed a successful orchestration since the operator has started
func (c *cluster) initialized() bool {
	return c.initCompleted
}

func (c *cluster) createInstance(rookImage string, cephVersion cephver.CephVersion) error {
	var err error
	c.setOrchestrationNeeded()

	// execute an orchestration until
	// there are no more unapplied changes to the cluster definition and
	// while no other goroutine is already running a cluster update
	for c.checkSetOrchestrationStatus() == true {
		if err != nil {
			logger.Errorf("There was an orchestration error, but there is another orchestration pending; proceeding with next orchestration run (which may succeed). %v", err)
		}
		// Use a DeepCopy of the spec to avoid using an inconsistent data-set
		spec := c.Spec.DeepCopy()

		err = c.doOrchestration(rookImage, cephVersion, spec)

		c.unsetOrchestrationStatus()
	}

	return err
}

func (c *cluster) doOrchestration(rookImage string, cephVersion cephver.CephVersion, spec *cephv1.ClusterSpec) error {
	// Create a configmap for overriding ceph config settings
	// These settings should only be modified by a user after they are initialized
	err := populateConfigOverrideConfigMap(c.context, c.Namespace, c.ownerRef)
	if err != nil {
		return errors.Wrapf(err, "failed to populate config override config map")
	}

	if c.Spec.External.Enable {
		// Apply CRD ConfigOverrideName to the external cluster
		err = config.SetDefaultConfigs(c.context, c.Namespace, c.Info)
		if err != nil {
			// Mons are up, so something else is wrong
			return errors.Wrapf(err, "failed to set Rook and/or user-defined Ceph config options on the external cluster monitors")
		}

		// The cluster Identity must be established at this point
		if !c.Info.IsInitialized() {
			return errors.Errorf("the cluster identity was not established: %+v", c.Info)
		}
	} else {
		// This gets triggered on CR update so let's not run that (mon/mgr/osd daemons)
		// Start the mon pods
		clusterInfo, err := c.mons.Start(c.Info, rookImage, cephVersion, *c.Spec, c.isUpgrade)
		if err != nil {
			return errors.Wrapf(err, "failed to start the mons")
		}
		c.Info = clusterInfo // mons return the cluster's info

		// The cluster Identity must be established at this point
		if !c.Info.IsInitialized() {
			return errors.Errorf("the cluster identity was not established: %+v", c.Info)
		}

		// Execute actions after the monitors are up and running
		logger.Debug("monitors are up and running, executing post actions")
		err = c.postMonStartupActions()
		if err != nil {
			return errors.Wrapf(err, "failed to execute post actions after all the monitors started")
		}

		mgrs := mgr.New(c.Info, c.context, c.Namespace, rookImage,
			spec.CephVersion, cephv1.GetMgrPlacement(spec.Placement), cephv1.GetMgrAnnotations(c.Spec.Annotations),
			spec.Network, spec.Dashboard, spec.Monitoring, spec.Mgr, cephv1.GetMgrResources(spec.Resources),
			cephv1.GetMgrPriorityClassName(spec.PriorityClassNames), c.ownerRef, c.Spec.DataDirHostPath, c.isUpgrade, c.Spec.SkipUpgradeChecks)
		err = mgrs.Start()
		if err != nil {
			return errors.Wrapf(err, "failed to start the ceph mgr")
		}

		// Start the OSDs
		osds := osd.New(c.Info, c.context, c.Namespace, rookImage, spec.CephVersion, spec.Storage, spec.DataDirHostPath,
			cephv1.GetOSDPlacement(spec.Placement), cephv1.GetOSDAnnotations(spec.Annotations), spec.Network,
			cephv1.GetOSDResources(spec.Resources), cephv1.GetPrepareOSDResources(spec.Resources),
			cephv1.GetOSDPriorityClassName(spec.PriorityClassNames), c.ownerRef, c.isUpgrade, c.Spec.SkipUpgradeChecks, c.Spec.ContinueUpgradeAfterChecksEvenIfNotHealthy)
		err = osds.Start()
		if err != nil {
			return errors.Wrapf(err, "failed to start the osds")
		}

		// Start the rbd mirroring daemon(s)
		rbdmirror := rbd.New(c.Info, c.context, c.Namespace, rookImage, spec.CephVersion, cephv1.GetRBDMirrorPlacement(spec.Placement),
			cephv1.GetRBDMirrorAnnotations(spec.Annotations), spec.Network, spec.RBDMirroring,
			cephv1.GetRBDMirrorResources(spec.Resources), cephv1.GetRBDMirrorPriorityClassName(spec.PriorityClassNames),
			c.ownerRef, c.Spec.DataDirHostPath, c.isUpgrade, c.Spec.SkipUpgradeChecks)
		err = rbdmirror.Start()
		if err != nil {
			return errors.Wrapf(err, "failed to start the rbd mirrors")
		}

		logger.Infof("Done creating rook instance in namespace %s", c.Namespace)
		c.initCompleted = true
	}

	// Notify the child controllers that the cluster spec might have changed
	logger.Debug("notifying CR child of the potential upgrade")
	for _, child := range c.childControllers {
		child.ParentClusterChanged(*c.Spec, c.Info, c.isUpgrade)
	}

	return nil
}

func clusterChanged(oldCluster, newCluster cephv1.ClusterSpec, clusterRef *cluster) (bool, string) {

	// sort the nodes by name then compare to see if there are changes
	sort.Sort(rookv1alpha2.NodesByName(oldCluster.Storage.Nodes))
	sort.Sort(rookv1alpha2.NodesByName(newCluster.Storage.Nodes))

	// any change in the crd will trigger an orchestration
	if !reflect.DeepEqual(oldCluster, newCluster) {
		diff := ""
		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Warningf("Encountered an issue getting cluster change differences: %v", err)
				}
			}()

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			diff = cmp.Diff(oldCluster, newCluster, resourceQtyComparer)
		}()
		if diff != "" {
			logger.Infof("The Cluster CR has changed. diff=%s", diff)
			return true, diff
		}

	}
	return false, ""
}

func (c *cluster) setOrchestrationNeeded() {
	c.orchMux.Lock()
	c.orchestrationNeeded = true
	c.orchMux.Unlock()
}

// unsetOrchestrationStatus resets the orchestrationRunning-flag
func (c *cluster) unsetOrchestrationStatus() {
	c.orchMux.Lock()
	defer c.orchMux.Unlock()
	c.orchestrationRunning = false
}

// checkSetOrchestrationStatus is responsible to do orchestration as long as there is a request needed
func (c *cluster) checkSetOrchestrationStatus() bool {
	c.orchMux.Lock()
	defer c.orchMux.Unlock()
	// check if there is an orchestration needed currently
	if c.orchestrationNeeded == true && c.orchestrationRunning == false {
		// there is an orchestration needed
		// allow to enter the orchestration-loop
		c.orchestrationNeeded = false
		c.orchestrationRunning = true
		return true
	}

	return false
}

// This function compare the Ceph spec image and the cluster running version
// It returns true if the image is different and false if identical
func diffImageSpecAndClusterRunningVersion(imageSpecVersion cephver.CephVersion, runningVersions client.CephDaemonsVersions) (bool, error) {
	numberOfCephVersions := len(runningVersions.Overall)
	if numberOfCephVersions == 0 {
		// let's return immediately
		return false, errors.Errorf("no 'overall' section in the ceph versions. %+v", runningVersions.Overall)
	}

	if numberOfCephVersions > 1 {
		// let's return immediately
		logger.Warningf("it looks like we have more than one ceph version running. triggering upgrade. %+v:", runningVersions.Overall)
		return true, nil
	}

	if numberOfCephVersions == 1 {
		for v := range runningVersions.Overall {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				logger.Errorf("failed to extract ceph version. %v", err)
				return false, err
			}
			clusterRunningVersion := *version

			// If this is the same version
			if cephver.IsIdentical(clusterRunningVersion, imageSpecVersion) {
				logger.Debugf("both cluster and image spec versions are identical, doing nothing %s", imageSpecVersion.String())
				return false, nil
			}

			if cephver.IsSuperior(imageSpecVersion, clusterRunningVersion) {
				logger.Infof("image spec version %s is higher than the running cluster version %s, upgrading", imageSpecVersion.String(), clusterRunningVersion.String())
				return true, nil
			}

			if cephver.IsInferior(imageSpecVersion, clusterRunningVersion) {
				return true, errors.Errorf("image spec version %s is lower than the running cluster version %s, downgrading is not supported", imageSpecVersion.String(), clusterRunningVersion.String())
			}
		}
	}

	return false, nil
}

// postMonStartupActions is a collection of actions to run once the monitors are up and running
// It gets executed right after the main mon Start() method
// Basically, it is executed between the monitors and the manager sequence
func (c *cluster) postMonStartupActions() error {
	// Create CSI Kubernetes Secrets
	err := csi.CreateCSISecrets(c.context, c.Namespace, &c.ownerRef)
	if err != nil {
		return errors.Wrapf(err, "failed to create csi kubernetes secrets")
	}

	// Create Crash Collector Secret
	// In 14.2.5 the crash daemon will read the client.crash key instead of the admin key
	if c.Info.CephVersion.IsAtLeast(version.CephVersion{Major: 14, Minor: 2, Extra: 5}) {
		err = crash.CreateCrashCollectorSecret(c.context, c.Namespace, &c.ownerRef)
		if err != nil {
			return errors.Wrapf(err, "failed to create crash collector kubernetes secret")
		}
	}

	// Enable Ceph messenger 2 protocol on Nautilus
	if c.Info.CephVersion.IsAtLeastNautilus() {
		v, err := client.GetCephMonVersion(c.context, c.Info.Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get ceph mon version")
		}
		if v.IsAtLeastNautilus() {
			versions, err := client.GetAllCephDaemonVersions(c.context, c.Info.Name)
			if err != nil {
				return errors.Wrapf(err, "failed to get ceph daemons versions")
			}
			if len(versions.Mon) == 1 {
				// If length is one, this clearly indicates that all the mons are running the same version
				// We are doing this because 'ceph version' might return the Ceph version that a majority of mons has but not all of them
				// so instead of trying to active msgr2 when mons are not ready, we activate it when we believe that's the right time
				client.EnableMessenger2(c.context, c.Namespace)
			}
		}
	}

	return nil
}
