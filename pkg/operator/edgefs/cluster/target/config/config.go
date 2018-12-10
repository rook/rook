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
	RtVerifyChidKey       = "rtVerifyChid"
	LmdbPageSizeKey       = "lmdbPageSize"
	UseBcacheKey          = "useBCache"
	UseBcacheWBKey        = "useBCacheWB"
	UseMetadataMaskKey    = "useMetadataMask"
	UseMetadataOffloadKey = "useMetadataOffload"
	UseAllSSDKey          = "useAllSSD"
	RtrdPlevelOverrideKey = "rtrdPLevelOverride"
)

type StoreConfig struct {
	// 0 (disabled), 1 (verify on write) or 2(verify on read/write)
	RtVerifyChid int `json:"rtVerifyChid,omitempty"`
	// 4096, 8192, 16384 or 32768
	LmdbPageSize int `json:"lmdbPageSize,omitempty"`
	// enable use of back cache
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
	RtrdPLevelOverride int `json:"rtrdPLevelOverride,omitempty"`
}

func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		RtVerifyChid:       1,
		LmdbPageSize:       16348,
		UseBCacheWB:        false,
		UseMetadataMask:    "0xff",
		UseMetadataOffload: false,
		UseAllSSD:          false,
		RtrdPLevelOverride: 0,
	}
}

var validLmbdPageSize = map[int]bool{
	4096:  true,
	8192:  true,
	16384: true,
	32768: true,
}

func ToStoreConfig(config map[string]string) StoreConfig {
	storeConfig := DefaultStoreConfig()
	for k, v := range config {
		switch k {
		case RtVerifyChidKey:
			if storeConfig.RtVerifyChid < 0 || storeConfig.RtVerifyChid > 2 {
				storeConfig.RtVerifyChid = convertToIntIgnoreErr(v)
			}
		case LmdbPageSizeKey:

			if validLmbdPageSize[storeConfig.LmdbPageSize] {
				storeConfig.LmdbPageSize = convertToIntIgnoreErr(v)
			}
		case UseBcacheWBKey:
			storeConfig.UseBCacheWB = convertToBoolIgnoreErr(v)
		case UseMetadataMaskKey:
			storeConfig.UseMetadataMask = v
		case UseMetadataOffloadKey:
			storeConfig.UseMetadataOffload = convertToBoolIgnoreErr(v)
		case UseAllSSDKey:
			storeConfig.UseAllSSD = convertToBoolIgnoreErr(v)
		case RtrdPlevelOverrideKey:
			storeConfig.RtrdPLevelOverride = convertToIntIgnoreErr(v)
		}
	}

	return storeConfig
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
