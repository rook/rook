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
*/

// Package kit for Kubernetes operators
package operatorkit

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

const (
	serverVersionV170 = "v1.7.0"
)

// NewHTTPClient creates a Kubernetes client to interact with API extensions for Custom Resources
func NewHTTPClient(group, version string, schemeBuilder runtime.SchemeBuilder) (rest.Interface, *runtime.Scheme, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, err
	}

	return NewHTTPClientFromConfig(group, version, schemeBuilder, config)
}

// NewHTTPClient creates a Kubernetes client from a given Kubernetes *rest.Config to interact with API extensions for Custom Resources
func NewHTTPClientFromConfig(group, version string, schemeBuilder runtime.SchemeBuilder, config *rest.Config) (rest.Interface, *runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := schemeBuilder.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	config.GroupVersion = &schema.GroupVersion{Group: group, Version: version}

	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	client, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, nil, err
	}

	return client, scheme, nil
}
