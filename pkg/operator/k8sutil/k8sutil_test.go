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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateNodeName(t *testing.T) {
	// An entry's key is the result. The first value in the []string is the format and the second is the nodeName
	tests := map[string][]string{
		"rook-ceph-osd-prepare-47d1eb58e063be11c11c995b74cc5fbe-config": { // 61 chars
			"rook-ceph-osd-prepare-%s-config",                                      // 29 chars (without format)
			"k8s-worker-1234567890.this.is.a.very.very.long.node.name.example.com", // 68 chars
		},
		"rook-ceph-osd-prepare-k8s01": { // 27 chars
			"rook-ceph-osd-prepare-%s", // 22 chars (without format)
			"k8s01",                    // 5 chars
		},
		"rook-ceph-osd-prepare-k8s-worker-500.this.is.a.not.so.long.name": { // 63 chars
			"rook-ceph-osd-prepare-%s",                  // 22 chars (without format)
			"k8s-worker-500.this.is.a.not.so.long.name", // 41 chars
		},
		"47d1eb58e063be11c11c995b74cc5fbe": { // 32 chars
			"%s", // 0 chars (without format)
			"k8s-worker-1234567890.this.is.a.very.very.long.node.name.example.com", // 68 chars
		},
		"rook-ceph-osd-prepare-test-very-long-name-47d1eb58e063be11c11c995b74cc5fbe": { // 74 chars
			"rook-ceph-osd-prepare-test-very-long-name-%s",                         // 42 chars (without format)
			"k8s-worker-1234567890.this.is.a.very.very.long.node.name.example.com", // 68 chars
		},
	}
	for result, params := range tests {
		assert.Equal(t, result, TruncateNodeName(params[0], params[1]))
	}
}

func TestValidateLabelValue(t *testing.T) {
	// The key is the result, and the value is the input.
	tests := map[string]string{
		"":                        "",
		"1.0":                     "1.0",
		"1.0.0-git1697.ga265cdfd": "1.0.0+git1697.ga265cdfd",
		"this-entry-is-more-than-63-characters-long-so-it-will-be-trunca": "this-entry-is-more-than-63-characters-long-so-it-will-be-truncated",
		"1": ".1.",
	}

	for result, input := range tests {
		assert.Equal(t, result, validateLabelValue(input))
	}
}
