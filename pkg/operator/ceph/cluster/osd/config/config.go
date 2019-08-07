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

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	OSDFSStoreNameFmt  = "rook-ceph-osd-%d-fs-backup"
	configStoreNameFmt = "rook-ceph-osd-%s-config"
	osdDirsKeyName     = "osd-dirs"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "osd-config")

func GetConfigStoreName(nodeName string) string {
	return k8sutil.TruncateNodeName(configStoreNameFmt, nodeName)
}

const (
	StoreTypeKey       = "storeType"
	WalSizeMBKey       = "walSizeMB"
	DatabaseSizeMBKey  = "databaseSizeMB"
	JournalSizeMBKey   = "journalSizeMB"
	OSDsPerDeviceKey   = "osdsPerDevice"
	EncryptedDeviceKey = "encryptedDevice"
	MetadataDeviceKey  = "metadataDevice"
	DeviceClassKey     = "deviceClass"
)

type StoreConfig struct {
	StoreType       string `json:"storeType,omitempty"`
	WalSizeMB       int    `json:"walSizeMB,omitempty"`
	DatabaseSizeMB  int    `json:"databaseSizeMB,omitempty"`
	JournalSizeMB   int    `json:"journalSizeMB,omitempty"`
	OSDsPerDevice   int    `json:"osdsPerDevice,omitempty"`
	EncryptedDevice bool   `json:"encryptedDevice,omitempty"`
	DeviceClass     string `json:"deviceClass,omitempty"`
}

func ToStoreConfig(config map[string]string) StoreConfig {
	storeConfig := StoreConfig{}
	for k, v := range config {
		switch k {
		case StoreTypeKey:
			storeConfig.StoreType = v
		case WalSizeMBKey:
			storeConfig.WalSizeMB = convertToIntIgnoreErr(v)
		case DatabaseSizeMBKey:
			storeConfig.DatabaseSizeMB = convertToIntIgnoreErr(v)
		case JournalSizeMBKey:
			storeConfig.JournalSizeMB = convertToIntIgnoreErr(v)
		case OSDsPerDeviceKey:
			storeConfig.OSDsPerDevice = convertToIntIgnoreErr(v)
		case EncryptedDeviceKey:
			storeConfig.EncryptedDevice = (v == "true")
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
