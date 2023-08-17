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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// compile-time assertions ensures CephCluster implements webhook.Validator so a webhook builder
// will be registered for the validating webhook.
var _ webhook.Validator = &CephCluster{}

// RequireMsgr2 checks if the network settings require the msgr2 protocol
func (c *ClusterSpec) RequireMsgr2() bool {
	if c.Network.Connections == nil {
		return false
	}
	if c.Network.Connections.RequireMsgr2 {
		return true
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

func (c *ClusterSpec) ZonesRequired() bool {
	return c.IsStretchCluster() || len(c.Mon.Zones) > 0
}

func (c *CephCluster) ValidateCreate() (admission.Warnings, error) {
	if c.Spec.ZonesRequired() {
		if len(c.Spec.Mon.Zones) < c.Spec.Mon.Count {
			return nil, errors.New("Not enough zones available for mons, please specify more zones")
		}
	}
	logger.Infof("validate create cephcluster %q", c.ObjectMeta.Name)
	//If external mode enabled, then check if other fields are empty
	if c.Spec.External.Enable {
		if !reflect.DeepEqual(c.Spec.Mon, MonSpec{}) || c.Spec.Dashboard != (DashboardSpec{}) || !reflect.DeepEqual(c.Spec.Monitoring, (MonitoringSpec{})) || c.Spec.DisruptionManagement != (DisruptionManagementSpec{}) || len(c.Spec.Mgr.Modules) > 0 || len(c.Spec.Network.Provider) > 0 || len(c.Spec.Network.Selectors) > 0 {
			return nil, errors.New("invalid create : external mode enabled cannot have mon,dashboard,monitoring,network,disruptionManagement,storage fields in CR")
		}
	}
	return nil, nil
}

func (c *CephCluster) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	logger.Infof("validate update cephcluster %q", c.ObjectMeta.Name)
	occ := old.(*CephCluster)
	return validateUpdatedCephCluster(c, occ)
}

func (c *CephCluster) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func validateUpdatedCephCluster(updatedCephCluster *CephCluster, found *CephCluster) (admission.Warnings, error) {
	if updatedCephCluster.Spec.DataDirHostPath != found.Spec.DataDirHostPath {
		return nil, errors.Errorf("invalid update: DataDirHostPath change from %q to %q is not allowed", found.Spec.DataDirHostPath, updatedCephCluster.Spec.DataDirHostPath)
	}

	// Allow an attempt to enable or disable host networking, but not other provider changes
	oldProvider := updatedCephCluster.Spec.Network.Provider
	newProvider := found.Spec.Network.Provider
	if oldProvider != newProvider && oldProvider != "host" && newProvider != "host" {
		return nil, errors.Errorf("invalid update: Provider change from %q to %q is not allowed", found.Spec.Network.Provider, updatedCephCluster.Spec.Network.Provider)
	}

	if len(updatedCephCluster.Spec.Storage.StorageClassDeviceSets) == len(found.Spec.Storage.StorageClassDeviceSets) {
		for i, storageClassDeviceSet := range found.Spec.Storage.StorageClassDeviceSets {
			if storageClassDeviceSet.Encrypted != updatedCephCluster.Spec.Storage.StorageClassDeviceSets[i].Encrypted {
				return nil, errors.Errorf("invalid update: StorageClassDeviceSet %q encryption change from %t to %t is not allowed", storageClassDeviceSet.Name, found.Spec.Storage.StorageClassDeviceSets[i].Encrypted, storageClassDeviceSet.Encrypted)
			}
		}
	}

	return nil, nil
}

func (c *CephCluster) GetStatusConditions() *[]Condition {
	return &c.Status.Conditions
}
