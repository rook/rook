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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// compile-time assertions ensures CephCluster implements webhook.Validator so a webhook builder
// will be registered for the validating webhook.
var _ webhook.Validator = &CephCluster{}

// RequireMsgr2 checks if the network settings require the msgr2 protocol
func (c *ClusterSpec) RequireMsgr2() bool {
	if c.Network.Connections == nil {
		return false
	}
	if c.Network.Connections.Compression != nil && c.Network.Connections.Compression.Enabled {
		return true
	}
	if c.Network.Connections.Encryption != nil && c.Network.Connections.Encryption.Enabled {
		return true
	}
	return false
}

func (c *ClusterSpec) IsStretchCluster() bool {
	return c.Mon.StretchCluster != nil && len(c.Mon.StretchCluster.Zones) > 0
}

func (c *CephCluster) ValidateCreate() error {
	logger.Infof("validate create cephcluster %q", c.ObjectMeta.Name)
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
	occ := old.(*CephCluster)
	return validateUpdatedCephCluster(c, occ)
}

func (c *CephCluster) ValidateDelete() error {
	return nil
}

func validateUpdatedCephCluster(updatedCephCluster *CephCluster, found *CephCluster) error {
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

func (c *CephCluster) GetStatusConditions() *[]Condition {
	return &c.Status.Conditions
}
