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

// Package controller provides Kubernetes controller/pod/container spec items used for many Ceph daemons
package controller

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// #nosec G101 since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	poolMirrorBoostrapPeerSecretName = "pool-peer-token"
	// #nosec G101 since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	fsMirrorBoostrapPeerSecretName = "fs-peer-token"
	// RBDMirrorBootstrapPeerSecretName #nosec G101 since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	RBDMirrorBootstrapPeerSecretName = "rbdMirrorBootstrapPeerSecretName"
	// FSMirrorBootstrapPeerSecretName #nosec G101 since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	FSMirrorBootstrapPeerSecretName = "fsMirrorBootstrapPeerSecretName"
)

// PeerToken is the content of the peer token
type PeerToken struct {
	ClusterFSID string `json:"fsid"`
	ClientID    string `json:"client_id"`
	Key         string `json:"key"`
	MonHost     string `json:"mon_host"`
	// These fields are added by Rook and NOT part of the output of client.CreateRBDMirrorBootstrapPeer()
	PoolID    int    `json:"pool_id"`
	Namespace string `json:"namespace"`
}

func CreateBootstrapPeerSecret(ctx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, object client.Object, namespacedName types.NamespacedName, scheme *runtime.Scheme) (reconcile.Result, error) {
	context := context.TODO()
	var err error
	var ns, name, daemonType string
	var boostrapToken []byte
	switch objectType := object.(type) {
	case *cephv1.CephBlockPool:
		ns = objectType.Namespace
		name = objectType.Name
		daemonType = "rbd"
		// Create rbd mirror bootstrap peer token
		boostrapToken, err = cephclient.CreateRBDMirrorBootstrapPeer(ctx, clusterInfo, name)
		if err != nil {
			return ImmediateRetryResult, errors.Wrapf(err, "failed to create %s-mirror bootstrap peer", daemonType)
		}
	case *cephv1.CephFilesystem:
		ns = objectType.Namespace
		name = objectType.Name
		daemonType = "cephfs"
		boostrapToken, err = cephclient.CreateFSMirrorBootstrapPeer(ctx, clusterInfo, name)
		if err != nil {
			return ImmediateRetryResult, errors.Wrapf(err, "failed to create %s-mirror bootstrap peer", daemonType)
		}
	default:
		return ImmediateRetryResult, errors.Wrap(err, "failed to create bootstrap peer unknown daemon type")
	}

	// Generate and create a Kubernetes Secret with this token
	s := GenerateBootstrapPeerSecret(object, boostrapToken)

	// set ownerref to the Secret
	err = controllerutil.SetControllerReference(object, s, scheme)
	if err != nil {
		return ImmediateRetryResult, errors.Wrapf(err, "failed to set owner reference for %s-mirror bootstrap peer secret %q", daemonType, s.Name)
	}

	// Create Secret
	logger.Debugf("store %s-mirror bootstrap token in a Kubernetes Secret %q", daemonType, s.Name)
	_, err = ctx.Clientset.CoreV1().Secrets(ns).Create(context, s, metav1.CreateOptions{})
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return ImmediateRetryResult, errors.Wrapf(err, "failed to create %s-mirror bootstrap peer %q secret", daemonType, s.Name)
	}

	return reconcile.Result{}, nil
}

// GenerateBootstrapPeerSecret generates a Kubernetes Secret for the mirror bootstrap peer token
func GenerateBootstrapPeerSecret(object client.Object, token []byte) *v1.Secret {
	var entityType, entityName, entityNamespace string

	switch objectType := object.(type) {
	case *cephv1.CephFilesystem:
		entityType = "fs"
		entityName = objectType.Name
		entityNamespace = objectType.Namespace
	case *cephv1.CephBlockPool:
		entityType = "pool"
		entityName = objectType.Name
		entityNamespace = objectType.Namespace
	}

	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildBoostrapPeerSecretName(object),
			Namespace: entityNamespace,
		},
		Data: map[string][]byte{
			"token":    token,
			entityType: []byte(entityName),
		},
		Type: k8sutil.RookType,
	}

	return s
}

func buildBoostrapPeerSecretName(object client.Object) string {
	switch objectType := object.(type) {
	case *cephv1.CephFilesystem:
		return fmt.Sprintf("%s-%s", fsMirrorBoostrapPeerSecretName, objectType.Name)
	case *cephv1.CephBlockPool:
		return fmt.Sprintf("%s-%s", poolMirrorBoostrapPeerSecretName, objectType.Name)
	}

	return ""
}

func GenerateStatusInfo(object client.Object) map[string]string {
	m := make(map[string]string)

	switch object.(type) {
	case *cephv1.CephFilesystem:
		m[FSMirrorBootstrapPeerSecretName] = buildBoostrapPeerSecretName(object)
	case *cephv1.CephBlockPool:
		m[RBDMirrorBootstrapPeerSecretName] = buildBoostrapPeerSecretName(object)
	}

	return m
}

func ValidatePeerToken(object client.Object, data map[string][]byte) error {
	if len(data) == 0 {
		return errors.Errorf("failed to lookup 'data' secret field (empty)")
	}

	// Lookup Secret keys and content
	keysToTest := []string{"token"}
	switch object.(type) {
	case *cephv1.CephBlockPool:
		keysToTest = append(keysToTest, "pool")
	}

	for _, key := range keysToTest {
		k, ok := data[key]
		if !ok || len(k) == 0 {
			return errors.Errorf("failed to lookup %q key in secret bootstrap peer (missing or empty)", key)
		}
	}

	return nil
}
