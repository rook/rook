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

Some of the code below came from https://github.com/digitalocean/ceph_exporter
which has the same license.
*/
package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
)

const (
	confirmFlag       = "--yes-i-really-mean-it"
	reallyConfirmFlag = "--yes-i-really-really-mean-it"
)

type CephStoragePoolSummary struct {
	Name   string `json:"poolname"`
	Number int    `json:"poolnum"`
}

type CephStoragePoolDetails struct {
	Name               string `json:"pool"`
	Number             int    `json:"pool_id"`
	Size               uint   `json:"size"`
	ErasureCodeProfile string `json:"erasure_code_profile"`
	FailureDomain      string
}

type CephStoragePoolStats struct {
	Pools []struct {
		Name  string `json:"name"`
		ID    int    `json:"id"`
		Stats struct {
			BytesUsed    float64 `json:"bytes_used"`
			RawBytesUsed float64 `json:"raw_bytes_used"`
			MaxAvail     float64 `json:"max_avail"`
			Objects      float64 `json:"objects"`
			DirtyObjects float64 `json:"dirty"`
			ReadIO       float64 `json:"rd"`
			ReadBytes    float64 `json:"rd_bytes"`
			WriteIO      float64 `json:"wr"`
			WriteBytes   float64 `json:"wr_bytes"`
		} `json:"stats"`
	} `json:"pools"`
}

func ListPoolSummaries(context *clusterd.Context, clusterName string) ([]CephStoragePoolSummary, error) {
	args := []string{"osd", "lspools"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}

	var pools []CephStoragePoolSummary
	err = json.Unmarshal(buf, &pools)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, string(buf))
	}

	return pools, nil
}

func GetPoolNamesByID(context *clusterd.Context, clusterName string) (map[int]string, error) {
	pools, err := ListPoolSummaries(context, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}
	names := map[int]string{}
	for _, p := range pools {
		names[p.Number] = p.Name
	}
	return names, nil
}

func GetPoolDetails(context *clusterd.Context, clusterName, name string) (CephStoragePoolDetails, error) {
	args := []string{"osd", "pool", "get", name, "all"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return CephStoragePoolDetails{}, fmt.Errorf("failed to get pool %s details: %+v", name, err)
	}

	// The response for osd pool get when passing var=all is actually malformed JSON similar to:
	// {"pool":"rbd","size":1}{"pool":"rbd","min_size":2}...
	// Note the multiple top level entities, one for each property returned.  To workaround this,
	// we split the JSON response string into its top level entities, then iterate through them, cleaning
	// up the JSON.  A single pool details object is repeatedly used to unmarshal each JSON snippet into.
	// Since previously set fields remain intact if they are not overwritten, the result is the JSON
	// unmarshalling of all properties in the response.
	var poolDetails CephStoragePoolDetails
	poolDetailsUnits := strings.Split(string(buf), "}{")
	for i := range poolDetailsUnits {
		pdu := poolDetailsUnits[i]
		if !strings.HasPrefix(pdu, "{") {
			pdu = "{" + pdu
		}
		if !strings.HasSuffix(pdu, "}") {
			pdu += "}"
		}
		err := json.Unmarshal([]byte(pdu), &poolDetails)
		if err != nil {
			return CephStoragePoolDetails{}, fmt.Errorf("unmarshal failed: %+v. raw buffer response: %s", err, string(buf))
		}
	}

	return poolDetails, nil
}

func CreatePoolWithProfile(context *clusterd.Context, clusterName string, newPoolReq model.Pool, appName string) error {
	newPool := ModelPoolToCephPool(newPoolReq)
	if newPoolReq.Type == model.ErasureCoded {
		// create a new erasure code profile for the new pool
		if err := CreateErasureCodeProfile(context, clusterName, newPoolReq.ErasureCodedConfig, newPool.ErasureCodeProfile, newPoolReq.FailureDomain); err != nil {
			return fmt.Errorf("failed to create erasure code profile for pool '%s': %+v", newPoolReq.Name, err)
		}
	}

	return CreatePoolForApp(context, clusterName, newPool, appName)
}

func DeletePool(context *clusterd.Context, clusterName string, name string) error {
	// check if the pool exists
	pool, err := GetPoolDetails(context, clusterName, name)
	if err != nil {
		logger.Infof("pool %s not found for deletion. %+v", name, err)
		return nil
	}

	logger.Infof("purging pool %s (id=%d)", name, pool.Number)
	args := []string{"osd", "pool", "delete", name, name, reallyConfirmFlag}
	_, err = ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to delete pool %s. %+v", name, err)
	}

	// remove the crush rule for this pool and ignore the error in case the rule is still in use or not found
	args = []string{"osd", "crush", "rule", "rm", name}
	_, err = ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		logger.Infof("did not delete crush rule %s. %+v", name, err)
	}

	logger.Infof("purge completed for pool %s", name)
	return nil
}

func CreatePool(context *clusterd.Context, clusterName string, newPool CephStoragePoolDetails) error {
	// for a generic/custom pool, just reuse the pool name for its app name
	return CreatePoolForApp(context, clusterName, newPool, newPool.Name)
}

func CreatePoolForApp(context *clusterd.Context, clusterName string, newPool CephStoragePoolDetails, appName string) error {
	// create a crush rule for a replicated pool, if a failure domain is specified
	replicated := newPool.ErasureCodeProfile == "" && newPool.Size > 0
	ruleName := newPool.Name
	if replicated && newPool.FailureDomain != "" {
		args := []string{"osd", "crush", "rule", "create-simple", ruleName, "default", newPool.FailureDomain}
		_, err := ExecuteCephCommand(context, clusterName, args)
		if err != nil {
			return fmt.Errorf("failed to crush rule %s. %+v", newPool.Name, err)
		}
	}

	args := []string{"osd", "pool", "create", newPool.Name, strconv.Itoa(newPool.Number)}
	if newPool.ErasureCodeProfile != "" {
		args = append(args, "erasure", newPool.ErasureCodeProfile)
	} else {
		args = append(args, "replicated")

		// Associate the crush rule created above with the new pool
		if newPool.FailureDomain != "" {
			args = append(args, ruleName)
		}
	}

	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to create pool %s. %+v", newPool.Name, err)
	}

	if replicated {
		// the pool is type replicated, set the size for the pool now that it's been created
		if err = SetPoolProperty(context, clusterName, newPool.Name, "size", strconv.FormatUint(uint64(newPool.Size), 10)); err != nil {
			return err
		}
	}

	// ensure that the newly created pool gets an application tag
	args = []string{"osd", "pool", "application", "enable", newPool.Name, appName, confirmFlag}
	_, err = ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to enable application %s on pool %s. %+v", appName, newPool.Name, err)
	}

	logger.Infof("creating pool %s succeeded, buf: %s", newPool.Name, string(buf))
	return nil
}

func SetPoolProperty(context *clusterd.Context, clusterName, name, propName string, propVal string) error {
	args := []string{"osd", "pool", "set", name, propName, propVal}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to set pool property %s on pool %s, %+v", propName, name, err)
	}
	return nil
}

func GetPoolStats(context *clusterd.Context, clusterName string) (*CephStoragePoolStats, error) {
	args := []string{"df", "detail"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool stats: %+v", err)
	}

	var poolStats CephStoragePoolStats
	if err := json.Unmarshal(buf, &poolStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pool stats response: %+v", err)
	}

	return &poolStats, nil
}

func GetPools(context *clusterd.Context, clusterName string) ([]model.Pool, error) {
	// list pool summaries using the ceph client
	cephPoolSummaries, err := ListPoolSummaries(context, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}

	// get the details for each pool from its summary information
	cephPools := make([]CephStoragePoolDetails, len(cephPoolSummaries))
	for i := range cephPoolSummaries {
		poolDetails, err := GetPoolDetails(context, clusterName, cephPoolSummaries[i].Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get details for pool %s. %+v", cephPoolSummaries[i].Name, err)
		}

		cephPools[i] = poolDetails
	}

	var ecProfileDetails map[string]CephErasureCodeProfile
	lookupECProfileDetails := false
	for i := range cephPools {
		if cephPools[i].ErasureCodeProfile != "" {
			// at least one pool is erasure coded, we'll need to look up erasure code profile details
			lookupECProfileDetails = true
			break
		}
	}
	if lookupECProfileDetails {
		// list each erasure code profile
		ecProfileNames, err := ListErasureCodeProfiles(context, clusterName)
		if err != nil {
			return nil, fmt.Errorf("failed to list erasure code profiles: %+v", err)
		}

		// get the details of each erasure code profile and store them in the map
		ecProfileDetails = make(map[string]CephErasureCodeProfile, len(ecProfileNames))
		for _, name := range ecProfileNames {
			ecp, err := GetErasureCodeProfileDetails(context, clusterName, name)
			if err != nil {
				return nil, fmt.Errorf("failed to get erasure code profile details for '%s': %+v", name, err)
			}
			ecProfileDetails[name] = ecp
		}
	}

	// convert the ceph pools details to model pools
	pools := make([]model.Pool, len(cephPools))
	for i, p := range cephPools {
		pool, err := cephPoolToModelPool(p, ecProfileDetails)
		if err != nil {
			return nil, fmt.Errorf("failed to convert ceph pool to model. %+v", err)
		}
		pools[i] = pool
	}
	return pools, nil
}

func cephPoolToModelPool(cephPool CephStoragePoolDetails, ecpDetails map[string]CephErasureCodeProfile) (model.Pool, error) {
	pool := model.Pool{
		Name:   cephPool.Name,
		Number: cephPool.Number,
	}

	if cephPool.ErasureCodeProfile != "" {
		ecpDetails, ok := ecpDetails[cephPool.ErasureCodeProfile]
		if !ok {
			return model.Pool{}, fmt.Errorf("failed to look up erasure code profile details for '%s'", cephPool.ErasureCodeProfile)
		}

		pool.Type = model.ErasureCoded
		pool.ErasureCodedConfig.DataChunkCount = ecpDetails.DataChunkCount
		pool.ErasureCodedConfig.CodingChunkCount = ecpDetails.CodingChunkCount
		pool.ErasureCodedConfig.Algorithm = fmt.Sprintf("%s::%s", ecpDetails.Plugin, ecpDetails.Technique)
	} else if cephPool.Size > 0 {
		pool.Type = model.Replicated
		pool.ReplicatedConfig.Size = cephPool.Size
	} else {
		pool.Type = model.PoolTypeUnknown
	}

	return pool, nil
}
