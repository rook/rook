/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	apibkt "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephobject "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	runtimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type statusError struct {
	Code      string `json:"Code,omitempty"`
	RequestID string `json:"RequestId,omitempty"`
	HostID    string `json:"HostId,omitempty"`
}

func TestGetUserCredentialsNoKeys(t *testing.T) {
	accessKey, secretKey, err := getUserCredentials(admin.User{ID: "alice"}, "alice")

	assert.EqualError(t, err, `Ceph object user "alice" has no access keys`)
	assert.Empty(t, accessKey)
	assert.Empty(t, secretKey)
}

func TestBucketGetUserCreds(t *testing.T) {
	tests := []struct {
		name         string
		userResponse string
		wantAccess   string
		wantSecret   string
		wantErr      string
	}{
		{
			name:         "no access keys",
			userResponse: `{"user_id":"alice","keys":[]}`,
			wantErr:      `Ceph object user "alice" has no access keys`,
		},
		{
			name:         "returns user access key",
			userResponse: `{"user_id":"alice","keys":[{"access_key":"access","secret_key":"secret"}]}`,
			wantAccess:   "access",
			wantSecret:   "secret",
		},
	}

	owner := "alice"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adminClient, err := admin.New("rgw.test", "accesskey", "secretkey", &cephobject.MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					assert.Equal(t, http.MethodGet, req.Method)
					assert.Equal(t, userPath, req.URL.Path)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(tt.userResponse)),
					}, nil
				},
			})
			require.NoError(t, err)

			p := &Provisioner{
				cephUserName:   "alice",
				adminOpsClient: adminClient,
				clusterInfo:    &client.ClusterInfo{Context: context.Background()},
			}
			b := &bucket{
				provisioner:      p,
				additionalConfig: &additionalConfigSpec{bucketOwner: &owner},
			}

			access, secret, err := b.getUserCreds()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantAccess, access)
			assert.Equal(t, tt.wantSecret, secret)
		})
	}
}

func TestRevokeUserNoKeys(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/admin/bucket":
			_, _ = io.WriteString(w, `{"bucket":"bucket","owner":"alice"}`)
		case "/admin/user":
			_, _ = io.WriteString(w, `{"user_id":"alice","keys":[]}`)
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	host, portString, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.ParseInt(portString, 10, 32)
	require.NoError(t, err)
	if port < 0 || port > math.MaxInt32 {
		t.Fatalf("unexpected port %d", port)
	}

	const namespace = "ns"
	const storeName = "store"
	testStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: storeName, Namespace: namespace},
		Spec: cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{
				Port:                 int32(port),
				ExternalRgwEndpoints: []cephv1.EndpointAddress{{IP: host}},
			},
		},
	}
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "sc"},
		Parameters: map[string]string{
			ObjectStoreName:     storeName,
			objectStoreEndpoint: net.JoinHostPort(host, portString),
		},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: cephobject.RGWAdminOpsUserSecretName, Namespace: namespace},
		Data:       map[string][]byte{"accessKey": []byte("access"), "secretKey": []byte("secret")},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	kubeClient := k8sfake.NewSimpleClientset(storageClass)
	ctx := &clusterd.Context{
		Clientset:     kubeClient,
		Client:        runtimefake.NewClientBuilder().WithScheme(scheme).WithObjects(adminSecret).Build(),
		RookClientset: rookclient.NewSimpleClientset(testStore),
	}
	clusterInfo := client.AdminTestClusterInfo(namespace)
	p := NewProvisioner(ctx, clusterInfo)
	ob := &bktv1alpha1.ObjectBucket{
		Spec: bktv1alpha1.ObjectBucketSpec{
			StorageClassName: storageClass.Name,
			Connection: &bktv1alpha1.Connection{
				Endpoint:        &bktv1alpha1.Endpoint{BucketName: "bucket"},
				AdditionalState: map[string]string{CephUser: "alice", ObjectStoreName: storeName},
			},
		},
	}

	err = p.Revoke(ob)
	assert.EqualError(t, err, `Ceph object user "alice" has no access keys`)
	assert.EqualValues(t, 2, requests.Load())
}

func TestCreateCephUserNoKeys(t *testing.T) {
	tests := []struct {
		name      string
		getStatus int
		getBody   string
		wantPut   bool
	}{
		{
			name:      "existing user has no keys",
			getStatus: http.StatusOK,
			getBody:   `{"user_id":"obc-ns-claim-uid","keys":[]}`,
		},
		{
			name:      "created user has no keys",
			getStatus: http.StatusNotFound,
			getBody:   `{"Code":"NoSuchUser","RequestId":"requestid","HostId":"hostid"}`,
			wantPut:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &cephobject.MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					assert.Equal(t, userPath, req.URL.Path)
					if req.Method == http.MethodGet {
						return &http.Response{
							StatusCode: tt.getStatus,
							Body:       io.NopCloser(strings.NewReader(tt.getBody)),
						}, nil
					}
					if req.Method == http.MethodPut && tt.wantPut {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader(`{"user_id":"obc-ns-claim-uid","keys":[]}`)),
						}, nil
					}
					return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
				},
			}
			adminClient, err := admin.New("rgw.test", "accesskey", "secretkey", mockClient)
			require.NoError(t, err)

			clusterInfo := client.AdminTestClusterInfo("ns")
			p := NewProvisioner(&clusterd.Context{}, clusterInfo)
			p.objectContext = cephobject.NewContext(p.context, clusterInfo, "store")
			p.cephUserName = "obc-ns-claim-uid"
			p.adminOpsClient = adminClient

			access, secret, err := p.createCephUser(p.cephUserName)
			assert.EqualError(t, err, `Ceph object user "obc-ns-claim-uid" has no access keys`)
			assert.Empty(t, access)
			assert.Empty(t, secret)
		})
	}
}

func TestDeleteBucket(t *testing.T) {
	clusterInfo := client.AdminTestClusterInfo("ns")
	p := NewProvisioner(&clusterd.Context{RookClientset: rookclient.NewSimpleClientset(), Clientset: test.New(t, 1)}, clusterInfo)
	mockClient := func(errCodeRemoveBucket string, errCodeGetBucketInfo string) *cephobject.MockClient {
		return &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "rook-ceph-rgw-my-store.mycluster.svc/admin/bucket" {
					if req.Method == http.MethodDelete {
						status, _ := json.Marshal(statusError{errCodeRemoveBucket, "requestid", "hostid"})
						return &http.Response{
							StatusCode: 404,
							Body:       io.NopCloser(bytes.NewReader([]byte(status))),
						}, nil
					}
					if req.Method == http.MethodGet {
						status, _ := json.Marshal(statusError{errCodeGetBucketInfo, "requestid", "hostid"})
						return &http.Response{
							StatusCode: 404,
							Body:       io.NopCloser(bytes.NewReader([]byte(status))),
						}, nil
					}
				}
				return nil, fmt.Errorf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, req.URL.Path)
			},
		}
	}

	t.Run("remove bucket returns NoSuchBucket", func(t *testing.T) {
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient("NoSuchBucket", ""))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		err = p.deleteBucket("bucket")
		assert.NoError(t, err)
	})

	t.Run("remove bucket returns NoSuchKey and get bucket info returns NoSuchBucket", func(t *testing.T) {
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient("NoSuchKey", "NoSuchBucket"))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		err = p.deleteBucket("bucket")
		assert.NoError(t, err)
	})

	t.Run("remove bucket returns NoSuchKey and get bucket info returns an error other than NoSuchBucket", func(t *testing.T) {
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient("NoSuchKey", "NoSuchKey"))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		err = p.deleteBucket("bucket")
		assert.Error(t, err)
	})
}

func TestIsObcGeneratedUser(t *testing.T) {
	clusterInfo := client.AdminTestClusterInfo("ns")
	p := NewProvisioner(&clusterd.Context{RookClientset: rookclient.NewSimpleClientset(), Clientset: test.New(t, 1)}, clusterInfo)

	t.Run("does not match any format", func(t *testing.T) {
		assert.False(t, p.isObcGeneratedUser("quix", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
		}))
	})

	t.Run("does not match any format or bucketOwner", func(t *testing.T) {
		assert.False(t, p.isObcGeneratedUser("quix", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				AdditionalConfig: map[string]string{
					"bucketOwner": "baz",
				},
			},
		}))
	})

	t.Run("matches current format", func(t *testing.T) {
		assert.True(t, p.isObcGeneratedUser("obc-bar-foo-6e7c4d3f-3494-4dc1-90dc-58527fdf05d7", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
		}))
	})

	t.Run("matches old format", func(t *testing.T) {
		assert.True(t, p.isObcGeneratedUser("obc-bar-foo", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
		}))
	})

	t.Run("matches really old format", func(t *testing.T) {
		assert.True(t, p.isObcGeneratedUser("ceph-user-12345678", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
		}))
	})

	t.Run("matches bucketOwner", func(t *testing.T) {
		assert.False(t, p.isObcGeneratedUser("quix", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				AdditionalConfig: map[string]string{
					"bucketOwner": "quix",
				},
			},
		}))
	})

	t.Run("matches bucketOwner and current format", func(t *testing.T) {
		assert.False(t, p.isObcGeneratedUser("obc-bar-foo-6e7c4d3f-3494-4dc1-90dc-58527fdf05d7", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				AdditionalConfig: map[string]string{
					"bucketOwner": "obc-bar-foo-6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
				},
			},
		}))
	})

	t.Run("matches bucketOwner and old format", func(t *testing.T) {
		assert.False(t, p.isObcGeneratedUser("obc-bar-foo", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				AdditionalConfig: map[string]string{
					"bucketOwner": "obc-bar-foo",
				},
			},
		}))
	})

	t.Run("matches bucketOwner and really old format", func(t *testing.T) {
		assert.False(t, p.isObcGeneratedUser("ceph-user-12345678", &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				UID:       "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7",
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				AdditionalConfig: map[string]string{
					"bucketOwner": "ceph-user-12345678",
				},
			},
		}))
	})
}

func TestCheckRelink(t *testing.T) {
	const (
		generatedUser = "obc-tenant-a-mine-6e7c4d3f-3494-4dc1-90dc-58527fdf05d7"
		otherObcUser  = "obc-tenant-b-theirs-0b6b0a4e-6d0b-4d55-8f8e-0f5e1c9b2a17"
	)
	alice, bob := "alice", "bob"

	newBucket := func(recordedUser string, bucketOwner *string) *bucket {
		p := &Provisioner{bucketName: "shared", cephUserName: recordedUser}
		if bucketOwner != nil {
			p.cephUserName = *bucketOwner
		}
		return &bucket{
			provisioner:      p,
			options:          &apibkt.BucketOptions{UserID: recordedUser},
			additionalConfig: &additionalConfigSpec{bucketOwner: bucketOwner},
		}
	}

	t.Run("refuses a bucket owned by another OBC without bucketOwner", func(t *testing.T) {
		err := newBucket(generatedUser, nil).checkRelink(otherObcUser)
		assert.ErrorContains(t, err, `owned by user "`+otherObcUser+`" instead of this OBC's user "`+generatedUser+`"`)
	})

	t.Run("refuses a bucket owned by an unrelated user with bucketOwner", func(t *testing.T) {
		err := newBucket(generatedUser, &alice).checkRelink(otherObcUser)
		assert.ErrorContains(t, err, `may only take over a bucket owned by this OBC's previous user "`+generatedUser+`"`)
	})

	t.Run("refuses a bucket owned by an unrelated user when bucketOwner changes", func(t *testing.T) {
		err := newBucket("alice", &bob).checkRelink("mallory")
		assert.ErrorContains(t, err, `owned by user "mallory"`)
	})

	t.Run("allows bucketOwner to take over from the OBC's generated user", func(t *testing.T) {
		assert.NoError(t, newBucket(generatedUser, &alice).checkRelink(generatedUser))
	})

	t.Run("allows bucketOwner to take over from the OBC's previous bucketOwner", func(t *testing.T) {
		assert.NoError(t, newBucket("alice", &bob).checkRelink("alice"))
	})
}
