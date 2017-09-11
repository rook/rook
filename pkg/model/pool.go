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

type PoolType int

const (
	Replicated PoolType = iota
	ErasureCoded
	PoolTypeUnknown
)

type ObjectStore struct {
	Name           string `json:"name"`
	DataConfig     Pool   `json:"dataConfig"`
	MetadataConfig Pool   `json:"metadataConfig"`
	RGW            RGW    `json:"rgw"`
}

type RGW struct {
	Port           int32  `json:"port"`
	Replicas       int32  `json:"rgwReplicas"`
	Certificate    string `json:"certificate"`
	CertificateRef string `json:"certificateRef"`
}

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
	ReplicatedConfig   ReplicatedPoolConfig   `json:"replicatedConfig"`
	ErasureCodedConfig ErasureCodedPoolConfig `json:"erasureCodedConfig"`
}

func PoolTypeToString(poolType PoolType) string {
	switch poolType {
	case Replicated:
		return "replicated"
	case ErasureCoded:
		return "erasure coded"
	default:
		return "unknown"
	}
}
