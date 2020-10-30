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

	"github.com/libopenstorage/secrets"
	kms "github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/config"
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

func getPVCOSDVolumes(osdProps *osdProperties) []v1.Volume {
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
			Name: fmt.Sprintf("%s-bridge", osdProps.pvc.ClaimName),
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{
					Medium: "Memory",
				},
			},
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
				Name: fmt.Sprintf("%s-bridge", osdProps.metadataPVC.ClaimName),
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{
						Medium: "Memory",
					},
				},
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
				Name: fmt.Sprintf("%s-bridge", osdProps.walPVC.ClaimName),
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{
						Medium: "Memory",
					},
				},
			},
		}

		volumes = append(volumes, walPVCVolume...)
	}

	logger.Debugf("volumes are %+v", volumes)

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
	// Determine whether we have a KMS configuration
	var isKMS bool
	if len(c.spec.Security.KeyManagementService.ConnectionDetails) != 0 {
		provider := kms.GetParam(c.spec.Security.KeyManagementService.ConnectionDetails, kms.Provider)
		if provider == secrets.TypeVault {
			isKMS = true
		}
	}

	// Generate volume
	var m int32 = 0400
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
	if isKMS {
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
	if isKMS {
		volumeMounts.ReadOnly = false
	}

	return volume, volumeMounts
}
