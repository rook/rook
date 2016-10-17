package castle

import (
	"fmt"
	"testing"

	"github.com/quantum/castle/pkg/castle/test"
	"github.com/quantum/castle/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestCreateBlockImage(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockCreateBlockImage: func(image model.BlockImage) (string, error) {
			return fmt.Sprintf("successfully created image %s", image.Name), nil
		},
	}

	out, err := createBlockImage("myimage1", "mypool1", 1024, c)
	assert.Nil(t, err)
	assert.Equal(t, "successfully created image myimage1", out)
}

func TestCreateBlockImageFailure(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockCreateBlockImage: func(image model.BlockImage) (string, error) {
			return "", fmt.Errorf("failed to create image %s", image.Name)
		},
	}

	out, err := createBlockImage("myimage1", "mypool1", 1024, c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
