/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code was modified from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package kit for Kubernetes operators
package kit

import (
	"fmt"
	"net/http"

	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// V1Alpha1 version for kubernetes resources
	V1Alpha1 = "v1alpha1"

	// V1Beta1 version for kubernetes resources
	V1Beta1 = "v1beta1"

	// V1 version for kubernetes resources
	V1 = "v1"
)

// KubeContext provides the context for connecting to Kubernetes APIs
type KubeContext struct {
	// Clientset is a connection to the core kubernetes API
	Clientset kubernetes.Interface

	// RetryDelay is the number of seconds to delay between retrying of kubernetes API calls.
	// Only used by the Retry function.
	RetryDelay int

	// MaxRetries is the number of times that an operation will be attempted by the Retry function.
	MaxRetries int

	// The host where the Kubernetes master is found.
	MasterHost string

	// An http connection to the Kubernetes API
	KubeHTTPCli *http.Client
}

// CustomResource is for creating a Kubernetes TPR/CRD
type CustomResource struct {
	// Name of the custom resource
	Name string

	// Group the custom resource belongs to
	Group string

	// Version which should be defined in a const above
	Version string

	// Description that is human readable
	Description string
}

// CreateCustomResources creates the given custom resources and waits for them to initialize
func CreateCustomResources(context KubeContext, resources []CustomResource) error {
	for _, resource := range resources {
		if err := CreateCustomResource(context, resource); err != nil {
			return fmt.Errorf("failed to init resource %s. %+v", resource.Name, err)
		}
	}

	for _, resource := range resources {
		if err := waitForCustomResourceInit(context, resource); err != nil {
			return fmt.Errorf("failed to complete init %s. %+v", resource.Name, err)
		}
	}

	return nil
}

// CreateCustomResource creates a single custom resource, but does not wait for it to initialize
func CreateCustomResource(context KubeContext, resource CustomResource) error {
	logger.Infof("creating %s resource", resource.Name)
	r := &v1beta1.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s.%s", resource.Name, resource.Group),
		},
		Versions: []v1beta1.APIVersion{
			{Name: resource.Version},
		},
		Description: resource.Description,
	}
	_, err := context.Clientset.ExtensionsV1beta1().ThirdPartyResources().Create(r)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s resource. %+v", resource.Name, err)
		}
	}

	return nil
}

func waitForCustomResourceInit(context KubeContext, resource CustomResource) error {
	restcli := context.Clientset.CoreV1().RESTClient()
	uri := resourceURI(resource, "")
	return Retry(context, func() (bool, error) {
		_, err := restcli.Get().RequestURI(uri).DoRaw()
		if err != nil {
			logger.Infof("did not yet find resource %s at %s. %+v", resource.Name, uri, err)
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func watchResource(context KubeContext, resource CustomResource, namespace, resourceVersion string) (*http.Response, error) {
	uri := fmt.Sprintf("%s/%s?watch=true&resourceVersion=%s", context.MasterHost, resourceURI(resource, namespace), resourceVersion)
	logger.Debugf("watching resource: %s", uri)
	return context.KubeHTTPCli.Get(uri)
}

func resourceURI(resource CustomResource, namespace string) string {
	if namespace == "" {
		// creates a uri that is for retrieving or watching a resource in all namespaces. For example:
		//   /apis/rook.io/v1alpha1/clusters
		return fmt.Sprintf("apis/%s/%s/%ss", resource.Group, resource.Version, resource.Name)
	}

	// create a uri that is for a specific namespace
	//   /apis/rook.io/v1alpha1/namespaces/rook/pools
	return fmt.Sprintf("apis/%s/%s/namespaces/%s/%ss", resource.Group, resource.Version, namespace, resource.Name)
}

// GetRawListNamespaced retrieves a list custom resources of the given type in a specific namespace
func GetRawListNamespaced(clientset kubernetes.Interface, resource CustomResource, namespace string) ([]byte, error) {
	restcli := clientset.CoreV1().RESTClient()
	uri := resourceURI(resource, namespace)
	return restcli.Get().RequestURI(uri).DoRaw()
}

// GetRawList retrieves a list of custom resources of the given type across all namespaces
func GetRawList(clientset kubernetes.Interface, resource CustomResource) ([]byte, error) {
	return GetRawListNamespaced(clientset, resource, "")
}
