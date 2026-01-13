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

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseObjectUserOpMask(t *testing.T) {
	t.Run("parse glob (`*`)", func(t *testing.T) {
		mask, err := ParseObjectUserOpMask("*")
		assert.NoError(t, err)
		assert.True(t, mask.Read)
		assert.True(t, mask.Write)
		assert.True(t, mask.Delete)
		assert.Equal(t, "read, write, delete", mask.String())
	})

	t.Run("parse all 3 operations in order", func(t *testing.T) {
		mask, err := ParseObjectUserOpMask("read, write, delete")
		assert.NoError(t, err)
		assert.True(t, mask.Read)
		assert.True(t, mask.Write)
		assert.True(t, mask.Delete)
		assert.Equal(t, "read, write, delete", mask.String())
	})

	t.Run("parse out of order operations", func(t *testing.T) {
		mask, err := ParseObjectUserOpMask("delete, read, write")
		assert.NoError(t, err)
		assert.True(t, mask.Read)
		assert.True(t, mask.Write)
		assert.True(t, mask.Delete)
		assert.Equal(t, "read, write, delete", mask.String())
	})

	t.Run("parse read only operation", func(t *testing.T) {
		mask, err := ParseObjectUserOpMask("read")
		assert.NoError(t, err)
		assert.True(t, mask.Read)
		assert.False(t, mask.Write)
		assert.False(t, mask.Delete)
		assert.Equal(t, "read", mask.String())
	})

	t.Run("parse write only operation", func(t *testing.T) {
		mask, err := ParseObjectUserOpMask("write")
		assert.NoError(t, err)
		assert.False(t, mask.Read)
		assert.True(t, mask.Write)
		assert.False(t, mask.Delete)
		assert.Equal(t, "write", mask.String())
	})

	t.Run("parse delete only operation", func(t *testing.T) {
		mask, err := ParseObjectUserOpMask("delete")
		assert.NoError(t, err)
		assert.False(t, mask.Read)
		assert.False(t, mask.Write)
		assert.True(t, mask.Delete)
		assert.Equal(t, "delete", mask.String())
	})

	t.Run("glob (`*`) operation combined with any other token fails to parse", func(t *testing.T) {
		_, err := ParseObjectUserOpMask("*, read")
		assert.EqualError(t, err, `invalid use of glob ("*") combined with other operations in op-mask`)
	})

	t.Run("unknown operation fails to parse", func(t *testing.T) {
		_, err := ParseObjectUserOpMask("read, write, delete, unknown")
		assert.EqualError(t, err, `invalid operation "unknown" in op-mask "read, write, delete, unknown"`)
	})
}

func TestObjectUserOpMaskString(t *testing.T) {
	t.Run("read op mask", func(t *testing.T) {
		mask := ObjectUserOpMask{
			Read: true,
		}
		assert.Equal(t, "read", mask.String())
	})

	t.Run("write op mask", func(t *testing.T) {
		mask := ObjectUserOpMask{
			Write: true,
		}
		assert.Equal(t, "write", mask.String())
	})

	t.Run("delete op mask", func(t *testing.T) {
		mask := ObjectUserOpMask{
			Delete: true,
		}
		assert.Equal(t, "delete", mask.String())
	})

	t.Run("read & write op masks", func(t *testing.T) {
		mask := ObjectUserOpMask{
			Read:  true,
			Write: true,
		}
		assert.Equal(t, "read, write", mask.String())
	})

	t.Run("read & delete op masks", func(t *testing.T) {
		mask := ObjectUserOpMask{
			Read:   true,
			Delete: true,
		}
		assert.Equal(t, "read, delete", mask.String())
	})

	t.Run("write & delete op masks", func(t *testing.T) {
		mask := ObjectUserOpMask{
			Write:  true,
			Delete: true,
		}
		assert.Equal(t, "write, delete", mask.String())
	})

	t.Run("all op masks", func(t *testing.T) {
		mask := ObjectUserOpMask{
			Read:   true,
			Write:  true,
			Delete: true,
		}
		assert.Equal(t, "read, write, delete", mask.String())
	})
}
