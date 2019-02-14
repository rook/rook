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

// Package rgw for the Ceph object store.
package object

import (
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgwdaemon "github.com/rook/rook/pkg/daemon/ceph/rgw"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	appName        = "rook-ceph-rgw"
	keyringName    = "keyring"
	certVolumeName = "rook-rgw-cert"
	certMountPath  = "/etc/rook/private"
	certKeyName    = "cert"
	certFilename   = "rgw-cert.pem"
)

type config struct {
	context     *clusterd.Context
	store       cephv1.CephObjectStore
	rookVersion string
	cephVersion cephv1.CephVersionSpec
	hostNetwork bool
	ownerRefs   []metav1.OwnerReference
}

// Start the rgw manager
func (c *config) createStore() error {
	return c.createOrUpdate(false)
}

func (c *config) updateStore() error {
	return c.createOrUpdate(true)
}

func (c *config) createOrUpdate(update bool) error {
	// validate the object store settings
	if err := validateStore(c.context, c.store); err != nil {
		return fmt.Errorf("invalid object store %s arguments. %+v", c.store.Name, err)
	}

	// check if the object store already exists
	exists, err := c.storeExists()
	if err == nil && exists {
		if !update {
			logger.Infof("object store %s exists in namespace %s", c.store.Name, c.store.Namespace)
			return c.startRGWPods(false)
		}
		logger.Infof("object store %s exists in namespace %s. checking for updates", c.store.Name, c.store.Namespace)
	}

	logger.Infof("creating object store %s in namespace %s", c.store.Name, c.store.Namespace)
	err = c.createKeyring()
	if err != nil {
		return fmt.Errorf("failed to create rgw keyring. %+v", err)
	}

	// start the service
	serviceIP, err := c.startService()
	if err != nil {
		return fmt.Errorf("failed to start rgw service. %+v", err)
	}

	// create the ceph artifacts for the object store
	objContext := rgwdaemon.NewContext(c.context, c.store.Name, c.store.Namespace)
	err = rgwdaemon.CreateObjectStore(objContext, *c.store.Spec.MetadataPool.ToModel(""), *c.store.Spec.DataPool.ToModel(""), serviceIP, c.store.Spec.Gateway.Port)
	if err != nil {
		return fmt.Errorf("failed to create pools. %+v", err)
	}

	if err := c.startRGWPods(update); err != nil {
		return fmt.Errorf("failed to start pods. %+v", err)
	}

	logger.Infof("created object store %s", c.store.Name)
	return nil
}

func (c *config) startRGWPods(update bool) error {

	// if intended to update, remove the old pods so they can be created with the new spec settings
	if update {
		err := k8sutil.DeleteDeployment(c.context.Clientset, c.store.Namespace, c.instanceName())
		if err != nil {
			logger.Warning(err.Error())
		}
		err = k8sutil.DeleteDaemonset(c.context.Clientset, c.store.Namespace, c.instanceName())
		if err != nil {
			logger.Warning(err.Error())
		}
	}

	// start the deployment or daemonset
	if c.store.Spec.Gateway.AllNodes {
		return c.startDaemonset()
	}
	return c.startDeployment()
}

// Delete the object store.
// WARNING: This is a very destructive action that deletes all metadata and data pools.
func (c *config) deleteStore() error {
	// check if the object store  exists
	exists, err := c.storeExists()
	if err != nil {
		return fmt.Errorf("failed to detect if there is an object store to delete. %+v", err)
	}
	if !exists {
		logger.Infof("Object store %s does not exist in namespace %s", c.store.Name, c.store.Namespace)
		return nil
	}

	logger.Infof("Deleting object store %s from namespace %s", c.store.Name, c.store.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the rgw service
	err = c.context.Clientset.CoreV1().Services(c.store.Namespace).Delete(c.instanceName(), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete rgw service. %+v", err)
	}

	// Make a best effort to delete the rgw pods
	err = k8sutil.DeleteDeployment(c.context.Clientset, c.store.Namespace, c.instanceName())
	if err != nil {
		logger.Warning(err.Error())
	}
	err = k8sutil.DeleteDaemonset(c.context.Clientset, c.store.Namespace, c.instanceName())
	if err != nil {
		logger.Warning(err.Error())
	}

	// Delete the rgw keyring
	err = c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Delete(c.instanceName(), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete rgw secret. %+v", err)
	}

	// Delete the realm and pools
	objContext := rgwdaemon.NewContext(c.context, c.store.Name, c.store.Namespace)
	err = rgwdaemon.DeleteObjectStore(objContext)
	if err != nil {
		return fmt.Errorf("failed to delete the realm and pools. %+v", err)
	}

	logger.Infof("Completed deleting object store %s", c.store.Name)
	return nil
}

// Check if the object store exists depending on either the deployment or the daemonset
func (c *config) storeExists() (bool, error) {
	_, err := c.context.Clientset.Apps().Deployments(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	if err == nil {
		// the deployment was found
		return true, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	_, err = c.context.Clientset.Apps().DaemonSets(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	if err == nil {
		//  the daemonset was found
		return true, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	// neither one was found
	return false, nil
}

func (c *config) createKeyring() error {
	_, err := c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	if err == nil {
		logger.Infof("the rgw keyring was already generated")
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get rgw secrets. %+v", err)
	}

	// create the keyring
	logger.Infof("generating rgw keyring")
	keyring, err := c.createRGWKeyring()
	if err != nil {
		return fmt.Errorf("failed to create keyring. %+v", err)
	}

	// store the secrets
	secrets := map[string]string{
		keyringName: keyring,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.instanceName(),
			Namespace: c.store.Namespace,
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	k8sutil.SetOwnerRefs(c.context.Clientset, c.store.Namespace, &secret.ObjectMeta, c.ownerRefs)
	_, err = c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save rgw secrets. %+v", err)
	}

	return nil
}

func (c *config) instanceName() string {
	return fmt.Sprintf("%s-%s", appName, c.store.Name)
}

// create a keyring for the rgw client with a limited set of privileges
func (c *config) createRGWKeyring() (string, error) {
	username := "client.radosgw.gateway"
	access := []string{"osd", "allow rwx", "mon", "allow rw"}

	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(c.context, c.store.Namespace, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for %store. %+v", username, err)
	}

	return key, err
}

// Validate the object store arguments
func validateStore(context *clusterd.Context, s cephv1.CephObjectStore) error {
	if s.Name == "" {
		return fmt.Errorf("missing name")
	}
	if s.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if err := pool.ValidatePoolSpec(context, s.Namespace, &s.Spec.MetadataPool); err != nil {
		return fmt.Errorf("invalid metadata pool spec. %+v", err)
	}
	if err := pool.ValidatePoolSpec(context, s.Namespace, &s.Spec.DataPool); err != nil {
		return fmt.Errorf("invalid data pool spec. %+v", err)
	}

	return nil
}
