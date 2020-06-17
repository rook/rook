/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/crash"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (c *ClusterController) configureExternalCephCluster(cluster *cluster) error {
	// Make sure the spec contains all the information we need
	err := validateExternalClusterSpec(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to validate external cluster specs")
	}

	config.ConditionExport(c.context, c.namespacedName, cephv1.ConditionConnecting, v1.ConditionTrue, "ClusterConnecting", "Cluster is connecting")

	// loop until we find the secret necessary to connect to the external cluster
	// then populate clusterInfo
	cluster.Info = mon.PopulateExternalClusterInfo(c.context, c.namespacedName.Namespace)

	// If the user to check the ceph health and status is not the admin,
	// we validate that ExternalCred has been populated correctly,
	// then we check if the key (whether admin or not) is encoded in base64
	if !mon.IsExternalHealthCheckUserAdmin(cluster.Info.AdminSecret) {
		if !cluster.Info.IsInitializedExternalCred(true) {
			return errors.New("invalid user health checker credentials")
		}
		if !cephconfig.IsKeyringBase64Encoded(cluster.Info.ExternalCred.Secret) {
			return errors.Errorf("invalid user health checker key %q", cluster.Info.ExternalCred.Username)
		}
	} else {
		// If the client.admin is used
		if !cephconfig.IsKeyringBase64Encoded(cluster.Info.AdminSecret) {
			return errors.Errorf("invalid user health checker key %q", client.AdminUsername)
		}
	}

	// Write connection info (ceph config file and keyring) for ceph commands
	if cluster.Spec.CephVersion.Image == "" {
		err = mon.WriteConnectionConfig(c.context, cluster.Info)
		if err != nil {
			logger.Errorf("failed to write config. attempting to continue. %v", err)
		}
	}

	// Validate versions (local and external)
	// If no image is specified we don't perform any checks
	if cluster.Spec.CephVersion.Image != "" {
		_, _, err = c.detectAndValidateCephVersion(cluster)
		if err != nil {
			return errors.Wrap(err, "failed to detect and validate ceph version")
		}

		// Write the rook-config-override configmap (used by various daemons to apply config overrides)
		// If we don't do this, daemons will never start, waiting forever for this configmap to be present
		//
		// Only do this when doing a bit of management...
		logger.Infof("creating %q configmap", k8sutil.ConfigOverrideName)
		err = populateConfigOverrideConfigMap(c.context, c.namespacedName.Namespace, cluster.ownerRef)
		if err != nil {
			return errors.Wrap(err, "failed to populate config override config map")
		}

		logger.Infof("creating %q secret", config.StoreName)
		err = config.GetStore(c.context, c.namespacedName.Namespace, &cluster.ownerRef).CreateOrUpdate(cluster.Info)
		if err != nil {
			return errors.Wrap(err, "failed to update the global config")
		}
	}

	// The cluster Identity must be established at this point
	if !cluster.Info.IsInitialized(true) {
		return errors.New("the cluster identity was not established")
	}
	logger.Info("external cluster identity established")

	// Create CSI Secrets only if the user has provided the admin key
	if cluster.Info.AdminSecret != mon.AdminSecretName {
		err = csi.CreateCSISecrets(c.context, c.namespacedName.Namespace, &cluster.ownerRef)
		if err != nil {
			return errors.Wrap(err, "failed to create csi kubernetes secrets")
		}
	}

	// Create CSI config map
	err = csi.CreateCsiConfigMap(c.namespacedName.Namespace, c.context.Clientset, &cluster.ownerRef)
	if err != nil {
		return errors.Wrap(err, "failed to create csi config map")
	}

	// Save CSI configmap
	err = csi.SaveClusterConfig(c.context.Clientset, c.namespacedName.Namespace, cluster.Info, c.csiConfigMutex)
	if err != nil {
		return errors.Wrap(err, "failed to update csi cluster config")
	}
	logger.Info("successfully updated csi config map")

	// Create Crash Collector Secret
	// In 14.2.5 the crash daemon will read the client.crash key instead of the admin key
	if !cluster.Spec.CrashCollector.Disable {
		err = crash.CreateCrashCollectorSecret(c.context, c.namespacedName.Namespace, &cluster.ownerRef)
		if err != nil {
			return errors.Wrap(err, "failed to create crash collector kubernetes secret")
		}
	}

	// Everything went well so let's update the CR's status to "connected"
	config.ConditionExport(c.context, c.namespacedName, cephv1.ConditionConnected, v1.ConditionTrue, "ClusterConnected", "Cluster connected successfully")

	// Mark initialization has done
	cluster.initCompleted = true

	return nil
}

func purgeExternalCluster(clientset kubernetes.Interface, namespace string) {
	// Purge the config maps
	cmsToDelete := []string{
		mon.EndpointConfigMapName,
		k8sutil.ConfigOverrideName,
	}
	for _, cm := range cmsToDelete {
		err := clientset.CoreV1().ConfigMaps(namespace).Delete(cm, &metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			logger.Errorf("failed to delete config map %q. %v", cm, err)
		}
	}

	// Purge the secrets
	secretsToDelete := []string{
		mon.AppName,
		mon.OperatorCreds,
		csi.CsiRBDNodeSecret,
		csi.CsiRBDProvisionerSecret,
		csi.CsiCephFSNodeSecret,
		csi.CsiCephFSProvisionerSecret,
		config.StoreName,
	}
	for _, secret := range secretsToDelete {
		err := clientset.CoreV1().Secrets(namespace).Delete(secret, &metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			logger.Errorf("failed to delete secret %q. %v", secret, err)
		}
	}
}

func validateExternalClusterSpec(cluster *cluster) error {
	if cluster.Spec.CephVersion.Image != "" {
		if cluster.Spec.DataDirHostPath == "" {
			return errors.New("dataDirHostPath must be specified")
		}
	}

	return nil
}
