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

package peermap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	mappingConfigName = "rook-ceph-csi-mapping-config"
	mappingConfigkey  = "csi-mapping-config-json"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "peer-map")

type PeerIDMapping struct {
	ClusterIDMapping map[string]string
	RBDPoolIDMapping []map[string]string
}

type PeerIDMappings []PeerIDMapping

// addClusterIDMapping adds cluster ID map if not present already
func (m *PeerIDMappings) addClusterIDMapping(newClusterIDMap map[string]string) {
	if m.clusterIDMapIndex(newClusterIDMap) == -1 {
		newDRMap := PeerIDMapping{
			ClusterIDMapping: newClusterIDMap,
		}
		*m = append(*m, newDRMap)
	}
}

// addRBDPoolIDMapping adds all the pool ID maps for a given cluster ID map
func (m *PeerIDMappings) addRBDPoolIDMapping(clusterIDMap, newPoolIDMap map[string]string) {
	for i := 0; i < len(*m); i++ {
		if reflect.DeepEqual((*m)[i].ClusterIDMapping, clusterIDMap) {
			(*m)[i].RBDPoolIDMapping = append((*m)[i].RBDPoolIDMapping, newPoolIDMap)
		}
	}
}

// updateRBDPoolIDMapping updates the Pool ID mappings between local and peer cluster.
// It adds the cluster and Pool ID mappings if not present, else updates the pool ID map if required.
func (m *PeerIDMappings) updateRBDPoolIDMapping(newMappings PeerIDMapping) {
	newClusterIDMap := newMappings.ClusterIDMapping
	newPoolIDMap := newMappings.RBDPoolIDMapping[0]
	peerPoolID, localPoolID := getMapKV(newPoolIDMap)

	// Append new mappings if no existing mappings are available
	if len(*m) == 0 {
		*m = append(*m, newMappings)
		return
	}
	clusterIDMapExists := false
	for i := 0; i < len(*m); i++ {
		if reflect.DeepEqual((*m)[i].ClusterIDMapping, newClusterIDMap) {
			clusterIDMapExists = true
			poolIDMapUpdated := false
			for j := 0; j < len((*m)[i].RBDPoolIDMapping); j++ {
				existingPoolMap := (*m)[i].RBDPoolIDMapping[j]
				if _, ok := existingPoolMap[peerPoolID]; ok {
					poolIDMapUpdated = true
					existingPoolMap[peerPoolID] = localPoolID
				}
			}
			if !poolIDMapUpdated {
				(*m)[i].RBDPoolIDMapping = append((*m)[i].RBDPoolIDMapping, newPoolIDMap)
			}
		}
	}

	if !clusterIDMapExists {
		*m = append(*m, newMappings)
	}
}

func (m *PeerIDMappings) clusterIDMapIndex(newClusterIDMap map[string]string) int {
	for i, mapping := range *m {
		if reflect.DeepEqual(mapping.ClusterIDMapping, newClusterIDMap) {
			return i
		}
	}
	return -1
}

func (m *PeerIDMappings) String() (string, error) {
	mappingInBytes, err := json.Marshal(m)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal peer cluster mapping config")
	}

	return string(mappingInBytes), nil
}

func toObj(in string) (PeerIDMappings, error) {
	var mappings PeerIDMappings
	err := json.Unmarshal([]byte(in), &mappings)
	if err != nil {
		return mappings, errors.Wrap(err, "failed to unmarshal peer cluster mapping config")
	}

	return mappings, nil
}

func ReconcilePoolIDMap(clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, pool *cephv1.CephBlockPool) error {
	if pool.Spec.Mirroring.Peers == nil {
		logger.Infof("no peer secrets added in ceph block pool %q. skipping pool ID mappings with peer cluster", pool.Name)
		return nil
	}

	mappings, err := getClusterPoolIDMap(clusterContext, clusterInfo, pool)
	if err != nil {
		return errors.Wrapf(err, "failed to get peer pool ID mappings for the pool %q", pool.Name)
	}

	err = CreateOrUpdateConfig(clusterContext, mappings)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update peer pool ID mappings configMap for the pool %q", pool.Name)
	}

	logger.Infof("successfully updated config map with cluster and RDB pool ID mappings for the pool %q", pool.Name)
	return nil
}

// getClusterPoolIDMap returns a mapping between local and peer cluster ID, and between local and peer pool ID
func getClusterPoolIDMap(clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, pool *cephv1.CephBlockPool) (*PeerIDMappings, error) {
	mappings := &PeerIDMappings{}

	// Get local cluster pool details
	localPoolDetails, err := cephclient.GetPoolDetails(clusterContext, clusterInfo, pool.Name)
	if err != nil {
		return mappings, errors.Wrapf(err, "failed to get details for the pool %q", pool.Name)
	}

	logger.Debugf("pool details of local cluster %+v", localPoolDetails)

	for _, peerSecret := range pool.Spec.Mirroring.Peers.SecretNames {
		s, err := clusterContext.Clientset.CoreV1().Secrets(clusterInfo.Namespace).Get(clusterInfo.Context, peerSecret, metav1.GetOptions{})
		if err != nil {
			return mappings, errors.Wrapf(err, "failed to fetch kubernetes secret %q bootstrap peer", peerSecret)
		}

		token := s.Data["token"]
		decodedTokenToGo, err := decodePeerToken(string(token))
		if err != nil {
			return mappings, errors.Wrap(err, "failed to decode bootstrap peer token")
		}

		peerClientName := fmt.Sprintf("client.%s", decodedTokenToGo.ClientID)
		credentials := cephclient.CephCred{
			Username: peerClientName,
			Secret:   decodedTokenToGo.Key,
		}

		// Add cluster ID mappings
		clusterIDMapping := map[string]string{
			decodedTokenToGo.Namespace: clusterInfo.Namespace,
		}

		mappings.addClusterIDMapping(clusterIDMapping)

		// Generate peer cluster keyring in a temporary file
		keyring := cephclient.CephKeyring(credentials)
		keyringFile, err := util.CreateTempFile(keyring)
		if err != nil {
			return mappings, errors.Wrap(err, "failed to create a temp keyring file")
		}
		defer os.Remove(keyringFile.Name())

		// Generate an empty config file to be passed as `--conf`argument in ceph CLI
		configFile, err := util.CreateTempFile("")
		if err != nil {
			return mappings, errors.Wrap(err, "failed to create a temp config file")
		}
		defer os.Remove(configFile.Name())

		// Build command
		args := []string{"osd", "pool", "get", pool.Name, "all",
			fmt.Sprintf("--cluster=%s", decodedTokenToGo.Namespace),
			fmt.Sprintf("--conf=%s", configFile.Name()),
			fmt.Sprintf("--fsid=%s", decodedTokenToGo.ClusterFSID),
			fmt.Sprintf("--mon-host=%s", decodedTokenToGo.MonHost),
			fmt.Sprintf("--keyring=%s", keyringFile.Name()),
			fmt.Sprintf("--name=%s", peerClientName),
			"--format", "json",
		}

		// Get peer cluster pool details
		peerPoolDetails, err := getPeerPoolDetails(clusterContext, args...)
		if err != nil {
			return mappings, errors.Wrapf(err, "failed to get pool details from peer cluster %q", decodedTokenToGo.Namespace)
		}

		logger.Debugf("pool details from peer cluster %+v", peerPoolDetails)

		// Add Pool ID mappings
		poolIDMapping := map[string]string{
			strconv.Itoa(peerPoolDetails.Number): strconv.Itoa(localPoolDetails.Number),
		}
		mappings.addRBDPoolIDMapping(clusterIDMapping, poolIDMapping)
	}

	return mappings, nil
}

func CreateOrUpdateConfig(clusterContext *clusterd.Context, mappings *PeerIDMappings) error {
	ctx := context.TODO()
	data, err := mappings.String()
	if err != nil {
		return errors.Wrap(err, "failed to convert peer cluster mappings struct to string")
	}

	opNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	request := types.NamespacedName{Name: mappingConfigName, Namespace: opNamespace}
	existingConfigMap := &v1.ConfigMap{}

	err = clusterContext.Client.Get(ctx, request, existingConfigMap)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Create new configMap
			return createConfig(clusterContext, request, data)
		}
		return errors.Wrapf(err, "failed to get existing mapping config map %q", existingConfigMap.Name)
	}

	existingCMData := existingConfigMap.Data[mappingConfigkey]
	if existingCMData == "[]" {
		existingConfigMap.Data[mappingConfigkey] = data
	} else {
		existingMappings, err := toObj(existingCMData)
		if err != nil {
			return errors.Wrapf(err, "failed to extract existing mapping data from the config map %q", existingConfigMap.Name)
		}
		updatedCMData, err := UpdateExistingData(&existingMappings, mappings)
		if err != nil {
			return errors.Wrapf(err, "failed to update existing mapping data from the config map %q", existingConfigMap.Name)
		}
		existingConfigMap.Data[mappingConfigkey] = updatedCMData
	}

	// Update existing configMap
	if err := clusterContext.Client.Update(ctx, existingConfigMap); err != nil {
		return errors.Wrapf(err, "failed to update existing mapping config map %q", existingConfigMap.Name)
	}

	return nil
}

func UpdateExistingData(existingMappings, newMappings *PeerIDMappings) (string, error) {
	for i, mapping := range *newMappings {
		if len(mapping.RBDPoolIDMapping) == 0 {
			logger.Warning("no pool ID mapping available between local and peer cluster")
			continue
		}
		existingMappings.updateRBDPoolIDMapping((*newMappings)[i])
	}

	data, err := existingMappings.String()
	if err != nil {
		return "", errors.Wrap(err, "failed to convert peer cluster mappings struct to string")
	}
	return data, nil
}

func createConfig(clusterContext *clusterd.Context, request types.NamespacedName, data string) error {
	newConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      request.Name,
			Namespace: request.Namespace,
		},
		Data: map[string]string{
			mappingConfigkey: data,
		},
	}

	// Get Operator owner reference
	operatorPodName := os.Getenv(k8sutil.PodNameEnvVar)
	ownerRef, err := k8sutil.GetDeploymentOwnerReference(clusterContext.Clientset, operatorPodName, request.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get operator owner reference")
	}
	if ownerRef != nil {
		blockOwnerDeletion := false
		ownerRef.BlockOwnerDeletion = &blockOwnerDeletion
	}

	ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, request.Namespace)

	// Set controller reference only when creating the configMap for the first time
	err = ownerInfo.SetControllerReference(newConfigMap)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference on configMap %q", newConfigMap.Name)
	}

	err = clusterContext.Client.Create(context.TODO(), newConfigMap)
	if err != nil {
		return errors.Wrapf(err, "failed to create mapping configMap %q", newConfigMap.Name)
	}
	return nil
}

func decodePeerToken(token string) (*cephclient.PeerToken, error) {
	// decode the base64 encoded token
	decodedToken, err := base64.StdEncoding.DecodeString(string(token))
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode bootstrap peer token")
	}

	// Unmarshal the decoded token to a Go type
	var decodedTokenToGo cephclient.PeerToken
	err = json.Unmarshal(decodedToken, &decodedTokenToGo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal decoded token")
	}

	logger.Debugf("peer cluster info %+v", decodedTokenToGo)

	return &decodedTokenToGo, nil
}

func getPeerPoolDetails(ctx *clusterd.Context, args ...string) (cephclient.CephStoragePoolDetails, error) {
	peerPoolDetails, err := ctx.Executor.ExecuteCommandWithTimeout(exec.CephCommandsTimeout, "ceph", args...)
	if err != nil {
		return cephclient.CephStoragePoolDetails{}, errors.Wrap(err, "failed to get pool details from peer cluster")
	}

	return cephclient.ParsePoolDetails([]byte(peerPoolDetails))
}

func getMapKV(input map[string]string) (string, string) {
	for k, v := range input {
		return k, v
	}
	return "", ""
}
