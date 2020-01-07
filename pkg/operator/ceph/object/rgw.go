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

// Package object for the Ceph object store.
package object

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type clusterConfig struct {
	clusterInfo       *cephconfig.ClusterInfo
	context           *clusterd.Context
	store             cephv1.CephObjectStore
	rookVersion       string
	clusterSpec       *cephv1.ClusterSpec
	ownerRef          metav1.OwnerReference
	DataPathMap       *config.DataPathMap
	isUpgrade         bool
	skipUpgradeChecks bool
}

type rgwConfig struct {
	ResourceName string
	DaemonID     string
}

const (
	oldRgwKeyName = "client.radosgw.gateway"
)

var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

func (c *clusterConfig) createOrUpdate() error {
	// validate the object store settings
	if err := validateStore(c.context, c.store); err != nil {
		return errors.Wrapf(err, "invalid object store %s arguments", c.store.Name)
	}

	logger.Infof("creating object store %s in namespace %s", c.store.Name, c.store.Namespace)

	// start the service
	serviceIP, err := c.startService()
	if err != nil {
		return errors.Wrapf(err, "failed to start rgw service")
	}

	// create the ceph artifacts for the object store
	objContext := NewContext(c.context, c.store.Name, c.store.Namespace)
	err = createObjectStore(objContext, *c.store.Spec.MetadataPool.ToModel(""), *c.store.Spec.DataPool.ToModel(""), serviceIP, c.store.Spec.Gateway.Port)
	if err != nil {
		return errors.Wrapf(err, "failed to create pools")
	}

	if err := c.startRGWPods(); err != nil {
		return errors.Wrapf(err, "failed to start pods")
	}

	logger.Infof("created object store %s", c.store.Name)
	return nil
}

func (c *clusterConfig) startRGWPods() error {
	// backward compatibility, triggered during updates
	if c.store.Spec.Gateway.AllNodes {
		// log we don't support that anymore
		logger.Warningf(
			"setting 'AllNodes' to %t is not supported anymore, please use 'instances' instead, removing old DaemonSets if any and replace them with Deployments in object store %s",
			c.store.Spec.Gateway.AllNodes, c.store.Name)
	}
	if c.store.Spec.Gateway.Instances < 1 {
		// Set the minimum of at least one instance
		logger.Warningf("spec.gateway.instances must be set to at least 1")
		c.store.Spec.Gateway.Instances = 1
	}

	// start a new deployment and scale up
	desiredRgwInstances := int(c.store.Spec.Gateway.Instances)
	for i := 0; i < desiredRgwInstances; i++ {
		var err error

		daemonLetterID := k8sutil.IndexToName(i)
		// Each rgw is id'ed by <store_name>-<letterID>
		daemonName := fmt.Sprintf("%s-%s", c.store.Name, daemonLetterID)
		// resource name is rook-ceph-rgw-<store_name>-<daemon_name>
		resourceName := fmt.Sprintf("%s-%s-%s", AppName, c.store.Name, daemonLetterID)

		rgwConfig := &rgwConfig{
			ResourceName: resourceName,
			DaemonID:     daemonName,
		}

		// Generate the keyring after starting the replication controller so that the keyring may use
		// the controller as its owner reference; the keyring is deleted with the controller
		keyring, err := c.generateKeyring(rgwConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to create rgw keyring")
		}

		// Check for existing deployment and set the daemon config flags
		_, err = c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Get(rgwConfig.ResourceName, metav1.GetOptions{})
		// We don't need to handle any error here
		if err != nil {
			// Apply the flag only when the deployment is not found
			if kerrors.IsNotFound(err) {
				logger.Info("setting rgw config flags")
				err = c.setDefaultFlagsMonConfigStore(rgwConfig.ResourceName)
				if err != nil {
					return errors.Wrapf(err, "failed to set default rgw config options")
				}
			}
		}

		// Create deployment
		deployment := c.createDeployment(rgwConfig)
		logger.Infof("object store %s deployment %s started", c.store.Name, deployment.Name)
		createdDeployment, createErr := c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Create(deployment)
		if createErr != nil {
			if !kerrors.IsAlreadyExists(createErr) {
				return errors.Wrapf(createErr, "failed to create rgw deployment")
			}
			logger.Infof("object store %s deployment %s already exists. updating if needed", c.store.Name, deployment.Name)
			createdDeployment, err = c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Get(deployment.Name, metav1.GetOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to get existing rgw deployment %s for update", deployment.Name)
			}
		}

		resourceControllerOwnerRef := &metav1.OwnerReference{
			UID:        createdDeployment.UID,
			APIVersion: "v1",
			Kind:       "deployment",
			Name:       rgwConfig.ResourceName,
		}

		err = c.associateKeyring(keyring, resourceControllerOwnerRef)
		if err != nil {
			logger.Warningf("failed to associate keyring with rgw deployment %q. %v", createdDeployment.Name, err)
		}

		// Generate the mime.types file after the rep. controller as well for the same reason as keyring
		if createErr != nil && kerrors.IsAlreadyExists(createErr) {
			// Always invoke ceph version before an upgrade so we are sure to be up-to-date
			daemon := string(config.RgwType)
			var cephVersionToUse cephver.CephVersion
			currentCephVersion, err := client.LeastUptodateDaemonVersion(c.context, c.clusterInfo.Name, daemon)
			if err != nil {
				logger.Warningf("failed to retrieve current ceph %s version. %v", daemon, err)
				logger.Debug("could not detect ceph version during update, this is likely an initial bootstrap, proceeding with c.clusterInfo.CephVersion")
				cephVersionToUse = c.clusterInfo.CephVersion

			} else {
				logger.Debugf("current cluster version for rgws before upgrading is: %+v", currentCephVersion)
				cephVersionToUse = currentCephVersion
			}
			if err := updateDeploymentAndWait(c.context, deployment, c.store.Namespace, daemon, daemonLetterID, cephVersionToUse, c.isUpgrade, c.skipUpgradeChecks, false); err != nil {
				return errors.Wrapf(err, "failed to update object store %s deployment %s", c.store.Name, deployment.Name)
			}
		}

		if err := c.generateMimeTypes(resourceControllerOwnerRef); err != nil {
			return errors.Wrapf(err, "failed to generate the rgw mime.types config")
		}
	}

	// scale down scenario
	deps, err := k8sutil.GetDeployments(c.context.Clientset, c.store.Namespace, c.storeLabelSelector())
	if err != nil {
		logger.Warning("could not get deployments for object store %q (matching label selector %q). %v", c.store.Name, c.storeLabelSelector(), err)
	}

	currentRgwInstances := int(len(deps.Items))
	if currentRgwInstances > desiredRgwInstances {
		logger.Infof("found more rgw deployments %d than desired %d in object store %q, scaling down", currentRgwInstances, c.store.Spec.Gateway.Instances, c.store.Name)
		diffCount := currentRgwInstances - desiredRgwInstances
		for i := 0; i < diffCount; {
			depIDToRemove := currentRgwInstances - 1
			depNameToRemove := fmt.Sprintf("%s-%s-%s", AppName, c.store.Name, k8sutil.IndexToName(depIDToRemove))
			if err := k8sutil.DeleteDeployment(c.context.Clientset, c.store.Namespace, depNameToRemove); err != nil {
				logger.Warning("error during deletion of deployment %q resource. %v", depNameToRemove, err)
			}
			currentRgwInstances = currentRgwInstances - 1
			i++

			// Delete the auth key
			err = client.AuthDelete(c.context, c.store.Namespace, generateCephXUser(depNameToRemove))
			if err != nil {
				logger.Infof("failed to delete rgw key %q. %v", depNameToRemove, err)
			}
		}
		// verify scale down was successful
		deps, err = k8sutil.GetDeployments(c.context.Clientset, c.store.Namespace, c.storeLabelSelector())
		if err != nil {
			logger.Warning("could not get deployments for object store %q (matching label selector %q). %v", c.store.Name, c.storeLabelSelector(), err)
		}
		currentRgwInstances = len(deps.Items)
		if currentRgwInstances == desiredRgwInstances {
			logger.Infof("successfully scaled down rgw deployments to %d in object store %q", desiredRgwInstances, c.store.Name)
		}
	}

	c.deleteLegacyDaemons()
	return nil
}

// deleteLegacyDaemons removes legacy rgw components that might have existed in Rook v1.0
func (c *clusterConfig) deleteLegacyDaemons() {
	// Make a best effort to delete the rgw pods daemonsets
	daemons, err := k8sutil.GetDaemonsets(c.context.Clientset, c.store.Namespace, c.storeLabelSelector())
	if err != nil {
		logger.Warningf("could not get deployments for object store %q (matching label selector %q). %v", c.store.Name, c.storeLabelSelector(), err)
	}
	daemonsetNum := len(daemons.Items)
	if daemonsetNum > 0 {
		for _, d := range daemons.Items {
			// Delete any existing daemonset
			if err := k8sutil.DeleteDaemonset(c.context.Clientset, c.store.Namespace, d.Name); err != nil {
				logger.Errorf("error during deletion of daemonset %q resource. %v", d.Name, err)
			}
		}
		// Delete legacy rgw key
		err = client.AuthDelete(c.context, c.store.Namespace, oldRgwKeyName)
		if err != nil {
			logger.Infof("failed to delete legacy rgw key %q. %v", oldRgwKeyName, err)
		}
	}

	// legacy deployment detection
	logger.Debugf("looking for legacy deployment in object store %q", c.store.Name)
	deps, err := k8sutil.GetDeployments(c.context.Clientset, c.store.Namespace, c.storeLabelSelector())
	if err != nil {
		logger.Warning("could not get deployments for object store %q (matching label selector %q). %v", c.store.Name, c.storeLabelSelector(), err)
	}
	for _, d := range deps.Items {
		if d.Name == c.instanceName() {
			logger.Infof("legacy deployment in object store %q found %q", c.store.Name, d.Name)
			if err := k8sutil.DeleteDeployment(c.context.Clientset, c.store.Namespace, d.Name); err != nil {
				logger.Warning("error during deletion of deployment %q resource. %v", d.Name, err)
			}
			// Delete legacy rgw key
			err = client.AuthDelete(c.context, c.store.Namespace, oldRgwKeyName)
			if err != nil {
				logger.Infof("failed to delete legacy rgw key %q. %v", oldRgwKeyName, err)
			}
		}
	}
}

// Delete the object store.
// WARNING: This is a very destructive action that deletes all metadata and data pools.
func (c *clusterConfig) deleteStore() error {
	logger.Infof("Deleting object store %s from namespace %s", c.store.Name, c.store.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the rgw service
	err := c.context.Clientset.CoreV1().Services(c.store.Namespace).Delete(c.instanceName(), options)
	if err != nil && !kerrors.IsNotFound(err) {
		logger.Warningf("failed to delete rgw service. %v", err)
	}

	// Make a best effort to delete the rgw pods deployments
	deps, err := k8sutil.GetDeployments(c.context.Clientset, c.store.Namespace, c.storeLabelSelector())
	if err != nil {
		logger.Warning("could not get deployments for object store %s (matching label selector %q). %v", c.store.Namespace, c.storeLabelSelector(), err)
	}
	for _, d := range deps.Items {
		if err := k8sutil.DeleteDeployment(c.context.Clientset, c.store.Namespace, d.Name); err != nil {
			logger.Warning("error during deletion of deployment %s resource. %v", d.Name, err)
		}
	}

	// Delete the rgw config map keyrings
	err = c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Delete(c.instanceName(), options)
	if err != nil && !kerrors.IsNotFound(err) {
		logger.Warningf("failed to delete rgw secret. %v", err)
	}

	// Delete rgw CephX keys
	for i := 0; i < int(c.store.Spec.Gateway.Instances); i++ {
		daemonLetterID := k8sutil.IndexToName(i)
		keyName := fmt.Sprintf("client.%s.%s", strings.Replace(c.store.Name, "-", ".", -1), daemonLetterID)
		err := client.AuthDelete(c.context, c.store.Namespace, keyName)
		if err != nil {
			return err
		}
	}

	// Delete the realm and pools
	objContext := NewContext(c.context, c.store.Name, c.store.Namespace)
	err = deleteRealmAndPools(objContext, c.store.Spec.PreservePoolsOnDelete)
	if err != nil {
		return errors.Wrapf(err, "failed to delete the realm and pools")
	}

	logger.Infof("Completed deleting object store %s", c.store.Name)
	return nil
}

func (c *clusterConfig) instanceName() string {
	return fmt.Sprintf("%s-%s", AppName, c.store.Name)
}

func (c *clusterConfig) storeLabelSelector() string {
	return fmt.Sprintf("rook_object_store=%s", c.store.Name)
}

// Validate the object store arguments
func validateStore(context *clusterd.Context, s cephv1.CephObjectStore) error {
	if s.Name == "" {
		return errors.New("missing name")
	}
	if s.Namespace == "" {
		return errors.New("missing namespace")
	}
	if err := pool.ValidatePoolSpec(context, s.Namespace, &s.Spec.MetadataPool); err != nil {
		return errors.Wrapf(err, "invalid metadata pool spec")
	}
	if err := pool.ValidatePoolSpec(context, s.Namespace, &s.Spec.DataPool); err != nil {
		return errors.Wrapf(err, "invalid data pool spec")
	}

	return nil
}
