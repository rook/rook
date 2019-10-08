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
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// StoreName is the name of the configmap containing ceph configuration options
	StoreName = "rook-ceph-config"

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
	// these are used for all ceph daemons on the commandline and must *always* be stored
	if err := s.createOrUpdateMonHostSecrets(clusterInfo); err != nil {
		return errors.Wrapf(err, "failed to store mon host configs")
	}

	return nil
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
			Name:      StoreName,
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

	_, err := clientset.CoreV1().Secrets(s.namespace).Get(StoreName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("creating config secret %+v", secret)
			if _, err := clientset.CoreV1().Secrets(s.namespace).Create(secret); err != nil {
				return errors.Wrapf(err, "failed to create config secret %+v", secret)
			}
		} else {
			return errors.Wrapf(err, "failed to get config secret %s", StoreName)
		}
	}

	logger.Debugf("updating config secret %+v", secret)
	if _, err := clientset.CoreV1().Secrets(s.namespace).Update(secret); err != nil {
		return errors.Wrapf(err, "failed to update config secret %+v", secret)
	}

	return nil
}

// StoredMonHostEnvVars returns a container environment variable defined by the most updated stored
// "mon_host" and "mon_initial_members" information.
func StoredMonHostEnvVars() []v1.EnvVar {
	return []v1.EnvVar{
		{Name: "ROOK_CEPH_MON_HOST",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{
					Name: StoreName},
					Key: monHostKey}}},
		{Name: "ROOK_CEPH_MON_INITIAL_MEMBERS",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{
					Name: StoreName},
					Key: monInitialMembersKey}}},
	}
}

// StoredMonHostEnvVarFlags returns Ceph commandline flag references to "mon_host" and
// "mon_initial_members" sourced from the StoredMonHostEnvVars.
func StoredMonHostEnvVarFlags() []string {
	return []string{
		NewFlag(monHostKey, "$(ROOK_CEPH_MON_HOST)"),
		NewFlag(monInitialMembersKey, "$(ROOK_CEPH_MON_INITIAL_MEMBERS)"),
	}
}
