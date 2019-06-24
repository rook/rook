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

// Package config allows a ceph config file to be stored in Kubernetes and mounted as volumes into
// Ceph daemon containers.
package config

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/go-ini/ini"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	storeName = "rook-ceph-config"

	configVolumeName = "rook-ceph-config"

	confFileName         = "ceph.conf"
	monHostKey           = "mon_host"
	monInitialMembersKey = "mon_initial_members"
	// Msgr2port is the listening port of the messenger v2 protocol
	Msgr2port = 3300
)

// Store manages storage of the Ceph config file shared by all daemons (if applicable) as well as an
// updated 'mon_host' which can be mapped to daemon containers and referenced in daemon command line
// arguments.
type Store struct {
	configMapStore *k8sutil.ConfigMapKVStore
	namespace      string
	context        *clusterd.Context
	ownerRef       *metav1.OwnerReference
}

// GetStore returns the Store for the cluster.
func GetStore(context *clusterd.Context, namespace string, ownerRef *metav1.OwnerReference) *Store {
	return &Store{
		configMapStore: k8sutil.NewConfigMapKVStore(namespace, context.Clientset, *ownerRef),
		namespace:      namespace,
		context:        context,
		ownerRef:       ownerRef,
	}
}

// CreateOrUpdate creates or updates the stored Ceph config based on the cluster info.
func (s *Store) CreateOrUpdate(clusterInfo *cephconfig.ClusterInfo) error {
	c := DefaultCentralizedConfigs(clusterInfo.CephVersion)

	// DefaultLegacyConfigs need to be added to the Ceph config file until the integration tests can be
	// made to override these options for the Ceph clusters it creates.
	c.Merge(DefaultLegacyConfigs())

	/* TODO: config overrides from the CRD will go here */

	f, err := c.IniFile()
	if err != nil {
		return fmt.Errorf("failed to store the Ceph config file. could not generate default ini. %+v", err)
	}

	// merge config overrides from the legacy override configmap
	b := new(bytes.Buffer)
	if err := s.applyLegacyOverrides(f); err != nil {
		logger.Warningf("failed to merge config override with default Ceph config file; using only the default config. %+v", err)
		f, err = c.IniFile() // regenerate the orig ini in case override attempt altered the struct
		if err != nil {
			return fmt.Errorf("failed to store the Ceph config file. could not generate default ini after merge failure. %+v", err)
		}
	}
	if _, err := f.WriteTo(b); err != nil {
		return fmt.Errorf("failed to convert merged Ceph config to text; continuing with original config. %+v", err)
	}
	txt := b.String()

	// Store the config in a configmap
	if err := s.configMapStore.SetValue(storeName, confFileName, txt); err != nil {
		return fmt.Errorf("failed to store the Ceph config file. failed to store config to configmap. %+v", err)
	}
	logger.Debugf("Generated and stored config file:\n%s", txt)

	// these are used for all ceph daemons on the commandline and must *always* be stored
	if err := s.createOrUpdateMonHostSecrets(clusterInfo); err != nil {
		return fmt.Errorf("failed to store mon host configs. %+v", err)
	}

	/*
		TODO:
		if Luminous {
			store config file in config map as above
		} else if Mimic + {
			set config values with `ceph config assimilage-conf t` after at least one mon is active
		}
	*/

	return nil
}

func (s *Store) applyLegacyOverrides(toFile *ini.File) error {
	ovrTxt := []byte(s.overrideConfig())
	if err := toFile.Append(ovrTxt); err != nil {
		return fmt.Errorf("failed to apply config file override from '''%s'''. %+v", ovrTxt, err)
	}
	b := new(bytes.Buffer)
	if _, err := toFile.WriteTo(b); err != nil {
		return fmt.Errorf("failed to write out toFile config file '''%+v'''. %+v", toFile, err)
	}
	return nil
}

func (s *Store) overrideConfig() string {
	cm, err := s.context.Clientset.CoreV1().ConfigMaps(s.namespace).
		Get(k8sutil.ConfigOverrideName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("override configmap does not exist")
			return "" // return empty string if cannot get override text
		}
		logger.Warningf("Error getting override configmap. %+v", err)
		return "" // return empty string if cannot get override text
	}
	// the override config map exists
	o, ok := cm.Data[k8sutil.ConfigOverrideVal]
	if !ok {
		logger.Warningf("The override config map does not have override text defined. %+v", o)
		return "" // return empty string if cannot get override text
	}
	return o
}

// update "mon_host" and "mon_initial_members" in the stored config
func (s *Store) createOrUpdateMonHostSecrets(clusterInfo *cephconfig.ClusterInfo) error {
	hosts := make([]string, len(clusterInfo.Monitors))
	members := make([]string, len(clusterInfo.Monitors))
	i := 0
	for _, m := range clusterInfo.Monitors {
		monIP := cephutil.GetIPFromEndpoint(m.Endpoint)

		// This tries to detect the current port if the mon already exists
		// This basically handles the transition between monitors running on 6790 to msgr2
		// So whatever the previous monitor port was we keep it
		currentMonPort := cephutil.GetPortFromEndpoint(m.Endpoint)

		monPorts := [2]string{strconv.Itoa(int(Msgr2port)), strconv.Itoa(int(currentMonPort))}
		msgr2Endpoint := net.JoinHostPort(monIP, monPorts[0])
		msgr1Endpoint := net.JoinHostPort(monIP, monPorts[1])

		// That's likely a fresh deployment
		if clusterInfo.CephVersion.IsAtLeastNautilus() && currentMonPort == 6789 {
			hosts[i] = "[v2:" + msgr2Endpoint + ",v1:" + msgr1Endpoint + "]"
		} else if clusterInfo.CephVersion.IsAtLeastNautilus() && currentMonPort != 6789 {
			// That's likely an upgrade from a Rook 0.9.x Mimic deployment
			hosts[i] = "v1:" + msgr1Endpoint
		} else {
			// That's before Nautilus
			hosts[i] = msgr1Endpoint
		}

		members[i] = m.Name
		i++
	}

	// store these in a secret instead of the configmap; secrets are required by CSI drivers
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// the config's secret store has the same name as the configmap store for consistency
			Name:      storeName,
			Namespace: s.namespace,
		},
		StringData: map[string]string{
			monHostKey:           strings.Join(hosts, ","),
			monInitialMembersKey: strings.Join(members, ","),
		},
		Type: k8sutil.RookType,
	}
	clientset := s.context.Clientset
	k8sutil.SetOwnerRef(&secret.ObjectMeta, s.ownerRef)

	_, err := clientset.CoreV1().Secrets(s.namespace).Get(storeName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("creating config secret %+v", secret)
			if _, err := clientset.CoreV1().Secrets(s.namespace).Create(secret); err != nil {
				return fmt.Errorf("failed to create config secret %+v. %+v", secret, err)
			}
		}
		return fmt.Errorf("failed to get config secret %s. %+v", storeName, err)
	}

	logger.Debugf("updating config secret %+v", secret)
	if _, err := clientset.CoreV1().Secrets(s.namespace).Update(secret); err != nil {
		return fmt.Errorf("failed to update config secret %+v. %+v", secret, err)
	}

	return nil
}

// StoredFileVolume returns a pod volume sourced from the stored config file.
func StoredFileVolume() v1.Volume {
	// TL;DR: mount the configmap's "ceph.conf" to a file called "ceph.conf" with 0400 permissions
	mode := int32(0400) // security: only allow the owner to read and no one to write
	return v1.Volume{
		Name: configVolumeName,
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{LocalObjectReference: v1.LocalObjectReference{
				Name: storeName},
				Items: []v1.KeyToPath{
					{Key: confFileName, Path: confFileName, Mode: &mode},
				}}}}
}

// StoredFileVolumeMount returns a container volume mount that mounts the stored config from
// StoredFileVolume into the container at `/etc/ceph/ceph.conf`.
func StoredFileVolumeMount() v1.VolumeMount {
	// configmap's "ceph.conf" to "/etc/ceph/ceph.conf"
	return v1.VolumeMount{
		Name:      storeName,
		ReadOnly:  true, // should be no reason to write to the config in pods, so enforce this
		MountPath: "/etc/ceph",
	}
}

// StoredMonHostEnvVars returns a container environment variable defined by the most updated stored
// "mon_host" and "mon_initial_members" information.
func StoredMonHostEnvVars() []v1.EnvVar {
	return []v1.EnvVar{
		{Name: "ROOK_CEPH_MON_HOST",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{
					Name: storeName},
					Key: monHostKey}}},
		{Name: "ROOK_CEPH_MON_INITIAL_MEMBERS",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{
					Name: storeName},
					Key: monInitialMembersKey}}},
	}
}

// StoredMonHostEnvVarReferences returns a small Ceph Config which references "mon_host" and
// "mon_initial_members" information from the StoredMonHostEnvVars. This config can be used to
// generate flags referencing the env vars or to generate the string representation of a Config.
func StoredMonHostEnvVarReferences() *Config {
	c := NewConfig()
	c.Section("global").
		Set(monHostKey, "$(ROOK_CEPH_MON_HOST)").
		Set(monInitialMembersKey, "$(ROOK_CEPH_MON_INITIAL_MEMBERS)")
	return c
}
