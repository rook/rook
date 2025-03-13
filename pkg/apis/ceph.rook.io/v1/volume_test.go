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
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func validateToVolumeSource(
	t *testing.T,
	fieldUnderTest string, fieldValue reflect.Value,
	in *ConfigFileVolumeSource,
) {
	got := in.ToKubernetesVolumeSource()

	// validate got
	vGot := reflect.ValueOf(got).Elem()
	for _, gField := range reflect.VisibleFields(vGot.Type()) {
		gFieldVal := vGot.FieldByName(gField.Name)

		if gField.Name != fieldUnderTest {
			assert.Nilf(t, gFieldVal.Interface(), "fields NOT under test should be nil")
			continue
		}

		assert.Equalf(t, fieldValue.Interface(), gFieldVal.Interface(),
			"fields under test should be deeply equal to what was created")
	}
}

func TestConfigFileVolumeSource_ToVolumeSource(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var in *ConfigFileVolumeSource = nil
		got := in.ToKubernetesVolumeSource()
		assert.Nil(t, got)
	})

	t.Run("zero-value receiver", func(t *testing.T) {
		in := &ConfigFileVolumeSource{}
		got := in.ToKubernetesVolumeSource()
		assert.Equal(t, v1.VolumeSource{}, *got)
	})

	for _, field := range reflect.VisibleFields(reflect.TypeOf(ConfigFileVolumeSource{})) {
		// for each struct field of ConfigFileVolumeSource, create a new CFVS with that field filled
		// in with some non-nil value to test ToVolumeSource() with. Then ensure that every
		// possible volume type of the CFVS converts to k8s' corev1.VolumeSource successfully
		in := &ConfigFileVolumeSource{}

		// use reflection to set the field under test with a non-nil created object
		vIn := reflect.ValueOf(in).Elem()
		fIn := vIn.FieldByName(field.Name)
		baseType := field.Type.Elem()
		fVal := reflect.New(baseType)
		fIn.Set(fVal)

		t.Run(fmt.Sprintf("%s: %s{}", field.Name, field.Type), func(t *testing.T) {
			// test with zero object
			validateToVolumeSource(t, field.Name, fVal, in)
		})

		t.Run(fmt.Sprintf("%s: %s{<some-data>}", field.Name, field.Type), func(t *testing.T) {
			// set some data set on the object
			setSomeFields(field.Type.Elem(), fVal.Elem())
			fIn.Set(fVal)

			validateToVolumeSource(t, field.Name, fVal, in)
		})
	}
}

func setSomeFields(t reflect.Type, v reflect.Value) {
	for _, f := range reflect.VisibleFields(t) {
		fVal := v.FieldByName(f.Name)
		setSomeData(fVal)
	}
}

func setSomeData(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		setSomeData(v.Elem())
	case reflect.String:
		v.SetString("string-data")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(0o755)
	}
}
