/*
Copyright 2023 The Rook Authors. All rights reserved.

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
	"reflect"

	corev1 "k8s.io/api/core/v1"
)

func (src *ConfigFileVolumeSource) ToKubernetesVolumeSource() *corev1.VolumeSource {
	if src == nil {
		return nil
	}

	dst := &corev1.VolumeSource{}
	vDst := reflect.ValueOf(dst).Elem()

	tSrc := reflect.TypeOf(*src)
	vSrc := reflect.ValueOf(*src)
	for _, srcField := range reflect.VisibleFields(tSrc) {
		if !srcField.IsExported() {
			continue
		}

		srcVal := vSrc.FieldByName(srcField.Name)
		if srcVal.IsNil() {
			continue // don't do anything if the src field is a nil ptr
		}

		dstVal := vDst.FieldByName(srcField.Name)
		dstVal.Set(srcVal)
	}

	return dst
}

func (t *VolumeClaimTemplate) ToPVC() *corev1.PersistentVolumeClaim {
	if t == nil {
		return nil
	}
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: *t.ObjectMeta.DeepCopy(),
		Spec:       *t.Spec.DeepCopy(),
	}
}
