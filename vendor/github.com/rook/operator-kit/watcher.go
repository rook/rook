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
	"errors"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var (
	// ErrVersionOutdated indicates that the custom resource is outdated and needs to be refreshed
	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")
)

// ResourceWatcher watches a custom resource for desired state
type ResourceWatcher struct {
	resource              CustomResource
	namespace             string
	resourceEventHandlers cache.ResourceEventHandlerFuncs
	client                rest.Interface
	scheme                *runtime.Scheme
}

// NewWatcher creates an instance of a custom resource watcher for the given resource
func NewWatcher(resource CustomResource, namespace string, handlers cache.ResourceEventHandlerFuncs, client rest.Interface) *ResourceWatcher {
	return &ResourceWatcher{
		resource:              resource,
		namespace:             namespace,
		resourceEventHandlers: handlers,
		client:                client,
	}
}

// Watch begins watching the custom resource (TPR/CRD). The call will block until a Done signal is raised during in the context.
// When the watch has detected a create, update, or delete event, it will handled by the functions in the resourceEventHandlers. After the callback returns, the watch loop will continue for the next event.
// If the callback returns an error, the error will be logged.
func (w *ResourceWatcher) Watch(objType runtime.Object, done <-chan struct{}) error {
	source := cache.NewListWatchFromClient(
		w.client,
		w.resource.Plural,
		w.namespace,
		fields.Everything())
	_, controller := cache.NewInformer(
		source,

		// The object type.
		objType,

		// resyncPeriod
		// Every resyncPeriod, all resources in the cache will retrigger events.
		// Set to 0 to disable the resync.
		0,

		// Your custom resource event handlers.
		w.resourceEventHandlers)

	go controller.Run(done)
	<-done
	return nil
}
