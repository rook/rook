/*
Copyright 2025 The Rook Authors. All rights reserved.

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
package log

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	"k8s.io/apimachinery/pkg/types"
)

func NamedInfo(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	NamespacedInfo(name.String(), logger, message, args...)
}

func NamedWarning(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	NamespacedWarning(name.String(), logger, message, args...)
}

func NamedError(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	NamespacedError(name.String(), logger, message, args...)
}

func NamedDebug(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	NamespacedDebug(name.String(), logger, message, args...)
}

func NamedTrace(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	NamespacedTrace(name.String(), logger, message, args...)
}

func NamespacedInfo(namespace string, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	logger.Infof("[%s] %s", namespace, fmt.Sprintf(message, args...))
}

func NamespacedWarning(namespace string, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	logger.Warningf("[%s] %s", namespace, fmt.Sprintf(message, args...))
}

func NamespacedError(namespace string, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	logger.Errorf("[%s] %s", namespace, fmt.Sprintf(message, args...))
}

func NamespacedDebug(namespace string, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	logger.Debugf("[%s] %s", namespace, fmt.Sprintf(message, args...))
}

func NamespacedTrace(namespace string, logger *capnslog.PackageLogger, message string, args ...interface{}) {
	logger.Tracef("[%s] %s", namespace, fmt.Sprintf(message, args...))
}
