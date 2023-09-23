/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package multus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_perNodeTypeCount_Increment(t *testing.T) {
	tests := []struct {
		name string
		a    *perNodeTypeCount
		want *perNodeTypeCount
	}{
		{"empty", &perNodeTypeCount{}, &perNodeTypeCount{"type": 1}},
		{"type already set", &perNodeTypeCount{"type": 2}, &perNodeTypeCount{"type": 3}},
		{"other already set", &perNodeTypeCount{"other": 3}, &perNodeTypeCount{"other": 3, "type": 1}},
		{"type and other already set", &perNodeTypeCount{"type": 1, "other": 3}, &perNodeTypeCount{"type": 2, "other": 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.a.Increment("type")
			assert.Equal(t, *tt.want, *tt.a)
		})
	}
}

func Test_perNodeTypeCount_Equal(t *testing.T) {
	tests := []struct {
		name string
		a    *perNodeTypeCount
		b    *perNodeTypeCount
		want bool
	}{
		{"empty vs empty", &perNodeTypeCount{}, &perNodeTypeCount{}, true},
		{"shared:1 vs empty", &perNodeTypeCount{"shared": 1}, &perNodeTypeCount{}, false},
		{"empty vs shared:1", &perNodeTypeCount{}, &perNodeTypeCount{"shared": 1}, false},
		{"shared:0 vs shared:0", &perNodeTypeCount{"shared": 0}, &perNodeTypeCount{"shared": 0}, true},
		{"shared:1 vs shared:0", &perNodeTypeCount{"shared": 1}, &perNodeTypeCount{"shared": 0}, false},
		{"shared:0 vs shared:1", &perNodeTypeCount{"shared": 0}, &perNodeTypeCount{"shared": 1}, false},
		{"shared:1 vs shared:1", &perNodeTypeCount{"shared": 1}, &perNodeTypeCount{"shared": 1}, true},
		{"shared:1 vs shared:2", &perNodeTypeCount{"shared": 1}, &perNodeTypeCount{"shared": 2}, false},
		{"shared:1 vs other:1", &perNodeTypeCount{"shared": 1}, &perNodeTypeCount{"other": 1}, false},
		{"shared:1 vs other:0", &perNodeTypeCount{"shared": 1}, &perNodeTypeCount{"other": 0}, false},
		{"shared:1,other:2 vs shared:1,other:2", &perNodeTypeCount{"shared": 1, "other": 2}, &perNodeTypeCount{"shared": 1, "other": 2}, true},
		{"shared:1,other:2 vs shared:2,other:2", &perNodeTypeCount{"shared": 1, "other": 2}, &perNodeTypeCount{"shared": 2, "other": 2}, false},
		{"shared:1,other:2 vs shared:2,other:1", &perNodeTypeCount{"shared": 1, "other": 2}, &perNodeTypeCount{"shared": 2, "other": 1}, false},
		{"shared:1,other:2 vs shared:1,other:1", &perNodeTypeCount{"shared": 1, "other": 2}, &perNodeTypeCount{"shared": 1, "other": 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("perNodeTypeCount.Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_perNodeTypeCount_Total(t *testing.T) {
	tests := []struct {
		name string
		a    *perNodeTypeCount
		want int
	}{
		{"empty", &perNodeTypeCount{}, 0},
		{"sample:1", &perNodeTypeCount{"sample": 1}, 1},
		{"sample:1, other:2", &perNodeTypeCount{"sample": 1, "other": 2}, 3},
		{"sample:0", &perNodeTypeCount{"sample": 0}, 0},
		{"sample:0, other:2", &perNodeTypeCount{"sample": 0, "other": 2}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Total(); got != tt.want {
				t.Errorf("perNodeTypeCount.Total() = %v, want %v", got, tt.want)
			}
		})
	}
}
