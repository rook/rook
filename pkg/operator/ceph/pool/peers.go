/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package pool

import (
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileCephBlockPool) reconcileAddBootstrapPeer(pool *cephv1.CephBlockPool,
	namespacedName types.NamespacedName,
) (reconcile.Result, error) {
	if pool.Spec.Mirroring.Peers == nil {
		return reconcile.Result{}, nil
	}

	// List all the peers secret, we can have more than one peer we might want to configure
	// For each, get the Kubernetes Secret and import the "peer token" so that we can configure the mirroring
	for _, peerSecret := range pool.Spec.Mirroring.Peers.SecretNames {
		logger.Debugf("fetching bootstrap peer kubernetes secret %q", peerSecret)
		s, err := r.context.Clientset.CoreV1().Secrets(r.clusterInfo.Namespace).Get(r.opManagerContext, peerSecret, metav1.GetOptions{})
		// We don't care about IsNotFound here, we still need to fail
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to fetch kubernetes secret %q bootstrap peer", peerSecret)
		}

		// Validate peer secret content
		err = opcontroller.ValidatePeerToken(pool, s.Data)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to validate rbd-mirror bootstrap peer secret %q data", peerSecret)
		}

		// Import bootstrap peer
		err = client.ImportRBDMirrorBootstrapPeer(r.context, r.clusterInfo, pool.Name, string(s.Data["direction"]), s.Data["token"])
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to import bootstrap peer token")
		}
	}

	return reconcile.Result{}, nil
}
