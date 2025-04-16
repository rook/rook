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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
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

func GetS3Credentials(objectStore *cephv1.CephObjectStore, installer *installer.CephInstaller) (string, string, error) {
	err, output := installer.Execute("radosgw-admin", []string{"user", "info", "--uid=dashboard-admin", fmt.Sprintf("--rgw-realm=%s", objectStore.Name)}, objectStore.Namespace)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get user info")
	}

	// extract api creds from json output
	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(output), &userInfo)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to unmarshal user info")
	}

	accessKey, ok := userInfo["keys"].([]interface{})[0].(map[string]interface{})["access_key"].(string)
	if !ok {
		return "", "", errors.New("failed to get access key")
	}

	secretKey, ok := userInfo["keys"].([]interface{})[0].(map[string]interface{})["secret_key"].(string)
	if !ok {
		return "", "", errors.New("failed to get secret key")
	}

	return accessKey, secretKey, nil
}

func GetS3Endpoint(objectStore *cephv1.CephObjectStore, k8sh *utils.K8sHelper, tlsEnable bool) (string, error) {
	ctx := context.TODO()

	// extract rgw endpoint from k8s svc
	svc, err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Get(ctx, objectStore.Name, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "failed to get objectstore svc")
	}

	schema := "http://"
	httpClient := &http.Client{}

	if tlsEnable {
		schema = "https://"
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				// nolint:gosec // skip TLS verification as this is a test
				InsecureSkipVerify: true,
			},
		}
	}
	endpoint := schema + svc.Spec.ClusterIP + ":80"

	return endpoint, nil
}

func NewClient(objectStore *cephv1.CephObjectStore, svc *corev1.Service, k8sh *utils.K8sHelper, installer *installer.CephInstaller, tlsEnable bool) (*sns.Client, error) {
	ctx := context.TODO()

	accessKey, secretKey, err := GetS3Credentials(objectStore, installer)
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

	endpoint, err := GetS3Endpoint(objectStore, k8sh, tlsEnable)
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
