/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateOrUpdateService creates a service or updates the service declaratively if it already exists.
func CreateOrUpdateService(
	clientset kubernetes.Interface, namespace string, serviceDefinition *v1.Service,
) (*v1.Service, error) {
	name := serviceDefinition.Name
	logger.Debugf("creating service %s", name)
	s, err := clientset.CoreV1().Services(namespace).Create(serviceDefinition)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create service %s. %+v", name, err)
		}
		s, err = UpdateService(clientset, namespace, serviceDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to update service %s. %+v", name, err)
		}
	}
	return s, err
}

// UpdateService updates a service declaratively. If the service does not exist this is considered
// an error condition.
func UpdateService(
	clientset kubernetes.Interface, namespace string, serviceDefinition *v1.Service,
) (*v1.Service, error) {
	name := serviceDefinition.Name
	logger.Debug("updating service %s")
	existing, err := clientset.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not get existing service %s in order to update. %+v", name, err)
	}
	// ClusterIP is immutable for k8s services and cannot be left empty in k8s v1 API
	serviceDefinition.Spec.ClusterIP = existing.Spec.ClusterIP
	// ResourceVersion required to update services in k8s v1 API to prevent race conditions
	serviceDefinition.ResourceVersion = existing.ResourceVersion
	return clientset.CoreV1().Services(namespace).Update(serviceDefinition)
}
