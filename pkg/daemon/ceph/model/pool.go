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

const (
	Replicated PoolType = iota
	ErasureCoded
	PoolTypeUnknown
)

type PoolType int

type ReplicatedPoolConfig struct {
	Size uint `json:"size"`
}

type ErasureCodedPoolConfig struct {
	DataChunkCount   uint   `json:"dataChunkCount"`
	CodingChunkCount uint   `json:"codingChunkCount"`
	Algorithm        string `json:"algorithm"`
}

type Pool struct {
	Name               string                 `json:"poolName"`
	Number             int                    `json:"poolNum"`
	Type               PoolType               `json:"type"`
	FailureDomain      string                 `json:"failureDomain"`
	CrushRoot          string                 `json:"crushRoot"`
	DeviceClass        string                 `json:"deviceClass"`
	ReplicatedConfig   ReplicatedPoolConfig   `json:"replicatedConfig"`
	ErasureCodedConfig ErasureCodedPoolConfig `json:"erasureCodedConfig"`
	NotEnableAppPool   bool                   `json:"notEnableAppPool"`
}
