package keyring

import (
	"path"

	v1 "k8s.io/api/core/v1"
)

const (
	keyringDir = "/etc/ceph/keyring-store/"

	// admin keyring path must be different from keyring path so that the two keyrings can be
	// mounted independently
	adminKeyringDir          = "/etc/ceph/admin-keyring-store/"
	crashCollectorKeyringDir = "/etc/ceph/crash-collector-keyring-store/"
)

// VolumeBuilder is a helper for creating Kubernetes pod volumes with content sourced by keyrings
// stored in the SecretStore.
type VolumeBuilder struct{}

// VolumeMountBuilder is a helper for creating Kubernetes container volume mounts that mount the
// keyring content from VolumeBuilder volumes.
type VolumeMountBuilder struct{}

// Volume returns a VolumeBuilder.
func Volume() *VolumeBuilder { return &VolumeBuilder{} }

// Resource returns a Kubernetes pod volume whose content is sourced by the keyring created for the
// resource using a SecretStore.
func (v *VolumeBuilder) Resource(resourceName string) v1.Volume {
	return v1.Volume{
		Name: keyringSecretName(resourceName),
		VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
			SecretName: keyringSecretName(resourceName),
		}},
	}
}

// Admin returns a kubernetes pod volume whose content is sourced by the SecretStore admin keyring.
func (v *VolumeBuilder) Admin() v1.Volume {
	return v.Resource(adminKeyringResourceName)
}

// CrashCollector returns a kubernetes pod volume whose content is sourced by the SecretStore crash collector keyring.
func (v *VolumeBuilder) CrashCollector() v1.Volume {
	return v.Resource(crashCollectorKeyringResourceName)
}

// VolumeMount returns a VolumeMountBuilder.
func VolumeMount() *VolumeMountBuilder { return &VolumeMountBuilder{} }

// Resource returns a Kubernetes container volume mount that mounts the content from the matching
// VolumeBuilder Resource volume for the same resource.
func (*VolumeMountBuilder) Resource(resourceName string) v1.VolumeMount {
	return v1.VolumeMount{
		Name:      keyringSecretName(resourceName),
		ReadOnly:  true, // should be no reason to write to the keyring in pods, so enforce this
		MountPath: keyringDir,
	}
}

// Admin returns a Kubernetes container volume mount that mounts the content from the matching
// VolumeBuilder Admin volume.
func (*VolumeMountBuilder) Admin() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      keyringSecretName(adminKeyringResourceName),
		ReadOnly:  true, // should be no reason to write to the keyring in pods, so enforce this
		MountPath: adminKeyringDir,
	}
}

// CrashCollector returns a Kubernetes container volume mount that mounts the content from the matching
// VolumeBuilder Crash Collector volume.
func (*VolumeMountBuilder) CrashCollector() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      keyringSecretName(crashCollectorKeyringResourceName),
		ReadOnly:  true, // should be no reason to write to the keyring in pods, so enforce this
		MountPath: crashCollectorKeyringDir,
	}
}

// KeyringFilePath returns the full path to the regular keyring file within a container.
func (*VolumeMountBuilder) KeyringFilePath() string {
	return path.Join(keyringDir, keyringFileName)
}

// AdminKeyringFilePath returns the full path to the admin keyring file within a container.
func (*VolumeMountBuilder) AdminKeyringFilePath() string {
	return path.Join(adminKeyringDir, keyringFileName)
}

// CrashCollectorKeyringFilePath returns the full path to the admin keyring file within a container.
func (*VolumeMountBuilder) CrashCollectorKeyringFilePath() string {
	return path.Join(crashCollectorKeyringDir, keyringFileName)
}
