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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a AnnotationsSpec) All() Annotations {
	return a[KeyAll]
}

// ApplyToObjectMeta adds or overwrites if exists annotations to object meta.
func (a Annotations) ApplyToObjectMeta(t *metav1.ObjectMeta) {
	if t.Annotations == nil {
		t.Annotations = map[string]string{}
	}
	for k, v := range a {
		if _, ok := t.Annotations[k]; !ok {
			t.Annotations[k] = v
		}
	}
}

// Merge returns an Annotations which results from merging the attributes of the
// original Annotations with the attributes of the supplied one. The supplied
// Placement's attributes will override the original ones if defined.
func (a Annotations) Merge(with Annotations) Annotations {
	ret := a
	for k, v := range with {
		if _, ok := ret[k]; !ok {
			ret[k] = v
		}
	}
	return ret
}

// GetMapStringString return the Annotations as a
func (a Annotations) GetMapStringString() map[string]string {
	res := map[string]string{}
	for k, v := range a {
		res[k] = v
	}
	return res
}
