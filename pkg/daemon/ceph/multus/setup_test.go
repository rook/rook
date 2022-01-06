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

package multus

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetermineNewLinkName(t *testing.T) {
	// When there are no mlink# interfaces present,
	// determineNewLinkName(interfaces) will return mlink0
	var interfaces []net.Interface = []net.Interface{
		{
			Name: "lo",
		},
		{
			Name: "eth0",
		},
	}

	newLinkName, err := DetermineNewLinkName(interfaces)
	assert.NoError(t, err)
	assert.Equal(t, newLinkName, "mlink0")

	// When there are mlink# interfaces present,
	// the function will return the next available interface.
	interfaces = append(interfaces, net.Interface{Name: "mlink0"})
	newLinkName, err = DetermineNewLinkName(interfaces)
	assert.NoError(t, err)
	assert.Equal(t, newLinkName, "mlink1")
}
