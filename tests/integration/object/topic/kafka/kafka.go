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

package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/coreos/pkg/capnslog"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	utilsns "github.com/rook/rook/tests/integration/object/util/sns"
)

var (
	defaultName = "test-topickafka"

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

	secret1 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret1",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"user-name": []byte("kafka-user1"),
		},
	}

	secret2 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret2",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"password": []byte("kafka-pass2"),
		},
	}

	secret3 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret3",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"user-name": []byte("kafka-user3"),
		},
	}

	secret4 = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-secret4",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"password": []byte("kafka-pass4"),
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

	obc1 = bktv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-obc1",
			Namespace: ns.Name,
			Labels: map[string]string{
				"bucket-notification-" + bn1.Name: bn1.Name,
			},
		},
		Spec: bktv1alpha1.ObjectBucketClaimSpec{
			BucketName:       defaultName + "-obc1",
			StorageClassName: storageClass.Name,
		},
	}

	bt1 = cephv1.CephBucketTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-topic1",
			Namespace: ns.Name,
		},
		Spec: cephv1.BucketTopicSpec{
			ObjectStoreName:      objectStore.Name,
			ObjectStoreNamespace: objectStore.Namespace,
			Persistent:           false,
			Endpoint: cephv1.TopicEndpointSpec{
				Kafka: &cephv1.KafkaEndpointSpec{
					URI:      "kafka://kafka.example.com:9094",
					AckLevel: "broker",
					UseSSL:   false,
					UserRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret1.Name,
						},
						Key: "user-name",
					},
					PassRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret2.Name,
						},
						Key: "password",
					},
				},
			},
		},
	}

	bn1 = cephv1.CephBucketNotification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-notification1",
			Namespace: ns.Name,
		},
		Spec: cephv1.BucketNotificationSpec{
			Topic: bt1.Name,
			Events: []cephv1.BucketNotificationEvent{
				"s3:ObjectCreated:*",
			},
		},
	}
)

func checkStatusSecrets(t *testing.T, k8sh *utils.K8sHelper, bt cephv1.CephBucketTopic, expectedSecrets []*corev1.Secret) {
	t.Run(fmt.Sprintf("cephBucketTopic %q has .status.secrets set", bt.Name), func(t *testing.T) {
		ctx := context.TODO()

		liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt.Name, metav1.GetOptions{})
		require.NoError(t, err)

		require.NotNil(t, liveBt.Status)
		assert.Len(t, liveBt.Status.Secrets, len(expectedSecrets))

		for _, secret := range expectedSecrets {
			secretRef, err := func(secretName string, keys []cephv1.SecretReference) (cephv1.SecretReference, error) {
				for _, secretRef := range keys {
					if secretRef.Name == secretName {
						return secretRef, nil
					}
				}
				return cephv1.SecretReference{}, fmt.Errorf("secretReference for secret %q not found in CephObjectStoreUser.status.keys", secret.Name)
			}(secret.Name, liveBt.Status.Secrets)
			require.NoError(t, err)

			// fetch the live secret for UID and ResourceVersion
			liveSecret, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Get(ctx, secret.Name, metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, liveSecret.Name, secretRef.Name)
			assert.Equal(t, liveSecret.Namespace, secretRef.Namespace)
			assert.Equal(t, liveSecret.UID, secretRef.UID)
			assert.Equal(t, liveSecret.ResourceVersion, secretRef.ResourceVersion)
		}
	})
}

func checkRgwTopicEndpoint(t *testing.T, snsClient *sns.Client, arn, user, pass string) {
	t.Run(fmt.Sprintf("rgw topic arn %q has basic auth set", arn), func(t *testing.T) {
		ctx := context.TODO()

		var uri *url.URL

		// assume that the .Phase doesn't change when updating keys
		inSync := utils.Retry(40, time.Second, "rgw topic basic auth in sync", func() bool {
			topicAttrs, err := snsClient.GetTopicAttributes(ctx, &sns.GetTopicAttributesInput{
				TopicArn: &arn,
			})
			require.NoError(t, err)

			// the sns endpoint attributes are returned as JSON
			var endpointJSON map[string]interface{}
			err = json.Unmarshal([]byte(topicAttrs.Attributes["EndPoint"]), &endpointJSON)
			require.NoError(t, err)

			uri, err = url.Parse(string(endpointJSON["EndpointAddress"].(string)))
			require.NoError(t, err)

			uriPassword, _ := uri.User.Password()

			return user == uri.User.Username() && pass == uriPassword
		})
		require.True(t, inSync)

		assert.Equal(t, "kafka", uri.Scheme)
		assert.Equal(t, "kafka.example.com:9094", uri.Host)
		assert.Equal(t, user, uri.User.Username())
		uriPassword, _ := uri.User.Password()
		assert.Equal(t, pass, uriPassword)
	})
}

func TestBucketTopicKafka(t *testing.T, k8sh *utils.K8sHelper, installer *installer.CephInstaller, logger *capnslog.PackageLogger, tlsEnable bool) {
	ctx := context.TODO()
	var snsClient *sns.Client

	t.Run("CephBucketTopic kafka", func(t *testing.T) {
		if tlsEnable {
			// Skip testing with and without TLS to reduce test time
			t.Skip("skipping test for TLS enabled clusters")
		}

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

				return liveOs.Status.Phase == cephv1.ConditionReady
			})
			require.True(t, osReady)
		})

		t.Run(fmt.Sprintf("create svc %q", objectStoreSvc.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Create(ctx, objectStoreSvc, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret1.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret1, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret2.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret2, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret3.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret3, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create secret %q", secret4.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Create(ctx, secret4, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		// the sc, obc, and CephBucketNotification are essentially unused for testing but are created for completeness
		t.Run(fmt.Sprintf("create sc %q", storageClass.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.StorageV1().StorageClasses().Create(ctx, storageClass, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create obc %q", obc1.Name), func(t *testing.T) {
			_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).Create(ctx, &obc1, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create CephBucketTopic %q", bt1.Name), func(t *testing.T) {
			_, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Create(ctx, &bt1, metav1.CreateOptions{})
			require.NoError(t, err)

			// user creation may be slow right after rgw start up
			btReady := utils.Retry(120, time.Second, "CephBucketTopic is Ready", func() bool {
				liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveBt.Status == nil {
					return false
				}

				return liveBt.Status.Phase == string(cephv1.ConditionReady)
			})
			require.True(t, btReady)
		})

		t.Run(fmt.Sprintf("create CephBucketNotification %q", bn1.Name), func(t *testing.T) {
			_, err := k8sh.RookClientset.CephV1().CephBucketNotifications(ns.Name).Create(ctx, &bn1, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run("setup sns client", func(t *testing.T) {
			var err error
			snsClient, err = utilsns.NewClient(objectStore, objectStoreSvc, k8sh, installer, tlsEnable)
			require.NoError(t, err)
		})

		{
			liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, string(secret1.Data["user-name"]), string(secret2.Data["password"]))

			secrets := []*corev1.Secret{secret1, secret2}

			checkStatusSecrets(t, k8sh, bt1, secrets)
		}

		t.Run("updating referenced secrets updates rgw topic", func(t *testing.T) {
			t.Run(fmt.Sprintf("update kafka auth on CephBucketTopic %q", bt1.Name), func(t *testing.T) {
				liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveBt.Spec.Endpoint.Kafka.UserRef = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret3.Name,
					},
					Key: "user-name",
				}

				liveBt.Spec.Endpoint.Kafka.PassRef = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret4.Name,
					},
					Key: "password",
				}

				_, err = k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Update(ctx, liveBt, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			{
				liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, string(secret3.Data["user-name"]), string(secret4.Data["password"]))

				secrets := []*corev1.Secret{secret3, secret4}

				checkStatusSecrets(t, k8sh, bt1, secrets)
			}
		})

		t.Run("removing referenced secrets updates rgw topic", func(t *testing.T) {
			t.Run(fmt.Sprintf("delete kafka auth on CephBucketTopic %q", bt1.Name), func(t *testing.T) {
				liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveBt.Spec.Endpoint.Kafka.UserRef = nil
				liveBt.Spec.Endpoint.Kafka.PassRef = nil

				_, err = k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Update(ctx, liveBt, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			{
				liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, "", "")

				secrets := []*corev1.Secret{}

				checkStatusSecrets(t, k8sh, bt1, secrets)
			}
		})

		// XXX WIP
		t.Run("adding referenced secrets updates rgw topic", func(t *testing.T) {
			t.Run(fmt.Sprintf("add kafka auth on CephBucketTopic %q", bt1.Name), func(t *testing.T) {
				liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveBt.Spec.Endpoint.Kafka.UserRef = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret3.Name,
					},
					Key: "user-name",
				}

				liveBt.Spec.Endpoint.Kafka.PassRef = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret4.Name,
					},
					Key: "password",
				}

				_, err = k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Update(ctx, liveBt, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			{
				liveBt, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, string(secret3.Data["user-name"]), string(secret4.Data["password"]))

				secrets := []*corev1.Secret{secret3, secret4}

				checkStatusSecrets(t, k8sh, bt1, secrets)
			}
		})
		/*
			// updating a secret already referenced by a CephObjectStoreUser should trigger a reconcile
			t.Run(fmt.Sprintf("update secret %q data", secret1.Name), func(t *testing.T) {
				liveSecret, err := k8sh.Clientset.CoreV1().Secrets(secret1.Namespace).Get(ctx, secret1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveSecret.Data = map[string][]byte{
					"AWS_ACCESS_KEY_ID":     []byte("foo"),
					"AWS_SECRET_ACCESS_KEY": []byte("bar"),
				}

				_, err = k8sh.Clientset.CoreV1().Secrets(secret1.Namespace).Update(ctx, liveSecret, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			{
				// fetch secret that we modified
				liveSecret, err := k8sh.Clientset.CoreV1().Secrets(secret1.Namespace).Get(ctx, secret1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				secrets := []*corev1.Secret{liveSecret, secret2, secret3, secret4, secret5}

				checkRgwUserKeys(t, adminClient, osu1, secrets, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
				checkStatusKeys(t, k8sh, osu1, secrets)
			}

			// deleting a secret referenced by a CephObjectStoreUser should trigger a reconcile (which fails)
			t.Run(fmt.Sprintf("delete secret %q", secret1.Name), func(t *testing.T) {
				err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Delete(ctx, secret1.Name, metav1.DeleteOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("CephObjectStoreUser %q has phase ReconcileFailed", osu1.Name), func(t *testing.T) {
				// user creation may be slow right after rgw start up
				osuReady := utils.Retry(40, time.Second, "CephObjectStoreUser is ReconcileFailed", func() bool {
					liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
					if err != nil {
						return false
					}

					if liveOsu.Status == nil {
						return false
					}

					return liveOsu.Status.Phase == string(cephv1.ReconcileFailed)
				})
				require.True(t, osuReady)
			})
		*/

		t.Run(fmt.Sprintf("delete CephBucketNotification %q", bn1.Name), func(t *testing.T) {
			err := k8sh.RookClientset.CephV1().CephBucketNotifications(ns.Name).Delete(ctx, bn1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			absent := utils.Retry(40, time.Second, "CephBucketNotification is absent", func() bool {
				_, err := k8sh.RookClientset.CephV1().CephBucketNotifications(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})

		t.Run(fmt.Sprintf("no CephBucketNotification(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := k8sh.RookClientset.CephV1().CephBucketNotifications(ns.Name).List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, list.Items, 0)
		})

		t.Run(fmt.Sprintf("delete CephBucketTopics %q", bt1.Name), func(t *testing.T) {
			err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Delete(ctx, bt1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			absent := utils.Retry(40, time.Second, "CephBucketTopic is absent", func() bool {
				_, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).Get(ctx, bt1.Name, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})

		t.Run(fmt.Sprintf("no CephBucketTopic(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name).List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, list.Items, 0)
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

		t.Run(fmt.Sprintf("no obc(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name).List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, list.Items, 0)
		})

		t.Run("CephBucketTopic deletion did not remove any secrets", func(t *testing.T) {
			t.Run(fmt.Sprintf("secret %q still exists", secret1.Name), func(t *testing.T) {
				_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Get(ctx, secret1.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("secret %q still exists", secret2.Name), func(t *testing.T) {
				_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Get(ctx, secret2.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("secret %q still exists", secret3.Name), func(t *testing.T) {
				_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Get(ctx, secret3.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("secret %q still exists", secret4.Name), func(t *testing.T) {
				_, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Get(ctx, secret4.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})
		})

		t.Run(fmt.Sprintf("delete sc %q", storageClass.Name), func(t *testing.T) {
			err := k8sh.Clientset.StorageV1().StorageClasses().Delete(ctx, storageClass.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete secret %q", secret4.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Delete(ctx, secret4.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete secret %q", secret3.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Delete(ctx, secret3.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete secret %q", secret2.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Delete(ctx, secret2.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete secret %q", secret1.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Secrets(ns.Name).Delete(ctx, secret1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("no secrets in ns %q", ns.Name), func(t *testing.T) {
			secrets, err := k8sh.Clientset.CoreV1().Secrets(ns.Name).List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, secrets.Items, 0)
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
