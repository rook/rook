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

import "github.com/rook/rook/tests/framework/leakguard"

const rgwLogs = "logs/*_rook-ceph-rgw-*_rgw_*.log"

// knownLeaks are the leaks the integration tests cannot fix yet. Each names
// the leak it covers, and its reason says where the leak comes from. The
// guard's report lists an entry that matched nothing as unused, so that one
// whose leak is fixed is noticed and removed.
var knownLeaks = []leakguard.KnownLeak{
	{
		Name:    "rgw-admin-ops-access-log",
		Reason:  "RGW's beast access log records each admin ops request line, query string included, so the secret key Rook sends when it sets a CephObjectStoreUser's spec.keys appears in the RGW log.",
		What:    []string{"k8s-secret/*/*:SecretKey", "k8s-secret/*/*:AWS_SECRET_ACCESS_KEY"},
		File:    rgwLogs,
		Markers: [][]string{{" beast: ", `"PUT /admin/user?`}},
	},
	{
		Name:   "rgw-keystone-password-config-log",
		Reason: "RGW logs config values at debug level 10, rgw_keystone_admin_password included, which Rook sets in the mon config store from the keystone service user's Secret; Ceph's recent-events dump writes those lines to the log.",
		What:   []string{"k8s-secret/*/*:OS_PASSWORD", "k8s-secret/*/keystone-user-passwords:*"},
		File:   rgwLogs,
		Markers: [][]string{
			{"set_mon_vals rgw_keystone_admin_password = "},
			{"rgw_keystone_admin_password cleared (was "},
		},
	},
}
