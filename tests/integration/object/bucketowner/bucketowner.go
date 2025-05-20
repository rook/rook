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

package bucketowner

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	defaultName = "test-bucket-owner"

	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
	}

	objectStore = &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
			// the CephObjectstore must be in the same ns as the CephCluster
			Namespace: "object-ns",
		},
		Spec: cephv1.ObjectStoreSpec{
			MetadataPool: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
			},
			DataPool: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
			},
			Gateway: cephv1.GatewaySpec{
				Port:      80,
				Instances: 1,
			},
			AllowUsersInNamespaces: []string{ns.Name},
		},
	}

	objectStoreSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectStore.Name,
			Namespace: objectStore.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":               "rook-ceph-rgw",
				"rook_cluster":      objectStore.Namespace,
				"rook_object_store": objectStore.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            corev1.ServiceTypeNodePort,
		},
	}

	storageClass = &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
		Provisioner: objectStore.Namespace + ".ceph.rook.io/bucket",
		Parameters: map[string]string{
			"objectStoreName":      objectStore.Name,
			"objectStoreNamespace": objectStore.Namespace,
		},
	}

	osu1 = cephv1.CephObjectStoreUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-user1",
			Namespace: ns.Name,
		},
		Spec: cephv1.ObjectStoreUserSpec{
			Store:            objectStore.Name,
			ClusterNamespace: objectStore.Namespace,
		},
	}

	osu2 = cephv1.CephObjectStoreUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-user2",
			Namespace: ns.Name,
		},
		Spec: cephv1.ObjectStoreUserSpec{
			Store:            objectStore.Name,
			ClusterNamespace: objectStore.Namespace,
		},
	}

	obc1 = bktv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-obc1",
			Namespace: ns.Name,
		},
		Spec: bktv1alpha1.ObjectBucketClaimSpec{
			BucketName:       defaultName + "-obc1",
			StorageClassName: storageClass.Name,
			AdditionalConfig: map[string]string{
				"bucketOwner": osu1.Name,
			},
		},
	}

	obc2 = bktv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-obc2",
			Namespace: ns.Name,
		},
		Spec: bktv1alpha1.ObjectBucketClaimSpec{
			BucketName:       defaultName + "-obc2",
			StorageClassName: storageClass.Name,
			AdditionalConfig: map[string]string{
				"bucketOwner": osu1.Name,
			},
		},
	}

	obcBogusOwner = bktv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-bogus-owner",
			Namespace: ns.Name,
		},
		Spec: bktv1alpha1.ObjectBucketClaimSpec{
			BucketName:       defaultName,
			StorageClassName: storageClass.Name,
			AdditionalConfig: map[string]string{
				"bucketOwner": defaultName + "-bogus-user",
			},
		},
	}
)

func WaitForPodLogContainingText(k8sh *utils.K8sHelper, namespace string, selector *labels.Selector, text string, timeout time.Duration) error {
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(
		context.Background(),
		metav1.ListOptions{
			LabelSelector: (*selector).String(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to list pods: %v", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found with labels %v in namespace %q", selector, namespace)
	}

	// if there are multiple pods, just pick the first one
	selectedPod := pods.Items[0]
	log.Printf("Found pod %q (first match) with labels %v\n", selectedPod.Name, *selector)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req := k8sh.Clientset.CoreV1().Pods(namespace).GetLogs(selectedPod.Name, &corev1.PodLogOptions{})

	logStream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to stream logs from pod %q: %v", selectedPod.Name, err)
	}
	defer logStream.Close()

	scanner := bufio.NewScanner(logStream)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, text) {
			break
		}
	}
	// Check for scanner error (could be context timeout, etc.)
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading log stream: %v", err)
	}

	return nil
}

func TestObjectBucketClaimBucketOwner(t *testing.T, k8sh *utils.K8sHelper, installer *installer.CephInstaller, logger *capnslog.PackageLogger, tlsEnable bool) {
	t.Run("OBC bucketOwner", func(t *testing.T) {
		if tlsEnable {
			// There is lots of coverage of rgw working with tls enabled; skipping to save time.
			// If tls is to be enabled, cert generation needs to be added and a
			// different CephObjectStore name needs to be set for with/without tls as
			// CephObjectStore does not currently cleanup the rgw realm.
			t.Skip("skipping test for TLS enabled clusters")
		}

		var adminClient *admin.API
		ctx := context.TODO()

		t.Run(fmt.Sprintf("create ns %q", ns.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create CephObjectStore %q", objectStore.Name), func(t *testing.T) {
			objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Create(ctx, objectStore, metav1.CreateOptions{})
			require.NoError(t, err)

			osReady := utils.Retry(180, time.Second, "CephObjectStore is Ready", func() bool {
				liveOs, err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Get(ctx, objectStore.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveOs.Status == nil {
					return false
				}

				// return liveOs.Status.Phase == "Ready"
				return liveOs.Status.Phase == cephv1.ConditionReady
			})
			require.True(t, osReady)
		})

		t.Run(fmt.Sprintf("create svc %q", objectStoreSvc.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Create(ctx, objectStoreSvc, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create sc %q", storageClass.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.StorageV1().StorageClasses().Create(ctx, storageClass, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		// since this is an obc specific subtest we assume that CephObjectStoreUser
		// is working and the rgw service state does not need to be inspected to
		// confirm user creation.
		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			_, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Create(ctx, &osu1, metav1.CreateOptions{})
			require.NoError(t, err)

			// user creation may be slow right after rgw start up
			osuReady := utils.Retry(120, time.Second, "CephObjectStoreUser is Ready", func() bool {
				liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveOsu.Status == nil {
					return false
				}

				return liveOsu.Status.Phase == "Ready"
			})
			require.True(t, osuReady)
		})

		t.Run(fmt.Sprintf("create obc %q with bucketOwner %q", obc1.Name, obc1.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Create(ctx, &obc1, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has bucketOwner %q set", obc1.Name, obc1.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			var liveObc *bktv1alpha1.ObjectBucketClaim
			obcBound := utils.Retry(40, time.Second, "OBC is Bound", func() bool {
				var err error
				liveObc, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == bktv1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			require.True(t, obcBound)

			// verify that bucketOwner is set on the live obc
			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], liveObc.Spec.AdditionalConfig["bucketOwner"])
		})

		t.Run(fmt.Sprintf("ob for obc %q has bucketOwner %q set", obc1.Name, obc1.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			var liveOb *bktv1alpha1.ObjectBucket
			obBound := utils.Retry(40, time.Second, "OB is Bound", func() bool {
				var err error
				liveOb, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == bktv1alpha1.ObjectBucketStatusPhaseBound
			})
			require.True(t, obBound)

			// verify that bucketOwner is set on the live ob
			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], liveOb.Spec.Connection.AdditionalState["bucketOwner"])
		})

		// the rgw admin api is used to verify bucket ownership
		t.Run("setup rgw admin api client", func(t *testing.T) {
			err, output := installer.Execute("radosgw-admin", []string{"user", "info", "--uid=dashboard-admin", fmt.Sprintf("--rgw-realm=%s", objectStore.Name)}, objectStore.Namespace)
			require.NoError(t, err)

			// extract api creds from json output
			var userInfo map[string]interface{}
			err = json.Unmarshal([]byte(output), &userInfo)
			require.NoError(t, err)

			s3AccessKey, ok := userInfo["keys"].([]interface{})[0].(map[string]interface{})["access_key"].(string)
			require.True(t, ok)
			require.NotEmpty(t, s3AccessKey)

			s3SecretKey, ok := userInfo["keys"].([]interface{})[0].(map[string]interface{})["secret_key"].(string)
			require.True(t, ok)
			require.NotEmpty(t, s3SecretKey)

			// extract rgw endpoint from k8s svc
			svc, err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Get(ctx, objectStore.Name, metav1.GetOptions{})
			require.NoError(t, err)

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
			s3Endpoint := schema + svc.Spec.ClusterIP + ":80"

			logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3Endpoint, s3AccessKey, s3SecretKey)

			adminClient, err = admin.New(s3Endpoint, s3AccessKey, s3SecretKey, httpClient)
			require.NoError(t, err)

			// verify that admin api is working
			_, err = adminClient.GetInfo(ctx)
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("bucket created with owner %q", obc1.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Spec.BucketName})
			require.NoError(t, err)

			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], bucket.Owner)
		})

		// obc should not modify pre-existing users
		t.Run(fmt.Sprintf("no user quota was set on %q", osu1.Name), func(t *testing.T) {
			liveQuota, err := adminClient.GetUserQuota(ctx, admin.QuotaSpec{UID: osu1.Name})
			require.NoError(t, err)

			assert.False(t, *liveQuota.Enabled)
			assert.Equal(t, int64(-1), *liveQuota.MaxSize)
			assert.Equal(t, int64(-1), *liveQuota.MaxObjects)
		})

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu2.Name), func(t *testing.T) {
			// create user2
			_, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Create(ctx, &osu2, metav1.CreateOptions{})
			require.NoError(t, err)

			osuReady := utils.Retry(40, time.Second, "CephObjectStoreUser is Ready", func() bool {
				liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu2.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveOsu.Status == nil {
					return false
				}

				return liveOsu.Status.Phase == "Ready"
			})
			require.True(t, osuReady)
		})

		t.Run(fmt.Sprintf("update obc %q to bucketOwner %q", obc1.Name, osu2.Name), func(t *testing.T) {
			// update obc bucketOwner
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveObc.Spec.AdditionalConfig["bucketOwner"] = osu2.Name

			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Update(ctx, liveObc, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has bucketOwner %q set", obc1.Name, osu2.Name), func(t *testing.T) {
			// obc .Status.Phase does not appear to change when updating the obc
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			// verify that bucketOwner is set on the live obc
			assert.Equal(t, osu2.Name, liveObc.Spec.AdditionalConfig["bucketOwner"])
		})

		t.Run(fmt.Sprintf("ob for obc %q has bucketOwner %q set", obc1.Name, osu2.Name), func(t *testing.T) {
			// ob .Status.Phase does not appear to change when updating the obc
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			var liveOb *bktv1alpha1.ObjectBucket
			inSync := utils.Retry(40, time.Second, "OB is Bound", func() bool {
				var err error
				liveOb, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return osu2.Name == liveOb.Spec.Connection.AdditionalState["bucketOwner"]
			})
			require.True(t, inSync)

			// verify that bucketOwner is set on the live ob
			assert.Equal(t, osu2.Name, liveOb.Spec.Connection.AdditionalState["bucketOwner"])
		})

		t.Run(fmt.Sprintf("bucket owner changed to %q", osu2.Name), func(t *testing.T) {
			var bucket admin.Bucket
			ownerSync := utils.Retry(40, time.Second, "bucket owner in sync", func() bool {
				var err error
				bucket, err = adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Name})
				if err != nil {
					return false
				}

				return bucket.Owner == osu2.Name
			})
			assert.True(t, ownerSync)
			assert.Equal(t, osu2.Name, bucket.Owner)
		})

		// obc should not modify pre-existing users
		t.Run(fmt.Sprintf("no user quota was set on %q", osu2.Name), func(t *testing.T) {
			liveQuota, err := adminClient.GetUserQuota(ctx, admin.QuotaSpec{UID: osu2.Name})
			require.NoError(t, err)

			assert.False(t, *liveQuota.Enabled)
			assert.Equal(t, int64(-1), *liveQuota.MaxSize)
			assert.Equal(t, int64(-1), *liveQuota.MaxObjects)
		})

		t.Run(fmt.Sprintf("remove obc %q bucketOwner", obc1.Name), func(t *testing.T) {
			// update/remove obc bucketOwner
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveObc.Spec.AdditionalConfig = map[string]string{}

			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Update(ctx, liveObc, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has no bucketOwner", obc1.Name), func(t *testing.T) {
			// verify that bucketOwner is unset on the live obc
			notSet := utils.Retry(40, time.Second, "bucketOwner not set", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				_, ok := liveObc.Spec.AdditionalConfig["bucketOwner"]
				return !ok
			})
			assert.True(t, notSet)
		})

		t.Run(fmt.Sprintf("ob for obc %q has no bucketOwner", obc1.Name), func(t *testing.T) {
			// ob .Status.Phase does not appear to change when updating the obc
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			notSet := utils.Retry(40, time.Second, "bucketOwner not set", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				_, ok := liveOb.Spec.Connection.AdditionalState["bucketOwner"]
				return !ok
			})
			assert.True(t, notSet)
		})

		// the ob should retain the existing owner and not revert to a generated user
		t.Run(fmt.Sprintf("bucket owner is still %q", osu2.Name), func(t *testing.T) {
			var bucket admin.Bucket
			ownerSync := utils.Retry(40, time.Second, "bucket owner in sync", func() bool {
				var err error
				bucket, err = adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Name})
				if err != nil {
					return false
				}

				return bucket.Owner == osu2.Name
			})
			assert.True(t, ownerSync)
			assert.Equal(t, osu2.Name, bucket.Owner)
		})

		// this covers setting bucketOwner on an obc initially created without an explicit owner
		t.Run(fmt.Sprintf("update obc %q to bucketOwner %q", obc1.Name, osu1.Name), func(t *testing.T) {
			// update obc bucketOwner
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveObc.Spec.AdditionalConfig = map[string]string{"bucketOwner": osu1.Name}

			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Update(ctx, liveObc, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has bucketOwner %q set", obc1.Name, osu1.Name), func(t *testing.T) {
			// obc .Status.Phase does not appear to change when updating the obc
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			// verify that bucketOwner is set on the live obc
			assert.Equal(t, osu1.Name, liveObc.Spec.AdditionalConfig["bucketOwner"])
		})

		t.Run(fmt.Sprintf("ob for obc %q has bucketOwner %q set", obc1.Name, osu1.Name), func(t *testing.T) {
			// ob .Status.Phase does not appear to change when updating the obc
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			var liveOb *v1alpha1.ObjectBucket
			inSync := utils.Retry(40, time.Second, "OB is Bound", func() bool {
				var err error
				liveOb, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return osu1.Name == liveOb.Spec.Connection.AdditionalState["bucketOwner"]
			})
			require.True(t, inSync)

			// verify that bucketOwner is set on the live ob
			assert.Equal(t, osu1.Name, liveOb.Spec.Connection.AdditionalState["bucketOwner"])
		})

		t.Run(fmt.Sprintf("bucket owner changed to %q", osu1.Name), func(t *testing.T) {
			var bucket admin.Bucket
			ownerSync := utils.Retry(40, time.Second, "bucket owner in sync", func() bool {
				var err error
				bucket, err = adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Name})
				if err != nil {
					return false
				}

				return bucket.Owner == osu1.Name
			})
			assert.True(t, ownerSync)
			assert.Equal(t, osu1.Name, bucket.Owner)
		})

		t.Run(fmt.Sprintf("bucket owner changed to %q", osu1.Name), func(t *testing.T) {
			var bucket admin.Bucket
			ownerSync := utils.Retry(40, time.Second, "bucket owner in sync", func() bool {
				var err error
				bucket, err = adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Name})
				if err != nil {
					return false
				}

				return bucket.Owner == osu1.Name
			})
			assert.True(t, ownerSync)
			assert.Equal(t, osu1.Name, bucket.Owner)
		})

		t.Run(fmt.Sprintf("create obc %q with bucketOwner %q", obc2.Name, obc2.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Create(ctx, &obc2, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has bucketOwner %q set", obc2.Name, obc2.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			var liveObc *v1alpha1.ObjectBucketClaim
			obcBound := utils.Retry(40, time.Second, "OBC is Bound", func() bool {
				var err error
				liveObc, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc2.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			require.True(t, obcBound)

			// verify that bucketOwner is set on the live obc
			assert.Equal(t, obc2.Spec.AdditionalConfig["bucketOwner"], liveObc.Spec.AdditionalConfig["bucketOwner"])
		})

		t.Run(fmt.Sprintf("ob for obc %q has bucketOwner %q set", obc2.Name, obc2.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc2.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			var liveOb *v1alpha1.ObjectBucket
			obBound := utils.Retry(40, time.Second, "OB is Bound", func() bool {
				var err error
				liveOb, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			require.True(t, obBound)

			// verify that bucketOwner is set on the live ob
			assert.Equal(t, obc2.Spec.AdditionalConfig["bucketOwner"], liveOb.Spec.Connection.AdditionalState["bucketOwner"])
		})

		t.Run(fmt.Sprintf("bucket %q and %q share the same owner", obc1.Spec.BucketName, obc2.Spec.BucketName), func(t *testing.T) {
			bucket1, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Spec.BucketName})
			require.NoError(t, err)

			bucket2, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc2.Spec.BucketName})
			require.NoError(t, err)

			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], bucket1.Owner)
			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], bucket2.Owner)
		})

		t.Run(fmt.Sprintf("delete obc %q", obc1.Name), func(t *testing.T) {
			// lookup ob name
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			// delete obc
			err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Delete(ctx, obc1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			absent := utils.Retry(40, time.Second, "OBC is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc1.Name, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)

			absent = utils.Retry(40, time.Second, "OB is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})

		t.Run(fmt.Sprintf("delete obc %q", obc2.Name), func(t *testing.T) {
			// lookup ob name
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc2.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			// delete obc
			err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Delete(ctx, obc2.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			absent := utils.Retry(40, time.Second, "OBC is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obc2.Name, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)

			absent = utils.Retry(40, time.Second, "OB is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})

		t.Run(fmt.Sprintf("user %q was not deleted by obc %q", osu1.Name, obc1.Name), func(t *testing.T) {
			user, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.NoError(t, err)

			assert.Equal(t, osu1.Name, user.ID)
		})

		t.Run(fmt.Sprintf("user %q was not deleted by obc %q", osu2.Name, obc1.Name), func(t *testing.T) {
			user, err := adminClient.GetUser(ctx, admin.User{ID: osu2.Name})
			require.NoError(t, err)

			assert.Equal(t, osu2.Name, user.ID)
		})

		// test obc creation with bucketOwner set to a non-existent user, which should fail
		// "failure" means the obc remains in Pending state
		t.Run(fmt.Sprintf("create obc %q with non-existent bucketOwner %q", obcBogusOwner.Name, obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Create(ctx, &obcBogusOwner, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("operator logs failed lookup for user %q", obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			selector := labels.SelectorFromSet(labels.Set{
				"app": "rook-ceph-operator",
			})
			text := `error provisioning bucket: unable to get user \"test-bucket-owner-bogus-user\" creds: Ceph object user \"test-bucket-owner-bogus-user\" not found: NoSuchUser`

			err := WaitForPodLogContainingText(k8sh, "object-ns-system", &selector, text, 10*time.Second)
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q stays Pending", obcBogusOwner.Name), func(t *testing.T) {
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcBogusOwner.Namespace).Get(ctx, obcBogusOwner.Name, metav1.GetOptions{})
			require.NoError(t, err)

			assert.True(t, v1alpha1.ObjectBucketClaimStatusPhasePending == liveObc.Status.Phase)
		})

		t.Run(fmt.Sprintf("user %q does not exist", obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			_, err := adminClient.GetUser(ctx, admin.User{ID: obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]})
			require.ErrorIs(t, err, admin.ErrNoSuchUser)
		})

		t.Run(fmt.Sprintf("delete obc %q", obcBogusOwner.Name), func(t *testing.T) {
			// lookup ob name
			liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obcBogusOwner.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			// delete obc
			err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Delete(ctx, obcBogusOwner.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			absent := utils.Retry(40, time.Second, "OBC is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Get(ctx, obcBogusOwner.Name, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)

			absent = utils.Retry(40, time.Second, "OB is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu2.Name), func(t *testing.T) {
			err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Delete(ctx, osu2.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			absent := utils.Retry(40, time.Second, "CephObjectStoreUser is absent", func() bool {
				_, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu2.Name, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Delete(ctx, osu1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			absent := utils.Retry(40, time.Second, "CephObjectStoreUser is absent", func() bool {
				_, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})

		t.Run(fmt.Sprintf("delete sc %q", storageClass.Name), func(t *testing.T) {
			err := k8sh.Clientset.StorageV1().StorageClasses().Delete(ctx, storageClass.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete svc %q", objectStoreSvc.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Delete(ctx, objectStoreSvc.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete CephObjectStore %q", objectStore.Name), func(t *testing.T) {
			err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Delete(ctx, objectStore.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete ns %q", ns.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})
	})
}
