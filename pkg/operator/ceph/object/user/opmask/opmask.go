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
	"slices"
	"strings"

	"github.com/pkg/errors"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

type OpMask struct {
	read   bool
	write  bool
	delete bool
}

// Convert a slice of strings of operation names into an OpMask.
func FromSlice(ops []cephv1.ObjectUserOpMask) (*OpMask, error) {
	mask := &OpMask{}

	if ops == nil {
		return nil, errors.New("nil slice provided")
	}

	if len(ops) == 0 {
		return mask, nil
	}

	if slices.Contains(ops, "read") {
		mask.read = true
	}
	if slices.Contains(ops, "write") {
		mask.write = true
	}
	if slices.Contains(ops, "delete") {
		mask.delete = true
	}

	return mask, nil
}

// Format the mask as a string with the operators normalized into the same order as is returned by the admin api and radosgw-admin.
func (mask *OpMask) String() string {
	// When no operations are set, the admin api and radosgw-admin return "<none>".
	if !mask.read && !mask.write && !mask.delete {
		return "<none>"
	}

	ops := []string{}
	if mask.read {
		ops = append(ops, "read")
	}
	if mask.write {
		ops = append(ops, "write")
	}
	if mask.delete {
		ops = append(ops, "delete")
	}

	return strings.Join(ops, ", ")
}
