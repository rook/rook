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
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
)

// daemonConfig for a single rbd-mirror
type daemonConfig struct {
	ResourceName string              // the name rook gives to mirror resources in k8s metadata
	DaemonID     string              // the ID of the Ceph daemon ("a", "b", ...)
	DataPathMap  *config.DataPathMap // location to store data in container
	ownerInfo    *k8sutil.OwnerInfo
}

func (r *ReconcileCephRBDMirror) generateKeyring(clusterInfo *client.ClusterInfo, daemonConfig *daemonConfig) (string, error) {
	user := fullDaemonName(daemonConfig.DaemonID)
	access := []string{"mon", "profile rbd-mirror", "osd", "profile rbd"}
	s := keyring.GetSecretStore(r.context, clusterInfo, daemonConfig.ownerInfo)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	// Delete legacy key store for upgrade from Rook v0.9.x to v1.0.x
	err = r.context.Clientset.CoreV1().Secrets(clusterInfo.Namespace).Delete(r.opManagerContext, daemonConfig.ResourceName, metav1.DeleteOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("legacy rbd-mirror key %q is already removed", daemonConfig.ResourceName)
		} else {
			logger.Warningf("legacy rbd-mirror key %q could not be removed. %v", daemonConfig.ResourceName, err)
		}
	}

	keyring := fmt.Sprintf(keyringTemplate, daemonConfig.DaemonID, key)
	return s.CreateOrUpdate(daemonConfig.ResourceName, keyring)
}

func fullDaemonName(daemonID string) string {
	return fmt.Sprintf("client.rbd-mirror.%s", daemonID)
}

func (r *ReconcileCephRBDMirror) reconcileAddBootstrapPeer(cephRBDMirror *cephv1.CephRBDMirror, namespacedName types.NamespacedName) (reconcile.Result, error) {
	// List all the peers secret, we can have more than one peer we might want to configure
	// For each, get the Kubernetes Secret and import the "peer token" so that we can configure the mirroring

	logger.Warning("(DEPRECATED) use of peer secret names in CephRBDMirror is deprecated. Please use CephBlockPool CR to configure peer secret names and import peers.")
	for _, peerSecret := range cephRBDMirror.Spec.Peers.SecretNames {
		logger.Debugf("fetching bootstrap peer kubernetes secret %q", peerSecret)
		s, err := r.context.Clientset.CoreV1().Secrets(r.clusterInfo.Namespace).Get(r.opManagerContext, peerSecret, metav1.GetOptions{})
		// We don't care about IsNotFound here, we still need to fail
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to fetch kubernetes secret %q bootstrap peer", peerSecret)
		}

		// Validate peer secret content
		err = opcontroller.ValidatePeerToken(cephRBDMirror, s.Data)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to validate rbd-mirror bootstrap peer secret %q data", peerSecret)
		}

		// Add Peer detail to the Struct
		r.peers[peerSecret] = &peerSpec{poolName: string(s.Data["pool"]), direction: string(s.Data["direction"])}

		// Add rbd-mirror peer
		err = r.addPeer(peerSecret, s.Data)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to add rbd-mirror bootstrap peer")
		}
	}

	return reconcile.Result{}, nil
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
