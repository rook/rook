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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// compile-time assertions ensures CephBucketNotification implements webhook.Validator so a webhook builder
// will be registered for the validating webhook.
var _ webhook.Validator = &CephBucketNotification{}

// ValidateNotificationSpec validate the bucket notification arguments
func ValidateNotificationSpec(n *CephBucketNotification) error {
	// done in the generated code by kubebuilder
	return nil
}

func (n *CephBucketNotification) ValidateCreate() (admission.Warnings, error) {
	logger.Infof("validate create cephbucketnotification %v", n)
	if err := ValidateNotificationSpec(n); err != nil {
		return nil, err
	}
	return nil, nil
}

func (n *CephBucketNotification) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	logger.Infof("validate update cephbucketnotification %v", n)
	if err := ValidateNotificationSpec(n); err != nil {
		return nil, err
	}
	return nil, nil
}

func (n *CephBucketNotification) ValidateDelete() (admission.Warnings, error) {
	logger.Infof("validate delete cephbucketnotification %v", n)
	return nil, nil
}
