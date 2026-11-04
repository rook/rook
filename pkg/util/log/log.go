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
	"encoding/json"
	"fmt"

	"github.com/coreos/pkg/capnslog"
	"k8s.io/apimachinery/pkg/types"
)

// LogAsJSON logs a value as indented JSON with a descriptive label using logf.
// Pass a formatted logging function such as logger.Infof or logger.Debugf.
// It returns an error if the value cannot be marshaled.
func LogAsJSON(logf func(string, ...any), label string, value any) error {
	output, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	logf("%s:\n%s", label, output)
	return nil
}

func NamedInfo(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...any) {
	NamespacedInfo(name.String(), logger, message, args...)
}

func NamedWarning(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...any) {
	NamespacedWarning(name.String(), logger, message, args...)
}

func NamedError(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...any) {
	NamespacedError(name.String(), logger, message, args...)
}

func NamedDebug(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...any) {
	NamespacedDebug(name.String(), logger, message, args...)
}

func NamedTrace(name types.NamespacedName, logger *capnslog.PackageLogger, message string, args ...any) {
	NamespacedTrace(name.String(), logger, message, args...)
}

func NamespacedInfo(namespace string, logger *capnslog.PackageLogger, message string, args ...any) {
	logger.Infof("[%s] %s", namespace, fmt.Sprintf(message, args...))
}

func NamespacedWarning(namespace string, logger *capnslog.PackageLogger, message string, args ...any) {
	logger.Warningf("[%s] %s", namespace, fmt.Sprintf(message, args...))
}

func NamespacedError(namespace string, logger *capnslog.PackageLogger, message string, args ...any) {
	logger.Errorf("[%s] %s", namespace, fmt.Sprintf(message, args...))
}

func NamespacedDebug(namespace string, logger *capnslog.PackageLogger, message string, args ...any) {
	logger.Debugf("[%s] %s", namespace, fmt.Sprintf(message, args...))
}

func NamespacedTrace(namespace string, logger *capnslog.PackageLogger, message string, args ...any) {
	logger.Tracef("[%s] %s", namespace, fmt.Sprintf(message, args...))
}
