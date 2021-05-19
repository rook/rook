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

package rook

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

// KeyType type safety
type KeyType string

// Labels are label for a given daemons
type Labels map[string]string

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
