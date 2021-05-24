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

package k8sutil

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateOrUpdateSecret creates a secret or updates the secret declaratively if it already exists.
func CreateOrUpdateSecret(clientset kubernetes.Interface, secretDefinition *v1.Secret) (*v1.Secret, error) {
	ctx := context.TODO()
	name := secretDefinition.Name
	logger.Debugf("creating secret %s", name)

	s, err := clientset.CoreV1().Secrets(secretDefinition.Namespace).Create(ctx, secretDefinition, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create secret %s. %+v", name, err)
		}
		s, err = clientset.CoreV1().Secrets(secretDefinition.Namespace).Update(ctx, secretDefinition, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to update secret %s. %+v", name, err)
		}
	} else {
		logger.Debugf("created secret %s", s.Name)
	}
	return s, err
}
