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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package cluster

import (
	"testing"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
	"github.com/stretchr/testify/assert"
)

func TestValidatePool(t *testing.T) {
	// must specify some replication or EC settings
	spec := PoolSpec{Name: "mypool", Namespace: "myns"}
	err := NewPool(spec).validate()
	assert.NotNil(t, err)

	// must specify name
	spec = PoolSpec{Namespace: "myns"}
	err = NewPool(spec).validate()
	assert.NotNil(t, err)

	// must specify namespace
	spec = PoolSpec{Name: "mypool"}
	err = NewPool(spec).validate()
	assert.NotNil(t, err)

	// must not specify both replication and EC settings
	spec = PoolSpec{Name: "mypool", Namespace: "myns"}
	spec.Replication.Count = 1
	spec.ErasureCoding.CodingChunks = 2
	spec.ErasureCoding.DataChunks = 3
	err = NewPool(spec).validate()
	assert.NotNil(t, err)

	// succeed with replication settings
	spec = PoolSpec{Name: "mypool", Namespace: "myns"}
	spec.Replication.Count = 1
	err = NewPool(spec).validate()
	assert.Nil(t, err)

	// succeed with ec settings
	spec = PoolSpec{Name: "mypool", Namespace: "myns"}
	spec.ErasureCoding.CodingChunks = 1
	spec.ErasureCoding.DataChunks = 2
	err = NewPool(spec).validate()
	assert.Nil(t, err)
}

func TestCreatePool(t *testing.T) {
	rclient := &test.MockRookRestClient{}
	spec := PoolSpec{Name: "mypool", Namespace: "myns"}
	spec.Replication.Count = 1
	p := NewPool(spec)

	exists, err := p.exists(rclient)
	assert.False(t, exists)
	err = p.Create(rclient)
	assert.Nil(t, err)

	// fail if both replication and EC are specified
	spec.ErasureCoding.CodingChunks = 2
	spec.ErasureCoding.DataChunks = 2
	p.PoolSpec = spec
	err = p.Create(rclient)
	assert.NotNil(t, err)

	// succeed with EC
	spec.Replication.Count = 0
	p.PoolSpec = spec
	err = p.Create(rclient)
	assert.Nil(t, err)
}

func TestDeletePool(t *testing.T) {
	rclient := &test.MockRookRestClient{
		MockGetPools: func() ([]model.Pool, error) {
			pools := []model.Pool{
				model.Pool{Name: "mypool"},
			}
			return pools, nil
		},
	}

	// delete a pool that exists
	spec := PoolSpec{Name: "mypool", Namespace: "myns"}
	p := NewPool(spec)
	exists, err := p.exists(rclient)
	assert.Nil(t, err)
	assert.True(t, exists)
	err = p.Delete(rclient)
	assert.Nil(t, err)

	// succeed even if the pool doesn't exist
	spec = PoolSpec{Name: "otherpool", Namespace: "myns"}
	p = NewPool(spec)
	exists, err = p.exists(rclient)
	assert.Nil(t, err)
	assert.False(t, exists)
	err = p.Delete(rclient)
	assert.Nil(t, err)
}
