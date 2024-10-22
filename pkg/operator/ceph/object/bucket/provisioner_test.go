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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/ceph/go-ceph/rgw/admin"
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

const (
	userPath   = "rgw.test/admin/user"
	bucketPath = "rgw.test/admin/bucket"
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

func TestQuanityToInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *int64
		wantErr bool
	}{
		{"foo is invalid", "foo", nil, true},
		{"2gb size is invalid", "2g", nil, true},
		{"2G size is valid", "2G", &(&struct{ i int64 }{2000000000}).i, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := quanityToInt64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("quanityToInt64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil && tt.want != nil && *got != *tt.want {
				t.Errorf("quanityToInt64() = %v, want %v", *got, *tt.want)
			} else if got != nil && tt.want == nil {
				t.Errorf("quanityToInt64() = %v, want %v", *got, tt.want)
			} else if got == nil && tt.want != nil {
				t.Errorf("quanityToInt64() = %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestProvisioner_setUserQuota(t *testing.T) {
	newProvisioner := func(t *testing.T, getResult *map[string]string, getSeen, putSeen *map[string][]string) *Provisioner {
		mockClient := &object.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path

				// t.Logf("HTTP req: %#v", req)
				t.Logf("HTTP %s: %s %s", req.Method, path, req.URL.RawQuery)

				statusCode := 200
				if _, ok := (*getResult)[path]; !ok {
					// path not configured
					statusCode = 500
				}
				responseBody := []byte(`[]`)

				switch method := req.Method; method {
				case http.MethodGet:
					(*getSeen)[path] = append((*getSeen)[path], req.URL.RawQuery)
					responseBody = []byte((*getResult)[path])
				case http.MethodPut:
					if (*putSeen)[path] == nil {
						(*putSeen)[path] = []string{}
					}
					(*putSeen)[path] = append((*putSeen)[path], req.URL.RawQuery)
				default:
					panic(fmt.Sprintf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, path))
				}

				return &http.Response{
					StatusCode: statusCode,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}, nil
			},
		}

		adminClient, err := admin.New("rgw.test", "accesskey", "secretkey", mockClient)
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

	t.Run("user quota should remain disabled", func(t *testing.T) {
		getResult := map[string]string{
			userPath: `{"enabled":false,"check_on_raw":false,"max_size":-1024,"max_size_kb":0,"max_objects":-1}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setUserQuota(&additionalConfigSpec{})
		assert.NoError(t, err)

		assert.Len(t, getSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("uid=bob", getSeen[userPath]), 1)

		assert.Len(t, putSeen[userPath], 0) // user quota should not be touched
	})

	t.Run("user quota should be disabled", func(t *testing.T) {
		getResult := map[string]string{
			userPath: `{"enabled":true,"check_on_raw":false,"max_size":-1024,"max_size_kb":0,"max_objects":2}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setUserQuota(&additionalConfigSpec{})
		assert.NoError(t, err)

		assert.Len(t, getSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("uid=bob", getSeen[userPath]), 1)

		assert.Len(t, putSeen[userPath], 1)
		assert.Equal(t, 1, numberOfCallsWithValue("enabled=false", putSeen[userPath]))
	})

	t.Run("user maxSize quota should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			userPath: `{"enabled":false,"check_on_raw":false,"max_size":-1024,"max_size_kb":0,"max_objects":-1}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setUserQuota(&additionalConfigSpec{
			maxSize: aws.Int64(2),
		})
		assert.NoError(t, err)

		assert.Len(t, getSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("uid=bob", getSeen[userPath]), 1)

		assert.Len(t, putSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("enabled=true", putSeen[userPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-size=2", putSeen[userPath]), 1)
	})

	t.Run("user maxObjects quota should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			userPath: `{"enabled":false,"check_on_raw":false,"max_size":-1024,"max_size_kb":0,"max_objects":-1}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setUserQuota(&additionalConfigSpec{
			maxObjects: aws.Int64(2),
		})
		assert.NoError(t, err)

		assert.Len(t, getSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("uid=bob", getSeen[userPath]), 1)

		assert.Len(t, putSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("enabled=true", putSeen[userPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-objects=2", putSeen[userPath]), 1)
	})

	t.Run("user maxObjects and maxSize quotas should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			userPath: `{"enabled":false,"check_on_raw":false,"max_size":-1024,"max_size_kb":0,"max_objects":-1}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setUserQuota(&additionalConfigSpec{
			maxObjects: aws.Int64(2),
			maxSize:    aws.Int64(3),
		})
		assert.NoError(t, err)

		assert.Len(t, getSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("uid=bob", getSeen[userPath]), 1)

		assert.Len(t, putSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("enabled=true", putSeen[userPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-objects=2", putSeen[userPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-size=3", putSeen[userPath]), 1)
	})

	t.Run("user quotas are enabled and need updated enabled", func(t *testing.T) {
		getResult := map[string]string{
			userPath: `{"enabled":true,"check_on_raw":false,"max_size":1,"max_size_kb":0,"max_objects":1}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setUserQuota(&additionalConfigSpec{
			maxObjects: aws.Int64(12),
			maxSize:    aws.Int64(13),
		})
		assert.NoError(t, err)

		assert.Len(t, getSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("uid=bob", getSeen[userPath]), 1)

		assert.Len(t, putSeen[userPath], 1)
		assert.Equal(t, numberOfCallsWithValue("enabled=true", putSeen[userPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-objects=12", putSeen[userPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-size=13", putSeen[userPath]), 1)
	})
}

func TestProvisioner_setBucketQuota(t *testing.T) {
	newProvisioner := func(t *testing.T, getResult *map[string]string, getSeen, putSeen *map[string][]string) *Provisioner {
		mockClient := &object.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path

				// t.Logf("HTTP req: %#v", req)
				t.Logf("HTTP %s: %s %s", req.Method, path, req.URL.RawQuery)

				statusCode := 200
				if _, ok := (*getResult)[path]; !ok {
					// path not configured
					statusCode = 500
				}
				responseBody := []byte(`[]`)

				switch method := req.Method; method {
				case http.MethodGet:
					(*getSeen)[path] = append((*getSeen)[path], req.URL.RawQuery)
					responseBody = []byte((*getResult)[path])
				case http.MethodPut:
					if (*putSeen)[path] == nil {
						(*putSeen)[path] = []string{}
					}
					(*putSeen)[path] = append((*putSeen)[path], req.URL.RawQuery)
				default:
					panic(fmt.Sprintf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, path))
				}

				return &http.Response{
					StatusCode: statusCode,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}, nil
			},
		}

		adminClient, err := admin.New("rgw.test", "accesskey", "secretkey", mockClient)
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

	t.Run("bucket quota should remain disabled", func(t *testing.T) {
		getResult := map[string]string{
			bucketPath: `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1024,"max_size_kb":0,"max_objects":-1}}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setBucketQuota(&additionalConfigSpec{})
		assert.NoError(t, err)

		assert.Len(t, getSeen[bucketPath], 1)
		assert.Equal(t, numberOfCallsWithValue("bucket=bob", getSeen[bucketPath]), 1)

		assert.Len(t, putSeen[bucketPath], 0) // bucket quota should not be touched
	})

	t.Run("bucket quota should be disabled", func(t *testing.T) {
		getResult := map[string]string{
			bucketPath: `{"owner": "bob","bucket_quota":{"enabled":true,"check_on_raw":false,"max_size":-1024,"max_size_kb":0,"max_objects":3}}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setBucketQuota(&additionalConfigSpec{})
		assert.NoError(t, err)

		assert.Len(t, getSeen[bucketPath], 1)
		assert.Equal(t, numberOfCallsWithValue("bucket=bob", getSeen[bucketPath]), 1)

		assert.Len(t, putSeen[bucketPath], 1)
		assert.Equal(t, 1, numberOfCallsWithValue("enabled=false", putSeen[bucketPath]))
	})

	t.Run("bucket maxObjects quota should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			bucketPath: `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":-1}}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setBucketQuota(&additionalConfigSpec{
			bucketMaxObjects: aws.Int64(4),
		})
		assert.NoError(t, err)

		assert.Len(t, getSeen[bucketPath], 1)
		assert.Equal(t, numberOfCallsWithValue("bucket=bob", getSeen[bucketPath]), 1)

		assert.Len(t, putSeen[bucketPath], 1)
		assert.Equal(t, numberOfCallsWithValue("enabled=true", putSeen[bucketPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-objects=4", putSeen[bucketPath]), 1)
	})

	t.Run("bucket maxSize quota should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			bucketPath: `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1024,"max_size_kb":0,"max_objects":-1}}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setBucketQuota(&additionalConfigSpec{
			bucketMaxSize: aws.Int64(5),
		})
		assert.NoError(t, err)

		assert.Len(t, getSeen[bucketPath], 1)
		assert.Equal(t, numberOfCallsWithValue("bucket=bob", getSeen[bucketPath]), 1)

		assert.Len(t, putSeen[bucketPath], 1)
		assert.Equal(t, numberOfCallsWithValue("enabled=true", putSeen[bucketPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-size=5", putSeen[bucketPath]), 1)
	})

	t.Run("bucket quotas are enabled and need updated enabled", func(t *testing.T) {
		getResult := map[string]string{
			bucketPath: `{"bucket_quota":{"enabled":true,"check_on_raw":false,"max_size":5,"max_size_kb":0,"max_objects":4}}`,
		}
		getSeen := map[string][]string{}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &getSeen, &putSeen)
		p.setBucketName("bob")

		err := p.setBucketQuota(&additionalConfigSpec{
			bucketMaxObjects: aws.Int64(14),
			bucketMaxSize:    aws.Int64(15),
		})
		assert.NoError(t, err)

		assert.Len(t, getSeen[bucketPath], 1)
		assert.Equal(t, numberOfCallsWithValue("bucket=bob", getSeen[bucketPath]), 1)

		assert.Len(t, putSeen[bucketPath], 1)
		assert.Equal(t, numberOfCallsWithValue("enabled=true", putSeen[bucketPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-objects=14", putSeen[bucketPath]), 1)
		assert.Equal(t, numberOfCallsWithValue("max-size=15", putSeen[bucketPath]), 1)
	})
}

func TestProvisioner_additionalConfigSpecFromMap(t *testing.T) {
	t.Run("does not fail on empty map", func(t *testing.T) {
		spec, err := additionalConfigSpecFromMap(map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{}, *spec)
	})

	t.Run("maxObjects field should be set", func(t *testing.T) {
		spec, err := additionalConfigSpecFromMap(map[string]string{"maxObjects": "2"})
		assert.NoError(t, err)
		var i int64 = 2
		assert.Equal(t, additionalConfigSpec{maxObjects: &i}, *spec)
	})

	t.Run("maxSize field should be set", func(t *testing.T) {
		spec, err := additionalConfigSpecFromMap(map[string]string{"maxSize": "3"})
		assert.NoError(t, err)
		var i int64 = 3
		assert.Equal(t, additionalConfigSpec{maxSize: &i}, *spec)
	})

	t.Run("bucketMaxObjects field should be set", func(t *testing.T) {
		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketMaxObjects": "4"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketMaxObjects: &(&struct{ i int64 }{4}).i}, *spec)
	})

	t.Run("bucketMaxSize field should be set", func(t *testing.T) {
		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketMaxSize": "5"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketMaxSize: &(&struct{ i int64 }{5}).i}, *spec)
	})
}

func numberOfCallsWithValue(substr string, strs []string) int {
	count := 0
	for _, s := range strs {
		if strings.Contains(s, substr) {
			count++
		}
	}
	return count
}
