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
package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/model"
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
	FailureDomain      string `json:"failureDomain"`
	CrushRoot          string `json:"crushRoot"`
	DeviceClass        string `json:"deviceClass"`
	NotEnableAppPool   bool   `json:"notEnableAppPool"`
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

type PoolStatistics struct {
	Images struct {
		Count            int `json:"count"`
		ProvisionedBytes int `json:"provisioned_bytes"`
		SnapCount        int `json:"snap_count"`
	} `json:"images"`
	Trash struct {
		Count            int `json:"count"`
		ProvisionedBytes int `json:"provisioned_bytes"`
		SnapCount        int `json:"snap_count"`
	} `json:"trash"`
}

func ListPoolSummaries(context *clusterd.Context, clusterName string) ([]CephStoragePoolSummary, error) {
	args := []string{"osd", "lspools"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pools")
	}

	var pools []CephStoragePoolSummary
	err = json.Unmarshal(buf, &pools)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(buf))
	}

	return pools, nil
}

func GetPoolNamesByID(context *clusterd.Context, clusterName string) (map[int]string, error) {
	pools, err := ListPoolSummaries(context, clusterName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pools")
	}
	names := map[int]string{}
	for _, p := range pools {
		names[p.Number] = p.Name
	}
	return names, nil
}

func GetPoolDetails(context *clusterd.Context, clusterName, name string) (CephStoragePoolDetails, error) {
	args := []string{"osd", "pool", "get", name, "all"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return CephStoragePoolDetails{}, errors.Wrapf(err, "failed to get pool %s details", name)
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
			return CephStoragePoolDetails{}, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(buf))
		}
	}

	return poolDetails, nil
}

func CreatePoolWithProfile(context *clusterd.Context, clusterName string, newPoolReq model.Pool, appName string) error {
	newPool := ModelPoolToCephPool(newPoolReq)
	if newPoolReq.Type == model.ErasureCoded {
		// create a new erasure code profile for the new pool
		if err := CreateErasureCodeProfile(context, clusterName, newPoolReq.ErasureCodedConfig, newPool.ErasureCodeProfile,
			newPoolReq.FailureDomain, newPoolReq.CrushRoot, newPoolReq.DeviceClass); err != nil {

			return errors.Wrapf(err, "failed to create erasure code profile for pool %q", newPoolReq.Name)
		}
	}

	isReplicatedPool := newPool.ErasureCodeProfile == "" && newPool.Size > 0
	if isReplicatedPool {
		return CreateReplicatedPoolForApp(context, clusterName, newPool, appName)
	}
	// If the pool is not a replicated pool, then the only other option is an erasure coded pool.
	return CreateECPoolForApp(
		context,
		clusterName,
		newPool,
		appName,
		true, /* enableECOverwrite */
		newPoolReq.ErasureCodedConfig,
	)
}

func checkForImagesInPool(context *clusterd.Context, name, namespace string) error {
	var err error
	var stats = new(PoolStatistics)
	logger.Infof("checking any images/snapshosts present in pool %s", name)
	stats, err = GetPoolStatistics(context, name, namespace)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			return nil
		}
		return errors.Wrapf(err, "failed to list images/snapshosts in pool %s", name)
	}
	if stats.Images.Count == 0 && stats.Images.SnapCount == 0 {
		logger.Infof("no images/snapshosts present in pool %s", name)
		return nil
	}

	return errors.Errorf("pool %s contains images/snapshosts", name)
}

func DeletePool(context *clusterd.Context, clusterName string, name string) error {
	// check if the pool exists
	pool, err := GetPoolDetails(context, clusterName, name)
	if err != nil {
		logger.Infof("pool %q not found for deletion. %v", name, err)
		return nil
	}

	err = checkForImagesInPool(context, name, clusterName)
	if err != nil {
		return errors.Wrapf(err, "failed to delete pool %q", name)
	}
	logger.Infof("purging pool %s (id=%d)", name, pool.Number)
	args := []string{"osd", "pool", "delete", name, name, reallyConfirmFlag}
	_, err = NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete pool %q", name)
	}

	// remove the crush rule for this pool and ignore the error in case the rule is still in use or not found
	args = []string{"osd", "crush", "rule", "rm", name}
	_, err = NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		logger.Infof("did not delete crush rule %q. %v", name, err)
	}

	logger.Infof("purge completed for pool %q", name)
	return nil
}

func givePoolAppTag(context *clusterd.Context, clusterName string, poolName string, appName string) error {
	args := []string{"osd", "pool", "application", "enable", poolName, appName, confirmFlag}
	_, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable application %s on pool %s", appName, poolName)
	}

	return nil
}

func CreateECPoolForApp(context *clusterd.Context, clusterName string, newPool CephStoragePoolDetails, appName string, enableECOverwrite bool, erasureCodedConfig model.ErasureCodedPoolConfig) error {
	args := []string{"osd", "pool", "create", newPool.Name, strconv.Itoa(newPool.Number), "erasure", newPool.ErasureCodeProfile}

	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create EC pool %s", newPool.Name)
	}

	if enableECOverwrite {
		if err = SetPoolProperty(context, clusterName, newPool.Name, "allow_ec_overwrites", "true"); err != nil {
			return errors.Wrapf(err, "failed to allow EC overwrite for pool %s", newPool.Name)
		}
	}

	if !newPool.NotEnableAppPool {
		err = givePoolAppTag(context, clusterName, newPool.Name, appName)
		if err != nil {
			return err
		}
	}

	logger.Infof("creating EC pool %s succeeded, buf: %s", newPool.Name, string(buf))
	return nil
}

func CreateReplicatedPoolForApp(context *clusterd.Context, clusterName string, newPool CephStoragePoolDetails, appName string) error {
	// create a crush rule for a replicated pool, if a failure domain is specified
	if err := createReplicationCrushRule(context, clusterName, newPool, newPool.Name); err != nil {
		return err
	}

	args := []string{"osd", "pool", "create", newPool.Name, strconv.Itoa(newPool.Number), "replicated", newPool.Name}

	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create replicated pool %s", newPool.Name)
	}

	// the pool is type replicated, set the size for the pool now that it's been created
	if err = SetPoolProperty(context, clusterName, newPool.Name, "size", strconv.FormatUint(uint64(newPool.Size), 10)); err != nil {
		return err
	}

	// ensure that the newly created pool gets an application tag
	if !newPool.NotEnableAppPool {
		err = givePoolAppTag(context, clusterName, newPool.Name, appName)
		if err != nil {
			return err
		}
	}

	logger.Infof("creating replicated pool %s succeeded, buf: %s", newPool.Name, string(buf))
	return nil
}

func createReplicationCrushRule(context *clusterd.Context, clusterName string, newPool CephStoragePoolDetails, ruleName string) error {
	failureDomain := newPool.FailureDomain
	if failureDomain == "" {
		failureDomain = cephv1.DefaultFailureDomain
	}

	// set the crush root to the default if not already specified
	crushRoot := "default"
	if newPool.CrushRoot != "" {
		crushRoot = newPool.CrushRoot
	}
	args := []string{"osd", "crush", "rule", "create-replicated", ruleName, crushRoot, failureDomain}

	var deviceClass string
	if newPool.DeviceClass != "" {
		deviceClass = newPool.DeviceClass
		args = append(args, deviceClass)
	}

	_, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create crush rule %s", ruleName)
	}

	return nil
}

func SetPoolProperty(context *clusterd.Context, clusterName, name, propName string, propVal string) error {
	args := []string{"osd", "pool", "set", name, propName, propVal}
	_, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set pool property %s on pool %s", propName, name)
	}
	return nil
}

func GetPoolStats(context *clusterd.Context, clusterName string) (*CephStoragePoolStats, error) {
	args := []string{"df", "detail"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pool stats")
	}

	var poolStats CephStoragePoolStats
	if err := json.Unmarshal(buf, &poolStats); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal pool stats response")
	}

	return &poolStats, nil
}

func GetPoolStatistics(context *clusterd.Context, name, clusterName string) (*PoolStatistics, error) {
	args := []string{"pool", "stats", name}
	cmd := NewRBDCommand(context, clusterName, args)
	cmd.JsonOutput = true
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pool stats")
	}

	var poolStats PoolStatistics
	if err := json.Unmarshal(buf, &poolStats); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal pool stats response")
	}

	return &poolStats, nil
}

func GetPools(context *clusterd.Context, clusterName string) ([]model.Pool, error) {
	// list pool summaries using the ceph client
	cephPoolSummaries, err := ListPoolSummaries(context, clusterName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pools")
	}

	// get the details for each pool from its summary information
	cephPools := make([]CephStoragePoolDetails, len(cephPoolSummaries))
	for i := range cephPoolSummaries {
		poolDetails, err := GetPoolDetails(context, clusterName, cephPoolSummaries[i].Name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get details for pool %s", cephPoolSummaries[i].Name)
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
			return nil, errors.Wrapf(err, "failed to list erasure code profiles")
		}

		// get the details of each erasure code profile and store them in the map
		ecProfileDetails = make(map[string]CephErasureCodeProfile, len(ecProfileNames))
		for _, name := range ecProfileNames {
			ecp, err := GetErasureCodeProfileDetails(context, clusterName, name)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get erasure code profile details for %q", name)
			}
			ecProfileDetails[name] = ecp
		}
	}

	// convert the ceph pools details to model pools
	pools := make([]model.Pool, len(cephPools))
	for i, p := range cephPools {
		pool, err := cephPoolToModelPool(p, ecProfileDetails)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert ceph pool to model")
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
			return model.Pool{}, errors.Errorf("failed to look up erasure code profile details for %q", cephPool.ErasureCodeProfile)
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
