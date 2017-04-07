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
	"strings"
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
			Objects      float64 `json:"services"`
			DirtyObjects float64 `json:"dirty"`
			ReadIO       float64 `json:"rd"`
			ReadBytes    float64 `json:"rd_bytes"`
			WriteIO      float64 `json:"wr"`
			WriteBytes   float64 `json:"wr_bytes"`
		} `json:"stats"`
	} `json:"pools"`
}

func ListPoolSummaries(conn Connection) ([]CephStoragePoolSummary, error) {
	cmd := map[string]interface{}{"prefix": "osd lspools"}
	buf, err := ExecuteMonCommand(conn, cmd, "list pools")
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

func GetPoolDetails(conn Connection, name string) (CephStoragePoolDetails, error) {
	cmd := map[string]interface{}{"prefix": "osd pool get", "pool": name, "var": "all"}
	buf, err := ExecuteMonCommand(conn, cmd, "get pool")
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

func CreatePool(conn Connection, newPool CephStoragePoolDetails) (string, error) {
	cmd := map[string]interface{}{"prefix": "osd pool create", "pool": newPool.Name}

	if newPool.ErasureCodeProfile != "" {
		cmd["pool_type"] = "erasure"
		cmd["erasure_code_profile"] = newPool.ErasureCodeProfile
	} else {
		cmd["pool_type"] = "replicated"
	}

	buf, info, err := ExecuteMonCommandWithInfo(conn, cmd, "create pool")
	if err != nil {
		return "", fmt.Errorf("mon_command %s failed, buf: %s, info: %s: %+v", cmd, string(buf), info, err)
	}

	if newPool.ErasureCodeProfile == "" && newPool.Size > 0 {
		// the pool is type replicated, set the size for the pool now that it's been created
		if err = SetPoolProperty(conn, newPool.Name, "size", newPool.Size); err != nil {
			return "", err
		}
	}

	logger.Infof("creating pool %s succeeded, info: %s, buf: %s", newPool.Name, info, string(buf))

	return info, nil
}

func SetPoolProperty(conn Connection, name, propName string, propVal interface{}) error {
	cmd := map[string]interface{}{"prefix": "osd pool set", "pool": name, "var": propName, "val": propVal}
	buf, info, err := ExecuteMonCommandWithInfo(conn, cmd, "set pool")
	if err != nil {
		return fmt.Errorf("mon_command %s failed, buf: %s, info: %s: %+v", cmd, string(buf), info, err)
	}

	return nil
}

func GetPoolStats(conn Connection) (*CephStoragePoolStats, error) {
	cmd := map[string]interface{}{"prefix": "df", "detail": "detail"}

	buf, err := ExecuteMonCommand(conn, cmd, "pool stats")
	if err != nil {
		return nil, fmt.Errorf("failed to get pool stats: %+v", err)
	}

	var poolStats CephStoragePoolStats
	if err := json.Unmarshal(buf, &poolStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pool stats response: %+v", err)
	}

	return &poolStats, nil
}
