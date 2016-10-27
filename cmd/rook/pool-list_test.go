package rook

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
