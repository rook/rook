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

package dependents

import (
	"fmt"
	"sort"
	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
)

func DeletionBlockedDueToDependentsCondition(blocked bool, message string) cephv1.Condition {
	status := corev1.ConditionFalse
	reason := cephv1.ObjectHasNoDependentsReason
	if blocked {
		status = corev1.ConditionTrue
		reason = cephv1.ObjectHasDependentsReason
	}
	return cephv1.Condition{
		Type:    cephv1.ConditionDeletionIsBlocked,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
}

func DeletionBlockedDueToNonEmptyPoolCondition(blocked bool, message string) cephv1.Condition {
	status := corev1.ConditionFalse
	reason := cephv1.PoolEmptyReason
	if blocked {
		status = corev1.ConditionTrue
		reason = cephv1.PoolNotEmptyReason
	}
	return cephv1.Condition{
		Type:    cephv1.ConditionPoolDeletionIsBlocked,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
}

func DeletionBlockedDueToNonEmptyRadosNSCondition(blocked bool, message string) cephv1.Condition {
	status := corev1.ConditionFalse
	reason := cephv1.RadosNamespaceEmptyReason
	if blocked {
		status = corev1.ConditionTrue
		reason = cephv1.RadosNamespaceNotEmptyReason
	}
	return cephv1.Condition{
		Type:    cephv1.ConditionRadosNSDeletionIsBlocked,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
}

// A DependentList represents a list of dependents of a resource. Each dependent has a plural Kind
// and a list of names of dependent resources.
type DependentList struct {
	d map[string][]string // map from dependent Resource to a list of dependent names
}

// NewDependentList creates a new empty DependentList.
func NewDependentList() *DependentList {
	return &DependentList{
		d: make(map[string][]string),
	}
}

// Empty returns true if the DependentList is empty or false otherwise.
func (d *DependentList) Empty() bool {
	return len(d.d) == 0
}

// Add adds a dependent name for a plural Kind to the DependentList.
func (d *DependentList) Add(pluralKind string, name string) {
	names, ok := d.d[pluralKind]
	if !ok {
		d.d[pluralKind] = []string{name}
		return
	}
	d.d[pluralKind] = append(names, name)
}

// PluralKinds returns the plural Kinds that have dependents.
func (d *DependentList) PluralKinds() []string {
	kinds := []string{}
	for k := range d.d {
		kinds = append(kinds, k)
	}
	return kinds
}

// OfKind returns the names of dependents of the Kind (plural), or an empty list if no
// dependents exist.
func (d *DependentList) OfKind(pluralKind string) []string {
	names, ok := d.d[pluralKind]
	if !ok {
		return []string{}
	}
	return names
}

// StringWithHeader outputs the dependent list as a pretty-printed string headed with the given
// formatting directive (followed by a colon). It outputs dependents in alphabetical order by the
// plural Kind.
// Example:
//
//	StringWithHeader("dependents of my %q", "mom")  -->
//	`dependents of my "mom": FirstResources: [name1], SecondResources: [name2 name2 name3]`
func (d *DependentList) StringWithHeader(headerFormat string, args ...interface{}) string {
	header := fmt.Sprintf(headerFormat, args...)
	if len(d.d) == 0 {
		return fmt.Sprintf("%s: none", header)
	}
	deps := make([]string, 0, len(d.d))
	for pluralKind, names := range d.d {
		deps = append(deps, fmt.Sprintf("%s: %v", pluralKind, names))
	}
	sort.Strings(deps) // always output a consistent ordering
	allDeps := strings.Join(deps, ", ")
	return fmt.Sprintf("%s: %s", header, allDeps)
}
