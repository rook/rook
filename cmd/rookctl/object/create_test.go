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
package object

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
)

func TestCreateObjectStore(t *testing.T) {
	c := &test.MockRookRestClient{}

	err := createObjectStore(c)
	assert.Nil(t, err)
}

func TestCreateObjectStoreError(t *testing.T) {
	c := &test.MockRookRestClient{
		MockCreateObjectStore: func(store model.ObjectStore) (string, error) {
			return "", fmt.Errorf("mock create object store failed")
		},
	}

	err := createObjectStore(c)
	assert.NotNil(t, err)
}
