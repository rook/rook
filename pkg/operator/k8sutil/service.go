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
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateOrUpdateService creates a service or updates the service declaratively if it already exists.
func CreateOrUpdateService(
	clientset kubernetes.Interface, namespace string, serviceDefinition *v1.Service,
) (*v1.Service, error) {
	ctx := context.TODO()
	name := serviceDefinition.Name
	logger.Debugf("creating service %s", name)

	s, err := clientset.CoreV1().Services(namespace).Create(ctx, serviceDefinition, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create service %s. %+v", name, err)
		}
		s, err = UpdateService(clientset, namespace, serviceDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to update service %s. %+v", name, err)
		}
	} else {
		logger.Debugf("created service %s", s.Name)
	}
	return s, err
}

// UpdateService updates a service declaratively. If the service does not exist this is considered
// an error condition.
func UpdateService(
	clientset kubernetes.Interface, namespace string, serviceDefinition *v1.Service,
) (*v1.Service, error) {
	ctx := context.TODO()
	name := serviceDefinition.Name
	logger.Debugf("updating service %s", name)
	existing, err := clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not get existing service %s in order to update. %+v", name, err)
	}
	// ClusterIP is immutable for k8s services and cannot be left empty in k8s v1 API
	serviceDefinition.Spec.ClusterIP = existing.Spec.ClusterIP
	// ResourceVersion required to update services in k8s v1 API to prevent race conditions
	serviceDefinition.ResourceVersion = existing.ResourceVersion
	return clientset.CoreV1().Services(namespace).Update(ctx, serviceDefinition, metav1.UpdateOptions{})
}

// DeleteService deletes a Service and returns the error if any
func DeleteService(clientset kubernetes.Interface, namespace, name string) error {
	ctx := context.TODO()
	err := clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
	}
	return err
}

// ParseServiceType parses a string and returns a*v1.ServiceType. If the ServiceType is invalid,
// this should be considered an error.
func ParseServiceType(serviceString string) v1.ServiceType {
	switch serviceString {
	case "ClusterIP":
		return v1.ServiceTypeClusterIP
	case "ExternalName":
		return v1.ServiceTypeExternalName
	case "NodePort":
		return v1.ServiceTypeNodePort
	case "LoadBalancer":
		return v1.ServiceTypeLoadBalancer
	}
	return v1.ServiceType("")
}
