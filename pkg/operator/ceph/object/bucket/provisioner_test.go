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

func TestProvisioner_setAdditionalSettings(t *testing.T) {
	newProvisioner := func(t *testing.T, getResult *map[string]string, putSeen *map[string][]string) *Provisioner {
		mockClient := &object.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				// t.Logf("HTTP req: %#v", req)
				t.Logf("HTTP %s: %s %s", req.Method, req.URL.Path, req.URL.RawQuery)

				{
					endpoint := "rgw.test/admin/user"

					if req.Method == http.MethodGet {
						if req.URL.Path == endpoint {
							assert.Contains(t, req.URL.RawQuery, "&uid=bob")

							statusCode := 200
							if (*getResult)[endpoint] == "" {
								statusCode = 500
							}
							return &http.Response{
								StatusCode: statusCode,
								Body:       io.NopCloser(bytes.NewReader([]byte((*getResult)[endpoint]))),
							}, nil
						}
					}
					if req.Method == http.MethodPut {
						if req.URL.Path == endpoint {
							assert.Contains(t, req.URL.RawQuery, "&uid=bob")

							if _, ok := (*putSeen)[endpoint]; !ok {
								(*putSeen)[endpoint] = []string{}
							}
							(*putSeen)[endpoint] = append((*putSeen)[endpoint], req.URL.RawQuery)
							return &http.Response{
								StatusCode: 200,
								Body:       io.NopCloser(bytes.NewReader([]byte(`[]`))),
							}, nil
						}
					}
				}

				{
					endpoint := "rgw.test/admin/bucket"

					if req.Method == http.MethodGet {
						if req.URL.Path == endpoint {
							statusCode := 200
							if (*getResult)[endpoint] == "" {
								statusCode = 500
							}
							return &http.Response{
								StatusCode: statusCode,
								Body:       io.NopCloser(bytes.NewReader([]byte((*getResult)[endpoint]))),
							}, nil
						}
					}
					if req.Method == http.MethodPut {
						if req.URL.Path == endpoint {
							if _, ok := (*putSeen)[endpoint]; !ok {
								(*putSeen)[endpoint] = []string{}
							}
							(*putSeen)[endpoint] = append((*putSeen)[endpoint], req.URL.RawQuery)
							return &http.Response{
								StatusCode: 200,
								Body:       io.NopCloser(bytes.NewReader([]byte(`[]`))),
							}, nil
						}
					}
				}

				panic(fmt.Sprintf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, req.URL.Path))
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

	t.Run("user and bucket quota should remain disabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			"rgw.test/admin/bucket": `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":-1}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{},
				},
			},
		})
		assert.NoError(t, err)
		assert.Len(t, putSeen["rgw.test/admin/user"], 0)
		assert.Len(t, putSeen["rgw.test/admin/bucket"], 0)
	})

	t.Run("user and bucket quota should be disabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":true,"max_size":-1,"max_objects":2}}`,
			"rgw.test/admin/bucket": `{"owner": "bob","bucket_quota":{"enabled":true,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":3}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)
		p.setBucketName("bob")

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{},
				},
			},
		})
		assert.NoError(t, err)

		// user quota should be disabled, and that's it
		userEndSeen := putSeen["rgw.test/admin/user"]
		assert.Len(t, userEndSeen, 1)
		assert.Equal(t, 1, numberOfPutsWithValue(`enabled=false`, userEndSeen))
		// bucket quota should be disabled, and that's it
		bucketEndSeen := putSeen["rgw.test/admin/bucket"]
		assert.Len(t, bucketEndSeen, 1)
		assert.Equal(t, 1, numberOfPutsWithValue(`enabled=false`, bucketEndSeen))
	})

	t.Run("user maxSize quota should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			"rgw.test/admin/bucket": `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":-1}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)

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

		userEndSeen := putSeen["rgw.test/admin/user"]
		assert.Zero(t, numberOfPutsWithValue(`enabled=false`, userEndSeen)) // no puts should disable

		assert.NotEmpty(t, putWithValue(`enabled=true`, userEndSeen)) // at least one put enables

		// there is only one time that max-size is set, and it's to the right value
		assert.Equal(t, 1, numberOfPutsWithValue(`max-size`, userEndSeen))
		assert.NotEmpty(t, putWithValue(`max-size=2`, userEndSeen))
	})

	t.Run("user maxObjects quota should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			"rgw.test/admin/bucket": `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":-1}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)

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

		userEndSeen := putSeen["rgw.test/admin/user"]
		assert.Zero(t, numberOfPutsWithValue(`enabled=false`, userEndSeen)) // no puts should disable

		assert.NotEmpty(t, putWithValue(`enabled=true`, userEndSeen)) // at least one put enables

		// there is only one time max-objects is set, and it's to the right value
		assert.NotEmpty(t, putWithValue(`max-objects=2`, userEndSeen))
		assert.Equal(t, 1, numberOfPutsWithValue(`max-objects`, userEndSeen))
	})

	t.Run("user maxObjects and maxSize quotas should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			"rgw.test/admin/bucket": `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":-1}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)

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

		userEndSeen := putSeen["rgw.test/admin/user"]
		assert.Zero(t, numberOfPutsWithValue(`enabled=false`, userEndSeen)) // no puts should disable

		assert.NotEmpty(t, putWithValue(`enabled=true`, userEndSeen)) // at least one put enables

		// there is only one time that max-size is set, and it's to the right value
		assert.Equal(t, 1, numberOfPutsWithValue(`max-size`, userEndSeen))
		assert.NotEmpty(t, putWithValue(`max-size=2`, userEndSeen))

		// there is only one time max-objects is set, and it's to the right value
		assert.Equal(t, 1, numberOfPutsWithValue(`max-objects`, userEndSeen))
		assert.NotEmpty(t, putWithValue(`max-objects=3`, userEndSeen))
	})

	t.Run("user quotas are enabled and need updated enabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":true,"max_size":1,"max_objects":1}}`,
			"rgw.test/admin/bucket": `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":-1}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)

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

		userEndSeen := putSeen["rgw.test/admin/user"]
		assert.Zero(t, numberOfPutsWithValue(`enabled=false`, userEndSeen)) // no puts should disable

		// there is only one time that max-size is set, and it's to the right value
		assert.Equal(t, 1, numberOfPutsWithValue(`max-size`, userEndSeen))
		assert.NotEmpty(t, putWithValue(`max-size=2`, userEndSeen))

		// there is only one time max-objects is set, and it's to the right value
		assert.NotEmpty(t, putWithValue(`max-objects=3`, userEndSeen))
		assert.Equal(t, 1, numberOfPutsWithValue(`max-objects`, userEndSeen))
	})

	t.Run("bucket maxSize quota should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			"rgw.test/admin/bucket": `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":-1}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)
		p.setBucketName("bob")

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{
						"bucketMaxSize": "4",
					},
				},
			},
		})
		assert.NoError(t, err)

		// user quota should not be touched
		userEndSeen := putSeen["rgw.test/admin/user"]
		assert.Len(t, userEndSeen, 0)
		// bucket quota be enabled
		bucketEndSeen := putSeen["rgw.test/admin/bucket"]
		assert.Len(t, bucketEndSeen, 1)
		assert.NotEmpty(t, putWithValue(`enabled=true`, bucketEndSeen))
		assert.NotEmpty(t, putWithValue(`max-size=4`, bucketEndSeen))
	})

	t.Run("bucket maxObjects quota should be enabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			"rgw.test/admin/bucket": `{"bucket_quota":{"enabled":false,"check_on_raw":false,"max_size":-1,"max_size_kb":0,"max_objects":-1}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)
		p.setBucketName("bob")

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{
						"bucketMaxObjects": "5",
					},
				},
			},
		})
		assert.NoError(t, err)

		// user quota should not be touched
		userEndSeen := putSeen["rgw.test/admin/user"]
		assert.Len(t, userEndSeen, 0)
		// bucket quota be enabled
		bucketEndSeen := putSeen["rgw.test/admin/bucket"]
		assert.Len(t, bucketEndSeen, 1)
		assert.NotEmpty(t, putWithValue(`enabled=true`, bucketEndSeen))
		assert.NotEmpty(t, putWithValue(`max-objects=5`, bucketEndSeen))
	})

	t.Run("bucket quotas are enabled and need updated enabled", func(t *testing.T) {
		getResult := map[string]string{
			"rgw.test/admin/user":   `{"user_quota":{"enabled":false,"max_size":-1,"max_objects":-1}}`,
			"rgw.test/admin/bucket": `{"bucket_quota":{"enabled":true,"check_on_raw":false,"max_size":4,"max_size_kb":0,"max_objects":5}}`,
		}
		putSeen := map[string][]string{}

		p := newProvisioner(t, &getResult, &putSeen)
		p.setBucketName("bob")

		err := p.setAdditionalSettings(&apibkt.BucketOptions{
			ObjectBucketClaim: &v1alpha1.ObjectBucketClaim{
				Spec: v1alpha1.ObjectBucketClaimSpec{
					AdditionalConfig: map[string]string{
						"bucketMaxSize":    "14",
						"bucketMaxObjects": "15",
					},
				},
			},
		})
		assert.NoError(t, err)

		// user quota should not be touched
		userEndSeen := putSeen["rgw.test/admin/user"]
		assert.Len(t, userEndSeen, 0)
		// bucket quota be enabled
		bucketEndSeen := putSeen["rgw.test/admin/bucket"]
		assert.Len(t, bucketEndSeen, 1)
		assert.NotEmpty(t, putWithValue(`max-size=14`, bucketEndSeen))
		assert.NotEmpty(t, putWithValue(`max-objects=15`, bucketEndSeen))
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
		assert.Equal(t, additionalConfigSpec{maxObjects: &(&struct{ i int64 }{2}).i}, *spec)
	})

	t.Run("maxSize field should be set", func(t *testing.T) {
		spec, err := additionalConfigSpecFromMap(map[string]string{"maxSize": "3"})
		assert.NoError(t, err)
		assert.Equal(t, additionalConfigSpec{maxSize: &(&struct{ i int64 }{3}).i}, *spec)
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
