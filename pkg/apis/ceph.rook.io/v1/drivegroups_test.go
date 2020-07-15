/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDriveGroupSpec_DeepCopy tests that DriveGroupSpec's DeepCopy method does indeed create a deep
// copy and not merely do a copy that results in any part of the copy having pointer references to
// the original.
func TestDriveGroupSpec_DeepCopy(t *testing.T) {
	dgJSON := `{"string": "test", "num": 1, "bool": true, "array": ["array-string", 2, false], "map": {"map-string": "tested", "map-num": 3, "map-bool": false, "map-array": ["mas", 4, true]}}`

	dg := DriveGroupSpec{}
	err := json.Unmarshal([]byte(dgJSON), &dg)
	assert.NoError(t, err)
	fmt.Printf("%+v\n", dg)

	copy := dg.DeepCopy()

	// type casting helper func to get stuff from DriveGroupSpec's so go compiler doesn't complain
	// get map item as array
	getArr := func(m map[string]interface{}, arrayKey string) []interface{} {
		i, ok := (m[arrayKey]).([]interface{})
		if !ok {
			panic("did not convert array")
		}
		return i
	}
	// get map item as another map
	getMap := func(m map[string]interface{}, mapKey string) map[string]interface{} {
		i, ok := (m[mapKey]).(map[string]interface{})
		if !ok {
			panic("did not convert map")
		}
		return i
	}

	// After copy, the two should be equal
	assert.Equal(t, dg, copy)
	assert.Equal(t, 5, len(copy))
	assert.Equal(t, "test", copy["string"])
	assert.Equal(t, float64(1), copy["num"]) // json umarshals nums as float64
	assert.Equal(t, true, copy["bool"])
	a := getArr(copy, "array")
	assert.Equal(t, 3, len(a))
	assert.Equal(t, "array-string", a[0])
	assert.Equal(t, float64(2), a[1])
	assert.Equal(t, false, a[2])
	m := getMap(copy, "map")
	assert.Equal(t, 4, len(m))
	assert.Equal(t, "tested", m["map-string"])
	assert.Equal(t, float64(3), m["map-num"])
	assert.Equal(t, false, m["map-bool"])
	a2 := getArr(m, "map-array")
	assert.Equal(t, 3, len(a2))
	assert.Equal(t, "mas", a2[0])
	assert.Equal(t, float64(4), a2[1])
	assert.Equal(t, true, a2[2])

	// If the copy's elements are changed, it should NOT change the original
	copy["string"] = "changed"
	assert.NotEqual(t, dg, copy)
	assert.Equal(t, "test", dg["string"])
	assert.Equal(t, "changed", copy["string"])

	// Create a new copy for testing more deeply nested changes
	copy = dg.DeepCopy()
	assert.Equal(t, dg, copy)                         // copy should be equal to orig again
	getArr(getMap(copy, "map"), "map-array")[1] = "4" // change array nested in map
	assert.NotEqual(t, dg, copy)                      // copy should no longer be equal
	assert.Equal(t, "4", getArr(getMap(copy, "map"), "map-array")[1])
	assert.Equal(t, float64(4), getArr(getMap(dg, "map"), "map-array")[1])
}
