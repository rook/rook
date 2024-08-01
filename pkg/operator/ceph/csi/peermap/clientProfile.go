/*
Copyright 2024 The Rook Authors. All rights reserved.

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
	"reflect"
	"slices"
	"strconv"

	csiopv1a1 "github.com/ceph/ceph-csi-operator/api/v1alpha1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileClientProfileMappings creates/updates a clientProfileMappings resource used by the ceph csi-operator
func ReconcileClientProfileMappings(ctx context.Context, clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, pool *cephv1.CephBlockPool) error {
	if pool.Spec.Mirroring.Peers == nil {
		logger.Debugf("no peer secrets added in cephBlockPool %q. skip creating clientProfileMappings resource", pool.Name)
		return nil
	}

	newClientProfileMappings, err := generateClientProfileMappingsCR(clusterContext, clusterInfo, pool)
	if err != nil {
		return errors.Wrapf(err, "failed to generate clientProfileMappings for cephBlockPool %q", pool.Name)
	}

	err = createOrUpdateClientProfileMappingsCR(clusterContext, clusterInfo, newClientProfileMappings)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update clientProfileMappings resource for cephBlockPool %q", pool.Name)
	}

	return nil
}

func createOrUpdateClientProfileMappingsCR(clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clientProfileMappings *csiopv1a1.ClientProfileMapping) error {
	existingClientProfileMappings := &csiopv1a1.ClientProfileMapping{}
	request := types.NamespacedName{Name: clientProfileMappings.Name, Namespace: clusterInfo.Namespace}
	err := clusterContext.Client.Get(clusterInfo.Context, request, existingClientProfileMappings)
	if err != nil {
		if kerrors.IsNotFound(err) {
			err := clusterContext.Client.Create(clusterInfo.Context, clientProfileMappings)
			if err != nil {
				return errors.Wrapf(err, "failed to create clientProfileMappings CR %q", clientProfileMappings.Name)
			}
			logger.Infof("successfully created clientProfileMappings CR %q", clientProfileMappings.Name)
			return nil
		}
		return errors.Wrapf(err, "failed to get existing clientProfileMappings CR %q", clientProfileMappings.Name)
	}

	if reflect.DeepEqual(existingClientProfileMappings.Spec, clientProfileMappings.Spec) {
		logger.Debug("no change in client profile mappings. Skipping update of clientProfileMappings resource")
		return nil
	}

	existingClientProfileMappings.Spec = clientProfileMappings.Spec
	err = clusterContext.Client.Update(clusterInfo.Context, existingClientProfileMappings)
	if err != nil {
		return errors.Wrapf(err, "failed to update existing clientProfileMappings CR  %q", clientProfileMappings.Name)
	}
	logger.Infof("successfully updated clientProfileMappings CR %q", clientProfileMappings.Name)

	return nil
}

func generateClientProfileMappingsCR(clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, pool *cephv1.CephBlockPool) (*csiopv1a1.ClientProfileMapping, error) {
	mappings, err := getClusterPoolIDMap(clusterContext, clusterInfo, pool)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get peer pool ID mappings for the pool %q", pool.Name)
	}

	blockPoolMappings := []csiopv1a1.BlockPoolMappingSpec{}
	for _, mapping := range *mappings {
		remoteClusterID, localClusterID := getMapKV(mapping.ClusterIDMapping)
		for _, poolIDMappings := range mapping.RBDPoolIDMapping {
			sortedPoolIDKeys := sortedKeys(poolIDMappings)
			for _, poolIDKey := range sortedPoolIDKeys {
				remotePoolID, err := strconv.Atoi(poolIDKey)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse remote cluster poolID %q to integer", poolIDKey)
				}

				localPoolID, err := strconv.Atoi(poolIDMappings[poolIDKey])
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse local cluster poolID %q to integer", poolIDKey)
				}

				blockPoolMapping := csiopv1a1.BlockPoolMappingSpec{
					Local:  csiopv1a1.BlockPoolRefSpec{ClientProfileName: localClusterID, PoolId: localPoolID},
					Remote: csiopv1a1.BlockPoolRefSpec{ClientProfileName: remoteClusterID, PoolId: remotePoolID},
				}

				logger.Debugf("cluster ID mappings: %q:%q", remoteClusterID, localClusterID)
				logger.Debugf("pool ID mappings: %d:%d", remotePoolID, localPoolID)
				blockPoolMappings = append(blockPoolMappings, blockPoolMapping)
			}
		}
	}

	blockOwnerDeletion := false
	clientProfileMappings := &csiopv1a1.ClientProfileMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pool.Name, //TODO: what should be the correct name for this resource?
			Namespace: clusterInfo.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         pool.APIVersion,
					Kind:               pool.Kind,
					Name:               pool.ObjectMeta.Name,
					UID:                pool.UID,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
		Spec: csiopv1a1.ClientProfileMappingSpec{
			BlockPoolMapping: blockPoolMappings,
		},
	}

	return clientProfileMappings, nil
}

// DeleteClientProfileMappingsCR deletes the clientProfileMapping resource
func DeleteClientProfileMappingsCR(ctx context.Context, c client.Client, poolName, Namespace string) error {
	clientProfileMapping := &csiopv1a1.ClientProfileMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      poolName,
			Namespace: Namespace,
		},
	}

	err := c.Delete(ctx, clientProfileMapping)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return errors.Wrapf(err, "failed to delete clientProfileMapping resource %q", poolName)
	}

	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	slices.Sort(keys)
	return keys
}
