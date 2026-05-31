/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package test

import (
	"reflect"
	"testing"

	"k8s.io/api/core/v1"
)

func TestGetEnv(t *testing.T) {
	type args struct {
		name string
		envs []v1.EnvVar
	}
	waldo := v1.EnvVar{Name: "waldo", Value: "peekaboo"}
	carmen := v1.EnvVar{Name: "carmen", Value: "sandiego"}
	batman := v1.EnvVar{Name: "batman", Value: "404: Not Found"}
	empty := v1.EnvVar{Name: "", Value: ""}
	tests := []struct {
		name    string
		args    args
		want    *v1.EnvVar
		wantErr bool
	}{
		{"env in 1-item list", args{"waldo", []v1.EnvVar{waldo}}, &waldo, false},
		{"env not in empty list", args{"carmen", []v1.EnvVar{}}, nil, true},
		{"env in 3-item list", args{"batman", []v1.EnvVar{waldo, carmen, batman}}, &batman, false},
		{"empty env not in list", args{"", []v1.EnvVar{waldo, carmen, batman}}, nil, true},
		{"empty env in list", args{"", []v1.EnvVar{waldo, carmen, batman, empty}}, &empty, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetEnv(tt.args.name, tt.args.envs)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Don't care about return value if it reports error
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
