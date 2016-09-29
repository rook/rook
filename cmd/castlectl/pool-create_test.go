package cmd

import (
	"fmt"
	"testing"

	"github.com/quantum/castle/pkg/castlectl/test"
	"github.com/quantum/castle/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestCreatePool(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockCreatePool: func(pool model.Pool) (string, error) {
			return fmt.Sprintf("successfully created pool '%s'", pool.Name), nil
		},
	}

	out, err := createPool("pool1", c)
	assert.Nil(t, err)
	assert.Equal(t, "successfully created pool 'pool1'", out)
}

func TestCreatePoolFailure(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockCreatePool: func(pool model.Pool) (string, error) {
			return "", fmt.Errorf("mock error")
		},
	}

	out, err := createPool("pool1", c)
	assert.NotNil(t, err)
	assert.Equal(t, "failed to create new pool '{Name:pool1 Number:0}': mock error", err.Error())
	assert.Equal(t, "", out)
}
