/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetIPFromEndpoint(t *testing.T) {
	assert.Equal(t, "192.168.0.1", GetIPFromEndpoint("192.168.0.1:6789"))
	assert.Equal(t, "::1", GetIPFromEndpoint("[::1]:6789"))
	assert.Equal(t, "10.0.0.1", GetIPFromEndpoint("10.0.0.1:3300"))
}

func TestGetPortFromEndpoint(t *testing.T) {
	assert.Equal(t, int32(6789), GetPortFromEndpoint("192.168.0.1:6789"))
	assert.Equal(t, int32(3300), GetPortFromEndpoint("[::1]:3300"))
	assert.Equal(t, int32(0), GetPortFromEndpoint("192.168.0.1:abc"))
	assert.Equal(t, int32(0), GetPortFromEndpoint("192.168.0.1:0"))
}
