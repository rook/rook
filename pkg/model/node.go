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
package model

import "time"

type NodeState int

const (
	Healthy NodeState = iota
	Unhealthy
)

type Node struct {
	NodeID      string        `json:"nodeId"`
	ClusterName string        `json:"clusterName"`
	PublicIP    string        `json:"publicIp"`
	PrivateIP   string        `json:"privateIp"`
	Storage     uint64        `json:"storage"`
	LastUpdated time.Duration `json:"lastUpdated"`
	State       NodeState     `json:"state"`
	Location    string        `json:"location"`
}

func NodeStateToString(state NodeState) string {
	switch state {
	case Healthy:
		return "OK"
	case Unhealthy:
		return "DOWN"
	default:
		return "UNKNOWN"
	}
}
