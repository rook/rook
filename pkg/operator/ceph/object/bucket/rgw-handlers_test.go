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
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephobject "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type statusError struct {
	Code      string `json:"Code,omitempty"`
	RequestID string `json:"RequestId,omitempty"`
	HostID    string `json:"HostId,omitempty"`
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
