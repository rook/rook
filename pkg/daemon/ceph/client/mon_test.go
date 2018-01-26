/*
Copyright 2018 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/digitalocean/ceph_exporter
which has the same license.
*/
package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCephArgs(t *testing.T) {
	// cluster a under /etc
	args := []string{}
	result := AppendAdminConnectionArgs(args, "/etc", "a")
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "--cluster=a", result[0])
	assert.Equal(t, "--conf=/etc/a/a.config", result[1])
	assert.Equal(t, "--keyring=/etc/a/client.admin.keyring", result[2])

	// cluster under /var/lib/rook
	args = []string{"myarg"}
	result = AppendAdminConnectionArgs(args, "/var/lib/rook", "rook")
	assert.Equal(t, 4, len(result))
	assert.Equal(t, "myarg", result[0])
	assert.Equal(t, "--cluster=rook", result[1])
	assert.Equal(t, "--conf=/var/lib/rook/rook/rook.config", result[2])
	assert.Equal(t, "--keyring=/var/lib/rook/rook/client.admin.keyring", result[3])

	// the default ceph cluster will not need the config args
	args = []string{"myarg"}
	result = AppendAdminConnectionArgs(args, "/etc", "ceph")
	assert.Equal(t, 1, len(result))
	assert.Equal(t, "myarg", result[0])
}
