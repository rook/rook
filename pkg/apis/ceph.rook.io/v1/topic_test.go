/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateHTTPTopicSpec(t *testing.T) {
	topic := &CephBucketTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fish-topic",
		},
		Spec: BucketTopicSpec{
			OpaqueData: "me@email.com",
			Persistent: true,
			Endpoint: TopicEndpointSpec{
				HTTP: &HTTPEndpointSpec{
					URI:              "http://myserver:9999",
					DisableVerifySSL: false,
					SendCloudEvents:  false,
				},
			},
		},
	}

	t.Run("valid", func(t *testing.T) {
		err := topic.ValidateTopicSpec()
		assert.NoError(t, err)
	})
	t.Run("invalid endpoint host", func(t *testing.T) {
		topic.Spec.Endpoint.HTTP.URI = "http://my server:9999"
		err := topic.ValidateTopicSpec()
		assert.Error(t, err)
	})
	t.Run("https host", func(t *testing.T) {
		topic.Spec.Endpoint.HTTP.URI = "https://127.0.0.1:9999"
		err := topic.ValidateTopicSpec()
		assert.NoError(t, err)
	})
	t.Run("invalid endpoint schema", func(t *testing.T) {
		topic.Spec.Endpoint.HTTP.URI = "kaboom://myserver:9999"
		err := topic.ValidateTopicSpec()
		assert.Error(t, err)
	})
}

func TestValidateAMQPTopicSpec(t *testing.T) {
	topic := &CephBucketTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fish-topic",
		},
		Spec: BucketTopicSpec{
			OpaqueData: "me@email.com",
			Persistent: true,
			Endpoint: TopicEndpointSpec{
				AMQP: &AMQPEndpointSpec{
					URI:              "amqp://myserver:9999",
					Exchange:         "fish-ex",
					DisableVerifySSL: true,
					AckLevel:         "broker",
				},
			},
		},
	}

	t.Run("valid", func(t *testing.T) {
		err := topic.ValidateTopicSpec()
		assert.NoError(t, err)
	})
	t.Run("amqps host", func(t *testing.T) {
		topic.Spec.Endpoint.AMQP.URI = "amqps://myserver:9999"
		err := topic.ValidateTopicSpec()
		assert.NoError(t, err)
	})
	t.Run("endpoint schema mismatch", func(t *testing.T) {
		topic.Spec.Endpoint.AMQP.URI = "http://myserver:9999"
		err := topic.ValidateTopicSpec()
		assert.Error(t, err)
	})
}

func TestValidateKafkaTopicSpec(t *testing.T) {
	topic := &CephBucketTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fish-topic",
		},
		Spec: BucketTopicSpec{
			OpaqueData: "me@email.com",
			Persistent: true,
			Endpoint: TopicEndpointSpec{
				Kafka: &KafkaEndpointSpec{
					URI:              "kafka://myserver:9999",
					UseSSL:           true,
					DisableVerifySSL: true,
					AckLevel:         "broker",
					Mechanism:        "SCRAM-SHA-512",
				},
			},
		},
	}

	t.Run("valid", func(t *testing.T) {
		err := topic.ValidateTopicSpec()
		assert.NoError(t, err)
	})
	t.Run("endpoint schema mismatch", func(t *testing.T) {
		topic.Spec.Endpoint.Kafka.URI = "http://myserver:9999"
		err := topic.ValidateTopicSpec()
		assert.Error(t, err)
	})
}

func TestInvalidTopicSpec(t *testing.T) {
	topic := &CephBucketTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fish-topic",
		},
		Spec: BucketTopicSpec{
			OpaqueData: "me@email.com",
			Persistent: true,
			Endpoint: TopicEndpointSpec{
				Kafka: &KafkaEndpointSpec{
					URI:              "kafka://myserver:9999",
					UseSSL:           true,
					DisableVerifySSL: true,
					AckLevel:         "broker",
				},
				AMQP: &AMQPEndpointSpec{
					URI:              "amqp://myserver:9999",
					Exchange:         "fish-ex",
					DisableVerifySSL: true,
					AckLevel:         "broker",
				},
			},
		},
	}

	t.Run("too many endpoint specs", func(t *testing.T) {
		err := topic.ValidateTopicSpec()
		assert.Error(t, err)
	})
	t.Run("valid", func(t *testing.T) {
		topic.Spec.Endpoint.AMQP = nil
		err := topic.ValidateTopicSpec()
		assert.NoError(t, err)
	})
	t.Run("too few endpoint specs", func(t *testing.T) {
		topic.Spec.Endpoint.Kafka = nil
		err := topic.ValidateTopicSpec()
		assert.Error(t, err)
	})
}
