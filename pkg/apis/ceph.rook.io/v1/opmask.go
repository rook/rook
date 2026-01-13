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

package v1

import (
	"strings"

	"github.com/pkg/errors"
)

// Parse the operations mask string and construct an opMask struct.
// Unlike radosgw-admin, unknown operations are not accepted.
func ParseObjectUserOpMask(doc string) (*ObjectUserOpMask, error) {
	mask := &ObjectUserOpMask{}

	if doc == "*" {
		mask.Read = true
		mask.Write = true
		mask.Delete = true
		return mask, nil
	}

	ops := strings.Split(doc, ",")
	for _, op := range ops {
		op = strings.TrimSpace(op)
		switch op {
		case "read":
			mask.Read = true
		case "write":
			mask.Write = true
		case "delete":
			mask.Delete = true
		default:
			if op == "*" {
				return nil, errors.Errorf(`invalid use of glob ("*") combined with other operations in op-mask`)
			}
			return nil, errors.Errorf("invalid operation %q in op-mask %q", op, doc)
		}
	}

	return mask, nil
}

// Format the mask as a string with the operators normalized into the same order as is returned by radosgw-admin.
func (mask *ObjectUserOpMask) String() string {
	ops := []string{}
	if mask.Read {
		ops = append(ops, "read")
	}
	if mask.Write {
		ops = append(ops, "write")
	}
	if mask.Delete {
		ops = append(ops, "delete")
	}

	return strings.Join(ops, ", ")
}
