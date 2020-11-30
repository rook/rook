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
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/errors"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/cluster/crash"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	detectVersionName = "rook-ceph-detect-version"
)

type cluster struct {
	ClusterInfo        *client.ClusterInfo
	context            *clusterd.Context
	Namespace          string
	Spec               *cephv1.ClusterSpec
	crdName            string
	mons               *mon.Cluster
	stopCh             chan struct{}
	closedStopCh       bool
	ownerInfo          *client.OwnerInfo
	isUpgrade          bool
	watchersActivated  bool
	monitoringChannels map[string]*clusterHealth
}

type clusterHealth struct {
	stopChan          chan struct{}
	monitoringRunning bool
}

func newCluster(c *cephv1.CephCluster, context *clusterd.Context, csiMutex *sync.Mutex, ownerInfo *client.OwnerInfo) *cluster {
	return &cluster{
		// at this phase of the cluster creation process, the identity components of the cluster are
		// not yet established. we reserve this struct which is filled in as soon as the cluster's
		// identity can be established.
		ClusterInfo:        client.AdminClusterInfo(c.Namespace),
		Namespace:          c.Namespace,
		Spec:               &c.Spec,
		context:            context,
		crdName:            c.Name,
		monitoringChannels: make(map[string]*clusterHealth),
		stopCh:             make(chan struct{}),
		ownerInfo:          ownerInfo,
		mons:               mon.New(context, c.Namespace, c.Spec, ownerInfo, csiMutex),
	}
}

func (c *cluster) doOrchestration(rookImage string, cephVersion cephver.CephVersion, spec *cephv1.ClusterSpec) error {
	// Create a configmap for overriding ceph config settings
	// These settings should only be modified by a user after they are initialized
	err := populateConfigOverrideConfigMap(c.context, c.Namespace, c.ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to populate config override config map")
	}

	// Start the mon pods
	clusterInfo, err := c.mons.Start(c.ClusterInfo, rookImage, cephVersion, *c.Spec)
	if err != nil {
		return errors.Wrap(err, "failed to start ceph monitors")
	}
	clusterInfo.OwnerInfo = *c.ownerInfo
	clusterInfo.SetName(c.crdName)
	c.ClusterInfo = clusterInfo

	// The cluster Identity must be established at this point
	if !c.ClusterInfo.IsInitialized(true) {
		return errors.New("the cluster identity was not established")
	}

	// Check whether we need to cancel the orchestration
	if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
		return err
	}

	// Execute actions after the monitors are up and running
	logger.Debug("monitors are up and running, executing post actions")
	err = c.postMonStartupActions()
	if err != nil {
		return errors.Wrap(err, "failed to execute post actions after all the ceph monitors started")
	}

	// If this is an upgrade, notify all the child controllers
	if c.isUpgrade {
		logger.Info("upgrade in progress, notifying child CRs")
		err := c.notifyChildControllerOfUpgrade()
		if err != nil {
			return errors.Wrap(err, "failed to notify child CRs of upgrade")
		}
	}

	// Start Ceph manager
	mgrs := mgr.New(c.context, c.ClusterInfo, *spec, rookImage)
	err = mgrs.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start ceph mgr")
	}

	// Start the OSDs
	osds := osd.New(c.context, c.ClusterInfo, *spec, rookImage)
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

func (c *ClusterController) initializeCluster(cluster *cluster, clusterObj *cephv1.CephCluster) error {
	ctx := context.TODO()
	cluster.Spec = &clusterObj.Spec

	// Check if the dataDirHostPath is located in the disallowed paths list
	cleanDataDirHostPath := path.Clean(cluster.Spec.DataDirHostPath)
	for _, b := range disallowedHostDirectories {
		if cleanDataDirHostPath == b {
			logger.Errorf("dataDirHostPath (given: %q) must not be used, conflicts with %q internal path", cluster.Spec.DataDirHostPath, b)
			return nil
		}
	}

	clusterInfo, _, _, err := mon.LoadClusterInfo(c.context, cluster.Namespace)
	if err != nil {
		logger.Infof("clusterInfo not yet found, must be a new cluster")
	} else {
		clusterInfo.OwnerInfo = *cluster.ownerInfo
		clusterInfo.SetName(cluster.crdName)
		cluster.ClusterInfo = clusterInfo
	}

	// Depending on the cluster type choose the correct orchestation
	if cluster.Spec.External.Enable {
		err := c.configureExternalCephCluster(cluster)
		if err != nil {
			config.ConditionExport(c.context, c.namespacedName, cephv1.ConditionFailure, v1.ConditionTrue, "ClusterFailure", "Failed to configure external ceph cluster")
			return errors.Wrap(err, "failed to configure external ceph cluster")
		}
	} else {
		// If the local cluster has already been configured, immediately start monitoring the cluster.
		// Test if the cluster has already been configured if the mgr deployment has been created.
		// If the mgr does not exist, the mons have never been verified to be in quorum.
		opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, mgr.AppName)}
		mgrDeployments, err := c.context.Clientset.AppsV1().Deployments(cluster.Namespace).List(ctx, opts)
		if err == nil && len(mgrDeployments.Items) > 0 && cluster.ClusterInfo != nil {
			c.configureCephMonitoring(cluster, clusterInfo)
		}

		err = c.configureLocalCephCluster(cluster)
		if err != nil {
			return errors.Wrap(err, "failed to configure local ceph cluster")
		}
	}

	// Populate ClusterInfo with the last value
	cluster.mons.ClusterInfo = cluster.ClusterInfo

	// Start the monitoring if not already started
	c.configureCephMonitoring(cluster, cluster.ClusterInfo)
	return nil
}

func (c *ClusterController) configureLocalCephCluster(cluster *cluster) error {
	// Cluster Spec validation
	err := c.preClusterStartValidation(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to perform validation before cluster creation")
	}

	// Pass down the client to interact with Kubernetes objects
	// This will be used later down by spec code to create objects like deployment, services etc
	cluster.context.Client = c.client

	// Run image validation job
	cephVersion, isUpgrade, err := c.detectAndValidateCephVersion(cluster)
	if err != nil {
		return errors.Wrap(err, "failed the ceph version check")
	}

	// Set the value of isUpgrade based on the image discovery done by detectAndValidateCephVersion()
	cluster.isUpgrade = isUpgrade

	// Set the condition to the cluster object
	message := config.CheckConditionReady(c.context, c.namespacedName)
	config.ConditionExport(c.context, c.namespacedName, cephv1.ConditionProgressing, v1.ConditionTrue, "ClusterProgressing", message)

	// Run the orchestration
	err = cluster.doOrchestration(c.rookImage, *cephVersion, cluster.Spec)
	if err != nil {
		config.ConditionExport(c.context, c.namespacedName, cephv1.ConditionFailure, v1.ConditionTrue, "ClusterFailure", "Failed to create cluster")
		return errors.Wrap(err, "failed to create cluster")
	}

	// Set the condition to the cluster object
	config.ConditionExport(c.context, c.namespacedName, cephv1.ConditionReady, v1.ConditionTrue, "ClusterCreated", "Cluster created successfully")

	return nil
}

func (c *cluster) notifyChildControllerOfUpgrade() error {
	ctx := context.TODO()
	version := strings.Replace(c.ClusterInfo.CephVersion.String(), " ", "-", -1)

	// List all child controllers
	cephFilesystems, err := c.context.RookClientset.CephV1().CephFilesystems(c.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list ceph filesystem CRs")
	}
	for _, cephFilesystem := range cephFilesystems.Items {
		if cephFilesystem.Labels == nil {
			cephFilesystem.Labels = map[string]string{}
		}
		cephFilesystem.Labels["ceph_version"] = version
		localcephFilesystem := cephFilesystem
		_, err := c.context.RookClientset.CephV1().CephFilesystems(c.Namespace).Update(ctx, &localcephFilesystem, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update ceph filesystem CR %q with new label", cephFilesystem.Name)
		}
	}

	cephObjectStores, err := c.context.RookClientset.CephV1().CephObjectStores(c.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list ceph object store CRs")
	}
	for _, cephObjectStore := range cephObjectStores.Items {
		if cephObjectStore.Labels == nil {
			cephObjectStore.Labels = map[string]string{}
		}
		cephObjectStore.Labels["ceph_version"] = version
		localcephObjectStore := cephObjectStore
		_, err := c.context.RookClientset.CephV1().CephObjectStores(c.Namespace).Update(ctx, &localcephObjectStore, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update ceph object store CR %q with new label", cephObjectStore.Name)
		}
	}

	cephNFSes, err := c.context.RookClientset.CephV1().CephNFSes(c.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list ceph nfs CRs")
	}
	for _, cephNFS := range cephNFSes.Items {
		if cephNFS.Labels == nil {
			cephNFS.Labels = map[string]string{}
		}
		cephNFS.Labels["ceph_version"] = version
		localcephNFS := cephNFS
		_, err := c.context.RookClientset.CephV1().CephNFSes(c.Namespace).Update(ctx, &localcephNFS, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update ceph nfs CR %q with new label", cephNFS.Name)
		}
	}

	return nil
}

// Validate the cluster Specs
func (c *ClusterController) preClusterStartValidation(cluster *cluster) error {
	ctx := context.TODO()
	if cluster.Spec.Mon.Count == 0 {
		logger.Warningf("mon count should be at least 1, will use default value of %d", mon.DefaultMonCount)
		cluster.Spec.Mon.Count = mon.DefaultMonCount
	}
	if cluster.Spec.Mon.Count%2 == 0 {
		return errors.Errorf("mon count %d cannot be even, must be odd to support a healthy quorum", cluster.Spec.Mon.Count)
	}
	if !cluster.Spec.Mon.AllowMultiplePerNode {
		// Check that there are enough nodes to have a chance of starting the requested number of mons
		nodes, err := c.context.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil && len(nodes.Items) < cluster.Spec.Mon.Count {
			return errors.Errorf("cannot start %d mons on %d node(s) when allowMultiplePerNode is false", cluster.Spec.Mon.Count, len(nodes.Items))
		}
	}
	if len(cluster.Spec.Storage.Directories) != 0 {
		logger.Warning("running osds on directory is not supported anymore, use devices instead.")
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
			_, err := c.context.NetworkClient.NetworkAttachmentDefinitions(multusNamespace).Get(ctx, nad, metav1.GetOptions{})
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
		err := kms.ValidateConnectionDetails(c.context, cluster.Spec, cluster.Namespace)
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
	err = crash.CreateCrashCollectorSecret(c.context, c.ClusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to create crash collector kubernetes secret")
	}

	// Enable Ceph messenger 2 protocol on Nautilus
	if err := client.EnableMessenger2(c.context, c.ClusterInfo); err != nil {
		return errors.Wrap(err, "failed to enable Ceph messenger version 2")
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

	return nil
}
