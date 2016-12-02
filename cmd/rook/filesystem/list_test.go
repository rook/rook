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

func TestListFilesystems(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetFilesystems: func() ([]model.Filesystem, error) {
			return []model.Filesystem{
				{Name: "myfs1", MetadataPool: "myfs1-metadata", DataPools: []string{"myfs1-data"}},
			}, nil
		},
	}

	out, err := listFilesystems(c)
	assert.Nil(t, err)

	expectedOut := "NAME      METADATA POOL    DATA POOLS\n" +
		"myfs1     myfs1-metadata   myfs1-data\n"
	assert.Equal(t, expectedOut, out)
}

func TestListFilesystemsError(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetFilesystems: func() ([]model.Filesystem, error) {
			return nil, fmt.Errorf("mock get filesystems failed")
		},
	}

	out, err := listFilesystems(c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
