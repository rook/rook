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

package mon

import "testing"

func TestWereMonEndpointsUpdated(t *testing.T) {
	type args struct {
		oldCMData map[string]string
		newCMData map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"no old mapping key", args{oldCMData: map[string]string{}, newCMData: map[string]string{"mapping": `{"node":{"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"}}}`}}, false},
		{"no new mapping key", args{oldCMData: map[string]string{"mapping": `{"node":{"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"}}}`}, newCMData: map[string]string{}}, false},
		{"identical content", args{oldCMData: map[string]string{"mapping": `{"node":{"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"}}}`}, newCMData: map[string]string{"mapping": `{"node":{"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"}}}`}}, false},
		{"identical content but different order", args{oldCMData: map[string]string{"mapping": `{"node":{"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"}}}`}, newCMData: map[string]string{"mapping": `{"node":{"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"}}}`}}, false},
		{"same length but different mons IP for 'i'", args{oldCMData: map[string]string{"mapping": `{"node":{"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"}}}`}, newCMData: map[string]string{"mapping": `{"node":{"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.187"}}}`}}, true},
		{"different length", args{oldCMData: map[string]string{"mapping": `{"node":{"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"}}}`}, newCMData: map[string]string{"mapping": `{"node":{"g":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"h":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.121"},"i":{"Name":"minikube","Hostname":"minikube","Address":"192.168.39.187"}}}`}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wereMonEndpointsUpdated(tt.args.oldCMData, tt.args.newCMData); got != tt.want {
				t.Errorf("whereMonEndpointsUpdated() = %v, want %v", got, tt.want)
			}
		})
	}
}
