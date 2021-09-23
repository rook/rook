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

package util

import (
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
)

func TestSetGlobalLogLevel(t *testing.T) {
	logger := capnslog.NewPackageLogger("github.com/rook/rook", "pkg/util/logging_test")

	tests := []struct {
		name                  string
		userLogLevelSelection string
		desiredLogLevel       capnslog.LogLevel
	}{
		{"INFO is supported", "INFO", capnslog.INFO},
		{"DEBUG is supported", "DEBUG", capnslog.DEBUG},
		{"WARNING is supported", "WARNING", capnslog.WARNING},
		{"ERROR is supported", "ERROR", capnslog.ERROR},
		{"TRACE will be turned into DEBUG", "TRACE", capnslog.DEBUG},
		{"TRACE_INSECURE will be turned into TRACE", "TRACE_INSECURE", capnslog.TRACE},
		{"an invalid input will be turned into INFO", "INVALID", capnslog.INFO},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetGlobalLogLevel(tt.userLogLevelSelection, logger)
			t.Logf("asserting that log level %d can be shown", tt.desiredLogLevel)
			assert.True(t, logger.LevelAt(tt.desiredLogLevel))
			// logger.LevelAt will only tell whether the logger will output logs at the desired log
			// level, but it won't actually tell what the log level is specifically. To make sure we
			// won't output MORE logs, we should check for the next more verbose log level also.
			nextMostVerboseLevel := tt.desiredLogLevel + 1
			t.Logf("asserting that log level %d can NOT be shown", nextMostVerboseLevel)
			assert.False(t, logger.LevelAt(nextMostVerboseLevel))
		})
	}
}
