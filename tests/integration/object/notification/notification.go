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

package notification

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const sinkPort int32 = 8080

const (
	putEvent    = "ObjectCreated:Put"
	deleteEvent = "ObjectRemoved:Delete"
)

// httpTopic builds a CephBucketTopic that delivers to an HTTP endpoint.
func httpTopic(name, namespace string, store *cephv1.CephObjectStore, uri string) *cephv1.CephBucketTopic {
	return &cephv1.CephBucketTopic{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: cephv1.BucketTopicSpec{
			ObjectStoreName:      store.Name,
			ObjectStoreNamespace: store.Namespace,
			Endpoint:             cephv1.TopicEndpointSpec{HTTP: &cephv1.HTTPEndpointSpec{URI: uri}},
		},
	}
}

// bucketNotification builds a CephBucketNotification for object put and delete events.
func bucketNotification(name, namespace, topic string) *cephv1.CephBucketNotification {
	return &cephv1.CephBucketNotification{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: cephv1.BucketNotificationSpec{
			Topic:  topic,
			Events: []cephv1.BucketNotificationEvent{"s3:ObjectCreated:Put", "s3:ObjectRemoved:Delete"},
		},
	}
}

// notificationLabel is the OBC label the operator watches to wire a notification to a bucket.
func notificationLabel(notification string) string {
	return "bucket-notification-" + notification
}

// notificationOBC builds an OBC labelled to subscribe its bucket to notification.
func notificationOBC(name, namespace, storageClass, notification string) *bktv1alpha1.ObjectBucketClaim {
	return &bktv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{notificationLabel(notification): notification},
		},
		Spec: bktv1alpha1.ObjectBucketClaimSpec{
			BucketName:       name,
			StorageClassName: storageClass,
		},
	}
}

const Namespace = "test-notification"

func TestBucketNotification(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		sinkName = defaultName + "-sink"
		sinkSel  = labels.SelectorFromSet(labels.Set{"app": sinkName})
		sinkURI  = fmt.Sprintf("http://%s.%s:%d", sinkName, ns.Name, sinkPort)

		storageClass = obc.StorageClass(defaultName, objectStore)

		topic        = httpTopic(defaultName+"-topic", ns.Name, objectStore, sinkURI)
		notification = bucketNotification(defaultName+"-notification", ns.Name, topic.Name)
		obc1         = notificationOBC(defaultName+"-obc", ns.Name, storageClass.Name, notification.Name)

		body        = "test bucket notification payload"
		contentType = "text/plain"
		key1        = "obj1"
		key2        = "obj2"

		btClient  = k8sh.RookClientset.CephV1().CephBucketTopics(ns.Name)
		bnClient  = k8sh.RookClientset.CephV1().CephBucketNotifications(ns.Name)
		obcClient = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
	)

	t.Run("CephBucketNotification http", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		requireHTTPSink(ctx, t, k8sh, ns.Name, sinkName)

		fixture.RequireStorageClass(t, k8sh, storageClass)

		t.Run(fmt.Sprintf("create CephBucketTopic %q", topic.Name), func(t *testing.T) {
			// topic creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, btClient, topic, wait4.BucketTopic, wait4.TimeoutMedium)
		})

		t.Run(fmt.Sprintf("create CephBucketNotification %q", notification.Name), func(t *testing.T) {
			_, err := bnClient.Create(ctx, notification, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		obc.RequireBound(ctx, t, k8sh, obc1)
		s3agent := obc.NewS3Agent(ctx, t, k8sh, store, ns.Name, obc1.Name)

		t.Run(fmt.Sprintf("notification %q is configured on bucket %q", notification.Name, obc1.Spec.BucketName), func(t *testing.T) {
			requireBucketNotification(ctx, t, s3agent, obc1.Spec.BucketName, notification.Name, true)
		})

		t.Run(fmt.Sprintf("put object %q delivers a notification", key1), func(t *testing.T) {
			_, err := s3agent.PutObjectInBucket(ctx, obc1.Spec.BucketName, body, key1, contentType)
			require.NoError(t, err)

			requireNotificationReceived(ctx, t, k8sh, ns.Name, sinkSel, putEvent, key1)
			assertNotificationNotReceived(ctx, t, k8sh, ns.Name, sinkSel, deleteEvent, key1)
		})

		t.Run(fmt.Sprintf("delete object %q delivers a notification", key1), func(t *testing.T) {
			_, err := s3agent.DeleteObjectInBucket(ctx, obc1.Spec.BucketName, key1)
			require.NoError(t, err)

			requireNotificationReceived(ctx, t, k8sh, ns.Name, sinkSel, deleteEvent, key1)
			assertNotificationNotReceived(ctx, t, k8sh, ns.Name, sinkSel, putEvent, key1)
		})

		t.Run("an unrelated OBC label change leaves the notification configured", func(t *testing.T) {
			t.Run(fmt.Sprintf("add a non-notification label to obc %q", obc1.Name), func(t *testing.T) {
				updateOBCLabels(ctx, t, k8sh, ns.Name, obc1.Name, func(l map[string]string) {
					l["test-label"] = "test-value"
				})
			})

			requireBucketNotification(ctx, t, s3agent, obc1.Spec.BucketName, notification.Name, true)

			t.Run(fmt.Sprintf("remove the non-notification label from obc %q", obc1.Name), func(t *testing.T) {
				updateOBCLabels(ctx, t, k8sh, ns.Name, obc1.Name, func(l map[string]string) {
					delete(l, "test-label")
				})
			})

			requireBucketNotification(ctx, t, s3agent, obc1.Spec.BucketName, notification.Name, true)
		})

		t.Run("removing the notification label removes the notification from the bucket", func(t *testing.T) {
			t.Run(fmt.Sprintf("remove the notification label from obc %q", obc1.Name), func(t *testing.T) {
				updateOBCLabels(ctx, t, k8sh, ns.Name, obc1.Name, func(l map[string]string) {
					delete(l, notificationLabel(notification.Name))
				})
			})

			requireBucketNotification(ctx, t, s3agent, obc1.Spec.BucketName, notification.Name, false)
		})

		t.Run("an unsubscribed bucket delivers no notifications", func(t *testing.T) {
			t.Run(fmt.Sprintf("put object %q", key2), func(t *testing.T) {
				_, err := s3agent.PutObjectInBucket(ctx, obc1.Spec.BucketName, body, key2, contentType)
				require.NoError(t, err)

				assertNotificationNotReceived(ctx, t, k8sh, ns.Name, sinkSel, putEvent, key2)
			})

			t.Run(fmt.Sprintf("delete object %q", key2), func(t *testing.T) {
				_, err := s3agent.DeleteObjectInBucket(ctx, obc1.Spec.BucketName, key2)
				require.NoError(t, err)

				assertNotificationNotReceived(ctx, t, k8sh, ns.Name, sinkSel, deleteEvent, key2)
			})
		})

		t.Run("a notification added to an existing OBC is configured on its bucket", func(t *testing.T) {
			addTopic := httpTopic(defaultName+"-topic-add", ns.Name, objectStore, sinkURI)
			addNotification := bucketNotification(defaultName+"-notification-add", ns.Name, addTopic.Name)

			t.Run(fmt.Sprintf("create CephBucketTopic %q", addTopic.Name), func(t *testing.T) {
				wait4.RequireCreate(ctx, t, btClient, addTopic, wait4.BucketTopic, wait4.TimeoutMedium)
			})

			t.Run(fmt.Sprintf("create CephBucketNotification %q", addNotification.Name), func(t *testing.T) {
				_, err := bnClient.Create(ctx, addNotification, metav1.CreateOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("add the notification label for %q to obc %q", addNotification.Name, obc1.Name), func(t *testing.T) {
				updateOBCLabels(ctx, t, k8sh, ns.Name, obc1.Name, func(l map[string]string) {
					l[notificationLabel(addNotification.Name)] = addNotification.Name
				})
			})

			requireBucketNotification(ctx, t, s3agent, obc1.Spec.BucketName, addNotification.Name, true)

			t.Run(fmt.Sprintf("delete CephBucketNotification %q", addNotification.Name), func(t *testing.T) {
				wait4.AssertDelete(ctx, t, bnClient, addNotification.Name, wait4.TimeoutShort)
			})

			t.Run(fmt.Sprintf("delete CephBucketTopic %q", addTopic.Name), func(t *testing.T) {
				wait4.AssertDelete(ctx, t, btClient, addTopic.Name, wait4.TimeoutShort)
			})
		})

		t.Run("a notification created before its topic is configured once the topic exists", func(t *testing.T) {
			revTopic := httpTopic(defaultName+"-topic-rev", ns.Name, objectStore, sinkURI)
			revNotification := bucketNotification(defaultName+"-notification-rev", ns.Name, revTopic.Name)
			obcRev := notificationOBC(defaultName+"-obc-rev", ns.Name, storageClass.Name, revNotification.Name)

			obc.RequireBound(ctx, t, k8sh, obcRev)
			revS3 := obc.NewS3Agent(ctx, t, k8sh, store, ns.Name, obcRev.Name)

			t.Run(fmt.Sprintf("create CephBucketNotification %q before its topic", revNotification.Name), func(t *testing.T) {
				_, err := bnClient.Create(ctx, revNotification, metav1.CreateOptions{})
				require.NoError(t, err)
			})

			t.Run(fmt.Sprintf("create CephBucketTopic %q", revTopic.Name), func(t *testing.T) {
				wait4.RequireCreate(ctx, t, btClient, revTopic, wait4.BucketTopic, wait4.TimeoutMedium)
			})

			t.Run(fmt.Sprintf("notification %q is configured on bucket %q once its topic exists", revNotification.Name, obcRev.Spec.BucketName), func(t *testing.T) {
				requireBucketNotification(ctx, t, revS3, obcRev.Spec.BucketName, revNotification.Name, true)
			})

			t.Run(fmt.Sprintf("delete CephBucketNotification %q", revNotification.Name), func(t *testing.T) {
				wait4.AssertDelete(ctx, t, bnClient, revNotification.Name, wait4.TimeoutShort)
			})

			t.Run(fmt.Sprintf("delete CephBucketTopic %q", revTopic.Name), func(t *testing.T) {
				wait4.AssertDelete(ctx, t, btClient, revTopic.Name, wait4.TimeoutShort)
			})

			t.Run(fmt.Sprintf("delete obc %q", obcRev.Name), func(t *testing.T) {
				obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obcRev.Name)
			})
		})

		t.Run(fmt.Sprintf("delete obc %q", obc1.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obc1.Name)
		})

		t.Run(fmt.Sprintf("delete CephBucketNotification %q", notification.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, bnClient, notification.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete CephBucketTopic %q", topic.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, btClient, topic.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("no obc(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := obcClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			assert.Len(t, list.Items, 0)
		})

		t.Run(fmt.Sprintf("no CephBucketNotification(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := bnClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			assert.Len(t, list.Items, 0)
		})

		t.Run(fmt.Sprintf("no CephBucketTopic(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := btClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			assert.Len(t, list.Items, 0)
		})
	})
}

// updateOBCLabels applies mutate to a live OBC's labels and updates it.
func updateOBCLabels(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, namespace, name string, mutate func(map[string]string)) {
	t.Helper()

	obcClient := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace)

	liveOBC, err := obcClient.Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)

	if liveOBC.Labels == nil {
		liveOBC.Labels = map[string]string{}
	}
	mutate(liveOBC.Labels)

	_, err = obcClient.Update(ctx, liveOBC, metav1.UpdateOptions{})
	require.NoError(t, err)
}

// requireHTTPSink deploys a sample HTTP server that records the bucket
// notifications rgw pushes to it, and waits for it to be ready. It needs no
// explicit teardown: deleting the test namespace removes it.
func requireHTTPSink(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, namespace, name string) {
	t.Helper()

	podLabels := map[string]string{"app": name}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: name,
						// TODO: third party image, see https://github.com/rook/rook/issues/9741
						Image: "quay.io/jthottan/pythonwebserver:latest",
						Ports: []corev1.ContainerPort{{ContainerPort: sinkPort}},
					}},
				},
			},
		},
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.ServiceSpec{
			Selector: podLabels,
			Ports: []corev1.ServicePort{{
				Port:       sinkPort,
				TargetPort: intstr.FromInt32(sinkPort),
			}},
		},
	}

	t.Run(fmt.Sprintf("create notification sink %q", name), func(t *testing.T) {
		wait4.RequireCreate(ctx, t, k8sh.Clientset.AppsV1().Deployments(namespace), deployment,
			func(d *appsv1.Deployment) bool { return d.Status.ReadyReplicas >= 1 }, wait4.TimeoutLong)
		wait4.RequireCreate(ctx, t, k8sh.Clientset.CoreV1().Services(namespace), service, nil, wait4.TimeoutShort)
	})
}

// requireBucketNotification waits until notificationName is (want=true) or is not
// (want=false) configured on the rgw bucket, read via the S3 API.
func requireBucketNotification(ctx context.Context, t *testing.T, agent *rgw.S3Agent, bucket, notificationName string, want bool) {
	t.Helper()

	desc := fmt.Sprintf("notification %q configured=%v on bucket %q", notificationName, want, bucket)
	wait4.RequireEventually(ctx, t, wait4.TimeoutMedium, desc, func(ctx context.Context) error {
		conf, err := agent.Client.GetBucketNotificationConfiguration(ctx, &s3.GetBucketNotificationConfigurationInput{
			Bucket: &bucket,
		})
		if err != nil {
			return err
		}

		found := false
		for _, tc := range conf.TopicConfigurations {
			if tc.Id != nil && *tc.Id == notificationName {
				found = true
				break
			}
		}
		if found != want {
			return fmt.Errorf("notification %q configured=%v, want %v", notificationName, found, want)
		}
		return nil
	})
}

// requireNotificationReceived blocks until the sink logs a line for the given
// event on the given object key.
func requireNotificationReceived(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, namespace string, sel labels.Selector, event, key string) {
	t.Helper()

	wait4.RequirePodLog(ctx, t, k8sh, namespace, sel, wait4.TimeoutShort,
		func(line string) bool { return strings.Contains(line, event) && strings.Contains(line, key) },
		"notification sink received %s for %q", event, key)
}

// notificationSettleDelay gives an unwanted notification time to arrive (if it
// were going to) before we assert its absence.
const notificationSettleDelay = 5 * time.Second

// assertNotificationNotReceived asserts the sink did not recently log the given
// event for the given key. It matches only the most recent log lines: an earlier,
// legitimately-received event for the same key may still be in the full history.
func assertNotificationNotReceived(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, namespace string, sel labels.Selector, event, key string) {
	t.Helper()

	time.Sleep(notificationSettleDelay)

	recent := tailSinkLog(ctx, t, k8sh, namespace, sel)
	assert.False(t, strings.Contains(recent, event) && strings.Contains(recent, key),
		"notification sink unexpectedly received %s for %q", event, key)
}

// sinkLogTailLines bounds assertNotificationNotReceived to recent log lines.
const sinkLogTailLines = 5

// tailSinkLog returns the last sinkLogTailLines log lines of the first pod
// matching sel.
func tailSinkLog(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, namespace string, sel labels.Selector) string {
	t.Helper()

	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items)

	tail := int64(sinkLogTailLines)
	stream, err := k8sh.Clientset.CoreV1().Pods(namespace).
		GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{TailLines: &tail}).
		Stream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	out, err := io.ReadAll(stream)
	require.NoError(t, err)
	return string(out)
}
