/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package v1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	// SkipReconcileLabelKey is a label indicating that the pod should not be reconciled
	SkipReconcileLabelKey = "ceph.rook.io/do-not-reconcile"
)

// LabelsSpec is the main spec label for all daemons
type LabelsSpec map[KeyType]Labels

// KeyType type safety
type KeyType string

// Labels are label for a given daemons
type Labels map[string]string

func (a LabelsSpec) All() Labels {
	return a[KeyAll]
}

// GetMgrLabels returns the Labels for the MGR service
func GetMgrLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyMgr)
}

// GetDashboardLabels returns the Labels for the Dashboard service
func GetDashboardLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyDashboard)
}

// GetMonLabels returns the Labels for the MON service
func GetMonLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyMon)
}

// GetKeyRotationLabels returns labels for the key Rotation job
func GetKeyRotationLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyRotation)
}

// GetOSDPrepareLabels returns the Labels for the OSD prepare job
func GetOSDPrepareLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyOSDPrepare)
}

// GetOSDLabels returns the Labels for the OSD service
func GetOSDLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyOSD)
}

// GetCleanupLabels returns the Labels for the cleanup job
func GetCleanupLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyCleanup)
}

// GetMonitoringLabels returns the Labels for monitoring resources
func GetMonitoringLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyMonitoring)
}

// GetCrashCollectorLabels returns the Labels for the crash collector resources
func GetCrashCollectorLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyCrashCollector)
}

func GetCephExporterLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyCephExporter)
}

func GetCmdReporterLabels(a LabelsSpec) Labels {
	return mergeAllLabelsWithKey(a, KeyCmdReporter)
}

func mergeAllLabelsWithKey(a LabelsSpec, name KeyType) Labels {
	all := a.All()
	if all != nil {
		return all.Merge(a[name])
	}
	return a[name]
}

// ApplyToObjectMeta adds labels to object meta unless the keys are already defined.
func (a Labels) ApplyToObjectMeta(t *metav1.ObjectMeta) {
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	for k, v := range a {
		if _, ok := t.Labels[k]; !ok {
			t.Labels[k] = v
		}
	}
}

// OverwriteApplyToObjectMeta adds labels to object meta, overwriting keys that are already defined.
func (a Labels) OverwriteApplyToObjectMeta(t *metav1.ObjectMeta) {
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	for k, v := range a {
		t.Labels[k] = v
	}
}

// Merge returns a Labels which results from merging the attributes of the
// original Labels with the attributes of the supplied one. The supplied
// Labels attributes will override the original ones if defined.
func (a Labels) Merge(with Labels) Labels {
	ret := Labels{}
	for k, v := range a {
		if _, ok := ret[k]; !ok {
			ret[k] = v
		}
	}
	for k, v := range with {
		if _, ok := ret[k]; !ok {
			ret[k] = v
		}
	}
	return ret
}

// ToValidDNSLabel converts a given string to a valid DNS-1035 spec label. The DNS-1035 spec
// follows the regex '[a-z]([-a-z0-9]*[a-z0-9])?' and is at most 63 chars long. DNS-1035 is used
// over DNS-1123 because it is more strict. Kubernetes docs are not always clear when a DNS_LABEL is
// supposed to be 1035 or 1123 compliant, so we use the more strict version for ease of use.
//   - Any input symbol that is not valid is converted to a dash ('-').
//   - Multiple resultant dashes in a row are compressed to a single dash.
//   - If the starting character is a number, a 'd' is prepended to preserve the number.
//   - Any non-alphanumeric starting or ending characters are removed.
//   - If the resultant string is longer than the maximum-allowed 63 characters], characters are
//     removed from the middle and replaced with a double dash ('--') to reduce the string to 63
//     characters.
func ToValidDNSLabel(input string) string {
	maxl := validation.DNS1035LabelMaxLength

	if input == "" {
		return ""
	}

	outbuf := make([]byte, len(input)+1)
	j := 0 // position in output buffer
	last := byte('-')
	for _, c := range []byte(input) {
		switch {
		case c >= 'a' && c <= 'z':
			outbuf[j] = c
		case c >= '0' && c <= '9':
			// if the first char is a number, add a 'd' (for decimal) in front
			if j == 0 {
				outbuf[j] = 'd' // for decimal
				j++
			}
			outbuf[j] = c
		case c >= 'A' && c <= 'Z':
			// convert to lower case
			outbuf[j] = c - 'A' + 'a' // convert to lower case
		default:
			if last == '-' {
				// don't write two dashes in a row
				continue
			}
			outbuf[j] = byte('-')
		}
		last = outbuf[j]
		j++
	}

	// set the length of the output buffer to the number of chars we copied to it so there aren't
	// \0x00 chars at the end
	outbuf = outbuf[:j]

	// trim any leading or trailing dashes
	out := strings.Trim(string(outbuf), "-")

	// if string is longer than max length, cut content from the middle to get it to length
	if len(out) > maxl {
		out = cutMiddle(out, maxl)
	}

	return out
}

// don't use this function for anything less than toSize=4 chars long
func cutMiddle(input string, toSize int) string {
	if len(input) <= toSize {
		return input
	}

	lenLeft := toSize / 2               // truncation rounds down the left side
	lenRight := toSize/2 + (toSize % 2) // modulo rounds up the right side

	buf := []byte(input)

	return string(buf[:lenLeft-1]) + "--" + string(buf[len(input)-lenRight+1:])
}
