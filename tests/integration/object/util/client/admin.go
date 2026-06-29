/*
Copyright 2025 The Rook Authors. All rights reserved.

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

// Package client builds the clients the object tests use to reach a
// CephObjectStore — the rgw admin and SNS clients and the S3 credentials and
// endpoint they need — along with the store's TLS certificate and a
// verification-skipping HTTP client for the TLS pass.
package client

import (
	"context"
	"net/http"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

func NewAdminClient(objectStore *cephv1.CephObjectStore, installer *installer.CephInstaller, k8sh *utils.K8sHelper, tlsEnable bool) (*admin.API, error) {
	accessKey, secretKey, err := GetS3Credentials(objectStore, installer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get s3 credentials")
	}

	endpoint, err := GetS3Endpoint(objectStore, k8sh, tlsEnable)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get s3 endpoint")
	}

	httpClient := &http.Client{}
	if tlsEnable {
		httpClient = InsecureHTTPClient()
	}

	adminClient, err := admin.New(endpoint, accessKey, secretKey, httpClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup ceph admin client")
	}

	// verify that admin api is working
	_, err = adminClient.GetInfo(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "admin client is not working")
	}

	return adminClient, nil
}
