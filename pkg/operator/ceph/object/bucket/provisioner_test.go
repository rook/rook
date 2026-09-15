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
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ceph/go-ceph/rgw/admin"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	apibkt "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
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
	p := NewProvisioner(&clusterd.Context{RookClientset: rookclient.NewSimpleClientset(), Clientset: test.New(t, 1)}, clusterInfo, nil)
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
			Kind: "CephObjectStore",
		},
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

		clusterInfo := &client.ClusterInfo{
			Context: context.Background(),
		}
		p := &Provisioner{
			clusterInfo:    clusterInfo,
			cephUserName:   "bob",
			adminOpsClient: adminClient,
			objectContext:  object.NewContext(&clusterd.Context{}, clusterInfo, "store"),
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{}}
		err := p.setUserQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{}}
		err := p.setUserQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{
			maxSize: aws.Int64(2),
		}}
		err := p.setUserQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{
			maxObjects: aws.Int64(2),
		}}
		err := p.setUserQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{
			maxObjects: aws.Int64(2),
			maxSize:    aws.Int64(3),
		}}
		err := p.setUserQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{
			maxObjects: aws.Int64(12),
			maxSize:    aws.Int64(13),
		}}
		err := p.setUserQuota(bucket)
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

		clusterInfo := &client.ClusterInfo{
			Context: context.Background(),
		}
		p := &Provisioner{
			clusterInfo:    clusterInfo,
			cephUserName:   "bob",
			adminOpsClient: adminClient,
			objectContext:  object.NewContext(&clusterd.Context{}, clusterInfo, "store"),
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{}}
		err := p.setBucketQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{}}
		err := p.setBucketQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{
			bucketMaxObjects: aws.Int64(4),
		}}
		err := p.setBucketQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{
			bucketMaxSize: aws.Int64(5),
		}}
		err := p.setBucketQuota(bucket)
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

		bucket := &bucket{additionalConfig: &additionalConfigSpec{
			bucketMaxObjects: aws.Int64(14),
			bucketMaxSize:    aws.Int64(15),
		}}
		err := p.setBucketQuota(bucket)
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
		opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"maxObjects": "2"})
		assert.NoError(t, err)
		var i int64 = 2
		assert.Equal(t, additionalConfigSpec{maxObjects: &i}, *spec)
	})

	t.Run("maxSize field should be set", func(t *testing.T) {
		opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"maxSize": "3"})
		assert.NoError(t, err)
		var i int64 = 3
		assert.Equal(t, additionalConfigSpec{maxSize: &i}, *spec)
	})

	t.Run("bucketMaxObjects field should be set", func(t *testing.T) {
		os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketMaxObjects")
		defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
		opcontroller.SetObcAllowAdditionalConfigFields()
		defer opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketMaxObjects": "4"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketMaxObjects: &(&struct{ i int64 }{4}).i}, *spec)
	})

	t.Run("bucketMaxSize field should be set", func(t *testing.T) {
		os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketMaxSize")
		defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
		opcontroller.SetObcAllowAdditionalConfigFields()
		defer opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketMaxSize": "5"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketMaxSize: &(&struct{ i int64 }{5}).i}, *spec)
	})

	t.Run("bucketPolicy field should be set", func(t *testing.T) {
		os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketPolicy")
		defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
		opcontroller.SetObcAllowAdditionalConfigFields()
		defer opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketPolicy": "foo"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketPolicy: &(&struct{ s string }{"foo"}).s}, *spec)
	})

	t.Run("bucketLifecycle field should be set", func(t *testing.T) {
		os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketLifecycle")
		defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
		opcontroller.SetObcAllowAdditionalConfigFields()
		defer opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketLifecycle": "foo"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketLifecycle: &(&struct{ s string }{"foo"}).s}, *spec)
	})

	t.Run("bucketOwner field should be set", func(t *testing.T) {
		os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketOwner")
		defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
		opcontroller.SetObcAllowAdditionalConfigFields()
		defer opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketOwner": "foo"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketOwner: &(&struct{ s string }{"foo"}).s}, *spec)
	})

	t.Run("bucketPlacement field should be set", func(t *testing.T) {
		os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketPlacement")
		defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
		opcontroller.SetObcAllowAdditionalConfigFields()
		defer opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketPlacement": "archive"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketPlacement: "archive"}, *spec)
	})

	t.Run("bucketStorageClass field should be set", func(t *testing.T) {
		os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketStorageClass")
		defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
		opcontroller.SetObcAllowAdditionalConfigFields()
		defer opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{"bucketStorageClass": "FOO"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{bucketStorageClass: "FOO"}, *spec)
	})

	t.Run("malformed placement values are rejected at parse time", func(t *testing.T) {
		os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketPlacement,bucketStorageClass")
		defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
		opcontroller.SetObcAllowAdditionalConfigFields()
		defer opcontroller.SetObcAllowAdditionalConfigFields()

		_, err := additionalConfigSpecFromMap(map[string]string{"bucketPlacement": "foo:bar"})
		require.ErrorIs(t, err, errInvalidPlacementValue)
		assert.Contains(t, err.Error(), `bucketPlacement "foo:bar"`)

		_, err = additionalConfigSpecFromMap(map[string]string{"bucketStorageClass": "FOO "})
		require.ErrorIs(t, err, errInvalidPlacementValue)
		assert.Contains(t, err.Error(), `bucketStorageClass "FOO "`)
	})

	t.Run("fields disallowed by default", func(t *testing.T) {
		opcontroller.SetObcAllowAdditionalConfigFields()

		for _, configKey := range []string{"bucketMaxObjects", "bucketMaxSize", "bucketPolicy", "bucketLifecycle", "bucketOwner", "bucketPlacement", "bucketStorageClass"} {
			_, err := additionalConfigSpecFromMap(map[string]string{configKey: "foo"})
			assert.Error(t, err)
		}
	})

	t.Run("does not fail on empty map", func(t *testing.T) {
		opcontroller.SetObcAllowAdditionalConfigFields()

		spec, err := additionalConfigSpecFromMap(map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{}, *spec)
	})
}

func TestValidatePlacementValue(t *testing.T) {
	tests := []struct {
		value   string
		wantErr bool
	}{
		{"", false},
		{"archive", false},
		{"a.b_c-d", false},
		{"loc-a", false},
		{"STANDARD_IA", false},
		{"foo:bar", true},
		{"a/b", true},
		{"loc a", true},
		{"loc-a/COLD", true},
		{"FOO ", true},
		{"\tCOLD", true},
	}
	for _, key := range []string{bucketPlacementKey, bucketStorageClassKey} {
		for _, tt := range tests {
			t.Run(key+" "+tt.value, func(t *testing.T) {
				err := validatePlacementValue(key, tt.value)
				if tt.wantErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), fmt.Sprintf("%s %q", key, tt.value))
					return
				}
				assert.NoError(t, err)
			})
		}
	}
}

func TestParsePlacementRule(t *testing.T) {
	tests := []struct {
		rule, placement, storageClass string
	}{
		{"loc-a", "loc-a", "STANDARD"},
		{"loc-a/STANDARD", "loc-a", "STANDARD"},
		{"loc-a/COLD", "loc-a", "COLD"},
		{"default/FOO", "default", "FOO"},
		{"loc-a/", "loc-a", "STANDARD"},
		{"a/b/COLD", "a", "b/COLD"},
		{"", "", "STANDARD"},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			placement, storageClass := parsePlacementRule(tt.rule)
			assert.Equal(t, tt.placement, placement)
			assert.Equal(t, tt.storageClass, storageClass)
		})
	}
}

func TestCheckPlacementRule(t *testing.T) {
	tests := []struct {
		name               string
		rule               string
		placement          string
		storageClass       string
		wantErr            bool
		wantErrContains    []string
		wantErrNotContains []string
	}{
		{name: "neither requested", rule: "loc-a/COLD"},
		{name: "placement matches", rule: "loc-a", placement: "loc-a"},
		{name: "placement matches with an explicit class", rule: "loc-a/COLD", placement: "loc-a"},
		{name: "storage class matches", rule: "loc-a/COLD", storageClass: "COLD"},
		{name: "STANDARD matches an absent class", rule: "loc-a", storageClass: "STANDARD"},
		{name: "STANDARD matches an explicit STANDARD", rule: "loc-a/STANDARD", storageClass: "STANDARD"},
		{name: "both match", rule: "loc-a/COLD", placement: "loc-a", storageClass: "COLD"},
		{
			name: "placement mismatch", rule: "default", placement: "loc-a", wantErr: true,
			wantErrContains:    []string{`bucketPlacement "loc-a"`, `placement "default"`},
			wantErrNotContains: []string{"bucketStorageClass"},
		},
		{
			name: "storage class mismatch against an absent class", rule: "loc-a", storageClass: "COLD", wantErr: true,
			wantErrContains:    []string{`bucketStorageClass "COLD"`, `storage class "STANDARD"`},
			wantErrNotContains: []string{"bucketPlacement"},
		},
		{
			name: "storage class mismatch against an explicit class", rule: "loc-a/COLD", storageClass: "FOO", wantErr: true,
			wantErrContains: []string{`bucketStorageClass "FOO"`, `storage class "COLD"`},
		},
		{
			name: "placement mismatch with a matching class", rule: "default/COLD", placement: "loc-a", storageClass: "COLD", wantErr: true,
			wantErrContains:    []string{`bucketPlacement "loc-a"`},
			wantErrNotContains: []string{"bucketStorageClass"},
		},
		{
			name: "both mismatch", rule: "default", placement: "loc-a", storageClass: "COLD", wantErr: true,
			wantErrContains: []string{`bucketPlacement "loc-a"`, `placement "default"`, `bucketStorageClass "COLD"`, `storage class "STANDARD"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPlacementRule(tt.rule, tt.placement, tt.storageClass)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tt.wantErrContains {
				assert.Contains(t, err.Error(), want)
			}
			for _, unwanted := range tt.wantErrNotContains {
				assert.NotContains(t, err.Error(), unwanted)
			}
		})
	}
}

// newPlacementBucket builds the per-call state the placement checks read:
// an OBC for the Event and the parsed additionalConfig.
func newPlacementBucket(placement, storageClass string) *bucket {
	return &bucket{
		options: &apibkt.BucketOptions{
			ObjectBucketClaim: &bktv1alpha1.ObjectBucketClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "my-obc", Namespace: "my-ns"},
			},
		},
		additionalConfig: &additionalConfigSpec{bucketPlacement: placement, bucketStorageClass: storageClass},
	}
}

func TestProvisioner_parseAdditionalConfig(t *testing.T) {
	os.Setenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS", "bucketPlacement,bucketStorageClass")
	defer os.Unsetenv("ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS")
	opcontroller.SetObcAllowAdditionalConfigFields()
	defer opcontroller.SetObcAllowAdditionalConfigFields()

	newOBC := func(additionalConfig map[string]string) *bktv1alpha1.ObjectBucketClaim {
		return &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "my-obc", Namespace: "my-ns"},
			Spec:       bktv1alpha1.ObjectBucketClaimSpec{AdditionalConfig: additionalConfig},
		}
	}

	t.Run("valid values record no event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := &Provisioner{recorder: recorder}
		spec, err := p.parseAdditionalConfig(newOBC(map[string]string{"bucketPlacement": "loc-a", "bucketStorageClass": "COLD"}), actionProvision)
		require.NoError(t, err)
		assert.Equal(t, "loc-a", spec.bucketPlacement)
		assert.Equal(t, "COLD", spec.bucketStorageClass)
		assert.Empty(t, recorder.Events)
	})

	t.Run("malformed placement fails and records an InvalidBucketPlacement event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := &Provisioner{recorder: recorder}
		_, err := p.parseAdditionalConfig(newOBC(map[string]string{"bucketPlacement": "foo:bar"}), actionGrant)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `bucketPlacement "foo:bar"`)

		require.Len(t, recorder.Events, 1)
		event := <-recorder.Events
		assert.Contains(t, event, "Warning "+EventReasonInvalidBucketPlacement+" ")
		assert.Contains(t, event, `bucketPlacement "foo:bar"`)
	})

	t.Run("malformed storage class fails and records an InvalidBucketPlacement event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := &Provisioner{recorder: recorder}
		_, err := p.parseAdditionalConfig(newOBC(map[string]string{"bucketStorageClass": "FOO "}), actionProvision)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `bucketStorageClass "FOO "`)
		assert.Len(t, recorder.Events, 1)
	})

	t.Run("a key the allowlist rejects records no event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := &Provisioner{recorder: recorder}
		_, err := p.parseAdditionalConfig(newOBC(map[string]string{"bucketPolicy": "{}"}), actionProvision)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not allowed")
		assert.Empty(t, recorder.Events)
	})

	t.Run("nil recorder is safe", func(t *testing.T) {
		p := &Provisioner{}
		_, err := p.parseAdditionalConfig(newOBC(map[string]string{"bucketPlacement": "a/b"}), actionProvision)
		assert.Error(t, err)
	})
}

func TestProvisioner_checkExistingBucketPlacement(t *testing.T) {
	info := &admin.Bucket{Bucket: "bkt", PlacementRule: "default"}

	t.Run("unset keys are not compared", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := &Provisioner{bucketName: "bkt", recorder: recorder}
		assert.NoError(t, p.checkExistingBucketPlacement(newPlacementBucket("", ""), info, actionProvision))
		assert.Empty(t, recorder.Events)
	})

	t.Run("matching request records no event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := &Provisioner{bucketName: "bkt", recorder: recorder}
		assert.NoError(t, p.checkExistingBucketPlacement(newPlacementBucket("default", "STANDARD"), info, actionGrant))
		assert.Empty(t, recorder.Events)
	})

	t.Run("mismatch fails and records a BucketPlacementMismatch event naming both values", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := &Provisioner{bucketName: "bkt", recorder: recorder}
		err := p.checkExistingBucketPlacement(newPlacementBucket("loc-a", "COLD"), info, actionGrant)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `bucket "bkt" already exists and its placement cannot be changed`)
		assert.Contains(t, err.Error(), `bucketPlacement "loc-a" was requested but the bucket is on placement "default"`)
		assert.Contains(t, err.Error(), `bucketStorageClass "COLD" was requested but the bucket has storage class "STANDARD"`)

		require.Len(t, recorder.Events, 1)
		event := <-recorder.Events
		assert.Contains(t, event, "Warning "+EventReasonBucketPlacementMismatch+" ")
		assert.Contains(t, event, err.Error())
	})

	t.Run("nil recorder is safe", func(t *testing.T) {
		p := &Provisioner{bucketName: "bkt"}
		assert.Error(t, p.checkExistingBucketPlacement(newPlacementBucket("loc-a", ""), info, actionProvision))
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
