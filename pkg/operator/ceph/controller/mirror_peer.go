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
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	//nolint:gosec // since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	poolMirrorBoostrapPeerSecretName = "pool-peer-token"
	//nolint:gosec // since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	fsMirrorBoostrapPeerSecretName = "fs-peer-token"
	//nolint:gosec // // since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	clusterMirrorBoostrapPeerSecretName = "cluster-peer-token"
	//nolint:gosec // since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	RBDMirrorBootstrapPeerSecretName = "rbdMirrorBootstrapPeerSecretName"
	//nolint:gosec // since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	FSMirrorBootstrapPeerSecretName = "fsMirrorBootstrapPeerSecretName"
)

func CreateBootstrapPeerSecret(ctx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, object client.Object, ownerInfo *k8sutil.OwnerInfo) (reconcile.Result, error) {
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

		// Add additional information to the peer token
		boostrapToken, err = expandBootstrapPeerToken(ctx, clusterInfo, boostrapToken)
		if err != nil {
			return ImmediateRetryResult, errors.Wrap(err, "failed to add extra information to rbd-mirror bootstrap peer")
		}

	case *cephv1.CephCluster:
		ns = objectType.Namespace
		daemonType = "cluster-rbd"
		// Create rbd mirror bootstrap peer token
		boostrapToken, err = cephclient.CreateRBDMirrorBootstrapPeerWithoutPool(ctx, clusterInfo)
		if err != nil {
			return ImmediateRetryResult, errors.Wrapf(err, "failed to create %s-mirror bootstrap peer", daemonType)
		}

		// Add additional information to the peer token
		boostrapToken, err = expandBootstrapPeerToken(ctx, clusterInfo, boostrapToken)
		if err != nil {
			return ImmediateRetryResult, errors.Wrap(err, "failed to add extra information to rbd-mirror bootstrap peer")
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
	err = ownerInfo.SetControllerReference(s)
	if err != nil {
		return ImmediateRetryResult, errors.Wrapf(err, "failed to set owner reference for %s-mirror bootstrap peer secret %q", daemonType, s.Name)
	}

	// Create Secret
	logger.Debugf("store %s-mirror bootstrap token in a Kubernetes Secret %q in namespace %q", daemonType, s.Name, ns)
	_, err = k8sutil.CreateOrUpdateSecret(clusterInfo.Context, ctx.Clientset, s)
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
	case *cephv1.CephCluster:
		entityType = "cluster"
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
	case *cephv1.CephCluster:
		return fmt.Sprintf("%s-%s", clusterMirrorBoostrapPeerSecretName, objectType.Name)
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
	case *cephv1.CephRBDMirror:
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

func expandBootstrapPeerToken(ctx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, token []byte) ([]byte, error) {
	// First decode the token, it's base64 encoded
	decodedToken, err := base64.StdEncoding.DecodeString(string(token))
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode bootstrap peer token")
	}

	// Unmarshal the decoded value to a Go type
	var decodedTokenToGo cephclient.PeerToken
	err = json.Unmarshal(decodedToken, &decodedTokenToGo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal decoded token")
	}

	decodedTokenToGo.Namespace = clusterInfo.Namespace

	// Marshal the Go type back to JSON
	decodedTokenBackToJSON, err := json.Marshal(decodedTokenToGo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode go type back to json")
	}

	// Return the base64 encoded token
	return []byte(base64.StdEncoding.EncodeToString(decodedTokenBackToJSON)), nil
}
