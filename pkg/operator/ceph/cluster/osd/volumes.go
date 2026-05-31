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
	"fmt"
	"path"
	"path/filepath"

	kms "github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

const (
	udevPath             = "/run/udev"
	udevVolName          = "run-udev"
	osdEncryptionVolName = "osd-encryption-key"
	dmPath               = "/dev/mapper"
	dmVolName            = "dev-mapper"
)

func getPvcOSDBridgeMount(claimName string) v1.VolumeMount {
	return v1.VolumeMount{
		Name:      fmt.Sprintf("%s-bridge", claimName),
		MountPath: "/mnt",
	}
}

func getPvcOSDBridgeMountActivate(mountPath, claimName string) v1.VolumeMount {
	return v1.VolumeMount{
		Name:      fmt.Sprintf("%s-bridge", claimName),
		MountPath: mountPath,
		SubPath:   path.Base(mountPath),
	}
}

func getPvcMetadataOSDBridgeMount(claimName string) v1.VolumeMount {
	return v1.VolumeMount{
		Name:      fmt.Sprintf("%s-bridge", claimName),
		MountPath: "/srv",
	}
}

func getPvcWalOSDBridgeMount(claimName string) v1.VolumeMount {
	return v1.VolumeMount{
		Name:      fmt.Sprintf("%s-bridge", claimName),
		MountPath: "/wal",
	}
}

func getDeviceMapperMount() v1.VolumeMount {
	return v1.VolumeMount{
		MountPath: dmPath,
		Name:      dmVolName,
	}
}

func getDeviceMapperVolume() (v1.Volume, v1.VolumeMount) {
	volume := v1.Volume{
		Name: dmVolName,
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{Path: dmPath},
		},
	}

	volumeMounts := v1.VolumeMount{
		Name:      dmVolName,
		MountPath: dmPath,
	}

	return volume, volumeMounts
}

func getDataBridgeVolumeSource(claimName, configDir, namespace string, inProvisioning bool) v1.VolumeSource {
	var source v1.VolumeSource
	if inProvisioning {
		source.EmptyDir = &v1.EmptyDirVolumeSource{
			Medium: "Memory",
		}
	} else {
		// We need to use hostPath to prevent multiple OSD pods from launching the same OSD and causing corruption.
		// Ceph avoids this problem by locking fsid file and block device file under the data bridge volume directory.
		// These locks are released by kernel once the process is gone, so until the ceph-osd daemon alives, the other
		// pods (same OSD) will not be able to acquire them and will continue to be restarted.
		// If we use emptyDir, this exclusive control doesn't work because the lock files aren't shared between OSD pods.
		hostPathType := v1.HostPathDirectoryOrCreate
		source.HostPath = &v1.HostPathVolumeSource{
			Path: filepath.Join(
				configDir,
				namespace,
				claimName),
			Type: &hostPathType,
		}
	}
	return source
}

func getPVCOSDVolumes(osdProps *osdProperties, configDir string, namespace string, prepare bool) []v1.Volume {
	volumes := []v1.Volume{
		{
			Name: osdProps.pvc.ClaimName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &osdProps.pvc,
			},
		},
		{
			// We need a bridge mount which is basically a common volume mount between the non privileged init container
			// and the privileged provision container or osd daemon container
			// The reason for this is mentioned in the comment for getPVCInitContainer() method
			Name:         fmt.Sprintf("%s-bridge", osdProps.pvc.ClaimName),
			VolumeSource: getDataBridgeVolumeSource(osdProps.pvc.ClaimName, configDir, namespace, prepare),
		},
	}

	// If we have a metadata PVC let's add it
	if osdProps.onPVCWithMetadata() {
		metadataPVCVolume := []v1.Volume{
			{
				Name: osdProps.metadataPVC.ClaimName,
				VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &osdProps.metadataPVC,
				},
			},
			{
				// We need a bridge mount which is basically a common volume mount between the non privileged init container
				// and the privileged provision container or osd daemon container
				// The reason for this is mentioned in the comment for getPVCInitContainer() method
				Name:         fmt.Sprintf("%s-bridge", osdProps.metadataPVC.ClaimName),
				VolumeSource: getDataBridgeVolumeSource(osdProps.metadataPVC.ClaimName, configDir, namespace, prepare),
			},
		}

		volumes = append(volumes, metadataPVCVolume...)
	}

	// If we have a wal PVC let's add it
	if osdProps.onPVCWithWal() {
		walPVCVolume := []v1.Volume{
			{
				Name: osdProps.walPVC.ClaimName,
				VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &osdProps.walPVC,
				},
			},
			{
				// We need a bridge mount which is basically a common volume mount between the non privileged init container
				// and the privileged provision container or osd daemon container
				// The reason for this is mentioned in the comment for getPVCInitContainer() method
				Name:         fmt.Sprintf("%s-bridge", osdProps.walPVC.ClaimName),
				VolumeSource: getDataBridgeVolumeSource(osdProps.walPVC.ClaimName, configDir, namespace, prepare),
			},
		}

		volumes = append(volumes, walPVCVolume...)
	}

	log.NamespacedDebug(namespace, logger, "volumes are %+v", volumes)

	return volumes
}

func getUdevVolume() (v1.Volume, v1.VolumeMount) {
	volume := v1.Volume{
		Name: udevVolName,
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{Path: udevPath},
		},
	}

	volumeMounts := v1.VolumeMount{
		Name:      udevVolName,
		MountPath: udevPath,
	}

	return volume, volumeMounts
}

func (c *Cluster) getEncryptionVolume(osdProps osdProperties) (v1.Volume, v1.VolumeMount) {
	// Generate volume
	var m int32 = 0o400
	volume := v1.Volume{
		Name: osdEncryptionVolName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: kms.GenerateOSDEncryptionSecretName(osdProps.pvc.ClaimName),
				Items: []v1.KeyToPath{
					{
						Key:  kms.OsdEncryptionSecretNameKeyName,
						Path: encryptionKeyFileName,
					},
				},
				DefaultMode: &m,
			},
		},
	}

	// On the KMS use case, we want the volume mount to be in memory since we pass write the KEK
	if c.spec.Security.KeyManagementService.IsEnabled() {
		volume.VolumeSource.Secret = nil
		volume.VolumeSource = v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{
				Medium: v1.StorageMediumMemory,
			},
		}
	}

	// Mounts /etc/ceph/luks_key
	volumeMounts := v1.VolumeMount{
		Name:      osdEncryptionVolName,
		ReadOnly:  true,
		MountPath: config.EtcCephDir,
	}

	// With KMS we must be able to write inside the directory to write the KEK
	if c.spec.Security.KeyManagementService.IsEnabled() {
		volumeMounts.ReadOnly = false
	}

	return volume, volumeMounts
}
