package rook

import (
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
	"github.com/stretchr/testify/assert"
)

func TestCreateBlockImage(t *testing.T) {
	c := &test.MockRookRestClient{
		MockCreateBlockImage: func(image model.BlockImage) (string, error) {
			return fmt.Sprintf("successfully created image %s", image.Name), nil
		},
	}

	out, err := createBlockImage("myimage1", "mypool1", 1024, c)
	assert.Nil(t, err)
	assert.Equal(t, "successfully created image myimage1", out)
}

func TestCreateBlockImageFailure(t *testing.T) {
	c := &test.MockRookRestClient{
		MockCreateBlockImage: func(image model.BlockImage) (string, error) {
			return "", fmt.Errorf("failed to create image %s", image.Name)
		},
	}

	out, err := createBlockImage("myimage1", "mypool1", 1024, c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
