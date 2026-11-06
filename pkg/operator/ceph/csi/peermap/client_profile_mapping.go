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

package peermap

import (
	"context"
	"fmt"
	"os"
	"reflect"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// clientProfileMappingName returns the name of the ClientProfileMapping of a pool. The resource
// is created in the operator namespace, which can be shared by multiple CephClusters, so the
// name includes the cluster namespace. A namespace cannot contain a "." so the name is unique.
func clientProfileMappingName(clusterNamespace, poolName string) string {
	return fmt.Sprintf("%s.%s", clusterNamespace, poolName)
}

// generateClientProfileMappingSpec converts the peer ID mappings of a pool to a ClientProfileMapping spec
func generateClientProfileMappingSpec(mappings *PeerIDMappings) csiopv1.ClientProfileMappingSpec {
	spec := csiopv1.ClientProfileMappingSpec{}
	for _, mapping := range *mappings {
		// the cluster ID map and each pool ID map have a single {peer: local} entry
		remoteClusterID, localClusterID := getMapKV(mapping.ClusterIDMapping)
		if remoteClusterID == "" || localClusterID == "" {
			// a bootstrap peer token that was not created by Rook does not have the peer cluster ID
			logger.Warningf("skipping clientProfileMapping for the cluster ID mapping %v since a cluster ID is missing", mapping.ClusterIDMapping)
			continue
		}

		poolIDPairs := []csiopv1.BlockPoolIdPair{}
		for _, poolIDMap := range mapping.RBDPoolIDMapping {
			remotePoolID, localPoolID := getMapKV(poolIDMap)
			poolIDPairs = append(poolIDPairs, csiopv1.BlockPoolIdPair{localPoolID, remotePoolID})
		}
		if len(poolIDPairs) == 0 {
			continue
		}

		spec.Mappings = append(spec.Mappings, csiopv1.MappingsSpec{
			LocalClientProfile:  localClusterID,
			RemoteClientProfile: remoteClusterID,
			BlockPoolIdMapping:  poolIDPairs,
		})
	}

	return spec
}

// reconcileClientProfileMapping creates or updates the ClientProfileMapping of a pool. The
// ceph-csi-operator uses it to generate the cluster-mapping.json in the ceph-csi-config configmap.
func reconcileClientProfileMapping(ctx context.Context, c client.Client, clusterNamespace, poolName string, mappings *PeerIDMappings) error {
	spec := generateClientProfileMappingSpec(mappings)
	if len(spec.Mappings) == 0 {
		logger.Debugf("no peer mappings available for the pool %q. skipping clientProfileMapping", poolName)
		return nil
	}

	clientProfileMapping := &csiopv1.ClientProfileMapping{}
	clientProfileMapping.Name = clientProfileMappingName(clusterNamespace, poolName)
	clientProfileMapping.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
	clientProfileMapping.Spec = spec

	existing := &csiopv1.ClientProfileMapping{}
	err := c.Get(ctx, types.NamespacedName{Name: clientProfileMapping.Name, Namespace: clientProfileMapping.Namespace}, existing)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get clientProfileMapping %q", clientProfileMapping.Name)
		}
		if err := c.Create(ctx, clientProfileMapping); err != nil {
			return errors.Wrapf(err, "failed to create clientProfileMapping %q", clientProfileMapping.Name)
		}
		logger.Infof("successfully created clientProfileMapping %q", clientProfileMapping.Name)
		return nil
	}

	if reflect.DeepEqual(existing.Spec, spec) {
		return nil
	}

	existing.Spec = spec
	if err := c.Update(ctx, existing); err != nil {
		return errors.Wrapf(err, "failed to update clientProfileMapping %q", existing.Name)
	}
	logger.Infof("successfully updated clientProfileMapping %q", existing.Name)

	return nil
}

// DeleteClientProfileMapping deletes the ClientProfileMapping of a pool. The pool cannot own the
// resource since it can be in a different namespace, so it must be deleted explicitly.
func DeleteClientProfileMapping(ctx context.Context, c client.Client, clusterNamespace, poolName string) error {
	clientProfileMapping := &csiopv1.ClientProfileMapping{}
	clientProfileMapping.Name = clientProfileMappingName(clusterNamespace, poolName)
	clientProfileMapping.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)

	err := c.Delete(ctx, clientProfileMapping)
	// the CR is already gone, or the ceph-csi-operator CRDs are not installed
	if err != nil && !kerrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return errors.Wrapf(err, "failed to delete clientProfileMapping %q", clientProfileMapping.Name)
	}

	return nil
}
