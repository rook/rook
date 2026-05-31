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
	"github.com/coreos/pkg/capnslog"
)

const DefaultLogLevel = capnslog.INFO

func SetGlobalLogLevel(userLogLevelSelection string, logger *capnslog.PackageLogger) {
	// capnslog supports trace level logging, but in Rook we want to treat trace logging as insecure
	// and block users from finding the value in most circumstances. If they request "TRACE" level
	// logging, just output debug logs.
	if userLogLevelSelection == "TRACE" {
		userLogLevelSelection = "DEBUG"
	}
	// only if users give the super secret "TRACE_INSECURE" log level will they get real trace
	// logging, which might leak credentials and other insecure nasties into their logs.
	if userLogLevelSelection == "TRACE_INSECURE" {
		userLogLevelSelection = "TRACE"
	}

	// parse given log level string then set up corresponding global logging level
	logLevel, err := capnslog.ParseLevel(userLogLevelSelection)
	if err != nil {
		logger.Errorf("failed to parse log level %q. defaulting to %q. %v", userLogLevelSelection, DefaultLogLevel.String(), err)
		logLevel = DefaultLogLevel
	}

	// If capnslog changes in the future to allow a more verbose level than TRACE and a user somehow
	// enters it, then reject that log level, and revert to default. This can't be unit tested, but
	// it'll probably never happen in the wild anyway, just here for safety.
	if logLevel > capnslog.TRACE {
		logger.Infof("not setting log level %q more verbose than TRACE. reverting to default %q", logLevel.String(), DefaultLogLevel.String())
		logLevel = DefaultLogLevel
	}

	capnslog.SetGlobalLogLevel(logLevel)
}
