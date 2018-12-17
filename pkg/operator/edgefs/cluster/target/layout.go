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

package target

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target/config"
	"github.com/rook/rook/pkg/util/sys"
)

type RTDevices struct {
	Devices []RTDevice `json:"devices"`
}

type RTDevice struct {
	Name              string `json:"name,omitempty"`
	Device            string `json:"device,omitempty"`
	Psize             int    `json:"psize,omitempty"`
	VerifyChid        int    `json:"verify_chid"`
	Journal           string `json:"journal,omitempty"`
	Metadata          string `json:"metadata,omitempty"`
	Bcache            int    `json:"bcache,omitempty"`
	BcacheWritearound int    `json:"bcache_writearound"`
	PlevelOverride    int    `json:"plevel_override,omitempty"`
	Sync              int    `json:"sync"`
}

func getIdDevLinkName(dls string) (dl string) {
	dlsArr := strings.Split(dls, " ")
	for i := range dlsArr {
		s := strings.Replace(dlsArr[i], "/dev/disk/by-id/", "", 1)
		if strings.Contains(s, "/") || strings.Contains(s, "wwn-") {
			continue
		}
		dl = s
		break
	}
	return dl
}

func GetRTDevices(nodeDisks []sys.LocalDisk, storeConfig *config.StoreConfig) (rtDevices []RTDevice, err error) {
	rtDevices = make([]RTDevice, 0)
	if storeConfig == nil {
		return rtDevices, fmt.Errorf("no pointer to StoreConfig provided")
	}

	if len(nodeDisks) == 0 {
		return rtDevices, nil
	}

	var ssds []sys.LocalDisk
	var hdds []sys.LocalDisk
	var devices []sys.LocalDisk

	for i := range nodeDisks {
		if !nodeDisks[i].Empty || len(nodeDisks[i].Partitions) > 0 {
			continue
		}
		if nodeDisks[i].Rotational {
			hdds = append(hdds, nodeDisks[i])
		} else {
			ssds = append(ssds, nodeDisks[i])
		}
		devices = append(devices, nodeDisks[i])
	}

	//var rtdevs []RTDevice
	if storeConfig.UseAllSSD {
		//
		// All flush media case (High Performance)
		//
		if len(ssds) == 0 {
			return rtDevices, fmt.Errorf("No SSD/NVMe media found")
		}
		if storeConfig.UseMetadataOffload {
			fmt.Println("Warning: useMetadataOffload parameter is ignored due to use useAllSSD=true")
		}

		for i := range devices {
			if devices[i].Rotational {
				continue
			}
			rtdev := RTDevice{
				Name:       getIdDevLinkName(devices[i].DevLinks),
				Device:     "/dev/" + devices[i].Name,
				Psize:      storeConfig.LmdbPageSize,
				VerifyChid: storeConfig.RtVerifyChid,
				Sync:       storeConfig.Sync,
			}
			if storeConfig.RtrdPLevelOverride != 0 {
				rtdev.PlevelOverride = storeConfig.RtrdPLevelOverride
			}
			rtDevices = append(rtDevices, rtdev)
		}
		return rtDevices, nil
	}

	if len(hdds) == 0 {
		return rtDevices, fmt.Errorf("No HDD media found")
	}

	if !storeConfig.UseMetadataOffload {
		//
		// All HDD media case (capacity, cold archive)
		//
		for i := range devices {
			if !devices[i].Rotational {
				continue
			}
			rtdev := RTDevice{
				Name:       getIdDevLinkName(devices[i].DevLinks),
				Device:     "/dev/" + devices[i].Name,
				Psize:      storeConfig.LmdbPageSize,
				VerifyChid: storeConfig.RtVerifyChid,
				Sync:       storeConfig.Sync,
			}
			if storeConfig.RtrdPLevelOverride != 0 {
				rtdev.PlevelOverride = storeConfig.RtrdPLevelOverride
			}
			rtDevices = append(rtDevices, rtdev)
		}
		return rtDevices, nil
	}

	//
	// Hybrid SSD/HDD media case (optimal)
	//
	if len(hdds) < len(ssds) || len(ssds) == 0 {
		return rtDevices, fmt.Errorf("Confusing use of useMetadataOffload parameter HDDs(%d) < SSDs(%d)\n", len(hdds), len(ssds))
	}

	chunkSize := int(math.Ceil(float64(len(hdds) / len(ssds))))
	var hdds_divided [][]sys.LocalDisk
	for i := 0; i < len(hdds); i += chunkSize {
		end := i + chunkSize

		if end > len(hdds) {
			end = len(hdds)
		}
		hdds_divided = append(hdds_divided, hdds[i:end])
	}

	for i := range hdds_divided {
		for j := range hdds_divided[i] {
			rtdev := RTDevice{
				Name:       getIdDevLinkName(hdds_divided[i][j].DevLinks),
				Device:     "/dev/" + hdds_divided[i][j].Name,
				Psize:      storeConfig.LmdbPageSize,
				VerifyChid: storeConfig.RtVerifyChid,
				Journal:    getIdDevLinkName(ssds[i].DevLinks),
				Metadata:   getIdDevLinkName(ssds[i].DevLinks) + "," + storeConfig.UseMetadataMask,
				Bcache:     0,
				Sync:       storeConfig.Sync,
			}
			if storeConfig.UseBCache {
				rtdev.Bcache = 1
				if storeConfig.UseBCacheWB {
					rtdev.BcacheWritearound = 0
				}
			}
			if storeConfig.RtrdPLevelOverride != 0 {
				rtdev.PlevelOverride = storeConfig.RtrdPLevelOverride
			}
			rtDevices = append(rtDevices, rtdev)
		}
	}
	return rtDevices, nil
}

func getRtlfsDevices(directories []rookalpha.Directory, storeConfig *config.StoreConfig) []RtlfsDevice {
	rtlfsDevices := make([]RtlfsDevice, 0)
	for _, dir := range directories {
		rtlfsDevice := RtlfsDevice{
			Name:            filepath.Base(dir.Path),
			Path:            dir.Path,
			CheckMountpoint: 1,
			Sync:            storeConfig.Sync,
		}
		rtlfsDevices = append(rtlfsDevices, rtlfsDevice)
	}
	return rtlfsDevices
}
