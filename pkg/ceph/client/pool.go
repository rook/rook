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

func CreatePool(context *clusterd.Context, clusterName string, newPool CephStoragePoolDetails) (string, error) {
	args := []string{"osd", "pool", "create", newPool.Name, strconv.Itoa(newPool.Number)}
	// not implemented: fix the pool create for the different profiles
	if newPool.ErasureCodeProfile != "" {
		args = append(args, "erasure", newPool.ErasureCodeProfile)
	} else {
		args = append(args, "replicated")
	}

	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return "", fmt.Errorf("mon_command failed. %+v", err)
	}

	if newPool.ErasureCodeProfile == "" && newPool.Size > 0 {
		// the pool is type replicated, set the size for the pool now that it's been created
		if err = SetPoolProperty(context, clusterName, newPool.Name, "size", strconv.FormatUint(uint64(newPool.Size), 10)); err != nil {
			return "", err
		}
	}

	logger.Infof("creating pool %s succeeded, buf: %s", newPool.Name, string(buf))
	return string(buf), nil
}

func SetPoolProperty(context *clusterd.Context, clusterName, name, propName string, propVal string) error {
	args := []string{"osd", "pool", "set", name, propName, propVal}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("mon_command failed, %+v", err)
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
