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

	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
)

func TestListPools(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetPools: func() ([]model.Pool, error) {
			pools := []model.Pool{
				{
					Name:              "replPool1",
					Number:            0,
					Type:              model.Replicated,
					ReplicationConfig: model.ReplicatedPoolConfig{Size: 3},
				},
				{
					Name:   "ecPool1",
					Number: 1,
					Type:   model.ErasureCoded,
					ErasureCodedConfig: model.ErasureCodedPoolConfig{
						DataChunkCount:   2,
						CodingChunkCount: 1,
						Algorithm:        "jerasure::reed_sol_van",
					},
				},
			}
			return pools, nil
		},
	}

	out, err := listPools(c)
	assert.Nil(t, err)

	expectedOut := "NAME        NUMBER    TYPE            SIZE      DATA      CODING    ALGORITHM\n" +
		"replPool1   0         replicated      3                             \n" +
		"ecPool1     1         erasure coded             2         1         jerasure::reed_sol_van\n"
	assert.Equal(t, expectedOut, out)
}

func TestListPoolsError(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetPools: func() ([]model.Pool, error) {
			return nil, fmt.Errorf("mock get pools error")
		},
	}

	out, err := listPools(c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
