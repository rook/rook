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

package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_quoteArg(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{"empty", "", "''"},
		{"plain word", "radosgw-admin", "radosgw-admin"},
		{"flag", "--rgw-realm=ceph-objectstore", "--rgw-realm=ceph-objectstore"},
		{"path", "/var/lib/rook/rook-ceph/rook-ceph.config", "/var/lib/rook/rook-ceph/rook-ceph.config"},
		{"space", "RGW Admin Ops User", "'RGW Admin Ops User'"},
		{"semicolon and glob", "buckets=*;users=*", "'buckets=*;users=*'"},
		{"command substitution", "$(id)", "'$(id)'"},
		{"backtick", "`id`", "'`id`'"},
		{"tilde", "~/keyring", "'~/keyring'"},
		{"pipe", "a|b", "'a|b'"},
		{"newline", "a\nb", "'a\nb'"},
		{"single quote", "it's", `'it'\''s'`},
		{"only a single quote", "'", `''\'''`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, quoteArg(tt.arg))
		})
	}
}

func TestFormatCommand(t *testing.T) {
	t.Run("no arguments", func(t *testing.T) {
		assert.Equal(t, "ceph", FormatCommand("ceph"))
	})

	t.Run("empty argument is not swallowed", func(t *testing.T) {
		assert.Equal(t, "ceph '' status", FormatCommand("ceph", "", "status"))
	})

	t.Run("rgw admin ops user", func(t *testing.T) {
		got := FormatCommand("radosgw-admin",
			"user", "create",
			"--uid", "rgw-admin-ops-user",
			"--display-name", "RGW Admin Ops User",
			"--caps", "accounts=*;buckets=*;users=*;usage=read;metadata=read;zone=read",
			"--rgw-realm=ceph-objectstore",
			"--cluster=rook-ceph",
			"--conf=/var/lib/rook/rook-ceph/rook-ceph.config",
			"--name=client.admin",
			"--keyring=/var/lib/rook/rook-ceph/client.admin.keyring",
		)

		assert.Equal(t, "radosgw-admin user create "+
			"--uid rgw-admin-ops-user "+
			"--display-name 'RGW Admin Ops User' "+
			"--caps 'accounts=*;buckets=*;users=*;usage=read;metadata=read;zone=read' "+
			"--rgw-realm=ceph-objectstore "+
			"--cluster=rook-ceph "+
			"--conf=/var/lib/rook/rook-ceph/rook-ceph.config "+
			"--name=client.admin "+
			"--keyring=/var/lib/rook/rook-ceph/client.admin.keyring", got)
	})
}
