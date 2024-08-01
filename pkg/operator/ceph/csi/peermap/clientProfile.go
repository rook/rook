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

// ReconcileClientProfileMapping creates/updates a clientProfileMapping resource used by the ceph csi-operator
func ReconcileClientProfileMapping(ctx context.Context, clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, pool *cephv1.CephBlockPool) error {
	if pool.Spec.Mirroring.Peers == nil {
		logger.Debugf("no peer secrets added in cephBlockPool %q. skip creating clientProfileMapping", pool.Name)
		return nil
	}

	newClientProfileMapping, err := generateClientProfileMappingCR(clusterContext, clusterInfo, pool)
	if err != nil {
		return errors.Wrapf(err, "failed to generate clientProfileMapping for cephBlockPool %q", pool.Name)
	}

	err = createOrUpdateClientProfileMappingCR(clusterContext, clusterInfo, newClientProfileMapping)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update clientProfileMapping resource for cephBlockPool %q", pool.Name)
	}

	return nil
}

func createOrUpdateClientProfileMappingCR(clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clientProfileMapping *csiopv1a1.ClientProfileMapping) error {
	existingClientProfileMapping := &csiopv1a1.ClientProfileMapping{}
	request := types.NamespacedName{Name: clientProfileMapping.Name, Namespace: clusterInfo.Namespace}
	err := clusterContext.Client.Get(clusterInfo.Context, request, existingClientProfileMapping)
	if err != nil {
		if kerrors.IsNotFound(err) {
			err := clusterContext.Client.Create(clusterInfo.Context, clientProfileMapping)
			if err != nil {
				return errors.Wrapf(err, "failed to create clientProfileMappings %q", clientProfileMapping.Name)
			}
			logger.Infof("successfully created clientProfileMappings %q", clientProfileMapping.Name)
			return nil
		}
		return errors.Wrapf(err, "failed to get existing clientProfileMappings %q", clientProfileMapping.Name)
	}

	if reflect.DeepEqual(existingClientProfileMapping.Spec, clientProfileMapping.Spec) {
		logger.Debug("no change in client profile mappings. Skipping update of clientProfileMapping")
		return nil
	}

	existingClientProfileMapping.Spec = clientProfileMapping.Spec
	err = clusterContext.Client.Update(clusterInfo.Context, existingClientProfileMapping)
	if err != nil {
		return errors.Wrapf(err, "failed to update existing clientProfileMappings %q", clientProfileMapping.Name)
	}
	logger.Infof("successfully updated clientProfileMappings CR %q", clientProfileMapping.Name)

	return nil
}

func generateClientProfileMappingCR(clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, pool *cephv1.CephBlockPool) (*csiopv1a1.ClientProfileMapping, error) {
	mappings, err := getClusterPoolIDMap(clusterContext, clusterInfo, pool)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get peer pool ID mappings for the pool %q", pool.Name)
	}

	clientProfileMappings := []csiopv1a1.MappingsSpec{}
	for _, mapping := range *mappings {
		mappingsSpec := csiopv1a1.MappingsSpec{
			BlockPoolIdMapping: []csiopv1a1.BlockPoolIdPair{},
		}
		remoteClusterID, localClusterID := getMapKV(mapping.ClusterIDMapping)
		logger.Debugf("cluster ID mappings: %q:%q", remoteClusterID, localClusterID)
		mappingsSpec.LocalClientProfile = localClusterID
		mappingsSpec.RemoteClientProfile = remoteClusterID
		for _, poolIDMappings := range mapping.RBDPoolIDMapping {
			sortedPoolIDKeys := sortedKeys(poolIDMappings)
			for _, poolIDKey := range sortedPoolIDKeys {
				remotePoolID := poolIDKey
				localPoolID := poolIDMappings[poolIDKey]
				logger.Debugf("pool ID mappings: %s:%s", remotePoolID, localPoolID)
				mappingIDs := []string{remotePoolID, localPoolID}
				mappingsSpec.BlockPoolIdMapping = append(mappingsSpec.BlockPoolIdMapping, mappingIDs)
			}
		}
		clientProfileMappings = append(clientProfileMappings, mappingsSpec)
	}

	blockOwnerDeletion := false
	clientProfileMapping := &csiopv1a1.ClientProfileMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pool.Name,
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
			Mappings: clientProfileMappings,
		},
	}

	return clientProfileMapping, nil
}

// DeleteClientProfileMapping deletes the clientProfileMapping resource
func DeleteClientProfileMapping(ctx context.Context, c client.Client, poolName, namespace string) error {
	clientProfileMapping := &csiopv1a1.ClientProfileMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      poolName,
			Namespace: namespace,
		},
	}

	err := c.Delete(ctx, clientProfileMapping)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return errors.Wrapf(err, "failed to delete clientProfileMapping %q", poolName)
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
