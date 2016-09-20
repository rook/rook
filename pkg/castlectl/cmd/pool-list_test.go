package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/quantum/castle/pkg/castlectl/test"
	"github.com/quantum/castle/pkg/model"
)

func TestListPools(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetPools: func() ([]model.Pool, error) {
			return []model.Pool{{Name: "pool1", Number: 1}}, nil
		},
	}

	out, err := listPools(c)
	assert.Nil(t, err)
	assert.Equal(t, "{Name:pool1 Number:1}", out)
}

func TestListPoolsError(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetPools: func() ([]model.Pool, error) {
			return nil, fmt.Errorf("mock get pools error")
		},
	}

	out, err := listPools(c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
