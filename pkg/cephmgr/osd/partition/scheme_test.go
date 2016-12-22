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
package partition

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleScheme(t *testing.T) {
	scheme, err := GetSimpleScheme(1, 123)
	assert.Nil(t, err)
	assert.Equal(t, 123, scheme.SizeMB)
	assert.Equal(t, 36, len(scheme.DiskUUID))
	assert.Equal(t, 3, len(scheme.PartitionUUIDs))
	assert.Equal(t, 1, scheme.ID)

	args := scheme.GetArgs("foo")
	assert.Equal(t, 11, len(args))

	err = scheme.Save("/tmp")
	defer os.Remove(path.Join("/tmp", schemeFilename))
	assert.Nil(t, err)

	loaded, err := LoadScheme("/tmp")
	assert.Nil(t, err)
	assert.Equal(t, 3, len(loaded.PartitionUUIDs))

	logger.Infof("scheme=%+v", scheme)
}
