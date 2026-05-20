/*
Copyright 2024 The Rook Authors. All rights reserved.

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModifyBucketPolicy(t *testing.T) {
	t.Run("replace existing SID in place without appending", func(t *testing.T) {
		original := NewPolicyStatement().WithSID("s1").Allows().Actions(GetObject)
		bp := NewBucketPolicy(*original)
		assert.Len(t, bp.Statement, 1)

		replacement := NewPolicyStatement().WithSID("s1").Denies().Actions(PutObject)
		bp.ModifyBucketPolicy(*replacement)

		assert.Len(t, bp.Statement, 1, "matching SID should replace in place, not append")
		assert.Equal(t, effectDeny, bp.Statement[0].Effect)
	})

	t.Run("append new SID", func(t *testing.T) {
		ps1 := NewPolicyStatement().WithSID("s1").Allows().Actions(GetObject)
		bp := NewBucketPolicy(*ps1)

		ps2 := NewPolicyStatement().WithSID("s2").Denies().Actions(PutObject)
		bp.ModifyBucketPolicy(*ps2)

		assert.Len(t, bp.Statement, 2)
		assert.Equal(t, "s1", bp.Statement[0].Sid)
		assert.Equal(t, "s2", bp.Statement[1].Sid)
	})
}
