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

package object

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractJson(t *testing.T) {
	s := "invalid json"
	_, err := extractJSON(s)
	assert.Error(t, err)

	s = `{"test": "test"}`
	match, err := extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))

	s = `this line can't be parsed as json
{"test": "test"}`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))

	s = `this line can't be parsed as json
{"test":
"test"}`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))

	s = `{"test": "test"}
this line can't be parsed as json`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))
}
