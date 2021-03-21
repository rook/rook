/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package config for OSD config managed by the operator
package config

import (
	"strconv"

	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	OSDFSStoreNameFmt  = "rook-ceph-osd-%d-fs-backup"
	configStoreNameFmt = "rook-ceph-osd-%s-config"
)

func GetConfigStoreName(nodeName string) string {
	return k8sutil.TruncateNodeName(configStoreNameFmt, nodeName)
}

const (
	WalSizeMBKey       = "walSizeMB"
	DatabaseSizeMBKey  = "databaseSizeMB"
	JournalSizeMBKey   = "journalSizeMB"
	OSDsPerDeviceKey   = "osdsPerDevice"
	EncryptedDeviceKey = "encryptedDevice"
	MetadataDeviceKey  = "metadataDevice"
	DeviceClassKey     = "deviceClass"
)

// StoreConfig represents the configuration of an OSD on a device.
type StoreConfig struct {
	WalSizeMB       int    `json:"walSizeMB,omitempty"`
	DatabaseSizeMB  int    `json:"databaseSizeMB,omitempty"`
	OSDsPerDevice   int    `json:"osdsPerDevice,omitempty"`
	EncryptedDevice bool   `json:"encryptedDevice,omitempty"`
	MetadataDevice  string `json:"metadataDevice,omitempty"`
	DeviceClass     string `json:"deviceClass,omitempty"`
}

// NewStoreConfig returns a StoreConfig with proper defaults set.
func NewStoreConfig() StoreConfig {
	return StoreConfig{
		OSDsPerDevice: 1,
	}
}

// ToStoreConfig converts a config string-string map to a StoreConfig.
func ToStoreConfig(config map[string]string) StoreConfig {
	storeConfig := NewStoreConfig()
	for k, v := range config {
		switch k {
		case WalSizeMBKey:
			storeConfig.WalSizeMB = convertToIntIgnoreErr(v)
		case DatabaseSizeMBKey:
			storeConfig.DatabaseSizeMB = convertToIntIgnoreErr(v)
		case OSDsPerDeviceKey:
			i := convertToIntIgnoreErr(v)
			if i > 0 { // only allow values 1 or more to be set
				storeConfig.OSDsPerDevice = i
			}
		case EncryptedDeviceKey:
			storeConfig.EncryptedDevice = (v == "true")
		case MetadataDeviceKey:
			storeConfig.MetadataDevice = v
		case DeviceClassKey:
			storeConfig.DeviceClass = v
		}
	}

	return storeConfig
}

func MetadataDevice(config map[string]string) string {
	for k, v := range config {
		switch k {
		case MetadataDeviceKey:
			return v
		}
	}

	return ""
}

func convertToIntIgnoreErr(raw string) int {
	val, err := strconv.Atoi(raw)
	if err != nil {
		val = 0
	}

	return val
}

// ConfiguredDevice is a device with a corresponding configuration.
type ConfiguredDevice struct {
	ID          string      `json:"id"`
	StoreConfig StoreConfig `json:"storeConfig"`
}
