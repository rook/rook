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

package opmask

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

func TestFromSlice(t *testing.T) {
	examples := []struct {
		enum []cephv1.ObjectUserOpMask
		mask *OpMask
	}{
		{[]cephv1.ObjectUserOpMask{"read", "write", "delete"}, &OpMask{read: true, write: true, delete: true}},
		{[]cephv1.ObjectUserOpMask{"delete", "read", "write"}, &OpMask{read: true, write: true, delete: true}},
		{[]cephv1.ObjectUserOpMask{"read"}, &OpMask{read: true}},
		{[]cephv1.ObjectUserOpMask{"write"}, &OpMask{write: true}},
		{[]cephv1.ObjectUserOpMask{"delete"}, &OpMask{delete: true}},
		{[]cephv1.ObjectUserOpMask{"read", "write"}, &OpMask{read: true, write: true}},
		{[]cephv1.ObjectUserOpMask{"read", "delete"}, &OpMask{read: true, delete: true}},
		{[]cephv1.ObjectUserOpMask{"write", "delete"}, &OpMask{write: true, delete: true}},
		{[]cephv1.ObjectUserOpMask{}, &OpMask{}},
	}

	for _, ex := range examples {
		t.Run(fmt.Sprintf("parse slice %v", ex.enum), func(t *testing.T) {
			mask, err := FromSlice(ex.enum)
			assert.NoError(t, err)
			assert.Equal(t, ex.mask, mask)
		})
	}
}

func TestString(t *testing.T) {
	examples := []struct {
		mask *OpMask
		str  string
	}{
		{&OpMask{read: true, write: true, delete: true}, "read, write, delete"},
		{&OpMask{read: true}, "read"},
		{&OpMask{write: true}, "write"},
		{&OpMask{delete: true}, "delete"},
		{&OpMask{read: true, write: true}, "read, write"},
		{&OpMask{read: true, delete: true}, "read, delete"},
		{&OpMask{write: true, delete: true}, "write, delete"},
		{&OpMask{}, "<none>"},
	}

	for _, ex := range examples {
		t.Run(fmt.Sprintf("stringify %v", ex.mask), func(t *testing.T) {
			assert.Equal(t, ex.mask.String(), ex.str)
		})
	}
}
