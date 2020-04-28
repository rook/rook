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
	"reflect"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type clusterConfig struct {
	clusterInfo       *cephconfig.ClusterInfo
	context           *clusterd.Context
	store             *cephv1.CephObjectStore
	rookVersion       string
	clusterSpec       *cephv1.ClusterSpec
	ownerRef          *metav1.OwnerReference
	DataPathMap       *config.DataPathMap
	skipUpgradeChecks bool
	client            client.Client
	scheme            *runtime.Scheme
	Network           cephv1.NetworkSpec
}

type rgwConfig struct {
	ResourceName string
	DaemonID     string
}

const (
	oldRgwKeyName = "client.radosgw.gateway"
)

var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

func (c *clusterConfig) createOrUpdateStore() error {
	logger.Infof("creating object store %q in namespace %q", c.store.Name, c.store.Namespace)

	if err := c.startRGWPods(); err != nil {
		return errors.Wrapf(err, "failed to start rgw pods")
	}

	logger.Infof("created object store %q in namespace %q", c.store.Name, c.store.Namespace)
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

	// Create the controller owner ref
	// It will be associated to all resources of the CephObjectStore
	ref, err := opcontroller.GetControllerObjectOwnerReference(c.store, c.scheme)
	if err != nil || ref == nil {
		return errors.Wrapf(err, "failed to get controller %q owner reference", c.store.Name)
	}
	c.ownerRef = ref

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

		// We set the owner reference of the Secret to the Object controller instead of the replicaset
		// because we watch for that resource and reconcile if anything happens to it
		_, err = c.generateKeyring(rgwConfig)
		if err != nil {
			return errors.Wrap(err, "failed to create rgw keyring")
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
					return errors.Wrap(err, "failed to set default rgw config options")
				}
			}
		}

		// Create deployment
		deployment := c.createDeployment(rgwConfig)
		logger.Infof("object store %q deployment %q started", c.store.Name, deployment.Name)

		// Set owner ref to cephObjectStore object
		err = controllerutil.SetControllerReference(c.store, deployment, c.scheme)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference for ceph object %q secret", deployment.Name)
		}

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(deployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set annotation for deployment %q", deployment.Name)
		}

		_, createErr := c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Create(deployment)
		if createErr != nil {
			if !kerrors.IsAlreadyExists(createErr) {
				return errors.Wrapf(createErr, "failed to create rgw deployment")
			}
			logger.Infof("object store %q deployment %q already exists. updating if needed", c.store.Name, deployment.Name)
			_, err = c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Get(deployment.Name, metav1.GetOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to get existing rgw deployment %q for update", deployment.Name)
			}
		}

		// Generate the mime.types file after the rep. controller as well for the same reason as keyring
		if createErr != nil && kerrors.IsAlreadyExists(createErr) {
			if err := updateDeploymentAndWait(c.context, deployment, c.store.Namespace, config.RgwType, daemonLetterID, c.skipUpgradeChecks, c.clusterSpec.ContinueUpgradeAfterChecksEvenIfNotHealthy); err != nil {
				return errors.Wrapf(err, "failed to update object store %q deployment %q", c.store.Name, deployment.Name)
			}
		}

		if err := c.generateMimeTypes(); err != nil {
			return errors.Wrap(err, "failed to generate the rgw mime.types config")
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
				logger.Warningf("error during deletion of deployment %q resource. %v", depNameToRemove, err)
			}
			currentRgwInstances = currentRgwInstances - 1
			i++

			// Delete the Secret key
			secretToRemove := c.generateSecretName(k8sutil.IndexToName(depIDToRemove))
			err = c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Delete(secretToRemove, &metav1.DeleteOptions{})
			if err != nil && !kerrors.IsNotFound(err) {
				logger.Warningf("failed to delete rgw secret %q. %v", secretToRemove, err)
			}

			err := c.deleteRgwCephObjects(depNameToRemove)
			if err != nil {
				logger.Warningf("%v", err)
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
		err = cephclient.AuthDelete(c.context, c.store.Namespace, oldRgwKeyName)
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
		if d.Name == instanceName(c.store.Name) {
			logger.Infof("legacy deployment in object store %q found %q", c.store.Name, d.Name)
			if err := k8sutil.DeleteDeployment(c.context.Clientset, c.store.Namespace, d.Name); err != nil {
				logger.Warningf("error during deletion of deployment %q resource. %v", d.Name, err)
			}
			// Delete legacy rgw key
			err = cephclient.AuthDelete(c.context, c.store.Namespace, oldRgwKeyName)
			if err != nil {
				logger.Infof("failed to delete legacy rgw key %q. %v", oldRgwKeyName, err)
			}
		}
	}
}

// Delete the object store.
// WARNING: This is a very destructive action that deletes all metadata and data pools.
func (c *clusterConfig) deleteStore() error {
	logger.Infof("deleting object store %q from namespace %q", c.store.Name, c.store.Namespace)

	// Delete rgw CephX keys and configuration in centralized mon database
	for i := 0; i < int(c.store.Spec.Gateway.Instances); i++ {
		daemonLetterID := k8sutil.IndexToName(i)
		depNameToRemove := fmt.Sprintf("%s-%s-%s", AppName, c.store.Name, daemonLetterID)

		err := c.deleteRgwCephObjects(depNameToRemove)
		if err != nil {
			return err
		}
	}

	// Delete the realm and pools
	objContext := NewContext(c.context, c.store.Name, c.store.Namespace)
	err := deleteRealmAndPools(objContext, c.store.Spec)
	if err != nil {
		return errors.Wrap(err, "failed to delete the realm and pools")
	}

	logger.Infof("completed deleting object store %q from namespace %q", c.store.Name, c.store.Namespace)
	return nil
}

func (c *clusterConfig) deleteRgwCephObjects(depNameToRemove string) error {
	logger.Infof("deleting rgw CephX key and configuration in centralized mon database for %q", depNameToRemove)

	// Delete configuration in centralized mon database
	err := c.deleteFlagsMonConfigStore(depNameToRemove)
	if err != nil {
		return err
	}

	err = cephclient.AuthDelete(c.context, c.store.Namespace, generateCephXUser(depNameToRemove))
	if err != nil {
		return err
	}

	logger.Infof("completed deleting rgw CephX key and configuration in centralized mon database for %q", depNameToRemove)
	return nil
}

func instanceName(name string) string {
	return fmt.Sprintf("%s-%s", AppName, name)
}

func (c *clusterConfig) storeLabelSelector() string {
	return fmt.Sprintf("rook_object_store=%s", c.store.Name)
}

// Validate the object store arguments
func validateStore(context *clusterd.Context, s *cephv1.CephObjectStore) error {
	if s.Name == "" {
		return errors.New("missing name")
	}
	if s.Namespace == "" {
		return errors.New("missing namespace")
	}
	securePort := s.Spec.Gateway.SecurePort
	if securePort < 0 || securePort > 65535 {
		return errors.Errorf("securePort value of %d must be between 0 and 65535", securePort)
	}

	// Validate the pool settings, but allow for empty pools specs in case they have already been created
	// such as by the ceph mgr
	if !emptyPool(s.Spec.MetadataPool) {
		if err := pool.ValidatePoolSpec(context, s.Namespace, &s.Spec.MetadataPool); err != nil {
			return errors.Wrap(err, "invalid metadata pool spec")
		}
	}
	if !emptyPool(s.Spec.DataPool) {
		if err := pool.ValidatePoolSpec(context, s.Namespace, &s.Spec.DataPool); err != nil {
			return errors.Wrap(err, "invalid data pool spec")
		}
	}

	return nil
}

func (c *clusterConfig) generateSecretName(id string) string {
	return fmt.Sprintf("%s-%s-%s-keyring", AppName, c.store.Name, id)
}

func emptyPool(pool cephv1.PoolSpec) bool {
	return reflect.DeepEqual(pool, cephv1.PoolSpec{})
}
