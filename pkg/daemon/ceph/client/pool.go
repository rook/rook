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
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	confirmFlag             = "--yes-i-really-mean-it"
	reallyConfirmFlag       = "--yes-i-really-really-mean-it"
	targetSizeRatioProperty = "target_size_ratio"
	CompressionModeProperty = "compression_mode"
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

func ListPoolSummaries(context *clusterd.Context, clusterInfo *ClusterInfo) ([]CephStoragePoolSummary, error) {
	args := []string{"osd", "lspools"}
	output, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list pools")
	}

	var pools []CephStoragePoolSummary
	err = json.Unmarshal(output, &pools)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(output))
	}

	return pools, nil
}

func GetPoolNamesByID(context *clusterd.Context, clusterInfo *ClusterInfo) (map[int]string, error) {
	pools, err := ListPoolSummaries(context, clusterInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list pools")
	}
	names := map[int]string{}
	for _, p := range pools {
		names[p.Number] = p.Name
	}
	return names, nil
}

// GetPoolDetails gets all the details of a given pool
func GetPoolDetails(context *clusterd.Context, clusterInfo *ClusterInfo, name string) (CephStoragePoolDetails, error) {
	args := []string{"osd", "pool", "get", name, "all"}
	output, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return CephStoragePoolDetails{}, errors.Wrapf(err, "failed to get pool %s details. %s", name, string(output))
	}

	return ParsePoolDetails(output)
}

func ParsePoolDetails(in []byte) (CephStoragePoolDetails, error) {

	// The response for osd pool get when passing var=all is actually malformed JSON similar to:
	// {"pool":"rbd","size":1}{"pool":"rbd","min_size":2}...
	// Note the multiple top level entities, one for each property returned.  To workaround this,
	// we split the JSON response string into its top level entities, then iterate through them, cleaning
	// up the JSON.  A single pool details object is repeatedly used to unmarshal each JSON snippet into.
	// Since previously set fields remain intact if they are not overwritten, the result is the JSON
	// unmarshalling of all properties in the response.
	var poolDetails CephStoragePoolDetails
	poolDetailsUnits := strings.Split(string(in), "}{")
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
			return CephStoragePoolDetails{}, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(in))
		}
	}

	return poolDetails, nil
}

func CreatePoolWithProfile(context *clusterd.Context, clusterInfo *ClusterInfo, clusterSpec *cephv1.ClusterSpec, poolName string, pool cephv1.PoolSpec, appName string) error {
	if pool.IsReplicated() {
		return CreateReplicatedPoolForApp(context, clusterInfo, clusterSpec, poolName, pool, DefaultPGCount, appName)
	}

	if !pool.IsErasureCoded() {
		// neither a replicated or EC pool
		return fmt.Errorf("pool %q type is not defined as replicated or erasure coded", poolName)
	}

	// create a new erasure code profile for the new pool
	ecProfileName := GetErasureCodeProfileForPool(poolName)
	if err := CreateErasureCodeProfile(context, clusterInfo, ecProfileName, pool); err != nil {
		return errors.Wrapf(err, "failed to create erasure code profile for pool %q", poolName)
	}

	// If the pool is not a replicated pool, then the only other option is an erasure coded pool.
	return CreateECPoolForApp(
		context,
		clusterInfo,
		poolName,
		ecProfileName,
		pool,
		DefaultPGCount,
		appName,
		true /* enableECOverwrite */)
}

func checkForImagesInPool(context *clusterd.Context, clusterInfo *ClusterInfo, name string) error {
	var err error
	logger.Debugf("checking any images/snapshosts present in pool %q", name)
	stats, err := GetPoolStatistics(context, clusterInfo, name)
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
func DeletePool(context *clusterd.Context, clusterInfo *ClusterInfo, name string) error {
	// check if the pool exists
	pool, err := GetPoolDetails(context, clusterInfo, name)
	if err != nil {
		return errors.Wrapf(err, "failed to get pool %q details", name)
	}

	err = checkForImagesInPool(context, clusterInfo, name)
	if err != nil {
		return errors.Wrapf(err, "failed to check if pool %q has rbd images", name)
	}

	logger.Infof("purging pool %q (id=%d)", name, pool.Number)
	args := []string{"osd", "pool", "delete", name, name, reallyConfirmFlag}
	_, err = NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete pool %q", name)
	}

	// remove the crush rule for this pool and ignore the error in case the rule is still in use or not found
	args = []string{"osd", "crush", "rule", "rm", name}
	_, err = NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		logger.Errorf("failed to delete crush rule %q. %v", name, err)
	}

	logger.Infof("purge completed for pool %q", name)
	return nil
}

func givePoolAppTag(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, appName string) error {
	args := []string{"osd", "pool", "application", "enable", poolName, appName, confirmFlag}
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable application %s on pool %s", appName, poolName)
	}

	return nil
}

func setCommonPoolProperties(context *clusterd.Context, clusterInfo *ClusterInfo, pool cephv1.PoolSpec, poolName, appName string) error {
	if len(pool.Parameters) == 0 {
		pool.Parameters = make(map[string]string)
	}

	if pool.Replicated.IsTargetRatioEnabled() {
		pool.Parameters[targetSizeRatioProperty] = strconv.FormatFloat(pool.Replicated.TargetSizeRatio, 'f', -1, 32)
	}

	if pool.IsCompressionEnabled() {
		pool.Parameters[CompressionModeProperty] = pool.CompressionMode
	}

	// Apply properties
	for propName, propValue := range pool.Parameters {
		err := SetPoolProperty(context, clusterInfo, poolName, propName, propValue)
		if err != nil {
			logger.Errorf("failed to set property %q to pool %q to %q. %v", propName, poolName, propValue, err)
		}
	}

	// ensure that the newly created pool gets an application tag
	if appName != "" {
		err := givePoolAppTag(context, clusterInfo, poolName, appName)
		if err != nil {
			return errors.Wrapf(err, "failed to tag pool %q for application %q", poolName, appName)
		}
	}

	// If the pool is mirrored, let's enable mirroring
	// we don't need to check if the pool is erasure coded or not, mirroring will still work, it will simply be slow
	if pool.Mirroring.Enabled {
		err := enablePoolMirroring(context, clusterInfo, pool, poolName)
		if err != nil {
			return errors.Wrapf(err, "failed to enable mirroring for pool %q", poolName)
		}

		// Schedule snapshots
		if pool.Mirroring.SnapshotSchedulesEnabled() && clusterInfo.CephVersion.IsAtLeastOctopus() {
			err = enableSnapshotSchedules(context, clusterInfo, pool, poolName)
			if err != nil {
				return errors.Wrapf(err, "failed to enable snapshot scheduling for pool %q", poolName)
			}
		}
	} else {
		if pool.Mirroring.Mode == "pool" {
			// Remove storage cluster peers
			mirrorInfo, err := GetPoolMirroringInfo(context, clusterInfo, poolName)
			if err != nil {
				return errors.Wrapf(err, "failed to get mirroring info for the pool %q", poolName)
			}
			for _, peer := range mirrorInfo.Peers {
				if peer.UUID != "" {
					err := removeClusterPeer(context, clusterInfo, poolName, peer.UUID)
					if err != nil {
						return errors.Wrapf(err, "failed to remove cluster peer with UUID %q for the pool %q", peer.UUID, poolName)
					}
				}
			}

			// Disable mirroring
			err = disablePoolMirroring(context, clusterInfo, poolName)
			if err != nil {
				return errors.Wrapf(err, "failed to disable mirroring for pool %q", poolName)
			}
		} else if pool.Mirroring.Mode == "image" {
			logger.Warningf("manually disable mirroring on images in the pool %q", poolName)
		}
	}

	// set maxSize quota
	if pool.Quotas.MaxSize != nil {
		// check for format errors
		maxBytesQuota, err := resource.ParseQuantity(*pool.Quotas.MaxSize)
		if err != nil {
			if err == resource.ErrFormatWrong {
				return errors.Wrapf(err, "maxSize quota incorrectly formatted for pool %q, valid units include k, M, G, T, P, E, Ki, Mi, Gi, Ti, Pi, Ei", poolName)
			}
			return errors.Wrapf(err, "failed setting quota for pool %q, maxSize quota parse error", poolName)
		}
		// set max_bytes quota, 0 value disables quota
		err = setPoolQuota(context, clusterInfo, poolName, "max_bytes", strconv.FormatInt(maxBytesQuota.Value(), 10))
		if err != nil {
			return errors.Wrapf(err, "failed to set max_bytes quota for pool %q", poolName)
		}
	} else if pool.Quotas.MaxBytes != nil {
		// set max_bytes quota, 0 value disables quota
		err := setPoolQuota(context, clusterInfo, poolName, "max_bytes", strconv.FormatUint(*pool.Quotas.MaxBytes, 10))
		if err != nil {
			return errors.Wrapf(err, "failed to set max_bytes quota for pool %q", poolName)
		}
	}
	// set max_objects quota
	if pool.Quotas.MaxObjects != nil {
		// set max_objects quota, 0 value disables quota
		err := setPoolQuota(context, clusterInfo, poolName, "max_objects", strconv.FormatUint(*pool.Quotas.MaxObjects, 10))
		if err != nil {
			return errors.Wrapf(err, "failed to set max_objects quota for pool %q", poolName)
		}
	}

	return nil
}

func GetErasureCodeProfileForPool(baseName string) string {
	return fmt.Sprintf("%s_ecprofile", baseName)
}

func CreateECPoolForApp(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, ecProfileName string, pool cephv1.PoolSpec, pgCount, appName string, enableECOverwrite bool) error {
	args := []string{"osd", "pool", "create", poolName, pgCount, "erasure", ecProfileName}
	output, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create EC pool %s. %s", poolName, string(output))
	}

	if enableECOverwrite {
		if err = SetPoolProperty(context, clusterInfo, poolName, "allow_ec_overwrites", "true"); err != nil {
			return errors.Wrapf(err, "failed to allow EC overwrite for pool %s", poolName)
		}
	}

	if err = setCommonPoolProperties(context, clusterInfo, pool, poolName, appName); err != nil {
		return err
	}

	logger.Infof("creating EC pool %s succeeded", poolName)
	return nil
}

func CreateReplicatedPoolForApp(context *clusterd.Context, clusterInfo *ClusterInfo, clusterSpec *cephv1.ClusterSpec, poolName string, pool cephv1.PoolSpec, pgCount, appName string) error {
	// The crush rule name is the same as the pool unless we have a stretch cluster.
	crushRuleName := poolName
	if clusterSpec.IsStretchCluster() {
		// A stretch cluster enforces using the same crush rule for all pools.
		// The stretch cluster rule is created initially by the operator when the stretch cluster is configured
		// so there is no need to create a new crush rule for the pools here.
		crushRuleName = defaultStretchCrushRuleName
	} else if pool.IsHybridStoragePool() {
		// Create hybrid crush rule
		err := createHybridCrushRule(context, clusterInfo, clusterSpec, crushRuleName, pool)
		if err != nil {
			return errors.Wrapf(err, "failed to create hybrid crush rule %q", crushRuleName)
		}
	} else {
		if pool.Replicated.ReplicasPerFailureDomain > 1 {
			// Create a two-step CRUSH rule for pools other than stretch clusters
			err := createStretchCrushRule(context, clusterInfo, clusterSpec, crushRuleName, pool)
			if err != nil {
				return errors.Wrapf(err, "failed to create two-step crush rule %q", crushRuleName)
			}
		} else {
			// create a crush rule for a replicated pool, if a failure domain is specified
			if err := createReplicationCrushRule(context, clusterInfo, clusterSpec, crushRuleName, pool); err != nil {
				return errors.Wrapf(err, "failed to create replicated crush rule %q", crushRuleName)
			}
		}
	}

	args := []string{"osd", "pool", "create", poolName, pgCount, "replicated", crushRuleName, "--size", strconv.FormatUint(uint64(pool.Replicated.Size), 10)}
	output, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create replicated pool %s. %s", poolName, string(output))
	}

	if !clusterSpec.IsStretchCluster() {
		// the pool is type replicated, set the size for the pool now that it's been created
		// Only set the size if not 0, otherwise ceph will fail to set size to 0
		if pool.Replicated.Size > 0 {
			if err := SetPoolReplicatedSizeProperty(context, clusterInfo, poolName, strconv.FormatUint(uint64(pool.Replicated.Size), 10)); err != nil {
				return errors.Wrapf(err, "failed to set size property to replicated pool %q to %d", poolName, pool.Replicated.Size)
			}
		}
	}

	if err = setCommonPoolProperties(context, clusterInfo, pool, poolName, appName); err != nil {
		return err
	}

	logger.Infof("creating replicated pool %s succeeded", poolName)
	return nil
}

func createStretchCrushRule(context *clusterd.Context, clusterInfo *ClusterInfo, clusterSpec *cephv1.ClusterSpec, ruleName string, pool cephv1.PoolSpec) error {
	// set the crush root to the default if not already specified
	if pool.CrushRoot == "" {
		pool.CrushRoot = GetCrushRootFromSpec(clusterSpec)
	}

	// set the crush failure domain to the "host" if not already specified
	if pool.FailureDomain == "" {
		pool.FailureDomain = cephv1.DefaultFailureDomain
	}

	// set the crush failure sub domain to the "host" if not already specified
	if pool.Replicated.SubFailureDomain == "" {
		pool.Replicated.SubFailureDomain = cephv1.DefaultFailureDomain
	}

	if pool.FailureDomain == pool.Replicated.SubFailureDomain {
		return errors.Errorf("failure and subfailure domains cannot be identical, current is %q", pool.FailureDomain)
	}

	crushMap, err := getCurrentCrushMap(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get current crush map")
	}

	if crushRuleExists(crushMap, ruleName) {
		logger.Debugf("CRUSH rule %q already exists", ruleName)
		return nil
	}

	// Build plain text rule
	ruleset := buildTwoStepPlainCrushRule(crushMap, ruleName, pool)

	return updateCrushMap(context, clusterInfo, ruleset)
}

func createHybridCrushRule(context *clusterd.Context, clusterInfo *ClusterInfo, clusterSpec *cephv1.ClusterSpec, ruleName string, pool cephv1.PoolSpec) error {
	// set the crush root to the default if not already specified
	if pool.CrushRoot == "" {
		pool.CrushRoot = GetCrushRootFromSpec(clusterSpec)
	}

	// set the crush failure domain to the "host" if not already specified
	if pool.FailureDomain == "" {
		pool.FailureDomain = cephv1.DefaultFailureDomain
	}

	crushMap, err := getCurrentCrushMap(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get current crush map")
	}

	if crushRuleExists(crushMap, ruleName) {
		logger.Debugf("CRUSH rule %q already exists", ruleName)
		return nil
	}

	ruleset := buildTwoStepHybridCrushRule(crushMap, ruleName, pool)

	return updateCrushMap(context, clusterInfo, ruleset)
}

func updateCrushMap(context *clusterd.Context, clusterInfo *ClusterInfo, ruleset string) error {

	// Fetch the compiled crush map
	compiledCRUSHMapFilePath, err := GetCompiledCrushMap(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get crush map")
	}
	defer func() {
		err := os.Remove(compiledCRUSHMapFilePath)
		if err != nil {
			logger.Errorf("failed to remove file %q. %v", compiledCRUSHMapFilePath, err)
		}
	}()

	// Decompile the plain text to CRUSH binary format
	err = decompileCRUSHMap(context, compiledCRUSHMapFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to compile crush map")
	}
	decompiledCRUSHMapFilePath := buildDecompileCRUSHFileName(compiledCRUSHMapFilePath)
	defer func() {
		err := os.Remove(decompiledCRUSHMapFilePath)
		if err != nil {
			logger.Errorf("failed to remove file %q. %v", decompiledCRUSHMapFilePath, err)
		}
	}()

	// Append plain rule to the decompiled crush map
	f, err := os.OpenFile(filepath.Clean(decompiledCRUSHMapFilePath), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0400)
	if err != nil {
		return errors.Wrapf(err, "failed to open decompiled crush map %q", decompiledCRUSHMapFilePath)
	}
	defer func() {
		err := f.Close()
		if err != nil {
			logger.Errorf("failed to close file %q. %v", f.Name(), err)
		}
	}()

	// Append the new crush rule into the crush map
	if _, err := f.WriteString(ruleset); err != nil {
		return errors.Wrapf(err, "failed to append replicated plain crush rule to decompiled crush map %q", decompiledCRUSHMapFilePath)
	}

	// Compile the plain text to CRUSH binary format
	err = compileCRUSHMap(context, decompiledCRUSHMapFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to compile crush map")
	}
	defer func() {
		err := os.Remove(buildCompileCRUSHFileName(decompiledCRUSHMapFilePath))
		if err != nil {
			logger.Errorf("failed to remove file %q. %v", buildCompileCRUSHFileName(decompiledCRUSHMapFilePath), err)
		}
	}()

	// Inject the new CRUSH Map
	err = injectCRUSHMap(context, clusterInfo, buildCompileCRUSHFileName(decompiledCRUSHMapFilePath))
	if err != nil {
		return errors.Wrap(err, "failed to inject crush map")
	}

	return nil
}

func createReplicationCrushRule(context *clusterd.Context, clusterInfo *ClusterInfo, clusterSpec *cephv1.ClusterSpec, ruleName string, pool cephv1.PoolSpec) error {
	failureDomain := pool.FailureDomain
	if failureDomain == "" {
		failureDomain = cephv1.DefaultFailureDomain
	}
	// set the crush root to the default if not already specified
	crushRoot := pool.CrushRoot
	if pool.CrushRoot == "" {
		crushRoot = GetCrushRootFromSpec(clusterSpec)
	}

	args := []string{"osd", "crush", "rule", "create-replicated", ruleName, crushRoot, failureDomain}

	var deviceClass string
	if pool.DeviceClass != "" {
		deviceClass = pool.DeviceClass
		args = append(args, deviceClass)
	}

	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create crush rule %s", ruleName)
	}

	return nil
}

// SetPoolProperty sets a property to a given pool
func SetPoolProperty(context *clusterd.Context, clusterInfo *ClusterInfo, name, propName, propVal string) error {
	args := []string{"osd", "pool", "set", name, propName, propVal}
	logger.Infof("setting pool property %q to %q on pool %q", propName, propVal, name)
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set pool property %q on pool %q", propName, name)
	}
	return nil
}

// setPoolQuota sets quotas on a given pool
func setPoolQuota(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, quotaType, quotaVal string) error {
	args := []string{"osd", "pool", "set-quota", poolName, quotaType, quotaVal}
	logger.Infof("setting quota %q=%q on pool %q", quotaType, quotaVal, poolName)
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set %q quota on pool %q", quotaType, poolName)
	}
	return nil
}

// SetPoolReplicatedSizeProperty sets the replica size of a pool
func SetPoolReplicatedSizeProperty(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, size string) error {
	propName := "size"
	args := []string{"osd", "pool", "set", poolName, propName, size}
	if size == "1" {
		args = append(args, "--yes-i-really-mean-it")
	}

	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set pool property %q on pool %q", propName, poolName)
	}

	return nil
}

func GetPoolStats(context *clusterd.Context, clusterInfo *ClusterInfo) (*CephStoragePoolStats, error) {
	args := []string{"df", "detail"}
	output, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get pool stats")
	}

	var poolStats CephStoragePoolStats
	if err := json.Unmarshal(output, &poolStats); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal pool stats response")
	}

	return &poolStats, nil
}

func GetPoolStatistics(context *clusterd.Context, clusterInfo *ClusterInfo, name string) (*PoolStatistics, error) {
	args := []string{"pool", "stats", name}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true
	output, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get pool stats")
	}

	var poolStats PoolStatistics
	if err := json.Unmarshal(output, &poolStats); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal pool stats response")
	}

	return &poolStats, nil
}

func crushRuleExists(crushMap CrushMap, ruleName string) bool {
	// Check if the crush rule already exists
	for _, rule := range crushMap.Rules {
		if rule.Name == ruleName {
			return true
		}
	}

	return false
}

func getCurrentCrushMap(context *clusterd.Context, clusterInfo *ClusterInfo) (CrushMap, error) {
	crushMap, err := GetCrushMap(context, clusterInfo)
	if err != nil {
		return CrushMap{}, errors.Wrap(err, "failed to get crush map")
	}

	return crushMap, nil
}
