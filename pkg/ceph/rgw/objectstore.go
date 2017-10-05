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
package rgw

import (
	"encoding/json"
	"fmt"
	"strings"

	ceph "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/model"
)

const (
	rootPool = ".rgw.root"
	appName  = "rgw"
)

var (
	metadataPools = []string{
		rootPool,
		"rgw.control",
		"rgw.meta",
		"rgw.log",
		"rgw.buckets.index",
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

func CreateObjectStore(context *Context, metadataSpec, dataSpec model.Pool, serviceIP string, port int32) error {
	err := createPools(context, metadataSpec, dataSpec)
	if err != nil {
		return fmt.Errorf("failed to create object pools. %+v", err)
	}

	err = createRealm(context, serviceIP, port)
	if err != nil {
		return fmt.Errorf("failed to create object store realm. %+v", err)
	}
	return nil
}

func DeleteObjectStore(context *Context) error {
	err := deleteRealm(context)
	if err != nil {
		return fmt.Errorf("failed to delete realm. %+v", err)
	}

	err = deletePools(context)
	if err != nil {
		return fmt.Errorf("failed to delete object store pools. %+v", err)
	}
	return nil
}

func createRealm(context *Context, serviceIP string, port int32) error {
	zoneArg := fmt.Sprintf("--rgw-zone=%s", context.Name)
	endpointArg := fmt.Sprintf("--endpoints=%s:%d", serviceIP, port)
	updatePeriod := false

	// The first realm must be marked as the default
	defaultArg := ""
	stores, err := GetObjectStores(context)
	if err != nil {
		return fmt.Errorf("failed to get object stores. %+v", err)
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
			return fmt.Errorf("failed to create rgw realm %s. %+v", context.Name, err)
		}
	}

	realmID, err := decodeID(output)
	if err != nil {
		return fmt.Errorf("failed to parse realm id. %+v", err)
	}

	// create the zonegroup if it doesn't exist yet
	output, err = runAdminCommand(context, "zonegroup", "get")
	if err != nil {
		updatePeriod = true
		output, err = runAdminCommand(context, "zonegroup", "create", "--master", endpointArg, defaultArg)
		if err != nil {
			return fmt.Errorf("failed to create rgw zonegroup for %s. %+v", context.Name, err)
		}
	}

	zoneGroupID, err := decodeID(output)
	if err != nil {
		return fmt.Errorf("failed to parse zone group id. %+v", err)
	}

	// create the zone if it doesn't exist yet
	output, err = runAdminCommand(context, "zone", "get", zoneArg)
	if err != nil {
		updatePeriod = true
		output, err = runAdminCommand(context, "zone", "create", "--master", endpointArg, zoneArg, defaultArg)
		if err != nil {
			return fmt.Errorf("failed to create rgw zonegroup for %s. %+v", context.Name, err)
		}
	}
	zoneID, err := decodeID(output)
	if err != nil {
		return fmt.Errorf("failed to parse zone id. %+v", err)
	}

	if updatePeriod {
		// the period will help notify other zones of changes if there are multi-zones
		_, err := runAdminCommandNoRealm(context, "period", "update", "--commit")
		if err != nil {
			return fmt.Errorf("failed to update period. %+v", err)
		}
	}

	logger.Infof("RGW: realm=%s, zonegroup=%s, zone=%s", realmID, zoneGroupID, zoneID)
	return nil
}

func deleteRealm(context *Context) error {
	//  <name>
	_, err := runAdminCommand(context, "realm", "delete", "--rgw-realm", context.Name)
	if err != nil {
		logger.Warningf("failed to delete rgw realm %s. %+v", context.Name, err)
	}

	_, err = runAdminCommand(context, "zonegroup", "delete", "--rgw-zonegroup", context.Name)
	if err != nil {
		logger.Warningf("failed to delete rgw zonegroup %s. %+v", context.Name, err)
	}

	_, err = runAdminCommand(context, "zone", "delete", "--rgw-zone", context.Name)
	if err != nil {
		logger.Warningf("failed to delete rgw zone %s. %+v", context.Name, err)
	}

	return nil
}

func decodeID(data string) (string, error) {
	var id idType
	err := json.Unmarshal([]byte(data), &id)
	if err != nil {
		return "", fmt.Errorf("Failed to unmarshal json: %+v", err)
	}

	return id.ID, err
}

func GetObjectStores(context *Context) ([]string, error) {
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
		return nil, fmt.Errorf("Failed to unmarshal realms: %+v", err)
	}

	return r.Realms, nil
}

func deletePools(context *Context) error {
	pools := append(metadataPools, dataPools...)
	for _, pool := range pools {
		if err := deletePool(context, pool); err != nil {
			logger.Warningf("failed to delete pool %s", pool)
		}
	}

	return nil
}

func deletePool(context *Context, pool string) error {
	logger.Infof("skipping deleting of pool %s", pool)
	return nil
}

func createPools(context *Context, metadataSpec, dataSpec model.Pool) error {
	if err := createSimilarPools(context, metadataPools, metadataSpec); err != nil {
		return fmt.Errorf("failed to create metadata pools. %+v", err)
	}

	if err := createSimilarPools(context, dataPools, dataSpec); err != nil {
		return fmt.Errorf("failed to create data pool. %+v", err)
	}

	return nil
}

func createSimilarPools(context *Context, pools []string, poolSpec model.Pool) error {
	poolSpec.Name = context.Name
	cephConfig := ceph.ModelPoolToCephPool(poolSpec)
	if cephConfig.ErasureCodeProfile != "" {
		// create a new erasure code profile for the new pool
		if err := ceph.CreateErasureCodeProfile(context.context, context.ClusterName, poolSpec.ErasureCodedConfig, cephConfig.ErasureCodeProfile); err != nil {
			return fmt.Errorf("failed to create erasure code profile for object store %s: %+v", context.Name, err)
		}
	}

	for _, pool := range pools {
		// create the pool if it doesn't exist yet
		name := pool
		if !strings.HasPrefix(pool, ".") {
			// the name of the pool is <instance>.<name>, except for the pool ".rgw.root" that spans object stores
			name = fmt.Sprintf("%s.%s", context.Name, pool)
		}
		if _, err := ceph.GetPoolDetails(context.context, context.ClusterName, name); err != nil {
			cephConfig.Name = name
			err := ceph.CreatePoolForApp(context.context, context.ClusterName, cephConfig, appName)
			if err != nil {
				return fmt.Errorf("failed to create pool %s for object store %s", name, context.Name)
			}
		}
	}
	return nil
}
