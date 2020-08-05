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

package v1

import (
	"testing"

	v1 "github.com/rook/rook/pkg/apis/rook.io/v1"
)

func Test_validateUpdatedCephCluster(t *testing.T) {
	type args struct {
		updatedCephCluster *CephCluster
		found              *CephCluster
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"everything is ok", args{&CephCluster{}, &CephCluster{}}, false},
		{"changed DataDirHostPath", args{&CephCluster{Spec: ClusterSpec{DataDirHostPath: "foo"}}, &CephCluster{Spec: ClusterSpec{DataDirHostPath: "bar"}}}, true},
		{"changed HostNetwork", args{&CephCluster{Spec: ClusterSpec{Network: NetworkSpec{HostNetwork: false}}}, &CephCluster{Spec: ClusterSpec{Network: NetworkSpec{HostNetwork: true}}}}, true},
		{"changed storageClassDeviceSet encryption", args{&CephCluster{Spec: ClusterSpec{Storage: v1.StorageScopeSpec{StorageClassDeviceSets: []v1.StorageClassDeviceSet{{Name: "foo", Encrypted: false}}}}}, &CephCluster{Spec: ClusterSpec{Storage: v1.StorageScopeSpec{StorageClassDeviceSets: []v1.StorageClassDeviceSet{{Name: "foo", Encrypted: true}}}}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateUpdatedCephCluster(tt.args.updatedCephCluster, tt.args.found); (err != nil) != tt.wantErr {
				t.Errorf("validateUpdatedCephCluster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
