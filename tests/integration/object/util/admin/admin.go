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

package admin

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	utils3 "github.com/rook/rook/tests/integration/object/util/s3"
)

func NewAdminClient(objectStore *cephv1.CephObjectStore, installer *installer.CephInstaller, k8sh *utils.K8sHelper, tlsEnable bool) (*admin.API, error) {
	accessKey, secretKey, err := utils3.GetS3Credentials(objectStore, installer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get s3 credentials")
	}

	endpoint, err := utils3.GetS3Endpoint(objectStore, k8sh, tlsEnable)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get s3 endpoint")
	}

	httpClient := &http.Client{}

	if tlsEnable {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				// nolint:gosec // skip TLS verification as this is a test
				InsecureSkipVerify: true,
			},
		}
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
