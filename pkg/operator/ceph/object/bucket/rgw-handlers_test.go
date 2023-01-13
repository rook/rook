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
	"net/http"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/object"
	cephobject "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
)

type statusError struct {
	Code      string `json:"Code,omitempty"`
	RequestID string `json:"RequestId,omitempty"`
	HostID    string `json:"HostId,omitempty"`
}

func TestDeleteOBCResource(t *testing.T) {
	clusterInfo := client.AdminTestClusterInfo("ns")
	p := NewProvisioner(&clusterd.Context{RookClientset: rookclient.NewSimpleClientset(), Clientset: test.New(t, 1)}, clusterInfo)
	p.cephUserName = "bob"
	mockClient := func(errCodeRemoveBucket string, errCodeGetBucketInfo string, success bool) *cephobject.MockClient {
		return &cephobject.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "rook-ceph-rgw-my-store.mycluster.svc/admin/bucket" {
					if req.Method == http.MethodDelete {
						if success {
							return &http.Response{
								StatusCode: 200,
								Body:       io.NopCloser(bytes.NewReader([]byte{})),
							}, nil
						}
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
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient("NoSuchBucket", "", false))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		err = p.deleteOBCResource("bucket")
		assert.NoError(t, err)
	})

	t.Run("remove bucket returns NoSuchKey and get bucket info returns NoSuchBucket", func(t *testing.T) {
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient("NoSuchKey", "NoSuchBucket", false))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		err = p.deleteOBCResource("bucket")
		assert.NoError(t, err)
	})

	t.Run("remove bucket returns NoSuchKey and get bucket info returns an error other than NoSuchBucket", func(t *testing.T) {
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient("NoSuchKey", "NoSuchKey", false))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		err = p.deleteOBCResource("bucket")
		assert.Error(t, err)
	})
	t.Run("remove bucket successfully", func(t *testing.T) {
		adminClient, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient("", "", true))
		assert.NoError(t, err)
		p.adminOpsClient = adminClient
		err = p.deleteOBCResource("bucket")
		assert.NoError(t, err)
	})
}

func TestGetCephUser(t *testing.T) {
	newProvisioner := func(t *testing.T, getBucketResult string) *Provisioner {
		mockClient := &object.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				t.Logf("HTTP req: %#v", req.URL)
				t.Logf("HTTP %s: %s %s", req.Method, req.URL.Path, req.URL.RawQuery)

				assert.Contains(t, req.URL.RawQuery, "uid=bob")

				if req.Method == http.MethodGet {
					if req.URL.Path == "my.endpoint.net/admin/user" {
						statusCode := 200
						if getBucketResult == "" {
							return &http.Response{
								StatusCode: 500,
								Body:       io.NopCloser(bytes.NewReader([]byte{})),
							}, errors.New("error")
						}
						return &http.Response{
							StatusCode: statusCode,
							Body:       io.NopCloser(bytes.NewReader([]byte(getBucketResult))),
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
			bucketName:     "test-bucket",
			adminOpsClient: adminClient,
		}

		return p
	}

	t.Run("Succeed to get ceph user", func(t *testing.T) {
		p := newProvisioner(t,
			`{"keys":[{"access_key":"ak","secret_key":"sk"}]}`,
		)

		ak, sk, err := p.getCephUser("bob")
		assert.NoError(t, err)
		assert.Equal(t, "ak", ak)
		assert.Equal(t, "sk", sk)
	})

	t.Run("Failed to get ceph user", func(t *testing.T) {
		p := newProvisioner(t, "")

		ak, sk, err := p.getCephUser("bob")
		assert.Error(t, err)
		assert.Equal(t, "", ak)
		assert.Equal(t, "", sk)
	})

}
