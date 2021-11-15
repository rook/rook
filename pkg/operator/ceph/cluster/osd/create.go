/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package osd

import (
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	kms "github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	osdconfig "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/version"
)

type createConfig struct {
	cluster                  *Cluster
	provisionConfig          *provisionConfig
	awaitingStatusConfigMaps sets.String    // These status configmaps were created for OSD prepare jobs
	finishedStatusConfigMaps sets.String    // Status configmaps are added here as provisioning is completed for them
	deployments              *existenceList // these OSDs have existing deployments
}

// allow overriding these functions for unit tests
var (
	createDaemonOnNodeFunc = createDaemonOnNode
	createDaemonOnPVCFunc  = createDaemonOnPVC

	updateConditionFunc = opcontroller.UpdateCondition
)

func (c *Cluster) newCreateConfig(
	provisionConfig *provisionConfig,
	awaitingStatusConfigMaps sets.String,
	deployments *existenceList,
) *createConfig {
	if awaitingStatusConfigMaps == nil {
		awaitingStatusConfigMaps = sets.NewString()
	}
	return &createConfig{
		c,
		provisionConfig,
		awaitingStatusConfigMaps,
		sets.NewString(),
		deployments,
	}
}

func (c *createConfig) progress() (completed, initial int) {
	return c.finishedStatusConfigMaps.Len(), c.awaitingStatusConfigMaps.Len()
}

func (c *createConfig) doneCreating() bool {
	return c.awaitingStatusConfigMaps.Len() == c.finishedStatusConfigMaps.Len()
}

func (c *createConfig) createNewOSDsFromStatus(
	status *OrchestrationStatus,
	nodeOrPVCName string,
	errs *provisionErrors,
) {
	if !c.awaitingStatusConfigMaps.Has(statusConfigMapName(nodeOrPVCName)) {
		// If there is a dangling OSD prepare configmap from another reconcile, don't process it
		logger.Infof("not creating deployments for OSD prepare results found in ConfigMap %q which was not created for the latest storage spec", statusConfigMapName(nodeOrPVCName))
		return
	}

	if c.finishedStatusConfigMaps.Has(statusConfigMapName(nodeOrPVCName)) {
		// If we have already processed this configmap, don't process it again
		logger.Infof("not creating deployments for OSD prepare results found in ConfigMap %q which was already processed", statusConfigMapName(nodeOrPVCName))
		return
	}

	for _, osd := range status.OSDs {
		if c.deployments.Exists(osd.ID) {
			// This OSD will be handled by the updater
			logger.Debugf("not creating deployment for OSD %d which already exists", osd.ID)
			continue
		}
		if status.PvcBackedOSD {
			logger.Infof("creating OSD %d on PVC %q", osd.ID, nodeOrPVCName)
			err := createDaemonOnPVCFunc(c.cluster, osd, nodeOrPVCName, c.provisionConfig)
			if err != nil {
				errs.addError("%v", errors.Wrapf(err, "failed to create OSD %d on PVC %q", osd.ID, nodeOrPVCName))
			}
		} else {
			logger.Infof("creating OSD %d on node %q", osd.ID, nodeOrPVCName)
			err := createDaemonOnNodeFunc(c.cluster, osd, nodeOrPVCName, c.provisionConfig)
			if err != nil {
				errs.addError("%v", errors.Wrapf(err, "failed to create OSD %d on node %q", osd.ID, nodeOrPVCName))
			}
		}
	}

	c.doneWithStatus(nodeOrPVCName)
}

// Call this if createNewOSDsFromStatus() isn't going to be called (like for a failed status)
func (c *createConfig) doneWithStatus(nodeOrPVCName string) {
	c.finishedStatusConfigMaps.Insert(statusConfigMapName(nodeOrPVCName))
}

// Returns a set of all the awaitingStatusConfigMaps that will be updated by provisioning jobs.
// Returns error only if the calling function should halt all OSD provisioning. Non-halting errors
// are added to provisionErrors.
//
// Creation of prepare jobs is most directly related to creating new OSDs. And we want to keep all
// usage of awaitingStatusConfigMaps in this file.
func (c *Cluster) startProvisioningOverPVCs(config *provisionConfig, errs *provisionErrors) (sets.String, error) {
	// Parsing storageClassDeviceSets and parsing it to volume sources
	c.prepareStorageClassDeviceSets(errs)

	// no valid VolumeSource is ready to run an osd
	if len(c.deviceSets) == 0 {
		logger.Info("no storageClassDeviceSets defined to configure OSDs on PVCs")
		return sets.NewString(), nil
	}

	// Check k8s version
	k8sVersion, err := k8sutil.GetK8SVersion(c.context.Clientset)
	if err != nil {
		errs.addError("failed to provision OSDs on PVCs. user has specified storageClassDeviceSets, but the Kubernetes version could not be determined. minimum Kubernetes version required: 1.13.0. %v", err)
		return sets.NewString(), nil
	}
	if !k8sVersion.AtLeast(version.MustParseSemantic("v1.13.0")) {
		errs.addError("failed to provision OSDs on PVCs. user has specified storageClassDeviceSets, but the Kubernetes version is not supported. user must update Kubernetes version. minimum Kubernetes version required: 1.13.0. version detected: %s", k8sVersion.String())
		return sets.NewString(), nil
	}

	existingDeployments, err := c.getExistingOSDDeploymentsOnPVCs()
	if err != nil {
		errs.addError("failed to provision OSDs on PVCs. failed to query existing OSD deployments on PVCs. %v", err)
		return sets.NewString(), nil
	}

	awaitingStatusConfigMaps := sets.NewString()
	for _, volume := range c.deviceSets {
		if c.clusterInfo.Context.Err() != nil {
			return awaitingStatusConfigMaps, c.clusterInfo.Context.Err()
		}
		dataSource, dataOK := volume.PVCSources[bluestorePVCData]

		// The data PVC template is required.
		if !dataOK {
			errs.addError("failed to create OSD provisioner for storageClassDeviceSet %q. missing the data template", volume.Name)
			continue
		}

		metadataSource, metadataOK := volume.PVCSources[bluestorePVCMetadata]
		if metadataOK {
			logger.Infof("OSD will have its main bluestore block on %q and its metadata device on %q", dataSource.ClaimName, metadataSource.ClaimName)
		} else {
			logger.Infof("OSD will have its main bluestore block on %q", dataSource.ClaimName)
		}

		walSource, walOK := volume.PVCSources[bluestorePVCWal]
		if walOK {
			logger.Infof("OSD will have its wal device on %q", walSource.ClaimName)
		}

		osdProps := osdProperties{
			crushHostname:    dataSource.ClaimName,
			pvc:              dataSource,
			metadataPVC:      metadataSource,
			walPVC:           walSource,
			resources:        volume.Resources,
			placement:        volume.Placement,
			preparePlacement: volume.PreparePlacement,
			portable:         volume.Portable,
			schedulerName:    volume.SchedulerName,
			encrypted:        volume.Encrypted,
			deviceSetName:    volume.Name,
		}
		osdProps.storeConfig.DeviceClass = volume.CrushDeviceClass

		if osdProps.encrypted {
			// If the deviceSet template has "encrypted" but the Ceph version is not compatible
			if !c.clusterInfo.CephVersion.IsAtLeast(cephVolumeRawEncryptionModeMinOctopusCephVersion) {
				errMsg := fmt.Sprintf("failed to validate storageClassDeviceSet %q. min required ceph version to support encryption is %q", volume.Name, cephVolumeRawEncryptionModeMinOctopusCephVersion.String())
				errs.addError(errMsg)
				continue
			}

			// create encryption Kubernetes Secret if the PVC is encrypted
			key, err := generateDmCryptKey()
			if err != nil {
				errMsg := fmt.Sprintf("failed to generate dmcrypt key for osd claim %q. %v", osdProps.pvc.ClaimName, err)
				errs.addError(errMsg)
				continue
			}

			// Initialize the KMS code
			kmsConfig := kms.NewConfig(c.context, &c.spec, c.clusterInfo)

			// We could set an env var in the Operator or a global var instead of the API call?
			// Hopefully, the API is cheap and we can always retrieve the token if it has changed...
			if c.spec.Security.KeyManagementService.IsTokenAuthEnabled() {
				err := kms.SetTokenToEnvVar(c.context, c.spec.Security.KeyManagementService.TokenSecretName, kmsConfig.Provider, c.clusterInfo.Namespace)
				if err != nil {
					errMsg := fmt.Sprintf("failed to fetch kms token secret %q. %v", c.spec.Security.KeyManagementService.TokenSecretName, err)
					errs.addError(errMsg)
					continue
				}
			}

			// Generate and store the encrypted key in whatever KMS is configured
			err = kmsConfig.PutSecret(osdProps.pvc.ClaimName, key)
			if err != nil {
				errMsg := fmt.Sprintf("failed to store secret. %v", err)
				errs.addError(errMsg)
				continue
			}
		}

		// Skip OSD prepare if deployment already exists for the PVC
		if existingDeployments.Has(dataSource.ClaimName) {
			logger.Debugf("skipping OSD prepare job creation for PVC %q because OSD daemon using the PVC already exists", osdProps.crushHostname)
			continue
		}

		// Update the orchestration status of this pvc to the starting state
		status := OrchestrationStatus{Status: OrchestrationStatusStarting, PvcBackedOSD: true}
		cmName := c.updateOSDStatus(osdProps.crushHostname, status)

		if err := c.runPrepareJob(&osdProps, config); err != nil {
			c.handleOrchestrationFailure(errs, osdProps.crushHostname, "%v", err)
			c.deleteStatusConfigMap(osdProps.crushHostname)
			continue // do not record the status CM's name
		}

		// record the name of the status configmap that will eventually receive results from the
		// OSD provisioning job we just created. This will help us determine when we are done
		// processing the results of provisioning jobs.
		awaitingStatusConfigMaps.Insert(cmName)
	}

	return awaitingStatusConfigMaps, nil
}

// Returns a set of all the awaitingStatusConfigMaps that will be updated by provisioning jobs.
// Returns error only if the calling function should halt all OSD provisioning. Non-halting errors
// are added to provisionErrors.
//
// Creation of prepare jobs is most directly related to creating new OSDs. And we want to keep all
// usage of awaitingStatusConfigMaps in this file.
func (c *Cluster) startProvisioningOverNodes(config *provisionConfig, errs *provisionErrors) (sets.String, error) {
	if !c.spec.Storage.UseAllNodes && len(c.spec.Storage.Nodes) == 0 {
		logger.Info("no nodes are defined for configuring OSDs on raw devices")
		return sets.NewString(), nil
	}

	if c.spec.Storage.UseAllNodes {
		if len(c.spec.Storage.Nodes) > 0 {
			logger.Warningf("useAllNodes is TRUE, but nodes are specified. NODES in the cluster CR will be IGNORED unless useAllNodes is FALSE.")
		}

		// Get the list of all nodes in the cluster. The placement settings will be applied below.
		hostnameMap, err := k8sutil.GetNodeHostNames(c.clusterInfo.Context, c.context.Clientset)
		if err != nil {
			errs.addError("failed to provision OSDs on nodes. failed to get node hostnames. %v", err)
			return sets.NewString(), nil
		}
		c.spec.Storage.Nodes = nil
		for _, hostname := range hostnameMap {
			storageNode := cephv1.Node{
				Name: hostname,
			}
			c.spec.Storage.Nodes = append(c.spec.Storage.Nodes, storageNode)
		}
		logger.Debugf("storage nodes: %+v", c.spec.Storage.Nodes)
	}
	// generally speaking, this finds nodes which are capable of running new osds
	validNodes := k8sutil.GetValidNodes(c.clusterInfo.Context, c.spec.Storage, c.context.Clientset, cephv1.GetOSDPlacement(c.spec.Placement))

	logger.Infof("%d of the %d storage nodes are valid", len(validNodes), len(c.spec.Storage.Nodes))

	c.ValidStorage = *c.spec.Storage.DeepCopy()
	c.ValidStorage.Nodes = validNodes

	// no valid node is ready to run an osd
	if len(validNodes) == 0 {
		logger.Warningf("no valid nodes available to run osds on nodes in namespace %q", c.clusterInfo.Namespace)
		return sets.NewString(), nil
	}

	if len(c.spec.DataDirHostPath) == 0 {
		errs.addError("failed to provision OSDs on nodes. user has specified valid nodes for storage, but dataDirHostPath is empty. user must set CephCluster dataDirHostPath")
		return sets.NewString(), nil
	}

	awaitingStatusConfigMaps := sets.NewString()
	for _, node := range c.ValidStorage.Nodes {
		if c.clusterInfo.Context.Err() != nil {
			return awaitingStatusConfigMaps, c.clusterInfo.Context.Err()
		}
		// fully resolve the storage config and resources for this node
		// don't care about osd device class resources since it will be overwritten later for prepareosd resources
		n := c.resolveNode(node.Name, "")
		if n == nil {
			logger.Warningf("node %q did not resolve", node.Name)
			continue
		}

		if n.Name == "" {
			logger.Warningf("skipping node with a blank name! %+v", n)
			continue
		}

		// create the job that prepares osds on the node
		storeConfig := osdconfig.ToStoreConfig(n.Config)
		metadataDevice := osdconfig.MetadataDevice(n.Config)
		osdProps := osdProperties{
			crushHostname:  n.Name,
			devices:        n.Devices,
			selection:      n.Selection,
			resources:      n.Resources,
			storeConfig:    storeConfig,
			metadataDevice: metadataDevice,
		}

		// update the orchestration status of this node to the starting state
		status := OrchestrationStatus{Status: OrchestrationStatusStarting}
		cmName := c.updateOSDStatus(n.Name, status)

		if err := c.runPrepareJob(&osdProps, config); err != nil {
			c.handleOrchestrationFailure(errs, n.Name, "%v", err)
			c.deleteStatusConfigMap(n.Name)
			continue // do not record the status CM's name
		}

		// record the name of the status configmap that will eventually receive results from the
		// OSD provisioning job we just created. This will help us determine when we are done
		// processing the results of provisioning jobs.
		awaitingStatusConfigMaps.Insert(cmName)
	}

	return awaitingStatusConfigMaps, nil
}

func (c *Cluster) runPrepareJob(osdProps *osdProperties, config *provisionConfig) error {
	nodeOrPVC := "node"
	if osdProps.onPVC() {
		nodeOrPVC = "PVC"
	}
	nodeOrPVCName := osdProps.crushHostname

	job, err := c.makeJob(*osdProps, config)
	if err != nil {
		return errors.Wrapf(err, "failed to generate osd provisioning job template for %s %q", nodeOrPVC, nodeOrPVCName)
	}

	if err := k8sutil.RunReplaceableJob(c.clusterInfo.Context, c.context.Clientset, job, false); err != nil {
		return errors.Wrapf(err, "failed to run osd provisioning job for %s %q", nodeOrPVC, nodeOrPVCName)
	}

	logger.Infof("started OSD provisioning job for %s %q", nodeOrPVC, nodeOrPVCName)
	return nil
}

func createDaemonOnPVC(c *Cluster, osd OSDInfo, pvcName string, config *provisionConfig) error {
	d, err := deploymentOnPVC(c, osd, pvcName, config)
	if err != nil {
		return err
	}

	message := fmt.Sprintf("Processing OSD %d on PVC %q", osd.ID, pvcName)
	updateConditionFunc(c.clusterInfo.Context, c.context, c.clusterInfo.NamespacedName(), cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, message)

	_, err = k8sutil.CreateDeployment(c.clusterInfo.Context, c.context.Clientset, d)
	return errors.Wrapf(err, "failed to create deployment for OSD %d on PVC %q", osd.ID, pvcName)
}

func createDaemonOnNode(c *Cluster, osd OSDInfo, nodeName string, config *provisionConfig) error {
	d, err := deploymentOnNode(c, osd, nodeName, config)
	if err != nil {
		return err
	}

	message := fmt.Sprintf("Processing OSD %d on node %q", osd.ID, nodeName)
	updateConditionFunc(c.clusterInfo.Context, c.context, c.clusterInfo.NamespacedName(), cephv1.ConditionProgressing, v1.ConditionTrue, cephv1.ClusterProgressingReason, message)

	_, err = k8sutil.CreateDeployment(c.clusterInfo.Context, c.context.Clientset, d)
	return errors.Wrapf(err, "failed to create deployment for OSD %d on node %q", osd.ID, nodeName)
}
