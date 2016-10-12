package main

import (
	"fmt"
	"testing"

	"github.com/quantum/castle/pkg/castlectl/test"
	"github.com/quantum/castle/pkg/model"
	"github.com/stretchr/testify/assert"
)

const (
	SuccessPoolCreatedMessage = "pool 'pool1' created"
)

func TestCreatePoolNoParams(t *testing.T) {
	c := &test.MockCastleRestClient{
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

	out, err := createPool("pool1", 0, 0, 0, c)
	assert.Nil(t, err)
	assert.Equal(t, SuccessPoolCreatedMessage, out)
}

func TestCreatePoolReplicated(t *testing.T) {
	c := &test.MockCastleRestClient{
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

	out, err := createPool("pool1", 3, 0, 0, c)
	assert.Nil(t, err)
	assert.Equal(t, SuccessPoolCreatedMessage, out)
}

func TestCreatePoolErasureCoded(t *testing.T) {
	c := &test.MockCastleRestClient{
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

	out, err := createPool("pool1", 0, 2, 1, c)
	assert.Nil(t, err)
	assert.Equal(t, SuccessPoolCreatedMessage, out)
}

func TestCreatePoolReplicationAndErasureCodedNotAllowed(t *testing.T) {
	c := &test.MockCastleRestClient{}
	out, err := createPool("pool1", 3, 2, 1, c)
	assert.NotNil(t, err)
	assert.Equal(t, "Pool cannot be both replicated and erasure coded.", err.Error())
	assert.Equal(t, "", out)
}

func TestCreatePoolFailure(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockCreatePool: func(pool model.Pool) (string, error) {
			return "", fmt.Errorf("mock error")
		},
	}

	out, err := createPool("pool1", 0, 0, 0, c)
	assert.NotNil(t, err)
	assert.Equal(t, "failed to create new pool 'pool1': mock error", err.Error())
	assert.Equal(t, "", out)
}
