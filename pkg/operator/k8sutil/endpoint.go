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

// CreateOrUpdateEndpointSlice creates an endpoint slice or updates the endpoint slice declaratively if it already exists.
func CreateOrUpdateEndpointSlice(ctx context.Context, clientset kubernetes.Interface, namespace string, endpointSliceDefinition *discoveryv1.EndpointSlice) (*discoveryv1.EndpointSlice, error) {
	name := endpointSliceDefinition.Name
	logger.Debugf("creating endpoint slice %q. %v", name, endpointSliceDefinition.Endpoints)
	es, err := clientset.DiscoveryV1().EndpointSlices(namespace).Create(ctx, endpointSliceDefinition, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create endpoint slice %q. %v", name, err)
		}
		es, err = clientset.DiscoveryV1().EndpointSlices(namespace).Update(ctx, endpointSliceDefinition, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to update endpoint slice %q. %v", name, err)
		}
	}

	return es, err
}
