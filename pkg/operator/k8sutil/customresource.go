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

// Package kit for Kubernetes operators
package k8sutil

import (
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// CustomResource is for creating a Kubernetes TPR/CRD
type CustomResource struct {
	// Name of the custom resource
	Name string

	// Plural of the custom resource in plural
	Plural string

	// Group the custom resource belongs to
	Group string

	// Version which should be defined in a const above
	Version string

	// Kind is the serialized interface of the resource.
	Kind string

	// APIVersion is the full API version name (combine Group and Version)
	APIVersion string
}

// WatchCR begins watching the custom resource (CRD). The call will block until a Done signal is raised during in the context.
// When the watch has detected a create, update, or delete event, it will handled by the functions in the resourceEventHandlers. After the callback returns, the watch loop will continue for the next event.
// If the callback returns an error, the error will be logged.
func WatchCR(resource CustomResource, namespace string, handlers cache.ResourceEventHandlerFuncs, client rest.Interface, objType runtime.Object, done <-chan struct{}) {
	source := cache.NewListWatchFromClient(
		client,
		resource.Plural,
		namespace,
		fields.Everything())
	_, controller := cache.NewInformerWithOptions(cache.InformerOptions{ListerWatcher: source, ObjectType: objType, ResyncPeriod: 0, Handler: handlers})

	go controller.Run(done)
	<-done
}
