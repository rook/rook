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
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// StoreName is the name of the configmap containing ceph configuration options
	StoreName            = "rook-ceph-config"
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
	ownerInfo      *k8sutil.OwnerInfo
}

// GetStore returns the Store for the cluster.
func GetStore(context *clusterd.Context, namespace string, ownerInfo *k8sutil.OwnerInfo) *Store {
	return &Store{
		configMapStore: k8sutil.NewConfigMapKVStore(namespace, context.Clientset, ownerInfo),
		namespace:      namespace,
		context:        context,
		ownerInfo:      ownerInfo,
	}
}

// CreateOrUpdate creates or updates the stored Ceph config based on the cluster info.
func (s *Store) CreateOrUpdate(clusterInfo *cephclient.ClusterInfo) error {
	// these are used for all ceph daemons on the commandline and must *always* be stored
	if err := s.createOrUpdateMonHostSecrets(clusterInfo); err != nil {
		return errors.Wrap(err, "failed to store mon host configs")
	}

	return nil
}

// update "mon_host" and "mon_initial_members" in the stored config
func (s *Store) createOrUpdateMonHostSecrets(clusterInfo *cephclient.ClusterInfo) error {

	// extract a list of just the monitor names, which will populate the "mon initial members"
	// and "mon hosts" global config field
	members, hosts := cephclient.PopulateMonHostMembers(clusterInfo.Monitors)

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
	err := s.ownerInfo.SetControllerReference(secret)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to moh host secret %q", secret.Name)
	}

	_, err = clientset.CoreV1().Secrets(s.namespace).Get(clusterInfo.Context, StoreName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("creating config secret %q", secret.Name)
			if _, err := clientset.CoreV1().Secrets(s.namespace).Create(clusterInfo.Context, secret, metav1.CreateOptions{}); err != nil {
				return errors.Wrapf(err, "failed to create config secret %+v", secret)
			}
		} else {
			return errors.Wrapf(err, "failed to get config secret %s", StoreName)
		}
	}

	logger.Debugf("updating config secret %q", secret.Name)
	if _, err := clientset.CoreV1().Secrets(s.namespace).Update(clusterInfo.Context, secret, metav1.UpdateOptions{}); err != nil {
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
