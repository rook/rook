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

package rbd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	keyringTemplate = `
[client.rbd-mirror.%s]
	key = %s
	caps mon = "profile rbd-mirror"
	caps osd = "profile rbd"
`
	keyringPeerTemplate = `
[client.%s]
	key = %s
`
	cephConfPeerTemplate = `
[global]
	fsid = %s
	mon_host = %s
`
	peerCephConfigKey               = "peerCephConfig"
	peerCephKeyringKey              = "peerCephKeyring"
	peerCephConfigFileConfigMapName = "rbd-mirror-peer-config"
	// #nosec G101 since this is not leaking any hardcoded credentials, it's just the secret name
	peerCephKeyringSecretName = "rbd-mirror-peer-keyring"
)

// daemonConfig for a single rbd-mirror
type daemonConfig struct {
	ResourceName string              // the name rook gives to mirror resources in k8s metadata
	DaemonID     string              // the ID of the Ceph daemon ("a", "b", ...)
	DataPathMap  *config.DataPathMap // location to store data in container
	ownerInfo    *k8sutil.OwnerInfo
}

// PeerToken is the content of the peer token
type PeerToken struct {
	ClusterFSID string `json:"fsid"`
	ClientID    string `json:"client_id"`
	Key         string `json:"key"`
	MonHost     string `json:"mon_host"`
}

func (r *ReconcileCephRBDMirror) generateKeyring(clusterInfo *client.ClusterInfo, daemonConfig *daemonConfig) (string, error) {
	ctx := context.TODO()
	user := fullDaemonName(daemonConfig.DaemonID)
	access := []string{"mon", "profile rbd-mirror", "osd", "profile rbd"}
	s := keyring.GetSecretStore(r.context, clusterInfo, daemonConfig.ownerInfo)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	// Delete legacy key store for upgrade from Rook v0.9.x to v1.0.x
	err = r.context.Clientset.CoreV1().Secrets(clusterInfo.Namespace).Delete(ctx, daemonConfig.ResourceName, metav1.DeleteOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("legacy rbd-mirror key %q is already removed", daemonConfig.ResourceName)
		} else {
			logger.Warningf("legacy rbd-mirror key %q could not be removed. %v", daemonConfig.ResourceName, err)
		}
	}

	keyring := fmt.Sprintf(keyringTemplate, daemonConfig.DaemonID, key)
	return keyring, s.CreateOrUpdate(daemonConfig.ResourceName, keyring)
}

func fullDaemonName(daemonID string) string {
	return fmt.Sprintf("client.rbd-mirror.%s", daemonID)
}

func (r *ReconcileCephRBDMirror) reconcileAddBoostrapPeer(cephRBDMirror *cephv1.CephRBDMirror, namespacedName types.NamespacedName) (reconcile.Result, error) {
	ctx := context.TODO()
	// List all the peers secret, we can have more than one peer we might want to configure
	// For each, get the Kubernetes Secret and import the "peer token" so that we can configure the mirroring
	for k, peerSecret := range cephRBDMirror.Spec.Peers.SecretNames {
		logger.Debugf("fetching bootstrap peer kubernetes secret %q", peerSecret)
		s, err := r.context.Clientset.CoreV1().Secrets(r.clusterInfo.Namespace).Get(ctx, peerSecret, metav1.GetOptions{})
		// We don't care about IsNotFound here, we still need to fail
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to fetch kubernetes secret %q bootstrap peer", peerSecret)
		}

		// Validate peer secret content
		peerSpec, err := validatePeerToken(s.Data)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to validate rbd-mirror bootstrap peer secret %q data", peerSecret)
		}

		// Add Peer detail to the Struct
		r.peers[peerSecret] = peerSpec

		// Get controller owner ref
		ownerInfo := k8sutil.NewOwnerInfo(cephRBDMirror, r.scheme)

		// Add rbd-mirror peer
		err = r.addPeer(peerSecret, s.Data)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to add rbd-mirror bootstrap peer")
		}

		// Only create the config and key Secret and ConfigMap for the first peer
		// Peers are always identical, at least this is the supported way today (Ceph Octopus), only the pool name differs
		if k == 0 {
			// Create the token ceph config file and key
			err = r.createBootstrapPeerConfigAndKey(peerSecret, string(s.Data["token"]), ownerInfo)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to create bootstrap peer config and key")
			}
		}
	}

	return reconcile.Result{}, nil
}

func validatePeerToken(data map[string][]byte) (*peerSpec, error) {
	if len(data) == 0 {
		return nil, errors.Errorf("failed to lookup 'data' secret field (empty)")
	}

	// Lookup Secret keys and content
	keysToTest := []string{"token", "pool"}
	for _, key := range keysToTest {
		k, ok := data[key]
		if !ok || len(k) == 0 {
			return nil, errors.Errorf("failed to lookup %q key in secret bootstrap peer (missing or empty)", key)
		}
	}

	return &peerSpec{poolName: string(data["pool"]), direction: string(data["direction"])}, nil
}

func (r *ReconcileCephRBDMirror) addPeer(peerSecret string, data map[string][]byte) error {
	// Import bootstrap peer
	err := client.ImportRBDMirrorBootstrapPeer(r.context, r.clusterInfo, r.peers[peerSecret].poolName, r.peers[peerSecret].direction, data["token"])
	if err != nil {
		return errors.Wrap(err, "failed to import bootstrap peer token")
	}

	// Now the bootstrap peer has been added so we can hydrate the pool mirror info
	poolMirrorInfo, err := client.GetPoolMirroringInfo(r.context, r.clusterInfo, r.peers[peerSecret].poolName)
	if err != nil {
		return errors.Wrap(err, "failed to get pool mirror information")
	}
	r.peers[peerSecret].info = poolMirrorInfo

	return nil
}

func validateSpec(r *cephv1.RBDMirroringSpec) error {
	if r.Count == 0 {
		return errors.New("rbd-mirror count must be at least one")
	}

	return nil
}

func (r *ReconcileCephRBDMirror) createBootstrapPeerConfigAndKey(peerSecret string, tokenBase64 string, ownerInfo *k8sutil.OwnerInfo) error {
	ctx := context.TODO()
	// Decode the base64 token
	decodeToken, err := base64.StdEncoding.DecodeString(tokenBase64)
	if err != nil {
		return errors.Wrap(err, "failed to decode bootstrap peer token")
	}

	// Unmarshal JSON into PeerToken struct
	var token PeerToken
	if err := json.Unmarshal([]byte(decodeToken), &token); err != nil {
		return errors.Wrap(err, "failed to unmarshal bootstrap peer token")
	}

	// Build peer ceph conf
	cephPeerConfig := fmt.Sprintf(cephConfPeerTemplate, token.ClusterFSID, token.MonHost)

	// Put it in a ConfigMap
	cm, err := generatePeerCephConfigFileConfigMap(r.peers[peerSecret].info.Peers[0].UUID, cephPeerConfig, r.clusterInfo.Namespace, ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to generate peer ceph config file configmap")
	}

	_, err = r.context.Clientset.CoreV1().ConfigMaps(r.clusterInfo.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil && !kerrors.IsNotFound(err) && !kerrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create kubernetes config map %q bootstrap peer config", cm.Name)
	}

	// Build the key file
	cephPeerKey := fmt.Sprintf(keyringPeerTemplate, token.ClientID, token.Key)

	// Put it in a Secret
	s, err := generatePeerKeyringSecret(r.peers[peerSecret].info.Peers[0].UUID, cephPeerKey, r.clusterInfo.Namespace, ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to generate peer keyring secret")
	}
	_, err = r.context.Clientset.CoreV1().Secrets(r.clusterInfo.Namespace).Create(ctx, s, metav1.CreateOptions{})
	if err != nil && !kerrors.IsNotFound(err) && !kerrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create kubernetes secret %q bootstrap peer key", s.Name)
	}

	return nil
}

func generatePeerCephConfigFileConfigMapName(siteName string) string {
	return fmt.Sprintf("%s-%s", peerCephConfigFileConfigMapName, siteName)
}

func generatePeerKeyringSecretName(siteName string) string {
	return fmt.Sprintf("%s-%s", peerCephKeyringSecretName, siteName)
}

func generatePeerCephConfigFileConfigMap(peerSiteUUID, peerCephConfig string, namespace string, ownerInfo *k8sutil.OwnerInfo) (*v1.ConfigMap, error) {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generatePeerCephConfigFileConfigMapName(peerSiteUUID),
			Namespace: namespace,
		},
		Data: map[string]string{
			peerCephConfigKey: peerCephConfig,
		},
	}
	err := ownerInfo.SetControllerReference(cm)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to configmap %q", cm.Name)
	}

	return cm, nil
}

func generatePeerKeyringSecret(peerSiteUUID, peerCephKey string, namespace string, ownerInfo *k8sutil.OwnerInfo) (*v1.Secret, error) {
	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generatePeerKeyringSecretName(peerSiteUUID),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			peerCephKeyringKey: []byte(peerCephKey),
		},
		Type: k8sutil.RookType,
	}
	err := ownerInfo.SetControllerReference(s)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to secret %q", s.Name)
	}
	return s, nil
}
