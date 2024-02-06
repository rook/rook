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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	apibkt "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
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
			got, err := toInt64(tt.args.maxSize)
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

func TestProvisioner_setAdditionalSettings(t *testing.T) {
	newProvisioner := func(t *testing.T, getUserResult string, putValsSeen *[]string) *Provisioner {
		mockClient := &object.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				// t.Logf("HTTP req: %#v", req)
				t.Logf("HTTP %s: %s %s", req.Method, req.URL.Path, req.URL.RawQuery)

				assert.Contains(t, req.URL.RawQuery, "&uid=bob")

				if req.Method == http.MethodGet {
					if req.URL.Path == "my.endpoint.net/admin/user" {
						statusCode := 200
						if getUserResult == "" {
							statusCode = 500
						}
						return &http.Response{
							StatusCode: statusCode,
							Body:       io.NopCloser(bytes.NewReader([]byte(getUserResult))),
						}, nil
					}
				}
				if req.Method == http.MethodPut {
					if req.URL.Path == "my.endpoint.net/admin/user" {
						*putValsSeen = append(*putValsSeen, req.URL.RawQuery)
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewReader([]byte(`[]`))),
						}, nil
					}
				}
				panic(fmt.Sprintf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, req.URL.Path))
			},
		}

		adminClient, err := admin.New("my.endpoint.net", "accesskey", "secretkey", mockClient)
		assert.NoError(t, err)

		p := &Provisioner{
			clusterInfo: &client.ClusterInfo{
				Context: context.Background(),
			},
			cephUserName:   "bob",
			adminOpsClient: adminClient,
		}

		return p
	}

	t.Run("quota should remain disabled", func(t *testing.T) {
		putValsSeen := []string{}
		p := newProvisioner(t,
			`{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			&putValsSeen,
		)

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{},
				},
			},
		})
		assert.NoError(t, err)
		assert.Len(t, putValsSeen, 0)
	})

	t.Run("quota should be disabled", func(t *testing.T) {
		putValsSeen := []string{}
		p := newProvisioner(t,
			`{"user_quota":{"enabled":true,"max_size":-1,"max_objects":2}}`,
			&putValsSeen,
		)

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{},
				},
			},
		})
		assert.NoError(t, err)

		// quota should be disabled, and that's it
		assert.Len(t, putValsSeen, 1)
		assert.Equal(t, 1, numberOfPutsWithValue(`enabled=false`, putValsSeen))
	})

	t.Run("maxSize quota should be enabled", func(t *testing.T) {
		putValsSeen := []string{}
		p := newProvisioner(t,
			`{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			&putValsSeen,
		)

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{
						"maxSize": "2",
					},
				},
			},
		})
		assert.NoError(t, err)

		assert.Zero(t, numberOfPutsWithValue(`enabled=false`, putValsSeen)) // no puts should disable

		assert.NotEmpty(t, putWithValue(`enabled=true`, putValsSeen)) // at least one put enables

		// there is only one time that max-size is set, and it's to the right value
		assert.Equal(t, 1, numberOfPutsWithValue(`max-size`, putValsSeen))
		assert.NotEmpty(t, putWithValue(`max-size=2`, putValsSeen))

	})

	t.Run("maxObjects quota should be enabled", func(t *testing.T) {
		putValsSeen := []string{}
		p := newProvisioner(t,
			`{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			&putValsSeen,
		)

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{
						"maxObjects": "2",
					},
				},
			},
		})
		assert.NoError(t, err)

		assert.Zero(t, numberOfPutsWithValue(`enabled=false`, putValsSeen)) // no puts should disable

		assert.NotEmpty(t, putWithValue(`enabled=true`, putValsSeen)) // at least one put enables

		// there is only one time max-objects is set, and it's to the right value
		assert.NotEmpty(t, putWithValue(`max-objects=2`, putValsSeen))
		assert.Equal(t, 1, numberOfPutsWithValue(`max-objects`, putValsSeen))

	})

	t.Run("maxObjects and maxSize quotas should be enabled", func(t *testing.T) {
		putValsSeen := []string{}
		p := newProvisioner(t,
			`{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			&putValsSeen,
		)

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{
						"maxSize":    "2",
						"maxObjects": "3",
					},
				},
			},
		})
		assert.NoError(t, err)

		assert.Zero(t, numberOfPutsWithValue(`enabled=false`, putValsSeen)) // no puts should disable

		assert.NotEmpty(t, putWithValue(`enabled=true`, putValsSeen)) // at least one put enables

		// there is only one time that max-size is set, and it's to the right value
		assert.Equal(t, 1, numberOfPutsWithValue(`max-size`, putValsSeen))
		assert.NotEmpty(t, putWithValue(`max-size=2`, putValsSeen))

		// there is only one time max-objects is set, and it's to the right value
		assert.NotEmpty(t, putWithValue(`max-objects=3`, putValsSeen))
		assert.Equal(t, 1, numberOfPutsWithValue(`max-objects`, putValsSeen))
	})

	t.Run("quotas are enabled and need updated enabled", func(t *testing.T) {
		putValsSeen := []string{}
		p := newProvisioner(t,
			`{"user_quota":{"enabled":true,"max_size":1,"max_objects":1}}`,
			&putValsSeen,
		)

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{
						"maxSize":    "2",
						"maxObjects": "3",
					},
				},
			},
		})
		assert.NoError(t, err)

		assert.Zero(t, numberOfPutsWithValue(`enabled=false`, putValsSeen)) // no puts should disable

		// there is only one time that max-size is set, and it's to the right value
		assert.Equal(t, 1, numberOfPutsWithValue(`max-size`, putValsSeen))
		assert.NotEmpty(t, putWithValue(`max-size=2`, putValsSeen))

		// there is only one time max-objects is set, and it's to the right value
		assert.NotEmpty(t, putWithValue(`max-objects=3`, putValsSeen))
		assert.Equal(t, 1, numberOfPutsWithValue(`max-objects`, putValsSeen))
	})
}

func numberOfPutsWithValue(substr string, strs []string) int {
	count := 0
	for _, s := range strs {
		if strings.Contains(s, substr) {
			count++
		}
	}
	return count
}

func putWithValue(substr string, strs []string) string {
	for _, s := range strs {
		if strings.Contains(s, substr) {
			return s
		}
	}
	return ""
}
