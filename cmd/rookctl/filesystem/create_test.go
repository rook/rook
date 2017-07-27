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
package filesystem

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
)

func TestCreateFilesystem(t *testing.T) {
	fsName := "myfs1"

	c := &test.MockRookRestClient{
		MockCreateFilesystem: func(fsr model.FilesystemRequest) (string, error) {
			assert.Equal(t, fsName, fsr.Name)
			assert.Equal(t, fsName, fsr.PoolName)
			return "", nil
		},
	}

	out, err := createFilesystem(fsName, c)
	assert.Nil(t, err)
	assert.Equal(t, "succeeded starting creation of shared filesystem myfs1", out)
}

func TestCreateFilesystemError(t *testing.T) {
	c := &test.MockRookRestClient{
		MockCreateFilesystem: func(fsr model.FilesystemRequest) (string, error) {
			return "", fmt.Errorf("mock create filesystem failed")
		},
	}

	out, err := createFilesystem("", c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
