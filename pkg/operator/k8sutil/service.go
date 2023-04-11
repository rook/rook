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
	"net"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	mcsv1Client "sigs.k8s.io/mcs-api/pkg/client/clientset/versioned"
)

// CreateOrUpdateService creates a service or updates the service declaratively if it already exists.
func CreateOrUpdateService(
	ctx context.Context, clientset kubernetes.Interface, namespace string, serviceDefinition *v1.Service,
) (*v1.Service, error) {
	name := serviceDefinition.Name
	logger.Debugf("creating service %s", name)

	s, err := clientset.CoreV1().Services(namespace).Create(ctx, serviceDefinition, metav1.CreateOptions{})
	if err != nil {
		if !kerror.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create service %s. %+v", name, err)
		}
		s, err = UpdateService(ctx, clientset, namespace, serviceDefinition)
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
	ctx context.Context, clientset kubernetes.Interface, namespace string, serviceDefinition *v1.Service,
) (*v1.Service, error) {
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
func DeleteService(ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	err := clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if kerror.IsNotFound(err) {
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

func IsServiceExported(ctx context.Context, c *clusterd.Context, name, namespace string) (bool, error) {
	client, err := mcsv1Client.NewForConfig(c.KubeConfig)
	if err != nil {
		return false, errors.Wrap(err, "failed to get mcs-api client")
	}

	_, err = client.MulticlusterV1alpha1().ServiceExports(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to get exported service %q", name)
	}

	return true, nil
}

// ExportService exports the service using MCS API and returns the external IP of the exported service
func ExportService(ctx context.Context, c *clusterd.Context, service *v1.Service, clusterID string) (string, error) {
	client, err := mcsv1Client.NewForConfig(c.KubeConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to get mcs-api client")
	}

	serviceExport := &mcsv1a1.ServiceExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:            service.Name,
			Namespace:       service.Namespace,
			OwnerReferences: service.GetOwnerReferences(),
		},
	}

	_, err = client.MulticlusterV1alpha1().ServiceExports(service.Namespace).Create(ctx, serviceExport, metav1.CreateOptions{})
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return "", errors.Wrapf(err, "failed to create exported service %q", service.Name)
	}

	var exportedIP string
	var serviceExportError error
	exportedIP, err = GetExportedServiceIP(fmt.Sprintf("%s.%s.%s.svc.clusterset.local", clusterID, service.Name, service.Namespace))

	if err != nil {
		serviceExportError = errors.Wrapf(err, "failed to get exported service IP for %q", service.Name)
		err := verifyExportedService(ctx, client, service.Name, service.Namespace)
		if err != nil {
			serviceExportError = errors.Wrapf(err, "failed to create service export successfully for the service %q", service.Name)
		}
		return "", serviceExportError
	}

	return exportedIP, nil
}

// verifies if the ServiceExport status conditions to determine if the service was exported correctly
func verifyExportedService(ctx context.Context, client *mcsv1Client.Clientset, name, namespace string) error {
	exportedService, err := client.MulticlusterV1alpha1().ServiceExports(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get exported service %q", name)
	}

	if len(exportedService.Status.Conditions) == 0 {
		return fmt.Errorf("no status conditions available in the exported service %q", name)
	}

	for _, condition := range exportedService.Status.Conditions {
		if condition.Type == mcsv1a1.ServiceExportValid && condition.Status == corev1.ConditionFalse {
			return fmt.Errorf(*condition.Message)
		}
	}

	return nil
}

func GetExportedServiceIP(fqdn string) (string, error) {
	retryCount := 20
	serviceExportWaitTime := 5 * time.Second

	ips := []net.IP{}
	var err error
	for i := 0; i < retryCount; i++ {
		ips, err = net.LookupIP(fqdn)
		if err != nil {
			if i < retryCount-1 {
				logger.Warningf("failed to resolve DNS for %q. Trying again...", fqdn)
				time.Sleep(serviceExportWaitTime)
				continue
			} else {
				return "", errors.Wrapf(err, "failed to resolve DNS for %q. %v", fqdn, err)
			}
		}
		logger.Debugf("available addresses for %q : %v", fqdn, ips)
		break
	}

	if len(ips) == 0 {
		return "", errors.New(fmt.Sprintf("no external IP found in the exported service %q", fqdn))
	}

	return ips[0].String(), nil
}
