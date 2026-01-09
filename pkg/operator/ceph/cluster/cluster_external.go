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

	cephcsi "github.com/ceph/ceph-csi/api/deploy/kubernetes"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/nodedaemon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
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

	opcontroller.UpdateCondition(c.OpManagerCtx, c.context, cluster.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionConnecting, v1.ConditionTrue, cephv1.ClusterConnectingReason, "Attempting to connect to an external Ceph cluster")

	// loop until we find the secret necessary to connect to the external cluster
	// then populate clusterInfo
	cluster.ClusterInfo, err = opcontroller.PopulateExternalClusterInfo(cluster.Spec, c.context, c.OpManagerCtx, cluster.namespacedName.Namespace, cluster.ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to populate external cluster info")
	}
	cluster.ClusterInfo.SetName(cluster.namespacedName.Name)
	cluster.ClusterInfo.Context = c.OpManagerCtx

	if !client.IsKeyringBase64Encoded(cluster.ClusterInfo.CephCred.Secret) {
		return errors.Errorf("invalid user health checker key for user %q", cluster.ClusterInfo.CephCred.Username)
	}

	// Write connection info (ceph config file and keyring) for ceph commands
	if cluster.Spec.CephVersion.Image == "" {
		err = mon.WriteConnectionConfig(c.context, cluster.ClusterInfo)
		if err != nil {
			log.NamespacedError(cluster.Namespace, logger, "failed to write config. attempting to continue. %v", err)
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
		log.NamespacedInfo(cluster.Namespace, logger, "creating %q configmap", k8sutil.ConfigOverrideName)
		err = populateConfigOverrideConfigMap(c.context, cluster.namespacedName.Namespace, cluster.ClusterInfo.OwnerInfo, cluster.clusterMetadata)
		if err != nil {
			return errors.Wrap(err, "failed to populate config override config map")
		}

		log.NamespacedInfo(cluster.Namespace, logger, "creating %q secret", config.StoreName)
		err = config.GetStore(c.context, cluster.namespacedName.Namespace, cluster.ClusterInfo.OwnerInfo).CreateOrUpdate(cluster.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to update the global config")
		}
	}

	// The cluster Identity must be established at this point
	if err := cluster.ClusterInfo.IsInitialized(); err != nil {
		return errors.Wrap(err, "the cluster identity was not established")
	}
	log.NamespacedInfo(cluster.Namespace, logger, "external cluster identity established")

	// update the msgr2 flag
	for _, m := range cluster.ClusterInfo.InternalMonitors {
		// m.Endpoint=10.1.115.104:3300
		monPort := util.GetPortFromEndpoint(m.Endpoint)
		if monPort == client.Msgr2port {
			if cluster.Spec.Network.Connections == nil {
				cluster.Spec.Network.Connections = &cephv1.ConnectionsSpec{}
			}
			cluster.Spec.Network.Connections.RequireMsgr2 = true
			log.NamespacedDebug(cluster.Namespace, logger, "a v2 port was found for a mon endpoint, so msgr2 is required")
			break
		}
	}

	// Save CSI configmap
	monEndpoints := csi.MonEndpoints(cluster.ClusterInfo.InternalMonitors, cluster.Spec.RequireMsgr2())
	csiConfigEntry := &csi.CSIClusterConfigEntry{
		Namespace: cluster.ClusterInfo.Namespace,
		ClusterInfo: cephcsi.ClusterInfo{
			Monitors: monEndpoints,
		},
	}

	clusterId := cluster.namespacedName.Namespace // cluster id is same as cluster namespace for CephClusters
	err = csi.SaveClusterConfig(c.context.Clientset, clusterId, cluster.namespacedName.Namespace, cluster.ClusterInfo, csiConfigEntry)
	if err != nil {
		return errors.Wrap(err, "failed to update csi cluster config")
	}
	log.NamespacedInfo(cluster.Namespace, logger, "successfully updated csi config map")

	// Create Crash Collector Secret
	if !cluster.Spec.CrashCollector.Disable {
		err = nodedaemon.CreateCrashCollectorSecret(c.context, cluster.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to create crash collector kubernetes secret")
		}
	}
	// Create exporter secret
	if !cluster.Spec.Monitoring.MetricsDisabled {
		if cluster.ClusterInfo.CephCred.Username == client.AdminUsername &&
			cluster.ClusterInfo.CephCred.Secret != opcontroller.AdminSecretNameKey {

			err = nodedaemon.CreateExporterSecret(c.context, cluster.ClusterInfo)
			if err != nil {
				return errors.Wrap(err, "failed to create exporter kubernetes secret")
			}
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
		c.updateClusterCephVersion(cluster, *externalVersion)

		err = c.configureExternalClusterMonitoring(c.context, cluster)
		if err != nil {
			return errors.Wrap(err, "failed to configure external cluster monitoring")
		}
	}

	if csi.EnableCSIOperator() {
		log.NamespacedInfo(cluster.Namespace, logger, "create cephConnection and defaultClientProfile for external mode")
		err = csi.CreateUpdateCephConnection(c.context.Client, cluster.ClusterInfo, *cluster.Spec)
		if err != nil {
			return errors.Wrap(err, "failed to create/update cephConnection")
		}
		err = csi.CreateDefaultClientProfile(c.context.Client, cluster.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to create/update default client profile")
		}
	}

	opcontroller.UpdateCondition(c.OpManagerCtx, c.context, cluster.namespacedName, k8sutil.ObservedGenerationNotAvailable, cephv1.ConditionConnected, v1.ConditionTrue, cephv1.ClusterConnectedReason, "Cluster connected successfully")
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
			log.NamespacedError(namespace, logger, "failed to delete config map %q. %v", cm, err)
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
			log.NamespacedError(namespace, logger, "failed to delete secret %q. %v", secret, err)
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
	service, err := manager.MakeMetricsService(opcontroller.ExternalMgrAppName, opcontroller.ServiceExternalMetricName)
	if err != nil {
		return err
	}
	log.NamespacedInfo(cluster.Namespace, logger, "creating mgr external monitoring service")
	_, err = k8sutil.CreateOrUpdateService(cluster.ClusterInfo.Context, context.Clientset, cluster.Namespace, service)
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return errors.Wrap(err, "failed to create or update mgr service")
	}
	log.NamespacedInfo(cluster.Namespace, logger, "mgr external metrics service created")

	// Configure external metrics endpoint
	err = opcontroller.ConfigureExternalMetricsEndpoint(context, cluster.Spec.Monitoring, cluster.ClusterInfo, cluster.ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to configure external metrics endpoint")
	}

	// Deploy external ServiceMonitor
	log.NamespacedInfo(cluster.Namespace, logger, "creating external service monitor")
	// servicemonitor takes some metadata from the service for easy mapping
	err = manager.EnableServiceMonitor()
	if err != nil {
		log.NamespacedError(cluster.Namespace, logger, "failed to enable external service monitor. %v", err)
	} else {
		log.NamespacedInfo(cluster.Namespace, logger, "external service monitor created")
	}
	return nil
}
