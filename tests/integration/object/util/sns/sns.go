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

package sns

import (
	"context"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	utils3 "github.com/rook/rook/tests/integration/object/util/s3"
)

// based on the s3 endpoint example in the official docs:
// https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-endpoints.html
type snsResolverV2 struct {
	endpoint string
}

func (r *snsResolverV2) ResolveEndpoint(ctx context.Context, params sns.EndpointParameters) (smithyendpoints.Endpoint, error) {
	u, err := url.Parse(r.endpoint)
	if err != nil {
		return smithyendpoints.Endpoint{}, err
	}
	return smithyendpoints.Endpoint{
		URI: *u,
	}, nil
}

func NewClient(objectStore *cephv1.CephObjectStore, svc *corev1.Service, k8sh *utils.K8sHelper, installer *installer.CephInstaller, tlsEnable bool) (*sns.Client, error) {
	ctx := context.TODO()

	accessKey, secretKey, err := utils3.GetS3Credentials(objectStore, installer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get s3 credentials")
	}

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				accessKey,
				secretKey,
				"",
			),
		),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load default aws config")
	}

	endpoint, err := utils3.GetS3Endpoint(objectStore, k8sh, tlsEnable)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sns endpoint")
	}

	snsClient := sns.NewFromConfig(cfg, func(o *sns.Options) {
		o.EndpointResolverV2 = &snsResolverV2{endpoint: endpoint}
	})

	// sanity check that client is working
	_, err = snsClient.ListTopics(ctx, &sns.ListTopicsInput{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to sns client sanity check")
	}

	return snsClient, nil
}
