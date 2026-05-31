/*
Copyright 2024 The Rook Authors. All rights reserved.

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

// Package osd for the Ceph OSDs.
package osd

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// OSDMigrationConfirmation is the confirmation provided by the user in the cephCluster spec.
	OSDMigrationConfirmation = "yes-really-migrate-osds"
	// OSDUpdateStoreConfirmation is the confirmation provided by the user to updated OSD backend store
	OSDUpdateStoreConfirmation = "yes-really-update-store"
	// OSDMigrationConfigName is the configMap that stores the ID of the last migrated OSD
	OSDMigrationConfigName = "osd-migration-config"
	// OSDIdKey is the key used to store the OSD ID inside the `osd-migration-config` configMap
	OSDIdKey = "osdID"
)

// migrationConfig represents the OSDs that need migration
type migrationConfig struct {
	// osds that require migration (map key is the OSD id)
	osds map[int]*OSDInfo
}

func (c *Cluster) newMigrationConfig() (*migrationConfig, error) {
	mc := migrationConfig{
		osds: map[int]*OSDInfo{},
	}

	osdDeployments, err := c.getOSDDeployments()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get existing OSD deployments in namespace %q", c.clusterInfo.Namespace)
	}

	// get OSDs that require migration due to change in encryption settings
	err = mc.migrateForEncryption(c, osdDeployments)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get OSDs that require migration due to change in encryption setting")
	}

	// get OSDs that require migration due the change in OSD store type settings
	err = mc.migrateForOSDStore(c, osdDeployments)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get OSDs that require migration due to change in OSD Store type setting")
	}

	return &mc, nil
}

// migrateForEncryption gets all the OSDs that require migration due to change in the cephCluster encryption setting
func (m *migrationConfig) migrateForEncryption(c *Cluster, osdDeployments *appsv1.DeploymentList) error {
	deviceSetMap := map[string]cephv1.StorageClassDeviceSet{}
	for i := range c.spec.Storage.StorageClassDeviceSets {
		deviceSetMap[c.spec.Storage.StorageClassDeviceSets[i].Name] = c.spec.Storage.StorageClassDeviceSets[i]
	}

	for i := range osdDeployments.Items {
		osdDeviceSetName := osdDeployments.Items[i].Labels[CephDeviceSetLabelKey]
		requestedEncryptionSetting := deviceSetMap[osdDeviceSetName].Encrypted
		actualEncryptedSetting := false
		if osdDeployments.Items[i].Labels["encrypted"] == "true" {
			actualEncryptedSetting = true
		}

		if requestedEncryptionSetting != actualEncryptedSetting {
			osdInfo, err := c.getOSDInfo(&osdDeployments.Items[i])
			if err != nil {
				return errors.Wrapf(err, "failed to details about the OSD %q", osdDeployments.Items[i].Name)
			}
			log.NamespacedInfo(c.clusterInfo.Namespace, logger, "migration is required for OSD.%d due to change in encryption settings from %t to %t in storageClassDeviceSet %q", osdInfo.ID, actualEncryptedSetting, requestedEncryptionSetting, osdDeviceSetName)
			if _, exists := m.osds[osdInfo.ID]; !exists {
				m.osds[osdInfo.ID] = &osdInfo
			}
		}
	}
	return nil
}

// migrateForOSDStore gets all the OSDs that require migration due to change in the cephCluster OSD storeType setting
func (m *migrationConfig) migrateForOSDStore(c *Cluster, osdDeployments *appsv1.DeploymentList) error {
	desiredOSDStore := c.spec.Storage.GetOSDStore()
	for i := range osdDeployments.Items {
		if osdStore, ok := osdDeployments.Items[i].Labels[osdStore]; ok {
			if osdStore != desiredOSDStore {
				osdInfo, err := c.getOSDInfo(&osdDeployments.Items[i])
				if err != nil {
					return errors.Wrapf(err, "failed to details about the OSD %q", osdDeployments.Items[i].Name)
				}
				log.NamespacedInfo(c.clusterInfo.Namespace, logger, "migration is required for OSD.%d to update storeType from %q to %q", osdInfo.ID, osdStore, desiredOSDStore)
				if _, exists := m.osds[osdInfo.ID]; !exists {
					m.osds[osdInfo.ID] = &osdInfo
				}
			}
		}
	}
	return nil
}

// getOSDToMigrate returns the next OSD to migrate from the list of OSDs that are pending migration.
func (m *migrationConfig) getOSDToMigrate() *OSDInfo {
	osdInfo := &OSDInfo{}
	osdID := -1
	for k, v := range m.osds {
		osdID, osdInfo = k, v
		break
	}
	delete(m.osds, osdID)
	return osdInfo
}

func (m *migrationConfig) getOSDIds() []int {
	osdIds := make([]int, len(m.osds))
	i := 0
	for k := range m.osds {
		osdIds[i] = k
		i++
	}
	return osdIds
}

// saveMigrationConfig saves the ID of the migrated OSD to a configMap
func saveMigrationConfig(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, osdID int) error {
	newConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OSDMigrationConfigName,
			Namespace: clusterInfo.Namespace,
		},
		Data: map[string]string{
			OSDIdKey: strconv.Itoa(osdID),
		},
	}

	err := clusterInfo.OwnerInfo.SetControllerReference(newConfigMap)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference on %q configMap", newConfigMap.Name)
	}

	_, err = k8sutil.CreateOrUpdateConfigMap(clusterInfo.Context, context.Clientset, newConfigMap)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update %q configMap", newConfigMap.Name)
	}

	return nil
}

// isLastOSDMigrationComplete checks if the deployment for the migrated OSD got created successfully.
func isLastOSDMigrationComplete(c *Cluster) (bool, error) {
	migratedOSDId, err := getLastMigratedOSDId(c.context, c.clusterInfo)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get last migrated OSD ID")
	}

	if migratedOSDId == -1 {
		return true, nil
	}

	deploymentName := fmt.Sprintf("rook-ceph-osd-%d", migratedOSDId)
	_, err = c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Get(c.clusterInfo.Context, deploymentName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			log.NamespacedInfo(c.clusterInfo.Namespace, logger, "deployment for the last migrated OSD with ID - %d is not found.", migratedOSDId)
			return false, nil
		}
	}

	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "migration of OSD.%d was successful", migratedOSDId)
	return true, nil
}

// getLastMigratedOSDId fetches the ID of the last migrated OSD from the "osd-migration-config" configmap
func getLastMigratedOSDId(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) (int, error) {
	cm, err := context.Clientset.CoreV1().ConfigMaps(clusterInfo.Namespace).Get(clusterInfo.Context, OSDMigrationConfigName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return -1, nil
		}
	}

	osdID, ok := cm.Data[OSDIdKey]
	if !ok || osdID == "" {
		log.NamespacedDebug(clusterInfo.Namespace, logger, "empty config map %q", OSDMigrationConfigName)
		return -1, nil
	}

	osdIDInt, err := strconv.Atoi(osdID)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to convert OSD id %q to integer.", osdID)
	}

	return osdIDInt, nil
}
