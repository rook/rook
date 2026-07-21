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

package object

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModifyBucketPolicy(t *testing.T) {
	t.Run("does not duplicate an existing SID", func(t *testing.T) {
		bp := NewBucketPolicy(*NewPolicyStatement().WithSID("s1").Allows().Actions(GetObject))
		bp.ModifyBucketPolicy(*NewPolicyStatement().WithSID("s1").Denies().Actions(PutObject))
		assert.Len(t, bp.Statement, 1)
		assert.Equal(t, effectDeny, bp.Statement[0].Effect)
	})

	t.Run("replaces existing statements instead of merging", func(t *testing.T) {
		bp := NewBucketPolicy(
			*NewPolicyStatement().WithSID("s1").Allows().Actions(GetObject),
			*NewPolicyStatement().WithSID("s2").Allows().Actions(GetObject),
		)
		bp.ModifyBucketPolicy(*NewPolicyStatement().WithSID("s3").Denies().Actions(PutObject))
		assert.Len(t, bp.Statement, 1)
		assert.Equal(t, "s3", bp.Statement[0].Sid)
	})

	t.Run("keeps all passed statements in order", func(t *testing.T) {
		bp := NewBucketPolicy(*NewPolicyStatement().WithSID("old").Allows().Actions(GetObject))
		bp.ModifyBucketPolicy(
			*NewPolicyStatement().WithSID("a").Allows().Actions(GetObject),
			*NewPolicyStatement().WithSID("b").Denies().Actions(PutObject),
		)
		assert.Len(t, bp.Statement, 2)
		assert.Equal(t, "a", bp.Statement[0].Sid)
		assert.Equal(t, "b", bp.Statement[1].Sid)
	})
}
