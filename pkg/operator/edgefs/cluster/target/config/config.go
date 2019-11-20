/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// Package config for Edgefs target config managed by the operator
package config

import (
	"strconv"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	configStoreNameFmt = "rook-edgefs-%s-config"
	osdDirsKeyName     = "target-dirs"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "target-config")

func GetConfigStoreName(nodeName string) string {
	return k8sutil.TruncateNodeName(configStoreNameFmt, nodeName)
}

const (
	RtVerifyChidKey        = "rtVerifyChid"
	MaxSizeGB              = "maxSizeGB"
	MDReserved             = "mdReserved"
	HDDReadAhead           = "hddReadAhead"
	LmdbPageSizeKey        = "lmdbPageSize"
	LmdbMdPageSizeKey      = "lmdbMdPageSize"
	UseBcacheKey           = "useBCache"
	UseBcacheWBKey         = "useBCacheWB"
	UseMetadataMaskKey     = "useMetadataMask"
	UseMetadataOffloadKey  = "useMetadataOffload"
	UseAllSSDKey           = "useAllSSD"
	RtPlevelOverrideKey    = "rtPLevelOverride"
	SyncKey                = "sync"
	ZoneKey                = "zone"
	UseRtkvsBackendKey     = "useRtkvsBackend"
	WalModeKey             = "walMode"
	PayloadS3URLKey        = "payloadS3URL"
	PayloadS3RegionKey     = "payloadS3Region"
	PayloadS3MinKbKey      = "payloadS3MinKb"
	PayloadS3CapacityKey   = "payloadS3CapacityGB"
	PayloadS3SecretKey     = "payloadS3Secret"
	PayloadS3SyncGetMaxKey = "payloadS3SyncGetMax"
)

type StoreConfig struct {
	// 0 (disabled), 1 (verify on write) or 2(verify on read/write)
	RtVerifyChid int `json:"rtVerifyChid,omitempty"`
	// 4096, 8192, 16384 or 32768
	LmdbPageSize int `json:"lmdbPageSize,omitempty"`
	// 4096, 8192, 16384 or 32768
	LmdbMdPageSize int `json:"lmdbMdPageSize,omitempty"`
	// in 10..99% of potential SSD partition
	MDReserved int `json:"mdReserved,omitempty"`
	// applies to data chunks on HDD partitions, in KBs
	HDDReadAhead int `json:"hddReadAhead,omitempty"`
	// rtlfs/rtkvs only, max size to use per directory (journal in rtkvs), in bytes
	MaxSize uint64 `json:"maxsize,omitempty"`
	// enable use of bcache
	UseBCache bool `json:"useBCache,omitempty"`
	// enable write back cache
	UseBCacheWB bool `json:"useBCacheWB,omitempty"`
	// what guts needs to go to SSD and what not
	UseMetadataMask string `json:"useMetadataMask,omitempty"`
	// when useAllSSD is false, enable metadata offload on SSD
	UseMetadataOffload bool `json:"useMetadataOffload,omitempty"`
	// only look for SSD/NVMe
	UseAllSSD bool `json:"useAllSSD,omitempty"`
	// if > 0, override automatic partitioning numbering logic
	RtPLevelOverride int `json:"rtPLevelOverride,omitempty"`
	// sync cluster option [0:3]
	Sync int `json:"sync"`
	// apply edgefs cluster zones id to whole cluster or node if zone value > 0
	Zone int `json:"zone,omitempty"`
	// Use RTKVS IO engine with specified backend name
	UseRtkvsBackend string `json:"useRtkvsBackend,omitempty"`
	// Write-ahead-log mode
	WalMode int `json:"walMode,omitempty"`
	// S3 payload bucket's URL
	PayloadS3URL string `json:"payloadS3URL,omitempty"`
	// S3 pyaload bucket's region
	PayloadS3Region string `json:"payloadS3Region,omitempty"`
	// Minimal payload size to be stored in a S3 bucket
	PayloadS3MinKb int `json:"payloadS3MinKb,omitempty"`
	// Capacity of the S3 paylod bucket
	PayloadS3Capacity int `json:"payloadS3Capacity,omitempty"`
	// k8s secret name to be used as a S3 secret file
	PayloadS3Secret string `json:"payloadS3Secret,omitempty"`
	// Maximum number of simultaneous S3 GET requests per node
	PayloadS3SyncGetMax int `json:"payloadS3SyncGetMax,omitempty"`
}

func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		RtVerifyChid:        1,
		LmdbPageSize:        16384,
		LmdbMdPageSize:      8192,
		MDReserved:          0,
		HDDReadAhead:        0,
		UseBCache:           false,
		UseBCacheWB:         false,
		UseMetadataMask:     "0xff",
		UseMetadataOffload:  false,
		UseAllSSD:           false,
		RtPLevelOverride:    0,
		Sync:                1,
		Zone:                0,
		UseRtkvsBackend:     "",
		WalMode:             0,
		PayloadS3URL:        "",
		PayloadS3Region:     "us-east-1",
		PayloadS3MinKb:      0,
		PayloadS3Capacity:   0,
		PayloadS3Secret:     "",
		PayloadS3SyncGetMax: 0,
	}
}

var validLmbdPageSize = map[int]bool{
	4096:  true,
	8192:  true,
	16384: true,
	32768: true,
}

var walModeMap = map[string]int{
	"on":       0,
	"off":      2,
	"metadata": 1,
}

func ToStoreConfig(config map[string]string) StoreConfig {
	storeConfig := DefaultStoreConfig()
	for k, v := range config {
		switch k {
		case RtVerifyChidKey:
			value := convertToIntIgnoreErr(v)
			if value >= 0 && value <= 2 {
				storeConfig.RtVerifyChid = value
			} else {
				logger.Warningf("Incorrect 'verifyChid' value %d ignored", value)
			}
		case MDReserved:
			value := convertToIntIgnoreErr(v)
			if value >= 10 && value <= 99 {
				storeConfig.MDReserved = value
			} else {
				logger.Warningf("Incorrect 'mdReserved' value %d ignored", value)
			}
		case HDDReadAhead:
			value := convertToIntIgnoreErr(v)
			if value >= 0 {
				storeConfig.HDDReadAhead = value
			} else {
				logger.Warningf("Incorrect 'hddReadAhead' value %d ignored", value)
			}
		case MaxSizeGB:
			value := convertToUint64IgnoreErr(v)
			if value > 0 {
				storeConfig.MaxSize = value * 1024 * 1024 * 1024
			} else {
				logger.Warningf("Incorrect 'MaxSizeGB' value %v ignored", value)
			}
		case LmdbPageSizeKey:
			value := convertToIntIgnoreErr(v)
			if validLmbdPageSize[value] {
				storeConfig.LmdbPageSize = value
			} else {
				logger.Warningf("Incorrect 'lmdbPageSize' value %d ignored", value)
			}
		case LmdbMdPageSizeKey:
			value := convertToIntIgnoreErr(v)
			if validLmbdPageSize[value] {
				storeConfig.LmdbMdPageSize = value
			} else {
				logger.Warningf("Incorrect 'lmdbMdPageSize' value %d ignored", value)
			}
		case UseBcacheKey:
			storeConfig.UseBCache = convertToBoolIgnoreErr(v)
		case UseBcacheWBKey:
			storeConfig.UseBCacheWB = convertToBoolIgnoreErr(v)
		case UseMetadataMaskKey:
			storeConfig.UseMetadataMask = v
		case UseMetadataOffloadKey:
			storeConfig.UseMetadataOffload = convertToBoolIgnoreErr(v)
		case UseAllSSDKey:
			storeConfig.UseAllSSD = convertToBoolIgnoreErr(v)
		case RtPlevelOverrideKey:
			storeConfig.RtPLevelOverride = convertToIntIgnoreErr(v)
		case SyncKey:
			value := convertToIntIgnoreErr(v)
			if value >= 0 && value <= 3 {
				storeConfig.Sync = value
			} else {
				logger.Warningf("Incorrect 'sync' value %d ignored", value)
			}
		case ZoneKey:
			value := convertToIntIgnoreErr(v)
			if value > 0 {
				storeConfig.Zone = value
			}
		case UseRtkvsBackendKey:
			storeConfig.UseRtkvsBackend = v
		case WalModeKey:
			if val, ok := walModeMap[v]; !ok {
				logger.Warningf("Incorrect 'walMode' value %s ignored", v)
			} else {
				storeConfig.WalMode = val
			}
		case PayloadS3URLKey:
			storeConfig.PayloadS3URL = v
		case PayloadS3SecretKey:
			storeConfig.PayloadS3Secret = v
		case PayloadS3RegionKey:
			storeConfig.PayloadS3Region = v
		case PayloadS3MinKbKey:
			value := convertToIntIgnoreErr(v)
			if value > 0 {
				storeConfig.PayloadS3MinKb = value
			}
		case PayloadS3CapacityKey:
			value := convertToIntIgnoreErr(v)
			if value > 0 {
				storeConfig.PayloadS3Capacity = value
			}
		case PayloadS3SyncGetMaxKey:
			value := convertToIntIgnoreErr(v)
			if value > 0 {
				storeConfig.PayloadS3SyncGetMax = value
			}
		}
	}

	return storeConfig
}

func convertToUint64IgnoreErr(raw string) uint64 {
	val, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		val = 0
	}

	return val
}

func convertToIntIgnoreErr(raw string) int {
	val, err := strconv.Atoi(raw)
	if err != nil {
		val = 0
	}

	return val
}

func convertToBoolIgnoreErr(raw string) bool {
	val, err := strconv.ParseBool(raw)
	if err != nil {
		val = false
	}

	return val
}
