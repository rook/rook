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

package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const lifecycle1 = `
{
  "Rules":[
    {
      "ID": "AbortIncompleteMultipartUploads",
      "Status": "Enabled",
      "Prefix": "",
      "AbortIncompleteMultipartUpload": {
        "DaysAfterInitiation": 1
      }
    }
  ]
}
`

// rules must be sorted by ID to be idempotent
const lifecycle2 = `
{
  "Rules": [
    {
      "ID": "AbortIncompleteMultipartUploads",
      "Status": "Enabled",
      "Prefix": "",
      "AbortIncompleteMultipartUpload": {
        "DaysAfterInitiation": 1
      }
    },
    {
      "ID": "ExpireAfter30Days",
      "Status": "Enabled",
      "Prefix": "",
      "Expiration": {
        "Days": 30
      }
    }
  ]
}
`

var lifecycleCmpOpts = cmp.Options{
	cmpopts.IgnoreUnexported(
		s3types.LifecycleRule{},
		s3types.LifecycleExpiration{},
		s3types.LifecycleRuleFilter{},
		s3types.LifecycleRuleAndOperator{},
		s3types.AbortIncompleteMultipartUpload{},
		s3types.NoncurrentVersionExpiration{},
		s3types.NoncurrentVersionTransition{},
		s3types.Transition{},
		s3types.Tag{},
	),
}

// requireLifecycleApplied waits until the bucket's live lifecycle rules match
// the expected JSON configuration.
func requireLifecycleApplied(ctx context.Context, t *testing.T, s3agent *rgw.S3Agent, bucket, expected string) {
	t.Helper()

	conf := &s3types.BucketLifecycleConfiguration{}
	require.NoError(t, json.Unmarshal([]byte(expected), conf))

	wait4.RequireEventually(ctx, t, wait4.TimeoutShort, "bucket lifecycle applied", func(ctx context.Context) error {
		resp, err := s3agent.Client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: &bucket})
		if err != nil {
			return err
		}
		if !cmp.Equal(conf.Rules, resp.Rules, lifecycleCmpOpts...) {
			return fmt.Errorf("bucket lifecycle rules not in sync: %s", cmp.Diff(conf.Rules, resp.Rules, lifecycleCmpOpts...))
		}
		return nil
	})
}

const Namespace = "test-bucketlifecycle"

func TestObjectBucketClaimLifecycle(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
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

		obc1 = &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bucketName,
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       bucketName,
				StorageClassName: storageClass.Name,
				AdditionalConfig: map[string]string{"bucketLifecycle": lifecycle1},
			},
		}

		obcClient  = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
		cephClient = k8sh.RookClientset.CephV1().CephClusters(objectStore.Namespace)
	)

	t.Run("ObjectBucketClaim lifecycle", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)
		fixture.RequireStorageClass(t, k8sh, storageClass)

		obc.RequireBound(ctx, t, k8sh, obc1)
		s3agent := obc.NewS3Agent(ctx, t, k8sh, store, ns.Name, obc1.Name)

		t.Run(fmt.Sprintf("lifecycle is applied verbatim to bucket %q", bucketName), func(t *testing.T) {
			requireLifecycleApplied(ctx, t, s3agent, bucketName, lifecycle1)
		})

		t.Run(fmt.Sprintf("update obc %q bucketLifecycle", obc1.Name), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obc1.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig["bucketLifecycle"] = lifecycle2
			})
		})

		t.Run(fmt.Sprintf("lifecycle update is applied verbatim to bucket %q", bucketName), func(t *testing.T) {
			requireLifecycleApplied(ctx, t, s3agent, bucketName, lifecycle2)
		})

		t.Run(fmt.Sprintf("remove obc %q bucketLifecycle", obc1.Name), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obc1.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig = map[string]string{}
			})
		})

		t.Run(fmt.Sprintf("lifecycle is removed from bucket %q", bucketName), func(t *testing.T) {
			clusters, err := cephClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			require.NotEmpty(t, clusters.Items)
			require.NotNil(t, clusters.Items[0].Status.CephVersion)
			if strings.HasPrefix(clusters.Items[0].Status.CephVersion.Version, "19.2.3") {
				t.Skip("waiting for the rgw fix for the lifecycle removal regression in v19.2.3")
			}

			wait4.RequireEventually(ctx, t, wait4.TimeoutShort, "bucket lifecycle removed", func(ctx context.Context) error {
				_, err := s3agent.Client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: &bucketName})
				if err == nil {
					return errors.New("bucket lifecycle still present")
				}

				var apiErr smithy.APIError
				if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchLifecycleConfiguration" {
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
