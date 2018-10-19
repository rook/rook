/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionAtLeast(t *testing.T) {
	// Valid combinations
	assert.True(t, VersionAtLeast(Luminous, Luminous))
	assert.False(t, VersionAtLeast(Luminous, Mimic))
	assert.False(t, VersionAtLeast(Luminous, Nautilus))
	assert.True(t, VersionAtLeast(Mimic, Luminous))
	assert.True(t, VersionAtLeast(Mimic, Mimic))
	assert.False(t, VersionAtLeast(Mimic, Nautilus))
	assert.True(t, VersionAtLeast(Nautilus, Luminous))
	assert.True(t, VersionAtLeast(Nautilus, Mimic))
	assert.True(t, VersionAtLeast(Nautilus, Nautilus))

	// Invalid combinations
	assert.False(t, VersionAtLeast(Mimic, "foo"))
	assert.False(t, VersionAtLeast("foo", Luminous))
}
