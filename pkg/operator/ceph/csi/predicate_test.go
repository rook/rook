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

package csi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_findCSIChange(t *testing.T) {
	t.Run("no match", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.False(t, b)
	})

	t.Run("match on addition", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
+       "ROOK_CSI_ENABLE_CEPHFS":             "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})

	t.Run("match on deletion", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
-       "ROOK_CSI_ENABLE_CEPHFS":             "true",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})

	t.Run("match on addition and deletion", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
-       "ROOK_CSI_ENABLE_CEPHFS":             "true",
+       "ROOK_CSI_ENABLE_CEPHFS":             "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})

	t.Run("match with CSI_ name", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
-       "CSI_PROVISIONER_TOLERATIONS":             "true",
+       "CSI_PROVISIONER_TOLERATIONS":             "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})

	t.Run("match with CSI_ and ROOK_CSI_ name", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
-       "CSI_PROVISIONER_TOLERATIONS":             "true",
+       "CSI_PROVISIONER_TOLERATIONS":             "false",
-       "ROOK_CSI_ENABLE_CEPHFS":             "true",
+       "ROOK_CSI_ENABLE_CEPHFS":             "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})
}
