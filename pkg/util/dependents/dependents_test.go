/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package dependents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDependentList(t *testing.T) {
	containsExactlyOne := func(s, substr string) {
		assert.Equal(t, 1, strings.Count(s, substr))
	}
	isBefore := func(s, before, after string) {
		assert.Less(t, strings.Index(s, before), strings.Index(s, after))
	}

	t.Run("empty", func(t *testing.T) {
		d := NewDependentList()
		assert.True(t, d.Empty())
	})

	t.Run("one resource, one dependent", func(t *testing.T) {
		d := NewDependentList()
		d.Add("MyResources", "my-resource-1")
		assert.False(t, d.Empty())
		assert.ElementsMatch(t, []string{"MyResources"}, d.PluralKinds())
		assert.ElementsMatch(t, []string{"my-resource-1"}, d.OfPluralKind("MyResources"))
		toString := d.StringWithHeader("header")
		containsExactlyOne(toString, "header:")
		containsExactlyOne(toString, "MyResources")
		containsExactlyOne(toString, "my-resource-1")
	})

	t.Run("one resource - multiple dependents", func(t *testing.T) {
		d := NewDependentList()
		d.Add("MyResources", "my-resource-1")
		d.Add("MyResources", "my-resource-2")
		d.Add("MyResources", "my-resource-3")
		assert.False(t, d.Empty())
		assert.ElementsMatch(t, []string{"MyResources"}, d.PluralKinds())
		assert.ElementsMatch(t, []string{"my-resource-1", "my-resource-2", "my-resource-3"}, d.OfPluralKind("MyResources"))
		assert.ElementsMatch(t, []string{}, d.OfPluralKind("OtherKinds"))
		toString := d.StringWithHeader("head with arg %d", 1)
		containsExactlyOne(toString, "head with arg 1:")
		containsExactlyOne(toString, "MyResources")
		containsExactlyOne(toString, "my-resource-1")
		containsExactlyOne(toString, "my-resource-2")
		containsExactlyOne(toString, "my-resource-3")

	})

	t.Run("multiple resources - multiple dependents", func(t *testing.T) {
		d := NewDependentList()
		d.Add("MyResources", "my-resource-2")
		d.Add("MyResources", "my-resource-4")
		d.Add("YourResources", "your-resource-1")
		d.Add("TheirResources", "their-resource-5")
		d.Add("TheirResources", "their-resource-6")
		assert.False(t, d.Empty())
		assert.ElementsMatch(t, []string{"MyResources", "YourResources", "TheirResources"}, d.PluralKinds())
		assert.ElementsMatch(t, []string{"my-resource-2", "my-resource-4"}, d.OfPluralKind("MyResources"))
		assert.ElementsMatch(t, []string{"your-resource-1"}, d.OfPluralKind("YourResources"))
		assert.ElementsMatch(t, []string{"their-resource-5", "their-resource-6"}, d.OfPluralKind("TheirResources"))
		assert.ElementsMatch(t, []string{}, d.OfPluralKind("OtherKinds"))
		toString := d.StringWithHeader("head with arg %s", "mom")
		t.Log(toString)
		containsExactlyOne(toString, "head with arg mom:")
		containsExactlyOne(toString, "MyResources")
		containsExactlyOne(toString, "my-resource-2")
		containsExactlyOne(toString, "my-resource-4")
		containsExactlyOne(toString, "YourResources")
		containsExactlyOne(toString, "your-resource-1")
		containsExactlyOne(toString, "TheirResources")
		containsExactlyOne(toString, "their-resource-5")
		containsExactlyOne(toString, "their-resource-6")
		// ensure alphabetical ordering
		isBefore(toString, "MyResources", "TheirResources")
		isBefore(toString, "TheirResources", "YourResources")
	})
}
