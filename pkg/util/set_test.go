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
package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubtract(t *testing.T) {
	set := CreateSet([]string{"a", "b", "x", "y", "z"})
	subset := CreateSet([]string{"b", "z"})
	set.Subtract(subset)
	assert.Equal(t, 3, set.Count())
	assert.True(t, set.Contains("a"))
	assert.False(t, set.Contains("b"))
	assert.True(t, set.Contains("x"))
	assert.True(t, set.Contains("y"))
	assert.False(t, set.Contains("z"))
}

func TestSubtractEmptySet(t *testing.T) {
	// Both sets empty
	set := NewSet()
	subset := NewSet()
	set.Subtract(subset)
	assert.Equal(t, 0, set.Count())

	// Subset is empty
	set = CreateSet([]string{"1", "2"})
	set.Subtract(subset)
	assert.Equal(t, 2, set.Count())
}

func TestAddSingle(t *testing.T) {
	set := NewSet()
	assert.True(t, set.Add("foo"))
	assert.False(t, set.Add("foo"))

	assert.Equal(t, 1, set.Count())
	assert.True(t, set.Contains("foo"))
	assert.False(t, set.Contains("bar"))

	assert.True(t, set.Add("bar"))
	assert.Equal(t, 2, set.Count())
	assert.True(t, set.Contains("foo"))
	assert.True(t, set.Contains("bar"))
	assert.False(t, set.Contains("baz"))
}

func TestAddMultiple(t *testing.T) {
	set := NewSet()
	set.AddMultiple([]string{"a", "b", "z"})
	assert.Equal(t, 3, set.Count())
	assert.True(t, set.Contains("a"))
	assert.True(t, set.Contains("b"))
	assert.False(t, set.Contains("c"))
	assert.True(t, set.Contains("z"))
}

func TestToSlice(t *testing.T) {
	set := CreateSet([]string{"1", "2", "3"})
	arr := set.ToSlice()
	assert.Equal(t, 3, len(arr))

	// Empty set
	set = CreateSet([]string{})
	setSlice := set.ToSlice()
	assert.NotNil(t, setSlice)
	assert.Equal(t, 0, len(setSlice))
}

func TestCopy(t *testing.T) {
	set := CreateSet([]string{"x", "y", "z"})
	copySet := set.Copy()
	assert.Equal(t, 3, copySet.Count())
	assert.True(t, copySet.Contains("x"))
	assert.True(t, copySet.Contains("y"))
	assert.True(t, copySet.Contains("z"))
	assert.False(t, copySet.Contains("a"))
}

func TestIter(t *testing.T) {
	set := CreateSet([]string{"a", "b", "c", "x", "y", "z"})
	count := 0
	for range set.Iter() {
		count++
	}
	assert.Equal(t, 6, count)
}

func TestSetEquals(t *testing.T) {
	set := CreateSet([]string{"a", "b"})
	assert.True(t, set.Equals(CreateSet([]string{"a", "b"})))
	assert.False(t, set.Equals(CreateSet([]string{"a", "b", "c"})))
	assert.False(t, set.Equals(CreateSet([]string{"a"})))
	assert.False(t, set.Equals(CreateSet([]string{"a", "x"})))

	set = CreateSet([]string{})
	assert.True(t, set.Equals(CreateSet([]string{})))
	assert.False(t, set.Equals(CreateSet([]string{"a"})))
}
