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
)

const (
	confirmFlag             = "--yes-i-really-mean-it"
	reallyConfirmFlag       = "--yes-i-really-really-mean-it"
	targetSizeRatioProperty = "target_size_ratio"
	compressionModeProperty = "compression_mode"
	PgAutoscaleModeProperty = "pg_autoscale_mode"
	PgAutoscaleModeOn       = "on"
)

type CephStoragePoolSummary struct {
	Name   string `json:"poolname"`
	Number int    `json:"poolnum"`
}

type CephStoragePoolDetails struct {
	Name                   string  `json:"pool"`
	Number                 int     `json:"pool_id"`
	Size                   uint    `json:"size"`
	ErasureCodeProfile     string  `json:"erasure_code_profile"`
	FailureDomain          string  `json:"failureDomain"`
	CrushRoot              string  `json:"crushRoot"`
	DeviceClass            string  `json:"deviceClass"`
	CompressionMode        string  `json:"compression_mode"`
	TargetSizeRatio        float64 `json:"target_size_ratio,omitempty"`
	RequireSafeReplicaSize bool    `json:"requireSafeReplicaSize,omitempty"`
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

func ListPoolSummaries(context *clusterd.Context, namespace string) ([]CephStoragePoolSummary, error) {
	args := []string{"osd", "lspools"}
	output, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pools")
	}

	var pools []CephStoragePoolSummary
	err = json.Unmarshal(output, &pools)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(output))
	}

	return pools, nil
}

func GetPoolNamesByID(context *clusterd.Context, namespace string) (map[int]string, error) {
	pools, err := ListPoolSummaries(context, namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pools")
	}
	names := map[int]string{}
	for _, p := range pools {
		names[p.Number] = p.Name
	}
	return names, nil
}

// GetPoolDetails gets all the details of a given pool
func GetPoolDetails(context *clusterd.Context, namespace, name string) (CephStoragePoolDetails, error) {
	args := []string{"osd", "pool", "get", name, "all"}
	output, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return CephStoragePoolDetails{}, errors.Wrapf(err, "failed to get pool %s details. %s", name, string(output))
	}

	// The response for osd pool get when passing var=all is actually malformed JSON similar to:
	// {"pool":"rbd","size":1}{"pool":"rbd","min_size":2}...
	// Note the multiple top level entities, one for each property returned.  To workaround this,
	// we split the JSON response string into its top level entities, then iterate through them, cleaning
	// up the JSON.  A single pool details object is repeatedly used to unmarshal each JSON snippet into.
	// Since previously set fields remain intact if they are not overwritten, the result is the JSON
	// unmarshalling of all properties in the response.
	var poolDetails CephStoragePoolDetails
	poolDetailsUnits := strings.Split(string(output), "}{")
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
			return CephStoragePoolDetails{}, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(output))
		}
	}

	return poolDetails, nil
}

func CreatePoolWithProfile(context *clusterd.Context, namespace, poolName string, pool cephv1.PoolSpec, appName string) error {
	if pool.IsReplicated() {
		return CreateReplicatedPoolForApp(context, namespace, poolName, pool, DefaultPGCount, appName)
	}

	if !pool.IsErasureCoded() {
		// neither a replicated or EC pool
		return fmt.Errorf("pool %q type is not defined as replicated or erasure coded", poolName)
	}

	// create a new erasure code profile for the new pool
	ecProfileName := GetErasureCodeProfileForPool(poolName)
	if err := CreateErasureCodeProfile(context, namespace, ecProfileName, pool); err != nil {
		return errors.Wrapf(err, "failed to create erasure code profile for pool %q", poolName)
	}

	// If the pool is not a replicated pool, then the only other option is an erasure coded pool.
	return CreateECPoolForApp(
		context,
		namespace,
		poolName,
		ecProfileName,
		pool,
		DefaultPGCount,
		appName,
		true /* enableECOverwrite */)
}

func checkForImagesInPool(context *clusterd.Context, name, namespace string) error {
	var err error
	var stats = new(PoolStatistics)
	logger.Debugf("checking any images/snapshosts present in pool %q", name)
	stats, err = GetPoolStatistics(context, name, namespace)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			return nil
		}
		return errors.Wrapf(err, "failed to list images/snapshosts in pool %s", name)
	}
	if stats.Images.Count == 0 && stats.Images.SnapCount == 0 {
		logger.Infof("no images/snapshosts present in pool %q", name)
		return nil
	}

	return errors.Errorf("pool %q contains images/snapshosts", name)
}

// DeletePool purges a pool from Ceph
func DeletePool(context *clusterd.Context, namespace string, name string) error {
	// check if the pool exists
	pool, err := GetPoolDetails(context, namespace, name)
	if err != nil {
		return errors.Wrapf(err, "failed to get pool %q details", name)
	}

	err = checkForImagesInPool(context, name, namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to check if pool %q has rbd images", name)
	}

	logger.Infof("purging pool %q (id=%d)", name, pool.Number)
	args := []string{"osd", "pool", "delete", name, name, reallyConfirmFlag}
	_, err = NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete pool %q", name)
	}

	// remove the crush rule for this pool and ignore the error in case the rule is still in use or not found
	args = []string{"osd", "crush", "rule", "rm", name}
	_, err = NewCephCommand(context, namespace, args).Run()
	if err != nil {
		logger.Errorf("failed to delete crush rule %q. %v", name, err)
	}

	logger.Infof("purge completed for pool %q", name)
	return nil
}

func givePoolAppTag(context *clusterd.Context, namespace string, poolName string, appName string) error {
	args := []string{"osd", "pool", "application", "enable", poolName, appName, confirmFlag}
	_, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable application %s on pool %s", appName, poolName)
	}

	return nil
}

func setCommonPoolProperties(context *clusterd.Context, pool cephv1.PoolSpec, namespace, poolName, appName string) error {
	if len(pool.Parameters) == 0 {
		pool.Parameters = make(map[string]string)
	}

	if pool.Replicated.IsTargetRatioEnabled() {
		pool.Parameters[targetSizeRatioProperty] = strconv.FormatFloat(pool.Replicated.TargetSizeRatio, 'f', -1, 32)
	}

	if pool.IsCompressionEnabled() {
		pool.Parameters[compressionModeProperty] = pool.CompressionMode
	}

	// Apply properties
	for propName, propValue := range pool.Parameters {
		err := SetPoolProperty(context, namespace, poolName, propName, propValue)
		if err != nil {
			logger.Errorf("failed to set property %q to pool %q to %q. %v", propName, poolName, propValue, err)
		}
	}

	// ensure that the newly created pool gets an application tag
	if appName != "" {
		err := givePoolAppTag(context, namespace, poolName, appName)
		if err != nil {
			return errors.Wrapf(err, "failed to tag pool %q for application %q", poolName, appName)
		}
	}

	return nil
}

func GetErasureCodeProfileForPool(baseName string) string {
	return fmt.Sprintf("%s_ecprofile", baseName)
}

func CreateECPoolForApp(context *clusterd.Context, namespace, poolName, ecProfileName string, pool cephv1.PoolSpec, pgCount, appName string, enableECOverwrite bool) error {
	args := []string{"osd", "pool", "create", poolName, pgCount, "erasure", ecProfileName}
	output, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create EC pool %s. %s", poolName, string(output))
	}

	if enableECOverwrite {
		if err = SetPoolProperty(context, namespace, poolName, "allow_ec_overwrites", "true"); err != nil {
			return errors.Wrapf(err, "failed to allow EC overwrite for pool %s", poolName)
		}
	}

	if err = setCommonPoolProperties(context, pool, namespace, poolName, appName); err != nil {
		return err
	}

	logger.Infof("creating EC pool %s succeeded", poolName)
	return nil
}

func CreateReplicatedPoolForApp(context *clusterd.Context, namespace, poolName string, pool cephv1.PoolSpec, pgCount, appName string) error {
	// create a crush rule for a replicated pool, if a failure domain is specified
	if err := createReplicationCrushRule(context, namespace, poolName, pool); err != nil {
		return err
	}

	args := []string{"osd", "pool", "create", poolName, pgCount, "replicated", poolName, "--size", strconv.FormatUint(uint64(pool.Replicated.Size), 10)}
	output, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create replicated pool %s. %s", poolName, string(output))
	}

	// the pool is type replicated, set the size for the pool now that it's been created
	if err := SetPoolReplicatedSizeProperty(context, namespace, poolName, strconv.FormatUint(uint64(pool.Replicated.Size), 10)); err != nil {
		return errors.Wrapf(err, "failed to set size property to replicated pool %q to %d", poolName, pool.Replicated.Size)
	}

	if err = setCommonPoolProperties(context, pool, namespace, poolName, appName); err != nil {
		return err
	}

	logger.Infof("creating replicated pool %s succeeded", poolName)
	return nil
}

func createReplicationCrushRule(context *clusterd.Context, namespace, ruleName string, pool cephv1.PoolSpec) error {
	failureDomain := pool.FailureDomain
	if failureDomain == "" {
		failureDomain = cephv1.DefaultFailureDomain
	}

	// set the crush root to the default if not already specified
	crushRoot := "default"
	if pool.CrushRoot != "" {
		crushRoot = pool.CrushRoot
	}
	args := []string{"osd", "crush", "rule", "create-replicated", ruleName, crushRoot, failureDomain}

	var deviceClass string
	if pool.DeviceClass != "" {
		deviceClass = pool.DeviceClass
		args = append(args, deviceClass)
	}

	_, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create crush rule %s", ruleName)
	}

	return nil
}

// SetPoolProperty sets a property to a given pool
func SetPoolProperty(context *clusterd.Context, namespace, name, propName, propVal string) error {
	args := []string{"osd", "pool", "set", name, propName, propVal}
	logger.Infof("setting pool property %q to %q on pool %q", propName, propVal, name)
	_, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set pool property %q on pool %q", propName, name)
	}
	return nil
}

// SetPoolReplicatedSizeProperty sets the replica size of a pool
func SetPoolReplicatedSizeProperty(context *clusterd.Context, namespace, poolName, size string) error {
	propName := "size"
	args := []string{"osd", "pool", "set", poolName, propName, size}
	if size == "1" {
		args = append(args, "--yes-i-really-mean-it")
	}

	_, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set pool property %q on pool %q", propName, poolName)
	}

	return nil
}

func GetPoolStats(context *clusterd.Context, namespace string) (*CephStoragePoolStats, error) {
	args := []string{"df", "detail"}
	output, err := NewCephCommand(context, namespace, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pool stats")
	}

	var poolStats CephStoragePoolStats
	if err := json.Unmarshal(output, &poolStats); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal pool stats response")
	}

	return &poolStats, nil
}

func GetPoolStatistics(context *clusterd.Context, name, namespace string) (*PoolStatistics, error) {
	args := []string{"pool", "stats", name}
	cmd := NewRBDCommand(context, namespace, args)
	cmd.JsonOutput = true
	output, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pool stats")
	}

	var poolStats PoolStatistics
	if err := json.Unmarshal(output, &poolStats); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal pool stats response")
	}

	return &poolStats, nil
}
