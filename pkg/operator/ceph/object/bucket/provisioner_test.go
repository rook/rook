/*
Copyright 2020 The Kubernetes Authors.

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

package bucket

import (
	"context"
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPopulateDomainAndPort(t *testing.T) {
	ctx := context.TODO()
	store := "test-store"
	namespace := "ns"
	clusterInfo := client.AdminTestClusterInfo(namespace)
	p := NewProvisioner(&clusterd.Context{RookClientset: rookclient.NewSimpleClientset(), Clientset: test.New(t, 1)}, clusterInfo)
	p.objectContext = object.NewContext(p.context, clusterInfo, store)
	sc := &storagev1.StorageClass{
		Parameters: map[string]string{
			"foo": "bar",
		},
	}

	// No endpoint and no CephObjectStore
	err := p.populateDomainAndPort(sc)
	assert.Error(t, err)

	// Endpoint is set but port is missing
	sc.Parameters["endpoint"] = "192.168.0.1"
	err = p.populateDomainAndPort(sc)
	assert.Error(t, err)

	// Endpoint is set but IP is missing
	sc.Parameters["endpoint"] = ":80"
	err = p.populateDomainAndPort(sc)
	assert.Error(t, err)

	// Endpoint is correct
	sc.Parameters["endpoint"] = "192.168.0.1:80"
	err = p.populateDomainAndPort(sc)
	assert.NoError(t, err)
	assert.Equal(t, "192.168.0.1", p.storeDomainName)
	assert.Equal(t, int32(80), p.storePort)

	// No endpoint but a CephObjectStore
	sc.Parameters["endpoint"] = ""
	sc.Parameters["objectStoreNamespace"] = namespace
	sc.Parameters["objectStoreName"] = store
	cephObjectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      store,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectStore"},
		Spec: cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{
				Port: int32(80),
			},
		},
	}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", object.AppName, store),
			Namespace: namespace,
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "192.168.0.1",
			Ports:     []v1.ServicePort{{Name: "port", Port: int32(80)}},
		},
	}

	_, err = p.context.RookClientset.CephV1().CephObjectStores(namespace).Create(ctx, cephObjectStore, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = p.context.Clientset.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
	assert.NoError(t, err)
	p.objectStoreName = store
	err = p.populateDomainAndPort(sc)
	assert.NoError(t, err)
	assert.Equal(t, "rook-ceph-rgw-test-store.ns.svc", p.storeDomainName)
}

func TestMaxSizeToInt64(t *testing.T) {
	type args struct {
		maxSize string
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr bool
	}{
		{"invalid size", args{maxSize: "foo"}, 0, true},
		{"2gb size is invalid", args{maxSize: "2g"}, 0, true},
		{"2G size is valid", args{maxSize: "2G"}, 2000000000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := maxSizeToInt64(tt.args.maxSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("maxSizeToInt64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("maxSizeToInt64() = %v, want %v", got, tt.want)
			}
		})
	}
}
