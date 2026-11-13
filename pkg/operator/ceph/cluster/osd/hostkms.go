/*
Copyright 2026 The Rook Authors. All rights reserved.

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
	"context"
	_ "embed"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	hostEncryptionOpenInitContainer = "encryption-open-host"
	hostEncryptionKeyVolumeName     = "osd-encryption-key"
	hostEncryptionKeyDir            = "/var/lib/rook-encryption-key"
)

//go:embed open-host-encrypted-block.sh
var openHostEncryptedBlockCode string

// hostOSDUsesKMS reports whether the OSD is an encrypted host-based raw mode OSD, whose key is in
// the KMS. Encrypted host OSDs only use raw mode with a KMS, so this does not depend on the
// current KMS settings: an OSD whose KMS settings were removed fails to open, and says why.
func hostOSDUsesKMS(osd OSDInfo, onPVC bool) bool {
	return !onPVC && osd.Encrypted && osd.CVMode == "raw"
}

// getHostEncryptionOpenInitContainer returns the init container that fetches the key of an
// encrypted host-based OSD from the KMS and opens its device, and the volumes it needs
func (c *Cluster) getHostEncryptionOpenInitContainer(osd OSDInfo, osdProps osdProperties) ([]v1.Volume, v1.Container) {
	keyVolume := v1.Volume{
		Name:         hostEncryptionKeyVolumeName,
		VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{Medium: v1.StorageMediumMemory}},
	}
	volumes := append([]v1.Volume{keyVolume}, c.kmsVolumes()...)

	envVars := c.getConfigEnvVars(osdProps, "", false)
	envVars = append(envVars, kms.ConfigToEnvVar(c.spec)...)
	envVars = append(envVars,
		blockPathEnvVariable(osd.BlockPath),
		v1.EnvVar{Name: "ROOK_OSD_UUID", Value: osd.UUID},
		v1.EnvVar{Name: "ROOK_ENCRYPTION_KEY_DIR", Value: hostEncryptionKeyDir},
	)

	mounts := []v1.VolumeMount{
		{Name: hostEncryptionKeyVolumeName, MountPath: hostEncryptionKeyDir},
		{Name: "devices", MountPath: "/dev"},
	}
	mounts = append(mounts, c.kmsVolumeMounts()...)

	return volumes, v1.Container{
		Name:            hostEncryptionOpenInitContainer,
		Image:           c.rookVersion,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(c.spec.CephVersion.ImagePullPolicy),
		Command:         []string{"/bin/bash", "-c", openHostEncryptedBlockCode},
		Env:             envVars,
		VolumeMounts:    mounts,
		SecurityContext: controller.PrivilegedContext(true),
		Resources:       osdProps.resources,
	}
}

// checkHostOSDKMS returns an error, and records a warning event on the CephCluster, if security.kms
// is not configured for an OSD whose key is kept in a KMS
func (c *Cluster) checkHostOSDKMS(osd OSDInfo) error {
	if c.spec.Security.KeyManagementService.IsEnabled() {
		return nil
	}
	err := errors.Errorf("not creating encrypted OSD %d: its key is kept in a KMS, but security.kms is not configured", osd.ID)
	if c.Recorder != nil {
		cephCluster := &cephv1.CephCluster{}
		if getErr := c.context.Client.Get(c.clusterInfo.Context, c.clusterInfo.NamespacedName(), cephCluster); getErr != nil {
			log.NamespacedWarning(c.clusterInfo.Namespace, logger, "failed to get the CephCluster to record an event. %v", getErr)
		} else {
			c.Recorder.Eventf(cephCluster, nil, v1.EventTypeWarning, "OSDKMSNotConfigured", "Reconcile", "%v", err)
		}
	}
	return err
}

// storeHostOSDKeyInKMS copies the encryption key of a new encrypted host-based OSD from the mons
// into the KMS under the OSD fsid. "ceph-volume raw prepare" sends the key to the mons with
// "ceph osd new", which is how it reaches the operator; the mon copy is kept, as for PVC OSDs.
// The key is read back, since IBM Key Protect's PutSecret returns no error on an existing alias.
func (c *Cluster) storeHostOSDKeyInKMS(osd OSDInfo) error {
	// kms.NewConfig would fall back to Kubernetes Secrets
	if err := c.checkHostOSDKMS(osd); err != nil {
		return err
	}
	kmsConfig, err := kms.NewOperatorConfig(c.clusterInfo.Context, c.context, &c.spec, c.clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to configure the KMS for the encryption key of OSD %d", osd.ID)
	}
	if key, err := kmsConfig.GetSecret(osd.UUID); err == nil && key != "" {
		return nil
	}

	configKey := fmt.Sprintf("dm-crypt/osd/%s/luks", osd.UUID)
	key, err := cephclient.NewCephCommand(c.context, c.clusterInfo, []string{"config-key", "get", configKey}).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to read the encryption key of OSD %d from the mons", osd.ID)
	}
	if err := kmsConfig.PutSecret(osd.UUID, string(key)); err != nil {
		return errors.Wrapf(err, "failed to store the encryption key of OSD %d in the %q KMS", osd.ID, kmsConfig.Provider)
	}
	if stored, err := kmsConfig.GetSecret(osd.UUID); err != nil || stored != string(key) {
		return errors.Errorf("failed to verify the encryption key of OSD %d in the %q KMS. %v", osd.ID, kmsConfig.Provider, err)
	}
	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "stored the encryption key of OSD %d in the %q KMS", osd.ID, kmsConfig.Provider)
	return nil
}

// HostOSDKeyNames returns the names of the KMS keys of the encrypted host-based OSDs that have a
// deployment in the namespace. shortcut: OSDs whose deployment is already gone are not listed; list
// the keys from the KMS if they turn out to remain.
func HostOSDKeyNames(ctx context.Context, clusterdContext *clusterd.Context, namespace string) ([]string, error) {
	selector := fmt.Sprintf("%s=%s,%s=true", k8sutil.AppAttr, AppName, encrypted)
	deployments, err := clusterdContext.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list encrypted osd deployments")
	}
	names := []string{}
	for i := range deployments.Items {
		d := &deployments.Items[i]
		container, err := findOSDContainer(d.Spec.Template.Spec.Containers)
		if err != nil {
			log.NamespacedWarning(namespace, logger, "not deleting the encryption key of OSD deployment %q. %v", d.Name, err)
			continue
		}
		// the env parsing of getOSDInfo() fails on more than this needs, and queries the nodes
		osd := OSDInfo{Encrypted: true}
		for _, env := range container.Env {
			switch env.Name {
			case "ROOK_OSD_UUID":
				osd.UUID = env.Value
			case cvModeVarName:
				osd.CVMode = env.Value
			}
		}
		_, onPVC := d.Labels[OSDOverPVCLabelKey]
		if hostOSDUsesKMS(osd, onPVC) && osd.UUID != "" {
			names = append(names, osd.UUID)
		}
	}
	return names, nil
}
