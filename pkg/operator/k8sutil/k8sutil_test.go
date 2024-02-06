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
		"rook-ceph-osd-prepare-801a3ba95fe6ce6a3bd879552ca2a8b0-config": { // 61 chars
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
		"801a3ba95fe6ce6a3bd879552ca2a8b0": { // 32 chars
			"%s", // 0 chars (without format)
			"k8s-worker-1234567890.this.is.a.very.very.long.node.name.example.com", // 68 chars
		},
		"rook-ceph-osd-prepare-test-very-long-name-801a3ba95fe6ce6a3bd879552ca2a8b0": { // 74 chars
			"rook-ceph-osd-prepare-test-very-long-name-%s",                         // 42 chars (without format)
			"k8s-worker-1234567890.this.is.a.very.very.long.node.name.example.com", // 68 chars
		},
	}
	for result, params := range tests {
		assert.Equal(t, result, TruncateNodeName(params[0], params[1]))
	}
}

func TestTruncateJobName(t *testing.T) {
	// An entry's key is the result. The first value in the []string is the format and the second is the nodeName
	tests := map[string][]string{
		"rook-ceph-osd-prepare-k8s01": { // 27 chars
			"rook-ceph-osd-prepare-%s", // 22 chars (without format)
			"k8s01",                    // 5 chars
		},
		"rook-ceph-osd-prepare-k8s-worker-500.this.is.a.not.so": { // 53 chars
			"rook-ceph-osd-prepare-%s",        // 22 chars (without format)
			"k8s-worker-500.this.is.a.not.so", // 31 chars
		},
		// 54 chars, but ok that it is longer than 53 since it ends in an alphanumeric char
		"rook-ceph-osd-prepare-4d2c3e33ccd2764180d42c20dce1d66a": {
			"rook-ceph-osd-prepare-%s",         // 22 chars (without format)
			"k8s-worker-ends-with-a-long-name", // 32 chars
		},
	}
	for result, params := range tests {
		assert.Equal(t, result, TruncateNodeNameForJob(params[0], params[1]))
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

func TestToValidDNSLabel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"single dash", "-", ""},
		{"multiple dashes", "----", ""},
		{"lc a", "a", "a"},
		{"lc z", "z", "z"},
		{"lc alphabet", "abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz"},
		{"UC A", "A", "a"},
		{"UC Z", "Z", "z"},
		{"UC ALPHABET", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz"},
		{"mixed case AlPhAbEt", "AbCdEfGhIjKlMnOpQrStUvWxYz", "abcdefghijklmnopqrstuvwxyz"},
		{"single 0", "0", "d0"},
		{"single 9", "9", "d9"},
		{"single 1", "1", "d1"},
		{"numbers", "01234567890", "d01234567890"},
		{"letters with numbers", "1a0b1c2d3e4f5g6h7i8j9k0", "d1a0b1c2d3e4f5g6h7i8j9k0"},
		{"single / symbol", "/", ""},
		{"single : symbol", ":", ""},
		{"single . symbol", ".", ""},
		{"bunch of symbols", "`~!@#$%^&*()_+-={}[]\\|;':\",.<>/?", ""},
		{"alphabet with symbols",
			"a~b!c@d#e$f^g&h*i(j)k_l-m+n+o[p]q{r}s|t:u;v'w<x,y>z", "a-b-c-d-e-f-g-h-i-j-k-l-m-n-o-p-q-r-s-t-u-v-w-x-y-z"},
		{"multiple symbols between letters", "a//b//c", "a-b-c"},
		{"symbol before", "/a/b/c", "a-b-c"},
		{"symbol after", "a/b/c/", "a-b-c"},
		{"symbols before and after", "/a/b/c/", "a-b-c"},
		{"multiple symbols before after between", "//a//b//c//", "a-b-c"},
		{"mix of all tests except length", "//1a//B-c/d_f/../00-thing.ini/", "d1a-b-c-d-f-00-thing-ini"},
		{"too long input -> middle trim",
			"qwertyuiopqwertyuiopqwertyuiopaaqwertyuiopqwertyuiopqwertyuiopaa",
			"qwertyuiopqwertyuiopqwertyuiop--wertyuiopqwertyuiopqwertyuiopaa"},
		{"too long input but symbols allow for no middle trim",
			"/qwertyuiopqwerty/uiopqwertyuiop//qwertyuiopqwerty/uiopqwertyuiop/",
			"qwertyuiopqwerty-uiopqwertyuiop-qwertyuiopqwerty-uiopqwertyuiop"},
		{"max allowed length but starts with number -> middle trim",
			"123qwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiop",
			"d123qwertyuiopqwertyuiopqwerty--pqwertyuiopqwertyuiopqwertyuiop"},
		{"max allowed length ok",
			"qwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiop123",
			"qwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiop123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ToValidDNSLabel(tt.input))
		})
	}
}

func Test_cutMiddle(t *testing.T) {
	// not an exported function, so don't bother with extreme cases like 0, 1, 2, or 3 len inputs
	t.Run("len 8 -> 6", func(t *testing.T) {
		assert.Equal(t, "ab--gh", cutMiddle("abcdefgh", 6))
	})
	t.Run("len 9 -> 6", func(t *testing.T) {
		assert.Equal(t, "ab--hi", cutMiddle("abcdefghi", 6))
	})
	t.Run("len 9 -> 7", func(t *testing.T) {
		assert.Equal(t, "ab--ghi", cutMiddle("abcdefghi", 7))
	})
	t.Run("len 10 -> 10", func(t *testing.T) {
		assert.Equal(t, "qwertyuiop", cutMiddle("qwertyuiop", 10))
	})
	// below is what we really want to test
	t.Run("len 63 -> 63", func(t *testing.T) {
		assert.Equal(t,
			"qwertyuiopqwertyuiopqwertyuiop12qwertyuiopqwertyuiopqwertyuiop1",
			cutMiddle("qwertyuiopqwertyuiopqwertyuiop12qwertyuiopqwertyuiopqwertyuiop1", 63))
	})
	t.Run("len 64 -> 63", func(t *testing.T) {
		assert.Equal(t,
			"qwertyuiopqwertyuiopqwertyuiop--wertyuiopqwertyuiopqwertyuiop12",
			cutMiddle("qwertyuiopqwertyuiopqwertyuiop12qwertyuiopqwertyuiopqwertyuiop12", 63))
	})
}
