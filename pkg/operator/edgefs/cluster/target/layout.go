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

package target

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	edgefsv1beta1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target/config"
	"github.com/rook/rook/pkg/util/sys"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	DefaultContainerMaxCapacity = "132Ti"
)

func ByteCountBinary(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// CreateQualifiedHeadlessServiceName creates a qualified name of the headless service for a given replica id and namespace,
// e.g., edgefs-0.edgefs.rook-edgefs
func CreateQualifiedHeadlessServiceName(replicaNum int, namespace string) string {
	return fmt.Sprintf("%s-%d.%s.%s", appName, replicaNum, appName, namespace)
}

// EdgeFS RT-RD driver needs SCSI-3, ATA, NVMe by-id link
func getIdDevLinkName(dls string) (dl string) {
	dlsArr := strings.Split(dls, " ")
	for i := range dlsArr {
		s := strings.Replace(dlsArr[i], "/dev/disk/by-id/", "", 1)
		// if result contains "/" then it is not by-id, so skip it
		if strings.Contains(s, "/") || strings.Contains(s, "wwn-") || strings.Contains(s, "nvme-nvme.") || strings.Contains(s, "nvme-eui.") {
			continue
		}
		dl = s
		break
	}
	return dl
}

type ContainerDevices struct {
	Ssds []sys.LocalDisk
	Hdds []sys.LocalDisk
}

func GetContainers(maxContainerCapacity int64, devices []sys.LocalDisk, storeConfig *config.StoreConfig) ([]ContainerDevices, error) {
	var ssds []sys.LocalDisk
	var hdds []sys.LocalDisk

	var maxCap uint64
	maxCap = uint64(maxContainerCapacity)
	if maxCap == 0 {
		maxCapQuantity, err := resource.ParseQuantity(DefaultContainerMaxCapacity)
		if err != nil {
			return nil, err
		}
		maxCap = uint64(maxCapQuantity.Value())
	}
	var totalCapacity uint64
	var containerDevicesSize uint64

	for i := range devices {
		if !devices[i].Empty || len(devices[i].Partitions) > 0 {
			continue
		}
		if devices[i].Rotational {
			if !storeConfig.UseAllSSD {
				totalCapacity += devices[i].Size
			}
			hdds = append(hdds, devices[i])
		} else {
			if storeConfig.UseAllSSD || storeConfig.UseMetadataOffload {
				totalCapacity += devices[i].Size
			}
			ssds = append(ssds, devices[i])
		}
	}

	// maps for already selected devices
	selectedSsdsDevices := make([]bool, len(ssds))
	for _, ssd := range ssds {
		logger.Infof("Type: SSD, Name: %s, Size: %s", ssd.Name, ByteCountBinary(ssd.Size))
	}
	selectedHddsDevices := make([]bool, len(hdds))
	for _, hdd := range hdds {
		logger.Infof("Type: HDD, Name: %s, Size: %s", hdd.Name, ByteCountBinary(hdd.Size))
	}

	logger.Infof("MaxContainerCapacity: %s", ByteCountBinary(maxCap))
	logger.Infof("TotalCapacity: %s", ByteCountBinary(totalCapacity))

	if totalCapacity == 0 {
		return nil, fmt.Errorf("No available disks for container")
	}

	// calculates max containers count for avalable devices
	numContainers := int(math.Ceil(float64(totalCapacity) / float64(maxCap)))
	maxSSDPerContainer := int(math.Ceil(float64(len(ssds)) / float64(numContainers)))
	maxHDDPerContainer := int(math.Ceil(float64(len(hdds)) / float64(numContainers)))

	containerDevices := make([]ContainerDevices, numContainers)
	for i := 0; i < len(containerDevices); i++ {
		containerDevicesSize = 0

		// get SSDs for container
		for ssdDevIndex, ssdDev := range ssds {
			//check device has been already selected
			if selectedSsdsDevices[ssdDevIndex] {
				continue
			}
			if containerDevicesSize+ssdDev.Size > maxCap {
				continue
			}

			if len(containerDevices[i].Ssds)+1 > maxSSDPerContainer {
				continue
			}

			containerDevicesSize += ssdDev.Size
			selectedSsdsDevices[ssdDevIndex] = true
			containerDevices[i].Ssds = append(containerDevices[i].Ssds, ssdDev)
		}
		// get HDDs for container
		for hddDevIndex, hddDev := range hdds {
			//check device has been already selected
			if selectedHddsDevices[hddDevIndex] {
				continue
			}
			if containerDevicesSize+hddDev.Size > maxCap {
				continue
			}

			if len(containerDevices[i].Hdds)+1 > maxHDDPerContainer {
				continue
			}

			containerDevicesSize += hddDev.Size
			selectedHddsDevices[hddDevIndex] = true
			containerDevices[i].Hdds = append(containerDevices[i].Hdds, hddDev)
		}

	}
	return containerDevices, nil
}

func GetContainersRTDevices(nodeName string, maxContainerCapacity int64, nodeDisks []sys.LocalDisk, storeConfig *config.StoreConfig) (rtDevices []edgefsv1beta1.RTDevices, err error) {
	if storeConfig == nil {
		return rtDevices, fmt.Errorf("no pointer to StoreConfig provided")
	}

	if len(nodeDisks) == 0 {
		return rtDevices, nil
	}

	logger.Infof("[%s] available devices:", nodeName)
	containers, err := GetContainers(maxContainerCapacity, nodeDisks, storeConfig)
	if err != nil {
		return nil, err
	}

	containersRtDevices := make([]edgefsv1beta1.RTDevices, len(containers))
	for i, container := range containers {
		rtDevices, err := getRTDevices(container, storeConfig)
		if err != nil {
			return nil, err
		}

		// Just for debugging
		for _, rt := range rtDevices {
			logger.Infof("[%s] Container[%d] Device: %s, Name: %s, Journal: %s", nodeName, i, rt.Device, rt.Name, rt.Journal)
		}
		containersRtDevices[i].Devices = rtDevices
	}
	return containersRtDevices, nil
}

func getRTDevices(cntDevs ContainerDevices, storeConfig *config.StoreConfig) (rtDevices []edgefsv1beta1.RTDevice, err error) {
	rtDevices = make([]edgefsv1beta1.RTDevice, 0)

	if storeConfig.UseAllSSD {
		//
		// All flush media case (High Performance)
		//
		if len(cntDevs.Ssds) == 0 {
			return rtDevices, fmt.Errorf("No SSD/NVMe media found")
		}
		if storeConfig.UseMetadataOffload {
			fmt.Println("Warning: useMetadataOffload parameter is ignored due to use useAllSSD=true")
		}

		for i := range cntDevs.Ssds {
			rtdev := edgefsv1beta1.RTDevice{
				Name:       getIdDevLinkName(cntDevs.Ssds[i].DevLinks),
				Device:     "/dev/" + cntDevs.Ssds[i].Name,
				Psize:      storeConfig.LmdbPageSize,
				VerifyChid: storeConfig.RtVerifyChid,
				Sync:       storeConfig.Sync,
			}
			if storeConfig.RtPLevelOverride != 0 {
				rtdev.PlevelOverride = storeConfig.RtPLevelOverride
			}
			rtDevices = append(rtDevices, rtdev)
		}
		return rtDevices, nil
	}

	if len(cntDevs.Hdds) == 0 {
		return rtDevices, fmt.Errorf("No HDD media found")
	}

	if !storeConfig.UseMetadataOffload {
		//
		// All HDD media case (capacity, cold archive)
		//
		for i := range cntDevs.Hdds {
			rtdev := edgefsv1beta1.RTDevice{
				Name:         getIdDevLinkName(cntDevs.Hdds[i].DevLinks),
				Device:       "/dev/" + cntDevs.Hdds[i].Name,
				Psize:        storeConfig.LmdbPageSize,
				VerifyChid:   storeConfig.RtVerifyChid,
				HDDReadAhead: storeConfig.HDDReadAhead,
				Sync:         storeConfig.Sync,
			}
			if storeConfig.RtPLevelOverride != 0 {
				rtdev.PlevelOverride = storeConfig.RtPLevelOverride
			}
			rtDevices = append(rtDevices, rtdev)
		}
		return rtDevices, nil
	}

	//
	// Hybrid SSD/HDD media case (optimal)
	//
	if len(cntDevs.Hdds) < len(cntDevs.Ssds) || len(cntDevs.Ssds) == 0 {
		return rtDevices, fmt.Errorf("Confusing use of useMetadataOffload parameter HDDs(%d) < SSDs(%d)\n", len(cntDevs.Hdds), len(cntDevs.Ssds))
	}

	var hdds_divided [][]sys.LocalDisk
	for i := len(cntDevs.Ssds); i > 0; i-- {
		chunkSize := len(cntDevs.Hdds) / i
		mod := len(cntDevs.Hdds) % i
		if mod > 0 {
			chunkSize++
		}

		if len(cntDevs.Hdds) < chunkSize {
			chunkSize = len(cntDevs.Hdds)
		}
		hdds_divided = append(hdds_divided, cntDevs.Hdds[:chunkSize])
		cntDevs.Hdds = cntDevs.Hdds[chunkSize:]
	}

	for i := range hdds_divided {
		for j := range hdds_divided[i] {
			rtdev := edgefsv1beta1.RTDevice{
				Name:              getIdDevLinkName(hdds_divided[i][j].DevLinks),
				Device:            "/dev/" + hdds_divided[i][j].Name,
				Psize:             storeConfig.LmdbPageSize,
				VerifyChid:        storeConfig.RtVerifyChid,
				HDDReadAhead:      storeConfig.HDDReadAhead,
				BcacheWritearound: (map[bool]int{true: 0, false: 1})[storeConfig.UseBCacheWB],
				Journal:           getIdDevLinkName(cntDevs.Ssds[i].DevLinks),
				Metadata:          getIdDevLinkName(cntDevs.Ssds[i].DevLinks) + "," + storeConfig.UseMetadataMask,
				Bcache:            0,
				Sync:              storeConfig.Sync,
			}

			if storeConfig.UseBCache {
				rtdev.Bcache = 1
				if storeConfig.UseBCacheWB {
					rtdev.BcacheWritearound = 0
				}
			}

			if storeConfig.RtPLevelOverride != 0 {
				rtdev.PlevelOverride = storeConfig.RtPLevelOverride
			}
			rtDevices = append(rtDevices, rtdev)
		}
	}
	return rtDevices, nil
}

func GetRtlfsDevices(directories []rookalpha.Directory, storeConfig *config.StoreConfig) []edgefsv1beta1.RtlfsDevice {
	rtlfsDevices := make([]edgefsv1beta1.RtlfsDevice, 0)
	for _, dir := range directories {
		rtlfsDevice := edgefsv1beta1.RtlfsDevice{
			Name:            filepath.Base(dir.Path),
			Path:            dir.Path,
			CheckMountpoint: 0,
			Psize:           storeConfig.LmdbPageSize,
			VerifyChid:      storeConfig.RtVerifyChid,
			Sync:            storeConfig.Sync,
		}
		if storeConfig.MaxSize != 0 {
			rtlfsDevice.Maxsize = storeConfig.MaxSize
		}
		if storeConfig.RtPLevelOverride != 0 {
			rtlfsDevice.PlevelOverride = storeConfig.RtPLevelOverride
		}
		rtlfsDevices = append(rtlfsDevices, rtlfsDevice)
	}
	return rtlfsDevices
}
