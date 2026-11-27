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

package installer

import (
	"testing"

	"github.com/rook/rook/tests/framework/leakguard"
	"github.com/stretchr/testify/assert"
)

func TestKnownLeaksAreValid(t *testing.T) {
	names := map[string]bool{}
	for _, k := range knownLeaks {
		assert.NoError(t, k.Validate())
		assert.False(t, names[k.Name], "known leak %q is listed twice", k.Name)
		names[k.Name] = true
	}
}

// The evidence lines have the shapes of the lines CI captured each known leak
// on, with VALUE in place of each secret and the fields after it shortened.
func TestKnownLeaksMatchTheirEvidence(t *testing.T) {
	const (
		rgwLog      = "logs/object-ns_rook-ceph-rgw-sharedstore-a-79b6f44759-rzvqh_rgw_0.log"
		keystoneLog = "logs/keystoneauth-ns_rook-ceph-rgw-default-a-569dfc54f5-4pvqm_rgw_0.log"
		cleanupSpec = "specs/smoke-ns_cluster-cleanup-job-kind-control-plane-s547l.txt"
	)
	tests := []struct {
		name string
		what string
		file string
		line string
		want string
	}{
		{
			name: "object user keys in the RGW access log",
			what: "k8s-secret/test-userkeys/rook-ceph-object-user-sharedstore-test-userkeys-user1:SecretKey",
			file: rgwLog,
			line: `debug 2026-11-21T16:00:39.519+0000 7fe1131e2640  1 beast: 0x7fded03881d0: 10.244.0.6 - admin [21/Nov/2026:16:00:39.516 +0000] "PUT /admin/user?key&uid=user1&secret-key=VALUE&format=json HTTP/1.1" 200`,
			want: "rgw-admin-ops-access-log",
		},
		{
			name: "user-provided keys in the RGW access log",
			what: "k8s-secret/test-userkeys/test-userkeys-secret1:AWS_SECRET_ACCESS_KEY",
			file: rgwLog,
			line: `debug 2026-11-21T16:00:38.608+0000 7fe06012f640  1 beast: 0x7fded04091d0: 10.244.0.6 - admin [21/Nov/2026:16:00:38.605 +0000] "PUT /admin/user?key&uid=user2&secret-key=VALUE&format=json HTTP/1.1" 200`,
			want: "rgw-admin-ops-access-log",
		},
		{
			name: "keystone password set from the mon config store",
			what: "k8s-secret/keystoneauth-ns/usersecret:OS_PASSWORD",
			file: keystoneLog,
			line: `debug  -1845> 2026-11-21T16:04:08.899+0000 7f8ed96be980 10 set_mon_vals rgw_keystone_admin_password = VALUE`,
			want: "rgw-keystone-password-config-log",
		},
		{
			name: "keystone password cleared from the config",
			what: "k8s-secret/keystoneauth-ns/keystone-user-passwords:rook-user",
			file: keystoneLog,
			line: `debug   -138> 2026-11-21T16:05:15.472+0000 7f8cc143e640 10 operator() rgw_keystone_admin_password cleared (was VALUE)`,
			want: "rgw-keystone-password-config-log",
		},
		{
			name: "keypairs in an operator log",
			what: "k8s-secret/smoke-ns/rook-ceph-object-user-store-user:SecretKey",
			file: "logs/smoke-system_rook-ceph-operator-7fb49fdb4f-bj6r9_rook-ceph-operator_0.log",
			line: `2026-11-21 16:02:12.800171 D | ceph-object-store-user-controller: [smoke-ns/user] reducing user "user" keypairs to [{user a VALUE    <nil>}]`,
		},
		{
			name: "mon secret of the cleanup Job",
			what: "k8s-secret/smoke-ns/rook-ceph-mon:mon-secret",
			file: cleanupSpec,
			line: `containers.host-cleanup.env.ROOK_MON_SECRET: VALUE`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := leakguard.Finding{Detector: "exact", What: tt.what, Location: leakguard.Location{File: tt.file, Line: 1}}
			var matched []string
			for _, k := range knownLeaks {
				if k.Matches(f, []byte(tt.line)) {
					matched = append(matched, k.Name)
				}
			}
			if tt.want == "" {
				assert.Empty(t, matched)
				return
			}
			assert.Equal(t, []string{tt.want}, matched)
		})
	}
}
