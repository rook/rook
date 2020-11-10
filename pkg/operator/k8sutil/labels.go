/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package k8sutil

import (
	"strings"
)

// ParseStringToLabels parse a label selector string into a map[string]string
func ParseStringToLabels(in string) map[string]string {
	labels := map[string]string{}

	if in == "" {
		return labels
	}

	for _, v := range strings.Split(in, ",") {
		labelSplit := strings.Split(v, "=")

		// When a value is set for a label k/v pair
		if len(labelSplit) > 2 {
			logger.Warningf("more than one value found for a label %q, only the first value will be used", labelSplit[0])
		}

		if len(labelSplit) > 1 {
			labels[labelSplit[0]] = labelSplit[1]
		} else {
			labels[labelSplit[0]] = ""
		}
	}

	return labels
}
