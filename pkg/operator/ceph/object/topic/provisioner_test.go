/*
Copyright 2022 The Rook Authors. All rights reserved.

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

// Package topic to manage a rook bucket topics.
package topic

import (
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTopicAttributesCreation(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")
	falseString := "false"
	trueString := "true"
	emptyString := ""

	t.Run("test HTTP attributes", func(t *testing.T) {
		uri := "http://localhost"
		expectedAttrs := map[string]*string{
			"OpaqueData":    &emptyString,
			"cloudevents":   &falseString,
			"persistent":    &falseString,
			"push-endpoint": &uri,
			"verify-ssl":    &trueString,
		}
		bucketTopic := &cephv1.CephBucketTopic{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "CephBucketTopic",
			},
			Spec: cephv1.BucketTopicSpec{
				ObjectStoreName:      store,
				ObjectStoreNamespace: namespace,
				Endpoint: cephv1.TopicEndpointSpec{
					HTTP: &cephv1.HTTPEndpointSpec{
						URI: uri,
					},
				},
			},
		}
		assert.Equal(t, expectedAttrs, createTopicAttributes(bucketTopic))
	})
	t.Run("test AMQP attributes", func(t *testing.T) {
		uri := "amqp://my-rabbitmq-service:5672/vhost1"
		ackLevel := "broker"
		exchange := "ex1"
		expectedAttrs := map[string]*string{
			"OpaqueData":     &emptyString,
			"persistent":     &falseString,
			"push-endpoint":  &uri,
			"verify-ssl":     &trueString,
			"amqp-exchange":  &exchange,
			"amqp-ack-level": &ackLevel,
		}
		bucketTopic := &cephv1.CephBucketTopic{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "CephBucketTopic",
			},
			Spec: cephv1.BucketTopicSpec{
				ObjectStoreName:      store,
				ObjectStoreNamespace: namespace,
				Endpoint: cephv1.TopicEndpointSpec{
					AMQP: &cephv1.AMQPEndpointSpec{
						URI:      uri,
						AckLevel: ackLevel,
						Exchange: exchange,
					},
				},
			},
		}
		assert.Equal(t, expectedAttrs, createTopicAttributes(bucketTopic))
	})
	t.Run("test Kafka attributes", func(t *testing.T) {
		uri := "kafka://my-kafka-service:9092"
		ackLevel := "broker"
		expectedAttrs := map[string]*string{
			"OpaqueData":      &emptyString,
			"persistent":      &falseString,
			"push-endpoint":   &uri,
			"verify-ssl":      &trueString,
			"kafka-ack-level": &ackLevel,
			"use-ssl":         &trueString,
		}
		bucketTopic := &cephv1.CephBucketTopic{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "CephBucketTopic",
			},
			Spec: cephv1.BucketTopicSpec{
				ObjectStoreName:      store,
				ObjectStoreNamespace: namespace,
				Endpoint: cephv1.TopicEndpointSpec{
					Kafka: &cephv1.KafkaEndpointSpec{
						URI:      uri,
						AckLevel: ackLevel,
						UseSSL:   true,
					},
				},
			},
		}
		assert.Equal(t, expectedAttrs, createTopicAttributes(bucketTopic))
	})
}
