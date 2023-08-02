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

	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateOrUpdateEndpoint creates a EndpointSlice or updates the EndpointSlice declaratively if it already exists.
func CreateOrUpdateEndpoint(ctx context.Context, clientset kubernetes.Interface, namespace string, endpointDefinition *discoveryv1.EndpointSlice) (*discoveryv1.EndpointSlice, error) {
	name := endpointDefinition.Name
	logger.Debugf("creating endpoint %q. %v", name, endpointDefinition.Endpoints)
	ep, err := clientset.DiscoveryV1().EndpointSlices(namespace).Create(ctx, endpointDefinition, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create endpoint %q. %v", name, err)
		}
		ep, err = clientset.DiscoveryV1().EndpointSlices(namespace).Update(ctx, endpointDefinition, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to update endpoint %q. %v", name, err)
		}
	}

	// Delete the old Endpoint resource if exists, as it has been replaced by a new EndpointSlice resource
	oldEndpoint, err := clientset.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return ep, nil
		}
		return nil, fmt.Errorf("failed to fetch old endpoint %q. %v", name, err)
	}
	err = clientset.CoreV1().Endpoints(namespace).Delete(ctx, oldEndpoint.Name, metav1.DeleteOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to delete old endpoint %q. %v", oldEndpoint.Name, err)
	}

	return ep, err
}
