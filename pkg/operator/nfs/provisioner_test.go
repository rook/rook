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

package nfs

import (
	"context"
	"os"
	"reflect"
	"testing"

	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	rookclientfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	k8sclientfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/v6/controller"
)

func init() {
	mountPath = "/tmp/test-rook-nfs"
}

func newDummyStorageClass(name string, nfsServerNamespacedName types.NamespacedName, reclaimPolicy corev1.PersistentVolumeReclaimPolicy) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Parameters: map[string]string{
			nfsServerNameSCParam:      nfsServerNamespacedName.Name,
			nfsServerNamespaceSCParam: nfsServerNamespacedName.Namespace,
			exportNameSCParam:         name,
		},
		ReclaimPolicy: &reclaimPolicy,
	}
}

func newDummyPVC(name, namespace string, capacity apiresource.Quantity, storageClassName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): capacity,
				},
			},
			StorageClassName: &storageClassName,
		},
	}
}

func newDummyPV(name, scName, expectedPath string, expectedCapacity apiresource.Quantity, expectedReclaimPolicy corev1.PersistentVolumeReclaimPolicy) *corev1.PersistentVolume {
	annotations := make(map[string]string)
	annotations[projectBlockAnnotationKey] = ""
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: expectedReclaimPolicy,
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): expectedCapacity,
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Path: expectedPath,
				},
			},
			StorageClassName: scName,
		},
	}
}

func TestProvisioner_Provision(t *testing.T) {
	ctx := context.TODO()
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		t.Error("error creating test provisioner directory")
	}

	defer os.RemoveAll(mountPath)

	fakeQuoater, err := NewFakeProjectQuota()
	if err != nil {
		t.Error(err)
	}

	nfsserver := newCustomResource(types.NamespacedName{Name: "test-nfsserver", Namespace: "test-nfsserver"}).WithExports("share-1", "ReadWrite", "none", "test-claim").Generate()

	type fields struct {
		client     kubernetes.Interface
		rookClient rookclient.Interface
		quoater    Quotaer
	}
	type args struct {
		options controller.ProvisionOptions
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *corev1.PersistentVolume
		wantErr bool
	}{
		{
			name: "success create volume",
			fields: fields{
				client: k8sclientfake.NewSimpleClientset(
					newServiceForNFSServer(nfsserver),
					newDummyStorageClass("share-1", types.NamespacedName{Name: nfsserver.Name, Namespace: nfsserver.Namespace}, corev1.PersistentVolumeReclaimDelete),
				),
				rookClient: rookclientfake.NewSimpleClientset(
					nfsserver,
				),
				quoater: fakeQuoater,
			},
			args: args{
				options: controller.ProvisionOptions{
					StorageClass: newDummyStorageClass("share-1", types.NamespacedName{Name: nfsserver.Name, Namespace: nfsserver.Namespace}, corev1.PersistentVolumeReclaimDelete),
					PVName:       "share-1-pvc",
					PVC:          newDummyPVC("share-1-pvc", "default", apiresource.MustParse("1Mi"), "share-1"),
				},
			},
			want: newDummyPV("share-1-pvc", "", "/tmp/test-rook-nfs/test-claim/default-share-1-pvc-share-1-pvc", apiresource.MustParse("1Mi"), corev1.PersistentVolumeReclaimDelete),
		},
		{
			name: "no matching export",
			fields: fields{
				client: k8sclientfake.NewSimpleClientset(
					newServiceForNFSServer(nfsserver),
					newDummyStorageClass("foo", types.NamespacedName{Name: nfsserver.Name, Namespace: nfsserver.Namespace}, corev1.PersistentVolumeReclaimDelete),
				),
				rookClient: rookclientfake.NewSimpleClientset(
					nfsserver,
				),
			},
			args: args{
				options: controller.ProvisionOptions{
					StorageClass: newDummyStorageClass("foo", types.NamespacedName{Name: nfsserver.Name, Namespace: nfsserver.Namespace}, corev1.PersistentVolumeReclaimDelete),
					PVName:       "share-1-pvc",
					PVC:          newDummyPVC("share-1-pvc", "default", apiresource.MustParse("1Mi"), "foo"),
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provisioner{
				client:     tt.fields.client,
				rookClient: tt.fields.rookClient,
				quotaer:    tt.fields.quoater,
			}
			got, _, err := p.Provision(ctx, tt.args.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("Provisioner.Provision() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Provisioner.Provision() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvisioner_Delete(t *testing.T) {
	ctx := context.TODO()
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		t.Error("error creating test provisioner directory")
	}

	defer os.RemoveAll(mountPath)

	fakeQuoater, err := NewFakeProjectQuota()
	if err != nil {
		t.Error(err)
	}

	nfsserver := newCustomResource(types.NamespacedName{Name: "test-nfsserver", Namespace: "test-nfsserver"}).WithExports("share-1", "ReadWrite", "none", "test-claim").Generate()
	type fields struct {
		client     kubernetes.Interface
		rookClient rookclient.Interface
		quoater    Quotaer
	}
	type args struct {
		volume *corev1.PersistentVolume
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "success delete volume",
			fields: fields{
				client: k8sclientfake.NewSimpleClientset(
					newServiceForNFSServer(nfsserver),
					newDummyStorageClass("share-1", types.NamespacedName{Name: nfsserver.Name, Namespace: nfsserver.Namespace}, corev1.PersistentVolumeReclaimDelete),
				),
				rookClient: rookclientfake.NewSimpleClientset(
					nfsserver,
				),
				quoater: fakeQuoater,
			},
			args: args{
				volume: newDummyPV("share-1-pvc", "share-1", "/tmp/test-rook-nfs/test-claim/default-share-1-pvc-share-1-pvc", apiresource.MustParse("1Mi"), corev1.PersistentVolumeReclaimDelete),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provisioner{
				client:     tt.fields.client,
				rookClient: tt.fields.rookClient,
				quotaer:    tt.fields.quoater,
			}
			if err := p.Delete(ctx, tt.args.volume); (err != nil) != tt.wantErr {
				t.Errorf("Provisioner.Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
