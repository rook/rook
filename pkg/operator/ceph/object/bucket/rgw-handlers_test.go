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
	"errors"
	"fmt"
	"io"
	"net/http"
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
	"k8s.io/client-go/tools/events"
)

type statusError struct {
	Code      string `json:"Code,omitempty"`
	RequestID string `json:"RequestId,omitempty"`
	HostID    string `json:"HostId,omitempty"`
}

func TestDeleteBucket(t *testing.T) {
	clusterInfo := client.AdminTestClusterInfo("ns")
	p := NewProvisioner(&clusterd.Context{RookClientset: rookclient.NewSimpleClientset(), Clientset: test.New(t, 1)}, clusterInfo, nil)
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

func TestBucketExists(t *testing.T) {
	clusterInfo := client.AdminTestClusterInfo("ns")
	p := NewProvisioner(&clusterd.Context{RookClientset: rookclient.NewSimpleClientset(), Clientset: test.New(t, 1)}, clusterInfo, nil)
	mockClient := func(statusCode int, body string) *cephobject.MockClient {
		return &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "rook-ceph-rgw-my-store.mycluster.svc/admin/bucket" && req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: statusCode,
						Body:       io.NopCloser(bytes.NewReader([]byte(body))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, req.URL.Path)
			},
		}
	}

	t.Run("bucket exists", func(t *testing.T) {
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(200, `{"bucket":"bkt","owner":"bob","placement_rule":"loc-a"}`))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		exists, info, err := p.bucketExists("bkt")
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "bob", info.Owner)
		assert.Equal(t, "loc-a", info.PlacementRule)
	})

	t.Run("bucket does not exist", func(t *testing.T) {
		status, _ := json.Marshal(statusError{"NoSuchBucket", "requestid", "hostid"})
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(404, string(status)))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		exists, info, err := p.bucketExists("bkt")
		assert.NoError(t, err)
		assert.False(t, exists)
		assert.Nil(t, info)
	})

	t.Run("error other than NoSuchBucket", func(t *testing.T) {
		status, _ := json.Marshal(statusError{"AccessDenied", "requestid", "hostid"})
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(403, string(status)))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		exists, info, err := p.bucketExists("bkt")
		assert.Error(t, err)
		assert.False(t, exists)
		assert.Nil(t, info)
	})
}

// s3RoundTripper answers every S3 request with status and body.
type s3RoundTripper struct {
	status int
	body   string
}

func (rt s3RoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: rt.status,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader([]byte(rt.body))),
	}, nil
}

// unreachableRoundTripper fails every S3 request before RGW answers.
type unreachableRoundTripper struct{}

func (unreachableRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial tcp: connection refused")
}

func TestProvisioner_createBucket(t *testing.T) {
	newProvisioner := func(t *testing.T, rt http.RoundTripper, recorder events.EventRecorder) *Provisioner {
		s3Agent, err := cephobject.NewS3Agent("accessKey", "secretKey", "rgw.test", false, nil, false, &http.Client{Transport: rt})
		require.NoError(t, err)
		return &Provisioner{
			clusterInfo: client.AdminTestClusterInfo("ns"),
			bucketName:  "bkt",
			s3Agent:     s3Agent,
			recorder:    recorder,
		}
	}
	newBucket := func(placement, storageClass string) *bucket {
		return &bucket{
			options: &apibkt.BucketOptions{
				ObjectBucketClaim: &bktv1alpha1.ObjectBucketClaim{ObjectMeta: metav1.ObjectMeta{Name: "my-obc", Namespace: "my-ns"}},
			},
			additionalConfig: &additionalConfigSpec{bucketPlacement: placement, bucketStorageClass: storageClass},
		}
	}
	rejected := s3RoundTripper{
		status: http.StatusBadRequest,
		body:   `<Error><Code>InvalidLocationConstraint</Code><Message>The specified location-constraint is not valid</Message></Error>`,
	}

	t.Run("success records no event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := newProvisioner(t, s3RoundTripper{status: http.StatusOK}, recorder)
		assert.NoError(t, p.createBucket(newBucket("loc-a", "COLD")))
		assert.Empty(t, recorder.Events)
	})

	t.Run("RGW rejection records a BucketPlacementRejected event carrying RGW's message", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := newProvisioner(t, rejected, recorder)
		err := p.createBucket(newBucket("nowhere", ""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `error creating bucket "bkt"`)
		assert.Contains(t, err.Error(), `placement "nowhere"`)
		assert.Contains(t, err.Error(), "The specified location-constraint is not valid")

		require.Len(t, recorder.Events, 1)
		event := <-recorder.Events
		assert.Contains(t, event, "Warning "+EventReasonBucketPlacementRejected+" ")
		assert.Contains(t, event, err.Error())
	})

	t.Run("a storage class alone is a placement request", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := newProvisioner(t, rejected, recorder)
		require.Error(t, p.createBucket(newBucket("", "FOO")))
		assert.Len(t, recorder.Events, 1)
	})

	t.Run("failure without a placement request records no event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := newProvisioner(t, rejected, recorder)
		require.Error(t, p.createBucket(newBucket("", "")))
		assert.Empty(t, recorder.Events)
	})

	t.Run("failure to reach RGW is not a rejection and records no event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		p := newProvisioner(t, unreachableRoundTripper{}, recorder)
		// a cancelled context makes the SDK give up on the first attempt
		// instead of sleeping through its retry backoff
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		p.clusterInfo.Context = ctx
		err := p.createBucket(newBucket("loc-a", "COLD"))
		require.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, recorder.Events)
	})

	t.Run("an RGW error that is not a placement refusal records no event", func(t *testing.T) {
		recorder := events.NewFakeRecorder(1)
		unrelated := s3RoundTripper{
			status: http.StatusBadRequest,
			body:   `<Error><Code>InvalidBucketName</Code><Message>The specified bucket is not valid.</Message></Error>`,
		}
		p := newProvisioner(t, unrelated, recorder)
		err := p.createBucket(newBucket("loc-a", ""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "InvalidBucketName")
		assert.Empty(t, recorder.Events)
	})

	t.Run("nil recorder is safe", func(t *testing.T) {
		p := newProvisioner(t, rejected, nil)
		assert.Error(t, p.createBucket(newBucket("loc-a", "")))
	})
}

func TestIsObcGeneratedUser(t *testing.T) {
	clusterInfo := client.AdminTestClusterInfo("ns")
	p := NewProvisioner(&clusterd.Context{RookClientset: rookclient.NewSimpleClientset(), Clientset: test.New(t, 1)}, clusterInfo, nil)

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
