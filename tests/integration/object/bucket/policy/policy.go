/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// bucketPolicy returns an S3 bucket policy allowing principal the given actions
// on every object in bucket.
func bucketPolicy(bucket, principal string, actions ...string) string {
	return fmt.Sprintf(`{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "AWS": "arn:aws:iam:::user/%s"
            },
            "Action": [
                "%s"
            ],
            "Resource": [
                "arn:aws:s3:::%s/*"
            ]
        }
    ]
}`, principal, strings.Join(actions, `",
                "`), bucket)
}

const Namespace = "test-bucketpolicy"

func TestObjectBucketClaimPolicy(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		storageClass = obc.StorageClass(defaultName, objectStore)

		bucketName = defaultName + "-obc1"

		policy1 = bucketPolicy(bucketName, "foo",
			"s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket", "s3:GetBucketLocation")
		policy2 = bucketPolicy(bucketName, "bar",
			"s3:GetObject", "s3:ListBucket", "s3:GetBucketLocation")

		obc1 = &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bucketName,
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       bucketName,
				StorageClassName: storageClass.Name,
				AdditionalConfig: map[string]string{"bucketPolicy": policy1},
			},
		}

		obcClient = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
	)

	t.Run("ObjectBucketClaim policy", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)
		fixture.RequireStorageClass(t, k8sh, storageClass)

		obc.RequireBound(ctx, t, k8sh, obc1)
		s3agent := obc.NewS3Agent(ctx, t, k8sh, store, ns.Name, obc1.Name)

		t.Run(fmt.Sprintf("policy is applied verbatim to bucket %q", bucketName), func(t *testing.T) {
			resp, err := s3agent.Client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &bucketName})
			require.NoError(t, err)
			require.NotNil(t, resp.Policy)
			assert.Equal(t, policy1, *resp.Policy)
		})

		t.Run(fmt.Sprintf("update obc %q bucketPolicy", obc1.Name), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obc1.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig["bucketPolicy"] = policy2
			})
		})

		t.Run(fmt.Sprintf("policy update is applied verbatim to bucket %q", bucketName), func(t *testing.T) {
			wait4.RequireEventually(ctx, t, wait4.TimeoutShort, "bucket policy updated", func(ctx context.Context) error {
				resp, err := s3agent.Client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &bucketName})
				if err != nil {
					return err
				}
				if resp.Policy == nil || *resp.Policy != policy2 {
					return errors.New("bucket policy not yet updated")
				}
				return nil
			})
		})

		t.Run(fmt.Sprintf("remove obc %q bucketPolicy", obc1.Name), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obc1.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig = map[string]string{}
			})
		})

		t.Run(fmt.Sprintf("policy is removed from bucket %q", bucketName), func(t *testing.T) {
			wait4.RequireEventually(ctx, t, wait4.TimeoutShort, "bucket policy removed", func(ctx context.Context) error {
				_, err := s3agent.Client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &bucketName})
				if err == nil {
					return errors.New("bucket policy still present")
				}

				var apiErr smithy.APIError
				if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchBucketPolicy" {
					return nil
				}
				return err
			})
		})

		t.Run(fmt.Sprintf("delete obc %q", obc1.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obc1.Name)
		})

		t.Run(fmt.Sprintf("no obc(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := obcClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			assert.Len(t, list.Items, 0)
		})
	})
}
