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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	apibkt "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephobject "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type statusError struct {
	Code      string `json:"Code,omitempty"`
	RequestID string `json:"RequestId,omitempty"`
	HostID    string `json:"HostId,omitempty"`
}

//nolint:gosec // only test values, not real credentials
const (
	testUserAccessKey = "OWNERACCESSKEY000001"
	testUserSecretKey = "ownersecretkey00000000000000000000000001"
	testUserKeysJSON  = `[{"access_key":"` + testUserAccessKey + `","secret_key":"` + testUserSecretKey + `"}]`
)

// testUserJSON is an RGW user info reply for user, with keys as its JSON list of S3 keys.
func testUserJSON(user, keys string) string {
	return fmt.Sprintf(`{"user_id":%q,"display_name":%q,"keys":%s}`, user, user, keys)
}

func rgwReply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestGetUserCredsBucketOwnerKeys(t *testing.T) {
	owner := "alice"

	for _, tt := range []struct {
		name       string
		keys       string
		wantErr    string
		wantAccess string
		wantSecret string
	}{
		{name: "owner without S3 keys", keys: `[]`, wantErr: `Ceph object user "alice" has no access keys`},
		{name: "owner with an S3 key", keys: testUserKeysJSON, wantAccess: testUserAccessKey, wantSecret: testUserSecretKey},
	} {
		t.Run(tt.name, func(t *testing.T) {
			adminClient, err := admin.New("rgw.test", "accesskey", "secretkey", &cephobject.MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					if req.Method == http.MethodGet && req.URL.Path == userPath {
						return rgwReply(http.StatusOK, testUserJSON(owner, tt.keys)), nil
					}
					return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
				},
			})
			require.NoError(t, err)
			b := &bucket{
				provisioner:      &Provisioner{cephUserName: owner, adminOpsClient: adminClient, clusterInfo: client.AdminTestClusterInfo("ns")},
				additionalConfig: &additionalConfigSpec{bucketOwner: &owner},
			}

			accessKey, secretKey, err := b.getUserCreds()

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantAccess, accessKey)
			assert.Equal(t, tt.wantSecret, secretKey)
		})
	}
}

func TestCreateCephUserKeys(t *testing.T) {
	const user = "obc-ns-claim-6e7c4d3f-3494-4dc1-90dc-58527fdf05d7"
	noSuchUser, err := json.Marshal(statusError{"NoSuchUser", "requestid", "hostid"})
	require.NoError(t, err)

	for _, tt := range []struct {
		name       string
		exists     bool
		keys       string
		wantErr    string
		wantAccess string
		wantSecret string
	}{
		{name: "existing user without S3 keys", exists: true, keys: `[]`, wantErr: `Ceph object user "` + user + `" has no access keys`},
		{name: "created user without S3 keys", keys: `[]`, wantErr: `Ceph object user "` + user + `" has no access keys`},
		{name: "existing user with an S3 key", exists: true, keys: testUserKeysJSON, wantAccess: testUserAccessKey, wantSecret: testUserSecretKey},
		{name: "created user with an S3 key", keys: testUserKeysJSON, wantAccess: testUserAccessKey, wantSecret: testUserSecretKey},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var created bool
			adminClient, err := admin.New("rgw.test", "accesskey", "secretkey", &cephobject.MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					switch {
					case req.URL.Path != userPath:
					case req.Method == http.MethodGet && !tt.exists:
						return rgwReply(http.StatusNotFound, string(noSuchUser)), nil
					case req.Method == http.MethodGet:
						return rgwReply(http.StatusOK, testUserJSON(user, tt.keys)), nil
					case req.Method == http.MethodPut && !tt.exists:
						created = true
						return rgwReply(http.StatusOK, testUserJSON(user, tt.keys)), nil
					}
					return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
				},
			})
			require.NoError(t, err)
			clusterInfo := client.AdminTestClusterInfo("ns")
			p := NewProvisioner(&clusterd.Context{}, clusterInfo)
			p.objectContext = cephobject.NewContext(p.context, clusterInfo, "store")
			p.adminOpsClient = adminClient

			accessKey, secretKey, err := p.createCephUser(user)

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantAccess, accessKey)
			assert.Equal(t, tt.wantSecret, secretKey)
			assert.Equal(t, !tt.exists, created)
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
