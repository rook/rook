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
	"context"
	"fmt"
	"os/exec"
	"path"
	"strconv"
	"sync"
	"syscall"

	"github.com/pkg/errors"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/nodedaemon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/telemetry"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	rookversion "github.com/rook/rook/pkg/version"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	detectVersionName = "rook-ceph-detect-version"
)

var telemetryMutex sync.Mutex

type cluster struct {
	ClusterInfo        *client.ClusterInfo
	context            *clusterd.Context
	Namespace          string
	Spec               *cephv1.ClusterSpec
	clusterMetadata    metav1.ObjectMeta
	namespacedName     types.NamespacedName
	mons               *mon.Cluster
	ownerInfo          *k8sutil.OwnerInfo
	isUpgrade          bool
	monitoringRoutines map[string]*controller.ClusterHealth
	observedGeneration int64
}

func newCluster(ctx context.Context, c *cephv1.CephCluster, context *clusterd.Context, ownerInfo *k8sutil.OwnerInfo) *cluster {
	return &cluster{
		// at this phase of the cluster creation process, the identity components of the cluster are
		// not yet established. we reserve this struct which is filled in as soon as the cluster's
		// identity can be established.
		ClusterInfo:        client.AdminClusterInfo(ctx, c.Namespace, c.Name),
		Namespace:          c.Namespace,
		Spec:               &c.Spec,
		clusterMetadata:    c.ObjectMeta,
		context:            context,
		namespacedName:     types.NamespacedName{Namespace: c.Namespace, Name: c.Name},
		monitoringRoutines: make(map[string]*controller.ClusterHealth),
		ownerInfo:          ownerInfo,
		mons:               mon.New(ctx, context, c.Namespace, c.Spec, ownerInfo),
		// update observedGeneration with current generation value,
		// because generation can be changed before reconcile got completed
		// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
		observedGeneration: c.ObjectMeta.Generation,
	}
}

func (c *cluster) reconcileCephDaemons(rookImage string, cephVersion cephver.CephVersion) error {
	// Create a configmap for overriding ceph config settings
	// These settings should only be modified by a user after they are initialized
	err := populateConfigOverrideConfigMap(c.context, c.Namespace, c.ownerInfo, c.clusterMetadata)
	if err != nil {
		return errors.Wrap(err, "failed to populate config override config map")
	}
	c.ClusterInfo.SetName(c.namespacedName.Name)

	// Execute actions before the monitors are up and running, if needed during upgrades.
	// These actions would be skipped in a new cluster.
	logger.Debug("monitors are about to reconcile, executing pre actions")
	err = c.preMonStartupActions(cephVersion)
	if err != nil {
		return errors.Wrap(err, "failed to execute actions before reconciling the ceph monitors")
	}

	// Start the mon pods
	controller.UpdateCondition(c.ClusterInfo.Context, c.context, c.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, "Configuring Ceph Mons")
	clusterInfo, err := c.mons.Start(c.ClusterInfo, rookImage, cephVersion, *c.Spec)
	if err != nil {
		return errors.Wrap(err, "failed to start ceph monitors")
	}
	clusterInfo.OwnerInfo = c.ownerInfo
	clusterInfo.SetName(c.namespacedName.Name)
	clusterInfo.Context = c.ClusterInfo.Context
	c.ClusterInfo = clusterInfo
	c.ClusterInfo.NetworkSpec = c.Spec.Network

	// The cluster Identity must be established at this point
	if err := c.ClusterInfo.IsInitialized(); err != nil {
		return errors.Wrap(err, "the cluster identity was not established")
	}

	if c.ClusterInfo.Context.Err() != nil {
		return c.ClusterInfo.Context.Err()
	}

	// Execute actions after the monitors are up and running
	logger.Debug("monitors are up and running, executing post actions")
	err = c.postMonStartupActions()
	if err != nil {
		return errors.Wrap(err, "failed to execute post actions after all the ceph monitors started")
	}

	// Start Ceph manager
	controller.UpdateCondition(c.ClusterInfo.Context, c.context, c.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, "Configuring Ceph Mgr(s)")
	mgrs := mgr.New(c.context, c.ClusterInfo, *c.Spec, rookImage)
	err = mgrs.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start ceph mgr")
	}

	// Start the OSDs
	controller.UpdateCondition(c.ClusterInfo.Context, c.context, c.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, "Configuring Ceph OSDs")
	osds := osd.New(c.context, c.ClusterInfo, *c.Spec, rookImage)
	err = osds.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start ceph osds")
	}

	// If a stretch cluster, enable the arbiter after the OSDs are created with the CRUSH map
	if c.Spec.IsStretchCluster() {
		if err := c.mons.ConfigureArbiter(); err != nil {
			return errors.Wrap(err, "failed to configure stretch arbiter")
		}
	}

	logger.Infof("done reconciling ceph cluster in namespace %q", c.Namespace)

	// We should be done updating by now
	if c.isUpgrade {
		c.printOverallCephVersion()

		// reset the isUpgrade flag
		c.isUpgrade = false
	}

	return nil
}

func (c *ClusterController) initializeCluster(cluster *cluster) error {
	// Check if the dataDirHostPath is located in the disallowed paths list
	cleanDataDirHostPath := path.Clean(cluster.Spec.DataDirHostPath)
	for _, b := range disallowedHostDirectories {
		if cleanDataDirHostPath == b {
			logger.Errorf("dataDirHostPath (given: %q) must not be used, conflicts with %q internal path", cluster.Spec.DataDirHostPath, b)
			return nil
		}
	}

	// Depending on the cluster type choose the correct orchestration
	if cluster.Spec.External.Enable {
		err := c.configureExternalCephCluster(cluster)
		if err != nil {
			controller.UpdateCondition(c.OpManagerCtx, c.context, c.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionFalse, cephv1.ClusterProgressingReason, err.Error())
			return errors.Wrap(err, "failed to configure external ceph cluster")
		}
	} else {
		clusterInfo, _, _, err := controller.LoadClusterInfo(c.context, c.OpManagerCtx, cluster.Namespace, cluster.Spec)
		if err != nil {
			if errors.Is(err, controller.ClusterInfoNoClusterNoSecret) {
				logger.Info("clusterInfo not yet found, must be a new cluster.")
			} else {
				return errors.Wrap(err, "failed to load cluster info")
			}
		} else {
			clusterInfo.OwnerInfo = cluster.ownerInfo
			clusterInfo.SetName(c.namespacedName.Name)
			clusterInfo.RequireMsgr2 = cluster.Spec.RequireMsgr2()
			cluster.ClusterInfo = clusterInfo
		}
		// If the local cluster has already been configured, immediately start monitoring the cluster.
		// Test if the cluster has already been configured if the mgr deployment has been created.
		// If the mgr does not exist, the mons have never been verified to be in quorum.
		opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, mgr.AppName)}
		mgrDeployments, err := c.context.Clientset.AppsV1().Deployments(cluster.Namespace).List(c.OpManagerCtx, opts)
		if err == nil && len(mgrDeployments.Items) > 0 && cluster.ClusterInfo != nil {
			c.configureCephMonitoring(cluster, clusterInfo)
		}

		err = c.configureLocalCephCluster(cluster)
		if err != nil {
			controller.UpdateCondition(c.OpManagerCtx, c.context, c.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionFalse, cephv1.ClusterProgressingReason, err.Error())
			return errors.Wrap(err, "failed to configure local ceph cluster")
		}

		// Asynchronously report the telemetry to allow another reconcile to proceed if needed
		go cluster.reportTelemetry()
	}

	// Populate ClusterInfo with the last value
	cluster.mons.ClusterInfo = cluster.ClusterInfo
	cluster.mons.ClusterInfo.SetName(c.namespacedName.Name)

	// Start the monitoring if not already started
	c.configureCephMonitoring(cluster, cluster.ClusterInfo)

	return nil
}

func (c *ClusterController) configureLocalCephCluster(cluster *cluster) error {
	// Cluster Spec validation
	err := preClusterStartValidation(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to perform validation before cluster creation")
	}

	// Run image validation job
	controller.UpdateCondition(c.OpManagerCtx, c.context, c.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, "Detecting Ceph version")
	cephVersion, isUpgrade, err := c.detectAndValidateCephVersion(cluster)
	if err != nil {
		return errors.Wrap(err, "failed the ceph version check")
	}
	// Set the value of isUpgrade based on the image discovery done by detectAndValidateCephVersion()
	cluster.isUpgrade = isUpgrade

	if cluster.Spec.IsStretchCluster() {
		stretchVersion := cephver.CephVersion{Major: 16, Minor: 2, Build: 5}
		if !cephVersion.IsAtLeast(stretchVersion) {
			return errors.Errorf("stretch clusters minimum ceph version is %q, but is running %s", stretchVersion.String(), cephVersion.String())
		}
	}

	controller.UpdateCondition(c.OpManagerCtx, c.context, c.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, "Configuring the Ceph cluster")

	cluster.ClusterInfo.Context = c.OpManagerCtx
	// Run the orchestration
	err = cluster.reconcileCephDaemons(c.rookImage, *cephVersion)
	if err != nil {
		return errors.Wrap(err, "failed to create cluster")
	}

	// Set the condition to the cluster object
	controller.UpdateCondition(c.OpManagerCtx, c.context, c.namespacedName, cluster.observedGeneration, cephv1.ConditionReady, v1.ConditionTrue, cephv1.ClusterCreatedReason, "Cluster created successfully")
	return nil
}

// Validate the cluster Specs
func preClusterStartValidation(cluster *cluster) error {
	if cluster.Spec.Mon.Count == 0 {
		logger.Warningf("mon count should be at least 1, will use default value of %d", mon.DefaultMonCount)
		cluster.Spec.Mon.Count = mon.DefaultMonCount
	}
	if !cluster.Spec.Mon.AllowMultiplePerNode {
		// Check that there are enough nodes to have a chance of starting the requested number of mons
		nodes, err := cluster.context.Clientset.CoreV1().Nodes().List(cluster.ClusterInfo.Context, metav1.ListOptions{})
		if err == nil && len(nodes.Items) < cluster.Spec.Mon.Count {
			return errors.Errorf("cannot start %d mons on %d node(s) when allowMultiplePerNode is false", cluster.Spec.Mon.Count, len(nodes.Items))
		}
	}
	if err := validateStretchCluster(cluster); err != nil {
		return err
	}
	if cluster.Spec.Network.IsMultus() {
		_, isPublic := cluster.Spec.Network.Selectors[config.PublicNetworkSelectorKeyName]
		_, isCluster := cluster.Spec.Network.Selectors[config.ClusterNetworkSelectorKeyName]
		if !isPublic && !isCluster {
			return errors.New("both network selector values for public and cluster selector cannot be empty for multus provider")
		}

		for _, selector := range config.NetworkSelectors {
			// If one selector is empty, we continue
			// This means a single interface is used both public and cluster network
			if _, ok := cluster.Spec.Network.Selectors[selector]; !ok {
				continue
			}

			multusNamespace, nad := config.GetMultusNamespace(cluster.Spec.Network.Selectors[selector])
			if multusNamespace == "" {
				multusNamespace = cluster.Namespace
			}

			// Get network attachment definition
			_, err := cluster.context.NetworkClient.NetworkAttachmentDefinitions(multusNamespace).Get(cluster.ClusterInfo.Context, nad, metav1.GetOptions{})
			if err != nil {
				if kerrors.IsNotFound(err) {
					return errors.Wrapf(err, "specified network attachment definition for selector %q does not exist", selector)
				}
				return errors.Wrapf(err, "failed to fetch network attachment definition for selector %q", selector)
			}
		}
	}

	// Validate on-PVC cluster encryption KMS settings
	if cluster.Spec.Storage.IsOnPVCEncrypted() && cluster.Spec.Security.KeyManagementService.IsEnabled() {
		// Validate the KMS details
		err := kms.ValidateConnectionDetails(cluster.ClusterInfo.Context, cluster.context, &cluster.Spec.Security.KeyManagementService, cluster.Namespace)
		if err != nil {
			return errors.Wrap(err, "failed to validate kms connection details")
		}
	}

	logger.Debug("cluster spec successfully validated")
	return nil
}

func validateStretchCluster(cluster *cluster) error {
	if !cluster.Spec.IsStretchCluster() {
		return nil
	}
	if len(cluster.Spec.Mon.StretchCluster.Zones) != 3 {
		return errors.Errorf("expecting exactly three zones for the stretch cluster, but found %d", len(cluster.Spec.Mon.StretchCluster.Zones))
	}
	if cluster.Spec.Mon.Count != 3 && cluster.Spec.Mon.Count != 5 {
		return errors.Errorf("invalid number of mons %d for a stretch cluster, expecting 5 (recommended) or 3 (minimal)", cluster.Spec.Mon.Count)
	}
	arbitersFound := 0
	for _, zone := range cluster.Spec.Mon.StretchCluster.Zones {
		if zone.Arbiter {
			arbitersFound++
		}
		if zone.Name == "" {
			return errors.New("missing zone name for the stretch cluster")
		}
	}
	if arbitersFound != 1 {
		return errors.Errorf("expecting to find exactly one arbiter zone, but found %d", arbitersFound)
	}
	return nil
}

func extractExitCode(err error) (int, bool) {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		return exitErr.ExitCode(), true
	}
	return 0, false
}

func (c *cluster) createCrushRoot(newRoot string) error {
	args := []string{"osd", "crush", "add-bucket", newRoot, "root"}
	cephCmd := client.NewCephCommand(c.context, c.ClusterInfo, args)
	_, err := cephCmd.Run()
	if err != nil {
		// returns zero if the bucket exists already, so any error is fatal
		return errors.Wrap(err, "failed to create CRUSH root")
	}

	return nil
}

func (c *cluster) replaceDefaultReplicationRule(newRoot string) error {
	args := []string{"osd", "crush", "rule", "rm", "replicated_rule"}
	cephCmd := client.NewCephCommand(c.context, c.ClusterInfo, args)
	_, err := cephCmd.Run()
	if err != nil {
		if code, ok := extractExitCode(err); ok && code == int(syscall.EBUSY) {
			// we do not want to delete the replicated_rule if it’s in use,
			// and we also do not care much. There are two possible causes:
			// - the user has created this rule with the non-default CRUSH
			//   root manually
			// - the user is using this rule despite the rule using the default
			//   CRUSH root
			// in both cases, we cannot do anything about it either way and
			// we’ll assume that the user knows what they’re doing.
			logger.Warning("replicated_rule is in use, not replaced")
			return nil
		}
		// the error does not refer to EBUSY -> return as error
		return errors.Wrap(err, "failed to remove default replicated_rule")
	}

	args = []string{
		"osd", "crush", "rule", "create-replicated",
		"replicated_rule", newRoot, "host",
	}
	cephCmd = client.NewCephCommand(c.context, c.ClusterInfo, args)
	_, err = cephCmd.Run()
	if err != nil {
		// returns zero if the rule exists already, so any error is fatal
		return errors.Wrap(err, "failed to create new default replicated_rule")
	}

	return nil
}

func (c *cluster) removeDefaultCrushRoot() error {
	args := []string{"osd", "crush", "rm", "default"}
	cephCmd := client.NewCephCommand(c.context, c.ClusterInfo, args)
	_, err := cephCmd.Run()
	if err != nil {
		if code, ok := extractExitCode(err); ok {
			if code == int(syscall.ENOTEMPTY) || code == int(syscall.EBUSY) {
				// we do not want to delete the default node if it’s in use,
				// and we also do not care much. There are two more causes here:
				// - a (non-root?) CRUSH node with the default label was created
				//   automatically, e.g. from topology labels, and OSDs (or sub
				//   nodes) have been placed in there. In this case, the node
				//   obviously needs to be preserved.
				// - the root=default CRUSH node is in use by a non-default
				//   CRUSH rule
				// - OSDs or subnodes have been placed under the root=default
				//   CRUSH node
				//
				// in all cases, we cannot do anything about it either way and
				// we’ll assume that the user knows what they’re doing.
				logger.Debug("default is not empty or is still in use, not removed")
				return nil
			}
		}
		// the error does not refer to EBUSY or ENOTEMPTY -> return as error
		return errors.Wrap(err, "failed to remove CRUSH node 'default'")
	}
	return nil
}

// Remove the default root=default and replicated_rule CRUSH objects which are created by Ceph on initial startup.
// Those objects may interfere with the normal operation of the cluster.
// Note that errors which indicate that the objects are in use are ignored and the objects will continue to exist in that case.
func (c *cluster) replaceDefaultCrushMap(newRoot string) (err error) {
	logger.Info("creating new CRUSH root if it does not exist")
	err = c.createCrushRoot(newRoot)
	if err != nil {
		return errors.Wrap(err, "failed to create CRUSH root")
	}

	logger.Info("replacing default replicated_rule CRUSH rule for use of non-default CRUSH root")
	err = c.replaceDefaultReplicationRule(newRoot)
	if err != nil {
		return errors.Wrap(err, "failed to replace default rule")
	}

	logger.Info("replacing default CRUSH node if applicable")
	err = c.removeDefaultCrushRoot()
	if err != nil {
		return errors.Wrap(err, "failed to remove default CRUSH root")
	}

	return nil
}

// preMonStartupActions is a collection of actions to run before the monitors are reconciled.
func (c *cluster) preMonStartupActions(cephVersion cephver.CephVersion) error {
	// Disable the mds sanity checks for the mons due to a ceph upgrade issue
	// for the mds to Pacific if 16.2.7 or greater. We keep it more general for any
	// Pacific upgrade greater than 16.2.7 in case they skip upgrading directly to 16.2.7.
	if c.isUpgrade && cephVersion.IsPacific() && cephVersion.IsAtLeast(cephver.CephVersion{Major: 16, Minor: 2, Extra: 7}) {
		if err := c.skipMDSSanityChecks(true); err != nil {
			// If there is an error, just print it and continue. Likely there is not a
			// negative consequence of continuing since several complex conditions must exist to hit
			// the upgrade issue where the sanity checks need to be disabled.
			logger.Warningf("failed to disable the mon_mds_skip_sanity. %v", err)
		}
	}
	return nil
}

func (c *cluster) skipMDSSanityChecks(skip bool) error {
	// In a running cluster disable the mds skip sanity setting during upgrades.
	monStore := config.GetMonStore(c.context, c.ClusterInfo)
	if skip {
		if err := monStore.Set("mon", "mon_mds_skip_sanity", "1"); err != nil {
			return err
		}
	} else {
		if err := monStore.Delete("mon", "mon_mds_skip_sanity"); err != nil {
			return err
		}
	}
	return nil
}

// postMonStartupActions is a collection of actions to run once the monitors are up and running
// It gets executed right after the main mon Start() method
// Basically, it is executed between the monitors and the manager sequence
func (c *cluster) postMonStartupActions() error {
	// Create CSI Kubernetes Secrets
	err := csi.CreateCSISecrets(c.context, c.ClusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to create csi kubernetes secrets")
	}

	// Create crash collector Kubernetes Secret
	err = nodedaemon.CreateCrashCollectorSecret(c.context, c.ClusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to create crash collector kubernetes secret")
	}

	// Always ensure the skip mds sanity checks setting is cleared, for all Pacific deployments
	if c.ClusterInfo.CephVersion.IsPacific() {
		if err := c.skipMDSSanityChecks(false); err != nil {
			// If there is an error, just print it and continue. We can just try again
			// at the next reconcile.
			logger.Warningf("failed to re-enable the mon_mds_skip_sanity. %v", err)
		}
	}

	if err := c.configureMsgr2(); err != nil {
		return errors.Wrap(err, "failed to configure msgr2")
	}

	crushRoot := client.GetCrushRootFromSpec(c.Spec)
	if crushRoot != "default" {
		// Remove the root=default and replicated_rule which are created by
		// default. Note that RemoveDefaultCrushMap ignores some types of errors
		// internally
		if err := c.replaceDefaultCrushMap(crushRoot); err != nil {
			return errors.Wrap(err, "failed to remove default CRUSH map")
		}
	}

	// Create cluster-wide RBD bootstrap peer token
	_, err = controller.CreateBootstrapPeerSecret(c.context, c.ClusterInfo, &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: c.namespacedName.Name, Namespace: c.Namespace}}, c.ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to create cluster rbd bootstrap peer token")
	}

	return nil
}

func (c *cluster) reportTelemetry() {
	// In the corner case that reconciles are started in quick succession and the telemetry
	// hasn't had a chance to complete yet from a previous reconcile, simply allow
	// a single goroutine to report telemetry at a time.
	telemetryMutex.Lock()
	defer telemetryMutex.Unlock()

	// Identify this as a rook cluster for Ceph telemetry by setting the Rook version.
	logger.Info("reporting cluster telemetry")
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.RookVersionKey, rookversion.Version)

	// Report the K8s version
	serverVersion, err := c.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		logger.Warningf("failed to report the K8s server version. %v", err)
	} else {
		telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.K8sVersionKey, serverVersion.String())
	}

	// Report the CSI version if it has been detected
	if telemetry.CSIVersion != "" {
		telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.CSIVersionKey, telemetry.CSIVersion)
	}

	// Report the max mon id
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.MonMaxIDKey, strconv.Itoa(c.mons.MaxMonID()))

	// Report the telemetry for mon settings
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.MonCountKey, strconv.Itoa(c.Spec.Mon.Count))
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.MonAllowMultiplePerNodeKey, strconv.FormatBool(c.Spec.Mon.AllowMultiplePerNode))
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.MonPVCEnabledKey, strconv.FormatBool(c.Spec.Mon.VolumeClaimTemplate != nil))
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.MonStretchEnabledKey, strconv.FormatBool(c.Spec.IsStretchCluster()))

	// Set the telemetry for device sets
	deviceSets := 0
	portableDeviceSets := 0
	nonportableDeviceSets := 0
	for _, deviceSet := range c.Spec.Storage.StorageClassDeviceSets {
		deviceSets++
		if deviceSet.Portable {
			portableDeviceSets++
		} else {
			nonportableDeviceSets++
		}
	}
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.DeviceSetTotalKey, strconv.Itoa(deviceSets))
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.DeviceSetPortableKey, strconv.Itoa(portableDeviceSets))
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.DeviceSetNonPortableKey, strconv.Itoa(nonportableDeviceSets))

	// Set the telemetry for network settings
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.NetworkProviderKey, c.Spec.Network.Provider)

	// Set the telemetry for external cluster settings
	telemetry.ReportKeyValue(c.context, c.ClusterInfo, telemetry.ExternalModeEnabledKey, strconv.FormatBool(c.Spec.External.Enable))
}

func (c *cluster) configureMsgr2() error {
	encryptionSetting := "secure"
	defaultRBDMapOptions := "ms_mode=secure"
	encryptionGlobalConfigSettings := map[string]string{
		"ms_cluster_mode":         encryptionSetting,
		"ms_service_mode":         encryptionSetting,
		"ms_client_mode":          encryptionSetting,
		"rbd_default_map_options": defaultRBDMapOptions,
	}
	monStore := config.GetMonStore(c.context, c.ClusterInfo)

	// If the user has disabled encryption, remove the encryption settings
	if c.Spec.Network.Connections == nil || c.Spec.Network.Connections.Encryption == nil || !c.Spec.Network.Connections.Encryption.Enabled {
		encryptionConfig := []config.Option{}
		for k := range encryptionGlobalConfigSettings {
			encryptionConfig = append(encryptionConfig, config.Option{Who: "global", Option: k})
		}
		if err := monStore.DeleteAll(encryptionConfig...); err != nil {
			return errors.Wrap(err, "failed to delete msgr2 encryption settings")
		}
	} else {
		logger.Infof("setting msgr2 encryption mode to %q", encryptionSetting)
		if err := monStore.SetAll("global", encryptionGlobalConfigSettings); err != nil {
			return err
		}
	}

	// Set network compression
	if c.ClusterInfo.CephVersion.IsAtLeastQuincy() {
		if c.Spec.Network.Connections == nil || c.Spec.Network.Connections.Compression == nil || !c.Spec.Network.Connections.Compression.Enabled {
			encryptionConfig := []config.Option{
				{Who: "global", Option: "ms_osd_compress_mode"},
			}
			if err := monStore.DeleteAll(encryptionConfig...); err != nil {
				return errors.Wrap(err, "failed to delete msgr2 compression settings")
			}
		} else {
			globalConfigSettings := map[string]string{
				"ms_osd_compress_mode": "force",
			}
			logger.Infof("setting msgr2 compression mode to %q", "force")
			if err := monStore.SetAll("global", globalConfigSettings); err != nil {
				return err
			}
		}

	} else {
		logger.Warningf("network compression requires Ceph Quincy (v17) or newer, skipping for current ceph %q", c.ClusterInfo.CephVersion.String())
	}

	return nil
}
