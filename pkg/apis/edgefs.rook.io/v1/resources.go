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
package v1

import (
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strconv"
)

const (
	ResourcesKeyMgr    = "mgr"
	ResourcesKeyTarget = "target"

	hostLocalTimeVolName = "host-local-time"
	hostLocalTimePath    = "/etc/localtime"
)

// GetMgrResources returns the placement for the MGR service
func GetMgrResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyMgr]
}

// GetTargetResources returns the placement for the Targets
func GetTargetResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyTarget]
}

// GetInitiatorEnvArr returns array of EnvVar for all the Initiator based
// on configured resources and its profile
func GetInitiatorEnvArr(svctype string, embedded bool, chunkCacheSize resource.Quantity, resources v1.ResourceRequirements) []v1.EnvVar {
	retArr := []v1.EnvVar{}
	rMemLim := resources.Limits.Memory()
	if !rMemLim.IsZero() {
		// adjust chunk cache maximum size
		cacheSize := rMemLim.Value() * 75 / 100
		if embedded {
			if chunkCacheSize.IsZero() {
				// embedded default case, 100mb
				cacheSize = 100 * 1024 * 1024
			} else {
				if chunkCacheSize.CmpInt64(cacheSize) < 0 {
					// user wants to set custom that is less then 75% of total
					cacheSize = chunkCacheSize.Value()
				}
			}
		}

		if svctype == "target" {
			// adjust target's built-in cgroup
			cgroupTgt := rMemLim.Value() * 95 / 100
			if cgroupTgt <= cacheSize {
				// in targets case chunk cache only needed for
				// background operations. This was probably not
				// a user's intent, so adjust to a much lower values
				cacheSize = cgroupTgt / 8
			}
			retArr = append(retArr, v1.EnvVar{
				Name:  "CCOWD_CGROUP_MEMLIM",
				Value: strconv.FormatInt(cgroupTgt, 10),
			})
		} else if svctype == "isgw" {
			// ISGW doesn't need large chunk cache
			cacheSize = rMemLim.Value() * 25 / 100
		} else if svctype == "s3" || svctype == "swift" {
			// adjust restapi's node.js GC settings. This memory
			// will be shared among all the HTTP worker processes
			// and will be used to calculate optimal # of workers too.
			svcMem := rMemLim.Value() * 95 / 100
			if svcMem <= cacheSize {
				cacheSize = svcMem / 2
			}
			retArr = append(retArr, v1.EnvVar{
				Name:  "SVC_MEM_LIMIT",
				Value: strconv.FormatInt(svcMem, 10),
			})
			svcMemPerWorker := int64(1 * 1024 * 1024 * 1024)
			if embedded {
				svcMemPerWorker = int64(256 * 1024 * 1024)
			}
			if svcMem < svcMemPerWorker {
				svcMemPerWorker = svcMem / 2
			}
			retArr = append(retArr, v1.EnvVar{
				Name:  "SVC_MEM_PER_WORKER",
				Value: strconv.FormatInt(svcMemPerWorker, 10),
			})
		}

		retArr = append(retArr, v1.EnvVar{
			Name:  "CCOW_MEMORY_LIMIT",
			Value: strconv.FormatInt(cacheSize, 10),
		})
	}

	if embedded {
		retArr = append(retArr, v1.EnvVar{
			Name:  "CCOW_EMBEDDED",
			Value: "1",
		})
		retArr = append(retArr, v1.EnvVar{
			Name:  "JE_MALLOC_CONF",
			Value: "tcache:false",
		})
	}

	return retArr
}

func GetHostLocalTimeVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      hostLocalTimeVolName,
		MountPath: hostLocalTimePath,
		ReadOnly:  true,
	}
}

func GetHostLocalTimeVolume() v1.Volume {
	return v1.Volume{
		Name: hostLocalTimeVolName,
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: hostLocalTimePath,
			},
		},
	}
}
