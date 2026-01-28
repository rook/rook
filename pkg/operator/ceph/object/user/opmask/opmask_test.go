/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package opmask

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpMask(t *testing.T) {
	t.Run("parse glob (`*`)", func(t *testing.T) {
		mask, err := Parse("*")
		assert.NoError(t, err)
		assert.True(t, mask.read)
		assert.True(t, mask.write)
		assert.True(t, mask.delete)
		assert.Equal(t, "read, write, delete", mask.String())
	})

	t.Run("parse all 3 operations in order", func(t *testing.T) {
		mask, err := Parse("read, write, delete")
		assert.NoError(t, err)
		assert.True(t, mask.read)
		assert.True(t, mask.write)
		assert.True(t, mask.delete)
		assert.Equal(t, "read, write, delete", mask.String())
	})

	t.Run("parse out of order operations", func(t *testing.T) {
		mask, err := Parse("delete, read, write")
		assert.NoError(t, err)
		assert.True(t, mask.read)
		assert.True(t, mask.write)
		assert.True(t, mask.delete)
		assert.Equal(t, "read, write, delete", mask.String())
	})

	t.Run("parse read only operation", func(t *testing.T) {
		mask, err := Parse("read")
		assert.NoError(t, err)
		assert.True(t, mask.read)
		assert.False(t, mask.write)
		assert.False(t, mask.delete)
		assert.Equal(t, "read", mask.String())
	})

	t.Run("parse write only operation", func(t *testing.T) {
		mask, err := Parse("write")
		assert.NoError(t, err)
		assert.False(t, mask.read)
		assert.True(t, mask.write)
		assert.False(t, mask.delete)
		assert.Equal(t, "write", mask.String())
	})

	t.Run("parse delete only operation", func(t *testing.T) {
		mask, err := Parse("delete")
		assert.NoError(t, err)
		assert.False(t, mask.read)
		assert.False(t, mask.write)
		assert.True(t, mask.delete)
		assert.Equal(t, "delete", mask.String())
	})

	t.Run("glob (`*`) operation combined with any other token fails to parse", func(t *testing.T) {
		_, err := Parse("*, read")
		assert.EqualError(t, err, `invalid use of glob ("*") combined with other operations in op-mask`)
	})

	t.Run("unknown operation fails to parse", func(t *testing.T) {
		_, err := Parse("read, write, delete, unknown")
		assert.EqualError(t, err, `invalid operation "unknown" in op-mask "read, write, delete, unknown"`)
	})
}
