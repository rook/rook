/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package object

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/model"
)

const (
	rootPool = ".rgw.root"

	// AppName is the name Rook uses for the object store's application
	AppName = "rook-ceph-rgw"
)

var (
	metadataPools = []string{
		// .rgw.root (rootPool) is appended to this slice where needed
		"rgw.control",
		"rgw.meta",
		"rgw.log",
		"rgw.buckets.index",
		"rgw.buckets.non-ec",
	}
	dataPools = []string{
		"rgw.buckets.data",
	}
)

type idType struct {
	ID string `json:"id"`
}

type realmType struct {
	Realms []string `json:"realms"`
}

func createObjectStore(context *Context, metadataSpec, dataSpec model.Pool, serviceIP string, port int32) error {
	err := createPools(context, metadataSpec, dataSpec)
	if err != nil {
		return errors.Wrapf(err, "failed to create object pools")
	}

	err = createRealm(context, serviceIP, port)
	if err != nil {
		return errors.Wrapf(err, "failed to create object store realm")
	}
	return nil
}

func deleteRealmAndPools(context *Context, preservePoolsOnDelete bool) error {
	stores, err := getObjectStores(context)
	if err != nil {
		return errors.Wrapf(err, "failed to detect object stores during deletion")
	}
	logger.Infof("Found stores %v when deleting store %s", stores, context.Name)

	err = deleteRealm(context)
	if err != nil {
		return errors.Wrapf(err, "failed to delete realm")
	}

	lastStore := false
	if len(stores) == 1 && stores[0] == context.Name {
		lastStore = true
	}

	if !preservePoolsOnDelete {
		err = deletePools(context, lastStore)
		if err != nil {
			return errors.Wrapf(err, "failed to delete object store pools")
		}
	} else {
		logger.Infof("PreservePoolsOnDelete is set in object store %s. Pools not deleted", context.Name)
	}
	return nil
}

func createRealm(context *Context, serviceIP string, port int32) error {
	zoneArg := fmt.Sprintf("--rgw-zone=%s", context.Name)
	endpointArg := fmt.Sprintf("--endpoints=%s:%d", serviceIP, port)
	updatePeriod := false

	// The first realm must be marked as the default
	defaultArg := ""
	stores, err := getObjectStores(context)
	if err != nil {
		return errors.Wrapf(err, "failed to get object stores")
	}
	if len(stores) == 0 {
		defaultArg = "--default"
	}

	// create the realm if it doesn't exist yet
	output, err := runAdminCommand(context, "realm", "get")
	if err != nil {
		updatePeriod = true
		output, err = runAdminCommand(context, "realm", "create", defaultArg)
		if err != nil {
			return errors.Wrapf(err, "failed to create rgw realm %s", context.Name)
		}
	}

	realmID, err := decodeID(output)
	if err != nil {
		return errors.Wrapf(err, "failed to parse realm id")
	}

	// create the zonegroup if it doesn't exist yet
	output, err = runAdminCommand(context, "zonegroup", "get")
	if err != nil {
		updatePeriod = true
		output, err = runAdminCommand(context, "zonegroup", "create", "--master", endpointArg, defaultArg)
		if err != nil {
			return errors.Wrapf(err, "failed to create rgw zonegroup for %s", context.Name)
		}
	}

	zoneGroupID, err := decodeID(output)
	if err != nil {
		return errors.Wrapf(err, "failed to parse zone group id")
	}

	// create the zone if it doesn't exist yet
	output, err = runAdminCommand(context, "zone", "get", zoneArg)
	if err != nil {
		updatePeriod = true
		output, err = runAdminCommand(context, "zone", "create", "--master", endpointArg, zoneArg, defaultArg)
		if err != nil {
			return errors.Wrapf(err, "failed to create rgw zonegroup for %s", context.Name)
		}
	}
	zoneID, err := decodeID(output)
	if err != nil {
		return errors.Wrapf(err, "failed to parse zone id")
	}

	if updatePeriod {
		// the period will help notify other zones of changes if there are multi-zones
		_, err := runAdminCommandNoRealm(context, "period", "update", "--commit")
		if err != nil {
			return errors.Wrapf(err, "failed to update period")
		}
	}

	logger.Infof("RGW: realm=%s, zonegroup=%s, zone=%s", realmID, zoneGroupID, zoneID)
	return nil
}

func deleteRealm(context *Context) error {
	//  <name>
	_, err := runAdminCommand(context, "realm", "delete", "--rgw-realm", context.Name)
	if err != nil {
		logger.Warningf("failed to delete rgw realm %q. %v", context.Name, err)
	}

	_, err = runAdminCommand(context, "zonegroup", "delete", "--rgw-zonegroup", context.Name)
	if err != nil {
		logger.Warningf("failed to delete rgw zonegroup %q. %v", context.Name, err)
	}

	_, err = runAdminCommand(context, "zone", "delete", "--rgw-zone", context.Name)
	if err != nil {
		logger.Warningf("failed to delete rgw zone %q. %v", context.Name, err)
	}

	return nil
}

func decodeID(data string) (string, error) {
	var id idType
	err := json.Unmarshal([]byte(data), &id)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to unmarshal json")
	}

	return id.ID, err
}

func getObjectStores(context *Context) ([]string, error) {
	output, err := runAdminCommandNoRealm(context, "realm", "list")
	if err != nil {
		if strings.Index(err.Error(), "exit status 2") != 0 {
			return []string{}, nil
		}
		return nil, err
	}

	var r realmType
	err = json.Unmarshal([]byte(output), &r)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to unmarshal realms")
	}

	return r.Realms, nil
}

func deletePools(context *Context, lastStore bool) error {
	pools := append(metadataPools, dataPools...)
	if lastStore {
		pools = append(pools, rootPool)
	}

	for _, pool := range pools {
		name := poolName(context.Name, pool)
		if err := ceph.DeletePool(context.Context, context.ClusterName, name); err != nil {
			logger.Warningf("failed to delete pool %q. %v", name, err)
		}
	}

	// Delete erasure code profile if any
	erasureCodes, err := ceph.ListErasureCodeProfiles(context.Context, context.ClusterName)
	if err != nil {
		return errors.Wrapf(err, "failed to list erasure code profiles for cluster %s", context.ClusterName)
	}
	// cleans up the EC profile for the data pool only. Metadata pools don't support EC (only replication is supported).
	objectStoreErasureCode := ceph.GetErasureCodeProfileForPool(context.Name)
	for i := range erasureCodes {
		if erasureCodes[i] == objectStoreErasureCode {
			if err := ceph.DeleteErasureCodeProfile(context.Context, context.ClusterName, objectStoreErasureCode); err != nil {
				return errors.Wrapf(err, "failed to delete erasure code profile %s for object store %s", objectStoreErasureCode, context.Name)
			}
			break
		}
	}

	return nil
}

func createPools(context *Context, metadataSpec, dataSpec model.Pool) error {
	if err := createSimilarPools(context, append(metadataPools, rootPool), metadataSpec); err != nil {
		return errors.Wrapf(err, "failed to create metadata pools")
	}

	if err := createSimilarPools(context, dataPools, dataSpec); err != nil {
		return errors.Wrapf(err, "failed to create data pool")
	}

	return nil
}

func createSimilarPools(context *Context, pools []string, poolSpec model.Pool) error {
	poolSpec.Name = context.Name
	cephConfig := ceph.ModelPoolToCephPool(poolSpec)
	isECPool := cephConfig.ErasureCodeProfile != ""
	if isECPool {
		// create a new erasure code profile for the new pool
		if err := ceph.CreateErasureCodeProfile(context.Context, context.ClusterName, poolSpec.ErasureCodedConfig, cephConfig.ErasureCodeProfile,
			poolSpec.FailureDomain, poolSpec.CrushRoot, poolSpec.DeviceClass); err != nil {
			return errors.Wrapf(err, "failed to create erasure code profile for object store %s", context.Name)
		}
	}

	for _, pool := range pools {
		// create the pool if it doesn't exist yet
		name := poolName(context.Name, pool)
		if _, err := ceph.GetPoolDetails(context.Context, context.ClusterName, name); err != nil {
			cephConfig.Name = name
			// If the ceph config has an EC profile, an EC pool must be created. Otherwise, it's necessary
			// to create a replicated pool.
			var err error
			if isECPool {
				// An EC pool backing an object store does not need to enable EC overwrites, so the pool is
				// created with that property disabled to avoid unnecessary performance impact.
				err = ceph.CreateECPoolForApp(context.Context, context.ClusterName, cephConfig, AppName, false /* enableECOverwrite */, poolSpec.ErasureCodedConfig)
			} else {
				err = ceph.CreateReplicatedPoolForApp(context.Context, context.ClusterName, cephConfig, AppName)
			}
			if err != nil {
				return errors.Errorf("failed to create pool %s for object store %s", name, context.Name)
			}
		}
	}
	return nil
}

func poolName(storeName, poolName string) string {
	if strings.HasPrefix(poolName, ".") {
		return poolName
	}
	// the name of the pool is <instance>.<name>, except for the pool ".rgw.root" that spans object stores
	return fmt.Sprintf("%s.%s", storeName, poolName)
}
