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

// Package topic to manage rook bucket topics.
package topic

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestTopicAttributesCreation(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")
	falseString := "false"
	trueString := "true"
	emptyString := ""

	t.Run("test HTTP attributes", func(t *testing.T) {
		uri := "http://localhost"
		expectedAttrs := map[string]string{
			"OpaqueData":    emptyString,
			"cloudevents":   falseString,
			"persistent":    falseString,
			"push-endpoint": uri,
			"verify-ssl":    trueString,
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

		attrs, _, err := createTopicAttributes(provisioner{}, bucketTopic)
		require.NoError(t, err)
		assert.Equal(t, expectedAttrs, attrs)
	})

	t.Run("test AMQP attributes", func(t *testing.T) {
		uri := "amqp://my-rabbitmq-service:5672/vhost1"
		ackLevel := "broker"
		exchange := "ex1"
		expectedAttrs := map[string]string{
			"OpaqueData":     emptyString,
			"persistent":     falseString,
			"push-endpoint":  uri,
			"verify-ssl":     trueString,
			"amqp-exchange":  exchange,
			"amqp-ack-level": ackLevel,
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

		attrs, _, err := createTopicAttributes(provisioner{}, bucketTopic)
		require.NoError(t, err)
		assert.Equal(t, expectedAttrs, attrs)
	})

	t.Run("test AMQP attributes with deprecated ack level", func(t *testing.T) {
		uri := "amqp://my-rabbitmq-service:5672/vhost1"
		exchange := "ex1"
		expectedAttrs := map[string]string{
			"OpaqueData":     emptyString,
			"persistent":     falseString,
			"push-endpoint":  uri,
			"verify-ssl":     trueString,
			"amqp-exchange":  exchange,
			"amqp-ack-level": "routable",
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
						AckLevel: "routeable",
						Exchange: exchange,
					},
				},
			},
		}

		attrs, _, err := createTopicAttributes(provisioner{}, bucketTopic)
		require.NoError(t, err)
		assert.Equal(t, expectedAttrs, attrs)
	})

	t.Run("test Kafka attributes", func(t *testing.T) {
		uri := "kafka://my-kafka-service:9092"
		ackLevel := "broker"
		mechanism := "SCRAM-SHA-512"
		expectedAttrs := map[string]string{
			"OpaqueData":      emptyString,
			"persistent":      falseString,
			"push-endpoint":   uri,
			"verify-ssl":      trueString,
			"kafka-ack-level": ackLevel,
			"use-ssl":         trueString,
			"mechanism":       mechanism,
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
						URI:       uri,
						AckLevel:  ackLevel,
						UseSSL:    true,
						Mechanism: mechanism,
					},
				},
			},
		}

		attrs, _, err := createTopicAttributes(provisioner{}, bucketTopic)
		require.NoError(t, err)
		assert.Equal(t, expectedAttrs, attrs)
	})
}

func TestAMQPAckLevel(t *testing.T) {
	nsName := types.NamespacedName{Name: name, Namespace: namespace}

	for _, tc := range []struct {
		name     string
		ackLevel string
		expected string
	}{
		{"unset", "", ""},
		{"none", "none", "none"},
		{"broker", "broker", "broker"},
		{"routable", "routable", "routable"},
		{"deprecated routeable", "routeable", "routable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, amqpAckLevel(nsName, tc.ackLevel))
		})
	}
}

//nolint:gosec // only a test value, not a real secret
const examplePassword = "EXAMPLEPASSWORD0000000000000000000001"

// captureLogsAtLevel redirects the capnslog sink into a buffer at the given level
// for the duration of the test, restoring the package default afterwards so later
// tests still log to stderr at the level NewPackageLogger starts from.
func captureLogsAtLevel(t *testing.T, level capnslog.LogLevel) *bytes.Buffer {
	t.Helper()

	logBuf := bytes.NewBuffer([]byte{})
	capnslog.SetFormatter(capnslog.NewLogFormatter(logBuf, "", 0))
	capnslog.SetGlobalLogLevel(level)
	t.Cleanup(func() {
		capnslog.SetFormatter(capnslog.NewDefaultFormatter(os.Stderr))
		capnslog.SetGlobalLogLevel(capnslog.INFO)
	})

	return logBuf
}

func topicWithEndpoint(endpoint cephv1.TopicEndpointSpec) *cephv1.CephBucketTopic {
	return &cephv1.CephBucketTopic{
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
			Endpoint:             endpoint,
		},
	}
}

func TestCreateTopicAttributesRedactsAMQPEndpoint(t *testing.T) {
	// an AMQP URI routinely carries the broker password in its userinfo, and this
	// endpoint is logged at INFO, so no debug logging has to be enabled to leak it
	t.Run("password in userinfo", func(t *testing.T) {
		uri := "amqps://topic-user:" + examplePassword + "@my-rabbitmq-service:5672/vhost1"
		logBuf := captureLogsAtLevel(t, capnslog.INFO)

		attrs, _, err := createTopicAttributes(provisioner{}, topicWithEndpoint(cephv1.TopicEndpointSpec{
			AMQP: &cephv1.AMQPEndpointSpec{URI: uri, AckLevel: "broker", Exchange: "ex1"},
		}))

		require.NoError(t, err)
		assert.NotContains(t, logBuf.String(), examplePassword)
		// the endpoint is what the line exists to report, and is still logged
		assert.Contains(t, logBuf.String(), "amqps://topic-user:xxxxx@my-rabbitmq-service:5672/vhost1")
		// the URI handed to the RGW is unchanged
		assert.Equal(t, uri, attrs["push-endpoint"])
	})

	t.Run("URI that does not parse", func(t *testing.T) {
		// a DEL byte is one of the control characters url.Parse rejects outright, so
		// there is no URL to redact and the URI is reported by length instead
		uri := "amqps://topic-user:" + examplePassword + "@my-rabbitmq-service:5672/vhost1\x7f"
		logBuf := captureLogsAtLevel(t, capnslog.INFO)

		attrs, _, err := createTopicAttributes(provisioner{}, topicWithEndpoint(cephv1.TopicEndpointSpec{
			AMQP: &cephv1.AMQPEndpointSpec{URI: uri, AckLevel: "broker", Exchange: "ex1"},
		}))

		// an endpoint rook cannot parse is still sent to the RGW, which validates it
		require.NoError(t, err)
		assert.Equal(t, uri, attrs["push-endpoint"])
		assert.NotContains(t, logBuf.String(), examplePassword)
		assert.Contains(t, logBuf.String(), fmt.Sprintf("(%d bytes)", len(uri)))
		assert.Contains(t, logBuf.String(), "invalid control character in URL")
		// the CephBucketTopic to go look at
		assert.Contains(t, logBuf.String(), namespace+"/"+name)
	})
}

func TestCreateTopicAttributesRedactsHTTPEndpoint(t *testing.T) {
	// an HTTP push endpoint can carry userinfo credentials, and this endpoint is
	// logged at INFO, so no debug logging has to be enabled to leak them
	t.Run("password in userinfo", func(t *testing.T) {
		uri := "https://topic-user:" + examplePassword + "@my-receiver:8443/events"
		logBuf := captureLogsAtLevel(t, capnslog.INFO)

		attrs, _, err := createTopicAttributes(provisioner{}, topicWithEndpoint(cephv1.TopicEndpointSpec{
			HTTP: &cephv1.HTTPEndpointSpec{URI: uri},
		}))

		require.NoError(t, err)
		assert.NotContains(t, logBuf.String(), examplePassword)
		// the endpoint is what the line exists to report, and is still logged
		assert.Contains(t, logBuf.String(), "https://topic-user:xxxxx@my-receiver:8443/events")
		// the URI handed to the RGW is unchanged
		assert.Equal(t, uri, attrs["push-endpoint"])
	})

	t.Run("URI that does not parse", func(t *testing.T) {
		uri := "https://topic-user:" + examplePassword + "@my-receiver:8443/events\x7f"
		logBuf := captureLogsAtLevel(t, capnslog.INFO)

		attrs, _, err := createTopicAttributes(provisioner{}, topicWithEndpoint(cephv1.TopicEndpointSpec{
			HTTP: &cephv1.HTTPEndpointSpec{URI: uri},
		}))

		// an endpoint rook cannot parse is still sent to the RGW, which validates it
		require.NoError(t, err)
		assert.Equal(t, uri, attrs["push-endpoint"])
		assert.NotContains(t, logBuf.String(), examplePassword)
		assert.Contains(t, logBuf.String(), fmt.Sprintf("(%d bytes)", len(uri)))
		assert.Contains(t, logBuf.String(), "invalid control character in URL")
		// the CephBucketTopic to go look at
		assert.Contains(t, logBuf.String(), namespace+"/"+name)
	})
}

func TestCreateTopicAttributesKafkaParseFailureOmitsURI(t *testing.T) {
	// the caller logs this error at ERROR and reports it as a Kubernetes event, and
	// the URI it names can carry a passphrase
	uri := "kafka://topic-user:" + examplePassword + "@my-kafka-service:9092\x7f"

	_, _, err := createTopicAttributes(provisioner{}, topicWithEndpoint(cephv1.TopicEndpointSpec{
		Kafka: &cephv1.KafkaEndpointSpec{URI: uri, AckLevel: "broker"},
	}))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), examplePassword)
	// url.Parse fails with a *url.Error that repeats the whole URI, so the parse
	// error cannot be wrapped either, and nothing from the URI is reported
	assert.NotContains(t, err.Error(), "my-kafka-service")
	// what is reported in the URI's place: its length, the reason it was rejected,
	// and the CephBucketTopic field to go look at
	assert.Contains(t, err.Error(), fmt.Sprintf("(%d bytes)", len(uri)))
	assert.Contains(t, err.Error(), "invalid control character in URL")
	assert.Contains(t, err.Error(), namespace+"/"+name)
	assert.Contains(t, err.Error(), ".spec.endpoint.kafka.URI")
}
