/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// CreateOrUpdateEndpoint creates a service or updates the service declaratively if it already exists.
func CreateOrUpdateEndpoint(ctx context.Context, clientset kubernetes.Interface, namespace string, endpointDefinition *v1.Endpoints) (*v1.Endpoints, error) {
	name := endpointDefinition.Name
	logger.Debugf("creating endpoint %q. %v", name, endpointDefinition.Subsets)
	ep, err := clientset.CoreV1().Endpoints(namespace).Create(ctx, endpointDefinition, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create endpoint %q. %v", name, err)
		}
		ep, err = clientset.CoreV1().Endpoints(namespace).Update(ctx, endpointDefinition, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to update endpoint %q. %v", name, err)
		}
	}

	return ep, err
}
