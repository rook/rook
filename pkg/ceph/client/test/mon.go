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
package test

import (
	"encoding/json"

	"github.com/rook/rook/pkg/ceph/client"
)

func MonInQuorumResponse() string {
	resp := client.MonStatusResponse{Quorum: []int{0}}
	resp.MonMap.Mons = []client.MonMapEntry{client.MonMapEntry{Name: "mon0", Rank: 0, Address: "1.2.3.4"}}
	serialized, _ := json.Marshal(resp)
	return string(serialized)
}
