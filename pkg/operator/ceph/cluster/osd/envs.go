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

package osd

import (
	"strconv"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"gopkg.in/ini.v1"
	v1 "k8s.io/api/core/v1"
)

const (
	osdDatabaseSizeEnvVarName = "ROOK_OSD_DATABASE_SIZE"
	osdWalSizeEnvVarName      = "ROOK_OSD_WAL_SIZE"
	osdsPerDeviceEnvVarName   = "ROOK_OSDS_PER_DEVICE"
	osdDeviceClassEnvVarName  = "ROOK_OSD_DEVICE_CLASS"
	osdConfigMapOverrideName  = "rook-ceph-osd-env-override"
	// EncryptedDeviceEnvVarName is used in the pod spec to indicate whether the OSD is encrypted or not
	EncryptedDeviceEnvVarName = "ROOK_ENCRYPTED_DEVICE"
	PVCNameEnvVarName         = "ROOK_PVC_NAME"
	// CephVolumeEncryptedKeyEnvVarName is the env variable used by ceph-volume to encrypt the OSD (raw mode)
	// Hardcoded in ceph-volume do NOT touch
	CephVolumeEncryptedKeyEnvVarName = "CEPH_VOLUME_DMCRYPT_SECRET"
	osdMetadataDeviceEnvVarName      = "ROOK_METADATA_DEVICE"
	osdWalDeviceEnvVarName           = "ROOK_WAL_DEVICE"
	// PVCBackedOSDVarName indicates whether the OSD is on PVC ("true") or not ("false")
	PVCBackedOSDVarName                 = "ROOK_PVC_BACKED_OSD"
	blockPathVarName                    = "ROOK_BLOCK_PATH"
	cvModeVarName                       = "ROOK_CV_MODE"
	lvBackedPVVarName                   = "ROOK_LV_BACKED_PV"
	CrushDeviceClassVarName             = "ROOK_OSD_CRUSH_DEVICE_CLASS"
	CrushInitialWeightVarName           = "ROOK_OSD_CRUSH_INITIAL_WEIGHT"
	OSDStoreTypeVarName                 = "ROOK_OSD_STORE_TYPE"
	ReplaceOSDIDVarName                 = "ROOK_REPLACE_OSD"
	CrushRootVarName                    = "ROOK_CRUSHMAP_ROOT"
	tcmallocMaxTotalThreadCacheBytesEnv = "TCMALLOC_MAX_TOTAL_THREAD_CACHE_BYTES"
)

var cephEnvConfigFile = "/etc/sysconfig/ceph"

func (c *Cluster) getConfigEnvVars(osdProps osdProperties, dataDir string, prepare bool) []v1.EnvVar {
	envVars := []v1.EnvVar{
		nodeNameEnvVar(osdProps.crushHostname),
		{Name: "ROOK_CLUSTER_ID", Value: string(c.clusterInfo.OwnerInfo.GetUID())},
		{Name: "ROOK_CLUSTER_NAME", Value: string(c.clusterInfo.NamespacedName().Name)},
		k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
		k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
		opmon.PodNamespaceEnvVar(c.clusterInfo.Namespace),
		opmon.EndpointEnvVar(),
		k8sutil.ConfigDirEnvVar(dataDir),
		k8sutil.ConfigOverrideEnvVar(),
		k8sutil.NodeEnvVar(),
		{Name: CrushRootVarName, Value: client.GetCrushRootFromSpec(&c.spec)},
	}
	if prepare {
		envVars = append(envVars, []v1.EnvVar{
			opmon.CephUsernameEnvVar(),
			{Name: "ROOK_FSID", ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{Name: "rook-ceph-mon"},
					Key:                  "fsid",
				},
			}},
		}...)

		envVars = append(envVars, osdStoreTypeEnvVar(c.spec.Storage.GetOSDStore()))
	}

	// Give a hint to the prepare pod for what the host in the CRUSH map should be
	crushmapHostname := osdProps.crushHostname
	if !osdProps.portable && osdProps.onPVC() {
		// If it's a pvc that's not portable we only know what the host name should be when inside the osd prepare pod
		crushmapHostname = ""
	}
	envVars = append(envVars, v1.EnvVar{Name: "ROOK_CRUSHMAP_HOSTNAME", Value: crushmapHostname})

	// Append ceph-volume environment variables
	envVars = append(envVars, cephVolumeEnvVar()...)

	if osdProps.storeConfig.DatabaseSizeMB != 0 {
		envVars = append(envVars, v1.EnvVar{Name: osdDatabaseSizeEnvVarName, Value: strconv.Itoa(osdProps.storeConfig.DatabaseSizeMB)})
	}

	if osdProps.storeConfig.WalSizeMB != 0 {
		envVars = append(envVars, v1.EnvVar{Name: osdWalSizeEnvVarName, Value: strconv.Itoa(osdProps.storeConfig.WalSizeMB)})
	}

	if osdProps.storeConfig.OSDsPerDevice != 0 {
		envVars = append(envVars, v1.EnvVar{Name: osdsPerDeviceEnvVarName, Value: strconv.Itoa(osdProps.storeConfig.OSDsPerDevice)})
	}

	if osdProps.storeConfig.EncryptedDevice {
		envVars = append(envVars, v1.EnvVar{Name: EncryptedDeviceEnvVarName, Value: "true"})
	}

	return envVars
}

func nodeNameEnvVar(name string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_NODE_NAME", Value: name}
}

func dataDevicesEnvVar(dataDevices string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DEVICES", Value: dataDevices}
}

func deviceFilterEnvVar(filter string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DEVICE_FILTER", Value: filter}
}

func devicePathFilterEnvVar(filter string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DEVICE_PATH_FILTER", Value: filter}
}

func dataDeviceClassEnvVar(deviceClass string) v1.EnvVar {
	return v1.EnvVar{Name: osdDeviceClassEnvVarName, Value: deviceClass}
}

func metadataDeviceEnvVar(metadataDevice string) v1.EnvVar {
	return v1.EnvVar{Name: osdMetadataDeviceEnvVarName, Value: metadataDevice}
}

func walDeviceEnvVar(walDevice string) v1.EnvVar {
	return v1.EnvVar{Name: osdWalDeviceEnvVarName, Value: walDevice}
}

func pvcBackedOSDEnvVar(pvcBacked string) v1.EnvVar {
	return v1.EnvVar{Name: PVCBackedOSDVarName, Value: pvcBacked}
}

func setDebugLogLevelEnvVar(debug bool) v1.EnvVar {
	level := "INFO"
	if debug {
		level = "DEBUG"
	}
	return v1.EnvVar{Name: "ROOK_LOG_LEVEL", Value: level}
}

func blockPathEnvVariable(lvPath string) v1.EnvVar {
	return v1.EnvVar{Name: blockPathVarName, Value: lvPath}
}

func cvModeEnvVariable(cvMode string) v1.EnvVar {
	return v1.EnvVar{Name: cvModeVarName, Value: cvMode}
}

func lvBackedPVEnvVar(lvBackedPV string) v1.EnvVar {
	return v1.EnvVar{Name: lvBackedPVVarName, Value: lvBackedPV}
}

func crushDeviceClassEnvVar(crushDeviceClass string) v1.EnvVar {
	return v1.EnvVar{Name: CrushDeviceClassVarName, Value: crushDeviceClass}
}

func osdStoreTypeEnvVar(storeType string) v1.EnvVar {
	return v1.EnvVar{Name: OSDStoreTypeVarName, Value: storeType}
}

func replaceOSDIDEnvVar(id string) v1.EnvVar {
	return v1.EnvVar{Name: ReplaceOSDIDVarName, Value: id}
}

func crushInitialWeightEnvVar(crushInitialWeight string) v1.EnvVar {
	return v1.EnvVar{Name: CrushInitialWeightVarName, Value: crushInitialWeight}
}

func encryptedDeviceEnvVar(encryptedDevice bool) v1.EnvVar {
	return v1.EnvVar{Name: EncryptedDeviceEnvVarName, Value: strconv.FormatBool(encryptedDevice)}
}

func pvcNameEnvVar(pvcName string) v1.EnvVar {
	return v1.EnvVar{Name: PVCNameEnvVarName, Value: pvcName}
}

func cephVolumeEnvVar() []v1.EnvVar {
	return []v1.EnvVar{
		{Name: "CEPH_VOLUME_DEBUG", Value: "1"},
		{Name: "CEPH_VOLUME_SKIP_RESTORECON", Value: "1"},
		// LVM will avoid interaction with udev.
		// LVM will manage the relevant nodes in /dev directly.
		{Name: "DM_DISABLE_UDEV", Value: "1"},
	}
}

func osdActivateEnvVar() []v1.EnvVar {
	monEnvVars := []v1.EnvVar{
		{
			Name: "ROOK_CEPH_MON_HOST",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: "rook-ceph-config",
					},
					Key: "mon_host",
				},
			},
		},
		{Name: "CEPH_ARGS", Value: "-m $(ROOK_CEPH_MON_HOST)"},
	}

	return append(cephVolumeEnvVar(), monEnvVars...)
}

func getEnvFromSources() []v1.EnvFromSource {
	optionalConfigMapRef := true

	return []v1.EnvFromSource{
		{
			ConfigMapRef: &v1.ConfigMapEnvSource{
				LocalObjectReference: v1.LocalObjectReference{Name: osdConfigMapOverrideName},
				Optional:             &optionalConfigMapRef,
			},
		},
	}
}

func getTcmallocMaxTotalThreadCacheBytes(tcmallocMaxTotalThreadCacheBytes string) v1.EnvVar {
	var value string
	// If empty we read the default value from the file coming with the package
	if tcmallocMaxTotalThreadCacheBytes == "" {
		value = getTcmallocMaxTotalThreadCacheBytesFromFile()
	} else {
		value = tcmallocMaxTotalThreadCacheBytes
	}

	return v1.EnvVar{Name: tcmallocMaxTotalThreadCacheBytesEnv, Value: value}
}

func getTcmallocMaxTotalThreadCacheBytesFromFile() string {
	iniCephEnvConfigFile, err := ini.Load(cephEnvConfigFile)
	if err != nil {
		return ""
	}

	return iniCephEnvConfigFile.Section("").Key(tcmallocMaxTotalThreadCacheBytesEnv).String()
}
