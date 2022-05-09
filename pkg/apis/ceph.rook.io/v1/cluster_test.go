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

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		{"good mon count", args{&CephCluster{Spec: ClusterSpec{Mon: MonSpec{Count: 1}}}, &CephCluster{}}, false},
		{"even mon count", args{&CephCluster{Spec: ClusterSpec{Mon: MonSpec{Count: 2}}}, &CephCluster{}}, false},
		{"good mon count", args{&CephCluster{Spec: ClusterSpec{Mon: MonSpec{Count: 3}}}, &CephCluster{}}, false},
		{"changed DataDirHostPath", args{&CephCluster{Spec: ClusterSpec{DataDirHostPath: "foo"}}, &CephCluster{Spec: ClusterSpec{DataDirHostPath: "bar"}}}, true},
		{"changed HostNetwork", args{&CephCluster{Spec: ClusterSpec{Network: NetworkSpec{HostNetwork: false}}}, &CephCluster{Spec: ClusterSpec{Network: NetworkSpec{HostNetwork: true}}}}, true},
		{"changed storageClassDeviceSet encryption", args{&CephCluster{Spec: ClusterSpec{Storage: StorageScopeSpec{StorageClassDeviceSets: []StorageClassDeviceSet{{Name: "foo", Encrypted: false}}}}}, &CephCluster{Spec: ClusterSpec{Storage: StorageScopeSpec{StorageClassDeviceSets: []StorageClassDeviceSet{{Name: "foo", Encrypted: true}}}}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateUpdatedCephCluster(tt.args.updatedCephCluster, tt.args.found); (err != nil) != tt.wantErr {
				t.Errorf("validateUpdatedCephCluster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCephClusterValidateCreate(t *testing.T) {
	c := &CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph",
		},
		Spec: ClusterSpec{
			DataDirHostPath: "/var/lib/rook",
		},
	}
	err := c.ValidateCreate()
	assert.NoError(t, err)
	c.Spec.External.Enable = true
	c.Spec.Monitoring = MonitoringSpec{
		Enabled: true,
	}
	err = c.ValidateCreate()
	assert.Error(t, err)
}

func TestCephClusterValidateUpdate(t *testing.T) {
	c := &CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph",
		},
		Spec: ClusterSpec{
			DataDirHostPath: "/var/lib/rook",
			Storage: StorageScopeSpec{
				StorageClassDeviceSets: []StorageClassDeviceSet{
					{
						Name:      "sc1",
						Encrypted: true,
					},
				},
			},
		},
	}
	err := c.ValidateCreate()
	assert.NoError(t, err)

	// Updating the CRD specs with invalid values
	uc := c.DeepCopy()
	uc.Spec.DataDirHostPath = "var/rook"
	uc.Spec.Storage.StorageClassDeviceSets[0].Encrypted = false
	err = uc.ValidateUpdate(c)
	assert.Error(t, err)

	// reverting the to older hostPath
	uc.Spec.DataDirHostPath = "/var/lib/rook"
	uc.Spec.Storage.StorageClassDeviceSets = []StorageClassDeviceSet{
		{
			Name:      "sc1",
			Encrypted: true,
		},
		{
			Name: "sc2",
		},
	}

	err = uc.ValidateUpdate(c)
	assert.NoError(t, err)
}
