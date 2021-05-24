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

// Package pool to manage a rook pool.
package pool

import (
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// #nosec G101 since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	boostrapPeerSecretName = "pool-peer-token"
	// RBDMirrorBootstrapPeerSecretName #nosec G101 since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	RBDMirrorBootstrapPeerSecretName = "rbdMirrorBootstrapPeerSecretName"
)

func (r *ReconcileCephBlockPool) createBootstrapPeerSecret(cephBlockPool *cephv1.CephBlockPool, namespacedName types.NamespacedName) (reconcile.Result, error) {
	// Create rbd mirror bootstrap peer token
	boostrapToken, err := cephclient.CreateRBDMirrorBootstrapPeer(r.context, r.clusterInfo, cephBlockPool.Name)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to create rbd-mirror bootstrap peer")
	}

	// Generate and create a Kubernetes Secret with this token
	s := GenerateBootstrapPeerSecret(cephBlockPool.Name, cephBlockPool.Namespace, boostrapToken)

	// set ownerref to the Secret
	err = controllerutil.SetControllerReference(cephBlockPool, s, r.scheme)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to set owner reference for rbd-mirror bootstrap peer secret %q", s.Name)
	}

	// Create Secret
	logger.Debugf("store rbd-mirror bootstrap token in a Kubernetes Secret %q", s.Name)
	_, err = k8sutil.CreateOrUpdateSecret(r.context.Clientset, s)
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to create or update rbd-mirror bootstrap peer %q secret", s.Name)
	}

	logger.Infof("successfully created bootstrap peer token secret for pool %q", cephBlockPool.Name)
	return reconcile.Result{}, nil
}

// GenerateBootstrapPeerSecret generates a Kubernetes Secret for the mirror bootstrap peer token
func GenerateBootstrapPeerSecret(name, namespace string, token []byte) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildBoostrapPeerSecretName(name),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"token": token,
			"pool":  []byte(name),
		},
		Type: k8sutil.RookType,
	}

	return s
}

func buildBoostrapPeerSecretName(name string) string {
	return fmt.Sprintf("%s-%s", boostrapPeerSecretName, name)
}

func generateStatusInfo(p *cephv1.CephBlockPool) map[string]string {
	m := make(map[string]string)
	m[RBDMirrorBootstrapPeerSecretName] = buildBoostrapPeerSecretName(p.Name)
	return m
}
