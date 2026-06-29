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

	"github.com/aws/aws-sdk-go-v2/service/sns"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/secrets"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

func checkStatusSecrets(t *testing.T, k8sh *utils.K8sHelper, bt *cephv1.CephBucketTopic, expected ...*corev1.Secret) {
	secrets.RequireStatusRefs(t, k8sh,
		k8sh.RookClientset.CephV1().CephBucketTopics(bt.Namespace), bt.Name,
		fmt.Sprintf("cephBucketTopic %q has .status.secrets set", bt.Name),
		func(topic *cephv1.CephBucketTopic) []cephv1.SecretReference {
			if topic.Status == nil {
				return nil
			}
			return topic.Status.Secrets
		},
		expected...)
}

func checkRgwTopicEndpoint(t *testing.T, snsClient *sns.Client, arn, user, pass string) {
	t.Run(fmt.Sprintf("rgw topic arn %q has basic auth set", arn), func(t *testing.T) {
		ctx := t.Context()

		var uri *url.URL

		wait4.RequireEventually(ctx, t, wait4.TimeoutShort, "rgw topic basic auth in sync", func(ctx context.Context) error {
			topicAttrs, err := snsClient.GetTopicAttributes(ctx, &sns.GetTopicAttributesInput{
				TopicArn: &arn,
			})
			if err != nil {
				return err
			}

			// the sns endpoint attributes are returned as JSON
			var endpointJSON map[string]interface{}
			if err := json.Unmarshal([]byte(topicAttrs.Attributes["EndPoint"]), &endpointJSON); err != nil {
				return err
			}

			addr, ok := endpointJSON["EndpointAddress"].(string)
			if !ok {
				return fmt.Errorf("topic endpoint has no EndpointAddress string: %v", endpointJSON["EndpointAddress"])
			}

			parsed, err := url.Parse(addr)
			if err != nil {
				return err
			}

			uriPassword, _ := parsed.User.Password()
			if user != parsed.User.Username() || pass != uriPassword {
				return fmt.Errorf("topic basic auth not yet in sync for user %q", parsed.User.Username())
			}

			// capture the matching sample for the post-wait assertions
			uri = parsed
			return nil
		})

		assert.Equal(t, "kafka", uri.Scheme)
		assert.Equal(t, "kafka.example.com:9094", uri.Host)
	})
}

// kvSecret returns a Secret holding a single key/value pair.
func kvSecret(ns, name, key, value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
}

// secretKeySelector returns a selector for key within the named secret.
func secretKeySelector(name, key string) *corev1.SecretKeySelector {
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: name,
		},
		Key: key,
	}
}

const Namespace = "test-topickafka"

func TestBucketTopicKafka(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		secret1 = kvSecret(ns.Name, defaultName+"-secret1", "user-name", "kafka-user1")
		secret2 = kvSecret(ns.Name, defaultName+"-secret2", "password", "kafka-pass2")
		secret3 = kvSecret(ns.Name, defaultName+"-secret3", "user-name", "kafka-user3")
		secret4 = kvSecret(ns.Name, defaultName+"-secret4", "password", "kafka-pass4")

		storageClass = fixture.StorageClass(defaultName, objectStore)

		bt1 = &cephv1.CephBucketTopic{
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
						URI:               "kafka://kafka.example.com:9094",
						AckLevel:          "broker",
						UseSSL:            false,
						UserSecretRef:     secretKeySelector(secret1.Name, "user-name"),
						PasswordSecretRef: secretKeySelector(secret2.Name, "password"),
					},
				},
			},
		}

		bn1 = &cephv1.CephBucketNotification{
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

		obc1 = &bktv1alpha1.ObjectBucketClaim{
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

		snsClient = store.SnsClient()

		btClient     = k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name)
		bnClient     = k8sh.RookClientset.CephV1().CephBucketNotifications(ns.Name)
		obcClient    = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
		obClient     = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets()
		secretClient = k8sh.Clientset.CoreV1().Secrets(ns.Name)
	)

	t.Run("CephBucketTopic kafka", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		for _, secret := range []*corev1.Secret{secret1, secret2, secret3, secret4} {
			t.Run(fmt.Sprintf("create secret %q", secret.Name), func(t *testing.T) {
				_, err := secretClient.Create(ctx, secret, metav1.CreateOptions{})
				require.NoError(t, err)
			})
		}

		// the sc, obc, and CephBucketNotification are essentially unused for testing but are created for completeness
		fixture.RequireStorageClass(t, k8sh, storageClass)

		t.Run(fmt.Sprintf("create obc %q", obc1.Name), func(t *testing.T) {
			_, err := obcClient.Create(ctx, obc1, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create CephBucketTopic %q", bt1.Name), func(t *testing.T) {
			// topic creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, btClient, bt1, wait4.BucketTopic, wait4.TimeoutMedium)
		})

		t.Run(fmt.Sprintf("create CephBucketNotification %q", bn1.Name), func(t *testing.T) {
			_, err := bnClient.Create(ctx, bn1, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		liveBt := wait4.RequireCondition(ctx, t, btClient, bt1.Name, wait4.BucketTopic, wait4.TimeoutMedium)
		checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, string(secret1.Data["user-name"]), string(secret2.Data["password"]))
		checkStatusSecrets(t, k8sh, bt1, secret1, secret2)

		t.Run("updating referenced secrets reconciles rgw topic", func(t *testing.T) {
			t.Run(fmt.Sprintf("update kafka auth on CephBucketTopic %q", bt1.Name), func(t *testing.T) {
				liveBt, err := btClient.Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveBt.Spec.Endpoint.Kafka.UserSecretRef = secretKeySelector(secret3.Name, "user-name")
				liveBt.Spec.Endpoint.Kafka.PasswordSecretRef = secretKeySelector(secret4.Name, "password")

				_, err = btClient.Update(ctx, liveBt, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			liveBt := wait4.RequireCondition(ctx, t, btClient, bt1.Name, wait4.BucketTopic, wait4.TimeoutMedium)
			checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, string(secret3.Data["user-name"]), string(secret4.Data["password"]))
			checkStatusSecrets(t, k8sh, bt1, secret3, secret4)
		})

		t.Run("removing referenced secrets reconciles rgw topic", func(t *testing.T) {
			t.Run(fmt.Sprintf("delete kafka auth on CephBucketTopic %q", bt1.Name), func(t *testing.T) {
				liveBt, err := btClient.Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveBt.Spec.Endpoint.Kafka.UserSecretRef = nil
				liveBt.Spec.Endpoint.Kafka.PasswordSecretRef = nil

				_, err = btClient.Update(ctx, liveBt, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			liveBt := wait4.RequireCondition(ctx, t, btClient, bt1.Name, wait4.BucketTopic, wait4.TimeoutMedium)
			checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, "", "")
			checkStatusSecrets(t, k8sh, bt1)
		})

		t.Run("adding new referenced secrets reconciles rgw topic", func(t *testing.T) {
			t.Run(fmt.Sprintf("add kafka auth on CephBucketTopic %q", bt1.Name), func(t *testing.T) {
				liveBt, err := btClient.Get(ctx, bt1.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveBt.Spec.Endpoint.Kafka.UserSecretRef = secretKeySelector(secret3.Name, "user-name")
				liveBt.Spec.Endpoint.Kafka.PasswordSecretRef = secretKeySelector(secret4.Name, "password")

				_, err = btClient.Update(ctx, liveBt, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			liveBt := wait4.RequireCondition(ctx, t, btClient, bt1.Name, wait4.BucketTopic, wait4.TimeoutMedium)
			checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, string(secret3.Data["user-name"]), string(secret4.Data["password"]))
			checkStatusSecrets(t, k8sh, bt1, secret3, secret4)
		})

		t.Run("updating secrets reconciles rgw topic", func(t *testing.T) {
			t.Run(fmt.Sprintf("update secret %q data", secret3.Name), func(t *testing.T) {
				liveSecret, err := secretClient.Get(ctx, secret3.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveSecret.Data = map[string][]byte{
					"user-name": []byte("kafka-user3-updated"),
				}

				_, err = secretClient.Update(ctx, liveSecret, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("update secret %q data", secret4.Name), func(t *testing.T) {
				liveSecret, err := secretClient.Get(ctx, secret4.Name, metav1.GetOptions{})
				require.NoError(t, err)

				liveSecret.Data = map[string][]byte{
					"password": []byte("kafka-pass4-updated"),
				}

				_, err = secretClient.Update(ctx, liveSecret, metav1.UpdateOptions{})
				require.NoError(t, err)
			})

			liveBt := wait4.RequireCondition(ctx, t, btClient, bt1.Name, wait4.BucketTopic, wait4.TimeoutMedium)

			liveSecret3, err := secretClient.Get(ctx, secret3.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveSecret4, err := secretClient.Get(ctx, secret4.Name, metav1.GetOptions{})
			require.NoError(t, err)

			checkRgwTopicEndpoint(t, snsClient, *liveBt.Status.ARN, string(liveSecret3.Data["user-name"]), string(liveSecret4.Data["password"]))
			checkStatusSecrets(t, k8sh, bt1, liveSecret3, liveSecret4)
		})

		t.Run("deleting a referenced secret triggers a reconcile, which fails", func(t *testing.T) {
			t.Run(fmt.Sprintf("delete secret %q", secret4.Name), func(t *testing.T) {
				err := secretClient.Delete(ctx, secret4.Name, metav1.DeleteOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("CephBucketTopic %q has phase ReconcileFailed", bt1.Name), func(t *testing.T) {
				wait4.RequireCondition(ctx, t, btClient, bt1.Name, wait4.BucketTopicPhase(string(cephv1.ReconcileFailed)), wait4.TimeoutShort)
			})

			t.Run(fmt.Sprintf("create secret %q", secret4.Name), func(t *testing.T) {
				_, err := secretClient.Create(ctx, secret4, metav1.CreateOptions{})
				require.NoError(t, err)
			})

			wait4.RequireCondition(ctx, t, btClient, bt1.Name, wait4.BucketTopic, wait4.TimeoutMedium)
		})

		t.Run(fmt.Sprintf("delete CephBucketNotification %q", bn1.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, bnClient, bn1.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("no CephBucketNotification(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := bnClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, list.Items, 0)
		})

		t.Run(fmt.Sprintf("delete CephBucketTopic %q", bt1.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, btClient, bt1.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("no CephBucketTopic(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := btClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, list.Items, 0)
		})

		t.Run(fmt.Sprintf("delete obc %q", obc1.Name), func(t *testing.T) {
			// lookup ob name
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			// delete obc; the backing ob is garbage-collected by the provisioner
			wait4.AssertDelete(ctx, t, obcClient, obc1.Name, wait4.TimeoutShort)
			wait4.AssertAbsent(ctx, t, obClient, obName, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("no obc(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := obcClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, list.Items, 0)
		})

		t.Run("CephBucketTopic deletion did not remove any secrets", func(t *testing.T) {
			t.Run(fmt.Sprintf("secret %q still exists", secret1.Name), func(t *testing.T) {
				_, err := secretClient.Get(ctx, secret1.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("secret %q still exists", secret2.Name), func(t *testing.T) {
				_, err := secretClient.Get(ctx, secret2.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("secret %q still exists", secret3.Name), func(t *testing.T) {
				_, err := secretClient.Get(ctx, secret3.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("secret %q still exists", secret4.Name), func(t *testing.T) {
				_, err := secretClient.Get(ctx, secret4.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})
		})

		t.Run(fmt.Sprintf("delete secret %q", secret4.Name), func(t *testing.T) {
			err := secretClient.Delete(ctx, secret4.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete secret %q", secret3.Name), func(t *testing.T) {
			err := secretClient.Delete(ctx, secret3.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete secret %q", secret2.Name), func(t *testing.T) {
			err := secretClient.Delete(ctx, secret2.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete secret %q", secret1.Name), func(t *testing.T) {
			err := secretClient.Delete(ctx, secret1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("no secrets in ns %q", ns.Name), func(t *testing.T) {
			secrets, err := secretClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, secrets.Items, 0)
		})
	})
}
