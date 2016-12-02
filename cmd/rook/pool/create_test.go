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
package pool

import (
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
	"github.com/stretchr/testify/assert"
)

const (
	SuccessPoolCreatedMessage = "pool 'pool1' created"
)

func TestCreatePoolValidTypeRequired(t *testing.T) {
	c := &test.MockRookRestClient{}
	out, err := createPool("pool1", "foo", 1, 0, 0, c)
	assert.NotNil(t, err)
	assert.Equal(t, "invalid pool type 'foo', allowed pool types are 'replicated' and 'erasure-coded'", err.Error())
	assert.Equal(t, "", out)
}

func TestCreatePoolErasureCodedParamsRequired(t *testing.T) {
	c := &test.MockRookRestClient{}
	out, err := createPool("pool1", PoolTypeErasureCoded, 0, 0, 0, c)
	assert.NotNil(t, err)
	assert.Equal(t, "both data chunks and coding chunks must be greater than zero for pool type 'erasure-coded'", err.Error())
	assert.Equal(t, "", out)
}

func TestCreatePoolReplicatedErasureCodedParamsNotAllowed(t *testing.T) {
	c := &test.MockRookRestClient{}
	out, err := createPool("pool1", PoolTypeReplicated, 0, 2, 1, c)
	assert.NotNil(t, err)
	assert.Equal(t, "both data chunks and coding chunks must be zero for pool type 'replicated'", err.Error())
	assert.Equal(t, "", out)
}

func TestCreatePoolReplicatedNoParams(t *testing.T) {
	c := &test.MockRookRestClient{
		MockCreatePool: func(actualPool model.Pool) (string, error) {
			expectedPool := model.Pool{
				Name:   "pool1",
				Number: 0,
				Type:   model.Replicated,
			}
			assert.Equal(t, expectedPool, actualPool)
			return SuccessPoolCreatedMessage, nil
		},
	}

	// replicated pool replica count of 0 is OK, it will get the ceph default
	out, err := createPool("pool1", PoolTypeReplicated, 0, 0, 0, c)
	assert.Nil(t, err)
	assert.Equal(t, SuccessPoolCreatedMessage, out)
}

func TestCreatePoolReplicated(t *testing.T) {
	c := &test.MockRookRestClient{
		MockCreatePool: func(actualPool model.Pool) (string, error) {
			expectedPool := model.Pool{
				Name:   "pool1",
				Number: 0,
				Type:   model.Replicated,
				ReplicationConfig: model.ReplicatedPoolConfig{
					Size: 3,
				},
			}
			assert.Equal(t, expectedPool, actualPool)
			return SuccessPoolCreatedMessage, nil
		},
	}

	out, err := createPool("pool1", PoolTypeReplicated, 3, 0, 0, c)
	assert.Nil(t, err)
	assert.Equal(t, SuccessPoolCreatedMessage, out)
}

func TestCreatePoolErasureCoded(t *testing.T) {
	c := &test.MockRookRestClient{
		MockCreatePool: func(actualPool model.Pool) (string, error) {
			expectedPool := model.Pool{
				Name:   "pool1",
				Number: 0,
				Type:   model.ErasureCoded,
				ErasureCodedConfig: model.ErasureCodedPoolConfig{
					DataChunkCount:   2,
					CodingChunkCount: 1,
				},
			}
			assert.Equal(t, expectedPool, actualPool)
			return SuccessPoolCreatedMessage, nil
		},
	}

	out, err := createPool("pool1", PoolTypeErasureCoded, 0, 2, 1, c)
	assert.Nil(t, err)
	assert.Equal(t, SuccessPoolCreatedMessage, out)
}

func TestCreatePoolFailure(t *testing.T) {
	c := &test.MockRookRestClient{
		MockCreatePool: func(pool model.Pool) (string, error) {
			return "", fmt.Errorf("mock error")
		},
	}

	out, err := createPool("pool1", PoolTypeReplicated, 0, 0, 0, c)
	assert.NotNil(t, err)
	assert.Equal(t, "failed to create new pool 'pool1': mock error", err.Error())
	assert.Equal(t, "", out)
}
