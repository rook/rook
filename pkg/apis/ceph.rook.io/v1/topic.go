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
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// compile-time assertions ensures CephBucketTopic implements webhook.Validator so a webhook builder
// will be registered for the validating webhook.
var _ webhook.Validator = &CephBucketTopic{}

func validateURI(uri string, expectedSchemas []string) error {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return err
	}
	schema := strings.ToLower(parsedURI.Scheme)
	for _, s := range expectedSchemas {
		if s == schema {
			return nil
		}
	}
	return errors.Errorf("URI schema %q no in %v", schema, expectedSchemas)
}

func ValidateHTTPSpec(s *HTTPEndpointSpec) error {
	return validateURI(s.URI, []string{"http", "https"})
}

func ValidateAMQPSpec(s *AMQPEndpointSpec) error {
	return validateURI(s.URI, []string{"amqp", "amqps"})
}

func ValidateKafkaSpec(s *KafkaEndpointSpec) error {
	return validateURI(s.URI, []string{"kafka"})
}

// ValidateTopicSpec validate the bucket notification topic arguments
func ValidateTopicSpec(t *CephBucketTopic) error {
	hasEndpoint := false
	if t.Spec.Endpoint.HTTP != nil {
		hasEndpoint = true
		if err := ValidateHTTPSpec(t.Spec.Endpoint.HTTP); err != nil {
			return err
		}
	}
	if t.Spec.Endpoint.AMQP != nil {
		if hasEndpoint {
			return errors.New("multiple endpoint specs")
		}
		hasEndpoint = true
		if err := ValidateAMQPSpec(t.Spec.Endpoint.AMQP); err != nil {
			return err
		}
	}
	if t.Spec.Endpoint.Kafka != nil {
		if hasEndpoint {
			return errors.New("multiple endpoint specs")
		}
		hasEndpoint = true
		if err := ValidateKafkaSpec(t.Spec.Endpoint.Kafka); err != nil {
			return err
		}
	}

	if !hasEndpoint {
		return errors.New("missing endpoint spec")
	}
	return nil
}

func (t *CephBucketTopic) ValidateCreate() error {
	logger.Infof("validate create cephbuckettopic %v", t)
	if err := ValidateTopicSpec(t); err != nil {
		return err
	}
	return nil
}

func (t *CephBucketTopic) ValidateUpdate(old runtime.Object) error {
	logger.Info("validate update cephbuckettopic")
	if err := ValidateTopicSpec(t); err != nil {
		return err
	}
	return nil
}

func (t *CephBucketTopic) ValidateDelete() error {
	logger.Info("validate delete cephbuckettopic")
	return nil
}
