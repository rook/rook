/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package v1alpha1

import "github.com/rook/rook/pkg/daemon/ceph/model"

func (p *PoolSpec) ToModel(name string) *model.Pool {
	pool := &model.Pool{Name: name, FailureDomain: p.FailureDomain, CrushRoot: p.CrushRoot}
	r := p.Replication()
	if r != nil {
		pool.ReplicatedConfig.Size = r.Size
		pool.Type = model.Replicated
	} else {
		ec := p.ErasureCode()
		if ec != nil {
			pool.ErasureCodedConfig.CodingChunkCount = ec.CodingChunks
			pool.ErasureCodedConfig.DataChunkCount = ec.DataChunks
			pool.Type = model.ErasureCoded
		}
	}
	return pool
}

func (p *PoolSpec) Replication() *ReplicatedSpec {
	if p.Replicated.Size > 0 {
		return &p.Replicated
	}
	return nil
}

func (p *PoolSpec) ErasureCode() *ErasureCodedSpec {
	ec := &p.ErasureCoded
	if ec.CodingChunks > 0 || ec.DataChunks > 0 {
		return ec
	}
	return nil
}
