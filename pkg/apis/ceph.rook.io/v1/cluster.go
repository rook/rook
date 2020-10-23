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

package v1

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// compile-time assertions ensures CephCluster implements webhook.Validator so a webhook builder
// will be registered for the validating webhook.
var _ webhook.Validator = &CephCluster{}

func (c *ClusterSpec) IsStretchCluster() bool {
	return c.Mon.StretchCluster != nil && len(c.Mon.StretchCluster.Zones) > 0
}

func (c *CephCluster) ValidateCreate() error {
	logger.Infof("validate create cephcluster %q", c.ObjectMeta.Name)

	if err := validateCommon(*c); err != nil {
		return err
	}

	//If external mode enabled, then check if other fields are empty
	if c.Spec.External.Enable {
		if c.Spec.Mon != (MonSpec{}) || c.Spec.Dashboard != (DashboardSpec{}) || !reflect.DeepEqual(c.Spec.Monitoring, (MonitoringSpec{})) || c.Spec.DisruptionManagement != (DisruptionManagementSpec{}) || len(c.Spec.Mgr.Modules) > 0 || len(c.Spec.Network.Provider) > 0 || len(c.Spec.Network.Selectors) > 0 {
			return errors.New("invalid create : external mode enabled cannot have mon,dashboard,monitoring,network,disruptionManagement,storage fields in CR")
		}
	}
	return nil
}

func (c *CephCluster) ValidateUpdate(old runtime.Object) error {
	logger.Infof("validate update cephcluster %q", c.ObjectMeta.Name)

	if err := validateCommon(*c); err != nil {
		return err
	}

	occ := old.(*CephCluster)
	return validateUpdatedCephCluster(c, occ)
}

func (c *CephCluster) ValidateDelete() error {
	return nil
}

func validateUpdatedCephCluster(updatedCephCluster *CephCluster, found *CephCluster) error {
	if updatedCephCluster.Spec.Mon.Count > 0 && updatedCephCluster.Spec.Mon.Count%2 == 0 {
		return errors.Errorf("mon count %d cannot be even, must be odd to support a healthy quorum", updatedCephCluster.Spec.Mon.Count)
	}

	if updatedCephCluster.Spec.DataDirHostPath != found.Spec.DataDirHostPath {
		return errors.Errorf("invalid update: DataDirHostPath change from %q to %q is not allowed", found.Spec.DataDirHostPath, updatedCephCluster.Spec.DataDirHostPath)
	}

	if updatedCephCluster.Spec.Network.HostNetwork != found.Spec.Network.HostNetwork {
		return errors.Errorf("invalid update: HostNetwork change from %q to %q is not allowed", strconv.FormatBool(found.Spec.Network.HostNetwork), strconv.FormatBool(updatedCephCluster.Spec.Network.HostNetwork))
	}

	if updatedCephCluster.Spec.Network.Provider != found.Spec.Network.Provider {
		return errors.Errorf("invalid update: Provider change from %q to %q is not allowed", found.Spec.Network.Provider, updatedCephCluster.Spec.Network.Provider)
	}

	for i, storageClassDeviceSet := range updatedCephCluster.Spec.Storage.StorageClassDeviceSets {
		if storageClassDeviceSet.Encrypted != found.Spec.Storage.StorageClassDeviceSets[i].Encrypted {
			return errors.Errorf("invalid update: StorageClassDeviceSet %q encryption change from %t to %t is not allowed", storageClassDeviceSet.Name, found.Spec.Storage.StorageClassDeviceSets[i].Encrypted, storageClassDeviceSet.Encrypted)
		}
	}

	return nil
}

// Validate resources that need validated for both creates and updates
func validateCommon(cluster CephCluster) error {
	// If drive groups are set, only storage for OSDs on PVCs can be used simultaneously
	if len(cluster.Spec.DriveGroups) > 0 {
		invalidConfigs := []string{}
		if cluster.Spec.Storage.UseAllNodes {
			invalidConfigs = append(invalidConfigs, "storage:useAllNodes is true")
		}
		if cluster.Spec.Storage.NodeCount > 0 {
			invalidConfigs = append(invalidConfigs, "storage:nodeCount is set")
		}
		if *cluster.Spec.Storage.UseAllDevices {
			invalidConfigs = append(invalidConfigs, "storage:useAllDevices is true")
		}
		if len(cluster.Spec.Storage.Nodes) > 0 {
			invalidConfigs = append(invalidConfigs, "storage:nodes are set")
		}
		if cluster.Spec.Storage.DeviceFilter != "" {
			invalidConfigs = append(invalidConfigs, "storage:deviceFilter is set")
		}
		if cluster.Spec.Storage.DevicePathFilter != "" {
			invalidConfigs = append(invalidConfigs, "storage:devicePathFilter is set")
		}
		if len(cluster.Spec.Storage.Devices) > 0 {
			invalidConfigs = append(invalidConfigs, "storage:devices are set")
		}
		if len(cluster.Spec.Storage.Config) > 0 {
			invalidConfigs = append(invalidConfigs, "storage:config is set")
		}

		if len(invalidConfigs) > 0 {
			j := strings.Join(invalidConfigs, ", ")
			return errors.Errorf("invalid config : cannot specify driveGroups along with any of the following current configs: %s", j)
		}
	}

	return nil
}
