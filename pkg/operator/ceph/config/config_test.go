/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFlag(t *testing.T) {
	assert.Equal(t, NewFlag("k", ""), "--k=")
	assert.Equal(t, NewFlag("a-key", "a"), "--a-key=a")
	assert.Equal(t, NewFlag("b_key", "b"), "--b-key=b")
	assert.Equal(t, NewFlag("c key", "c"), "--c-key=c")
	assert.Equal(t, NewFlag("quotes", "\"quoted\""), "--quotes=\"quoted\"")
}
