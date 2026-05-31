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

import "testing"

func TestPathToVolumeName(t *testing.T) {
	tests := []struct {
		name string // test name
		path string // argument
		want string
	}{
		{"simple", "simple", "simple"},
		{"preceding slash", "/preceding", "preceding"},
		{"trailing slash", "trailing/", "trailing"},
		{"convert to lower case", "ASDFGHJKLQWERTYUIOPZXCVBNM", "asdfghjklqwertyuiopzxcvbnm"},
		{"preserve nums", "0123456789", "0123456789"},
		{"preserve lower case", "qwertyuiopasdfghjklzxcvbnm", "qwertyuiopasdfghjklzxcvbnm"},
		{
			"convert any non-lower-case-alphanum symbols to dash",
			"a/.;,=[]_~!@`#$%^&*()_+-<>?:\"\\'|}{z", // symbols on U.S. keyboard
			"a---------------------------------z",
		},
		{
			"various currency symbols", // only those written left-to-right
			"z£¢©®™¥€§฿₽₨₱¤₡₫ƒ₲₴₭č₾֏₣лвдине₤₺₼₥₦₱៛₹₪৳₸₩ła",
			"z------------------------------------------a",
		},
		{"full-width symbols", "q￠￥￦℃℉p", "q-----p"}, // only those written left-to-right
		{
			"longer than 63 chars", // if you change the arg string, the hash on the end will change
			"/this/is/some-path/.that/is_longer/than/$63/chars/1234567890/and/i'm/still/typing",
			"this-is-some-path--that-i---7890-and-i-m-still-typing-b6b6f18b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PathToVolumeName(tt.path); got != tt.want {
				t.Errorf("PathToVolumeName() = %v, want %v", got, tt.want)
			}
		})
	}
}
