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
	"context"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/crash"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
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

	opcontroller.UpdateCondition(c.context, c.namespacedName, cephv1.ConditionConnecting, v1.ConditionTrue, cephv1.ClusterConnectingReason, "Attempting to connect to an external Ceph cluster")

	// loop until we find the secret necessary to connect to the external cluster
	// then populate clusterInfo

	cluster.ClusterInfo = mon.PopulateExternalClusterInfo(c.context, c.OpManagerCtx, c.namespacedName.Namespace, cluster.ownerInfo)
	cluster.ClusterInfo.SetName(c.namespacedName.Name)
	cluster.ClusterInfo.Context = c.OpManagerCtx

	if !client.IsKeyringBase64Encoded(cluster.ClusterInfo.CephCred.Secret) {
		return errors.Errorf("invalid user health checker key for user %q", cluster.ClusterInfo.CephCred.Username)
	}

	// Write connection info (ceph config file and keyring) for ceph commands
	if cluster.Spec.CephVersion.Image == "" {
		err = mon.WriteConnectionConfig(c.context, cluster.ClusterInfo)
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
		err = populateConfigOverrideConfigMap(c.context, c.namespacedName.Namespace, cluster.ClusterInfo.OwnerInfo)
		if err != nil {
			return errors.Wrap(err, "failed to populate config override config map")
		}

		logger.Infof("creating %q secret", config.StoreName)
		err = config.GetStore(c.context, c.namespacedName.Namespace, cluster.ClusterInfo.OwnerInfo).CreateOrUpdate(cluster.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to update the global config")
		}
	}

	// The cluster Identity must be established at this point
	if !cluster.ClusterInfo.IsInitialized(true) {
		return errors.New("the cluster identity was not established")
	}
	logger.Info("external cluster identity established")

	// Create CSI Secrets only if the user has provided the admin key
	if cluster.ClusterInfo.CephCred.Username == client.AdminUsername {
		err = csi.CreateCSISecrets(c.context, cluster.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to create csi kubernetes secrets")
		}
	}

	// Create CSI config map
	err = csi.CreateCsiConfigMap(c.namespacedName.Namespace, c.context.Clientset, cluster.ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to create csi config map")
	}

	// Save CSI configmap
	err = csi.SaveClusterConfig(c.context.Clientset, c.namespacedName.Namespace, cluster.ClusterInfo, c.csiConfigMutex)
	if err != nil {
		return errors.Wrap(err, "failed to update csi cluster config")
	}
	logger.Info("successfully updated csi config map")

	// Create Crash Collector Secret
	// In 14.2.5 the crash daemon will read the client.crash key instead of the admin key
	if !cluster.Spec.CrashCollector.Disable {
		err = crash.CreateCrashCollectorSecret(c.context, cluster.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to create crash collector kubernetes secret")
		}
	}

	// enable monitoring if `monitoring: enabled: true`
	// We need the Ceph version
	if cluster.Spec.Monitoring.Enabled {
		// Discover external Ceph version to detect which service monitor to inject
		externalVersion, err := client.GetCephMonVersion(c.context, cluster.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to get external ceph mon version")
		}
		cluster.ClusterInfo.CephVersion = *externalVersion

		// Populate ceph version
		c.updateClusterCephVersion("", *externalVersion)

		err = c.configureExternalClusterMonitoring(c.context, cluster)
		if err != nil {
			return errors.Wrap(err, "failed to configure external cluster monitoring")
		}
	}

	// We don't update the connection status since it is done by the health go routine
	return nil
}

func purgeExternalCluster(clientset kubernetes.Interface, namespace string) {
	ctx := context.TODO()
	// Purge the config maps
	cmsToDelete := []string{
		mon.EndpointConfigMapName,
		k8sutil.ConfigOverrideName,
	}
	for _, cm := range cmsToDelete {
		err := clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, cm, metav1.DeleteOptions{})
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
		err := clientset.CoreV1().Secrets(namespace).Delete(ctx, secret, metav1.DeleteOptions{})
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

	// Validate external services port
	if cluster.Spec.Monitoring.Enabled {
		if cluster.Spec.Monitoring.ExternalMgrPrometheusPort == 0 {
			cluster.Spec.Monitoring.ExternalMgrPrometheusPort = mgr.DefaultMetricsPort
		}
	}

	return nil
}

func (c *ClusterController) configureExternalClusterMonitoring(context *clusterd.Context, cluster *cluster) error {
	// Initialize manager object
	manager := mgr.New(
		context,
		cluster.ClusterInfo,
		*cluster.Spec,
		"", // We don't need the image since we are not running any mgr deployment
	)

	// Create external monitoring Service
	service, err := manager.MakeMetricsService(opcontroller.ExternalMgrAppName, "", opcontroller.ServiceExternalMetricName)
	if err != nil {
		return err
	}
	logger.Info("creating mgr external monitoring service")
	_, err = k8sutil.CreateOrUpdateService(context.Clientset, cluster.Namespace, service)
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return errors.Wrap(err, "failed to create or update mgr service")
	}
	logger.Info("mgr external metrics service created")

	// Configure external metrics endpoint
	err = opcontroller.ConfigureExternalMetricsEndpoint(context, cluster.Spec.Monitoring, cluster.ClusterInfo, cluster.ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to configure external metrics endpoint")
	}

	// Deploy external ServiceMonittor
	logger.Info("creating external service monitor")
	// servicemonitor takes some metadata from the service for easy mapping
	err = manager.EnableServiceMonitor("")
	if err != nil {
		logger.Errorf("failed to enable external service monitor. %v", err)
	} else {
		logger.Info("external service monitor created")
	}

	// namespace in which the prometheusRule should be deployed
	// if left empty, it will be deployed in current namespace
	namespace := cluster.Spec.Monitoring.RulesNamespace
	if namespace == "" {
		namespace = cluster.Namespace
	}

	logger.Info("creating external prometheus rule")
	err = manager.DeployPrometheusRule(mgr.PrometheusExternalRuleName, namespace)
	if err != nil {
		logger.Errorf("failed to create external prometheus rule. %v", err)
	} else {
		logger.Info("external prometheus rule created")
	}

	return nil
}
