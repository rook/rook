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
)

type CephUsage struct {
	Stats struct {
		TotalBytes      json.Number `json:"total_bytes"`
		TotalUsedBytes  json.Number `json:"total_used_bytes"`
		TotalAvailBytes json.Number `json:"total_avail_bytes"`
		TotalObjects    json.Number `json:"total_objects"`
	} `json:"stats"`
}

func Usage(conn Connection) (*CephUsage, error) {
	cmd := map[string]interface{}{"prefix": "df", "detail": "detail"}
	buf, err := ExecuteMonCommand(conn, cmd, "df")
	if err != nil {
		return nil, fmt.Errorf("failed to get usage: %+v", err)
	}

	var usage CephUsage
	if err := json.Unmarshal(buf, &usage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal usage response: %+v", err)
	}

	return &usage, nil
}
