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
	"fmt"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/config"
)

func MonInQuorumResponse() string {
	resp := client.MonStatusResponse{Quorum: []int{0}}
	resp.MonMap.Mons = []client.MonMapEntry{
		{
			Name:    "a",
			Rank:    0,
			Address: "1.2.3.1",
		},
	}
	serialized, _ := json.Marshal(resp)
	return string(serialized)
}

func MonInQuorumResponseFromMons(mons map[string]*config.MonInfo) string {
	resp := client.MonStatusResponse{Quorum: []int{}}
	i := 0
	for name := range mons {
		resp.MonMap.Mons = append(resp.MonMap.Mons, client.MonMapEntry{
			Name:    name,
			Rank:    i,
			Address: fmt.Sprintf("1.2.3.%d", i),
		})
		resp.Quorum = append(resp.Quorum, i)
		i++
	}
	serialized, _ := json.Marshal(resp)
	return string(serialized)
}

func MonInQuorumResponseMany(count int) string {
	resp := client.MonStatusResponse{Quorum: []int{0}}
	resp.MonMap.Mons = []client.MonMapEntry{}
	for i := 0; i <= count; i++ {
		resp.MonMap.Mons = append(resp.MonMap.Mons, client.MonMapEntry{
			Name:    fmt.Sprintf("rook-ceph-mon%d", i),
			Rank:    0,
			Address: fmt.Sprintf("1.2.3.%d", i),
		})
	}
	serialized, _ := json.Marshal(resp)
	return string(serialized)
}
