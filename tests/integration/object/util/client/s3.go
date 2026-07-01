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

package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

func GetS3Credentials(objectStore *cephv1.CephObjectStore, installer *installer.CephInstaller) (string, string, error) {
	output, err := installer.Execute("radosgw-admin", []string{"user", "info", "--uid=dashboard-admin", fmt.Sprintf("--rgw-realm=%s", objectStore.Name)}, objectStore.Namespace)
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

	schema, port := "http://", "80"
	if tlsEnable {
		schema, port = "https://", "443"
	}

	endpoint := schema + svc.Spec.ClusterIP + ":" + port

	return endpoint, nil
}

// NewS3Client builds an S3 client for the given access/secret key pair against
// the object store's endpoint. Unlike NewAdminClient/NewSNSClient it does not
// derive credentials, so callers can act as a specific CephObjectStoreUser.
func NewS3Client(objectStore *cephv1.CephObjectStore, k8sh *utils.K8sHelper, tlsEnable bool, accessKey, secretKey string) (*s3.Client, error) {
	ctx := context.TODO()

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				accessKey,
				secretKey,
				"",
			),
		),
	}
	if tlsEnable {
		loadOpts = append(loadOpts, config.WithHTTPClient(InsecureHTTPClient()))
	}

	cfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load default aws config")
	}

	endpoint, err := GetS3Endpoint(objectStore, k8sh, tlsEnable)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get s3 endpoint")
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return s3Client, nil
}
