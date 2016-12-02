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
package block

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
