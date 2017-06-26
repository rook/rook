/*
Package kit for Kubernetes operators

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
package kit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rook/rook/pkg/clusterd"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/pkg/api/v1"
)

var (
	// ErrVersionOutdated indicates that the custom resource is outdated and needs to be refreshed
	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")
)

// ResourceWatcher watches a custom resource for desired state
type ResourceWatcher struct {
	context         clusterd.KubeContext
	resource        CustomResource
	namespace       string
	group           string
	watchVersion    string
	callback        func(event *RawEvent) error
	checkStaleCache func() (string, error)
}

// RawEvent is the raw json message retrieved from the TPR/CRD update
type RawEvent struct {
	Type   kwatch.EventType
	Object json.RawMessage
}

// NewWatcher creates an instance of a custom resource watcher for the given resource
func NewWatcher(context clusterd.KubeContext, resource CustomResource, namespace, watchVersion string,
	callback func(event *RawEvent) error,
	checkStaleCache func() (string, error)) *ResourceWatcher {

	// Assign a default func for resources that don't have a cache
	if checkStaleCache == nil {
		checkStaleCache = func() (string, error) {
			// there is no cache for the resource so we indicate to always reset the watch on error
			return "", nil
		}
	}
	return &ResourceWatcher{
		context:         context,
		resource:        resource,
		namespace:       namespace,
		watchVersion:    watchVersion,
		callback:        callback,
		checkStaleCache: checkStaleCache,
	}
}

// Watch begins watching the custom resource (TPR/CRD). The call will block until an error is raised during the watch.
// When the watch has detected a create, update, or delete event, the raw event will be passed to the caller
// in the callback. After the callback returns, the watch loop will continue for the next event.
// If the callback returns an error, the error will be logged but will not abort the event loop.
func (w *ResourceWatcher) Watch() error {
	if w.namespace == "" {
		logger.Infof("start watching %s resource in all namespaces at %s", w.resource.Name, w.watchVersion)
	} else {
		logger.Infof("start watching %s resource in namespace %s at %s", w.resource.Name, w.namespace, w.watchVersion)
	}

	eventCh, errCh := w.watch()

	go func() {

		timer := &panicTimer{
			duration: time.Minute,
			message:  fmt.Sprintf("unexpected long blocking (> 1 Minute) when handling %s event", w.resource.Name),
		}

		for event := range eventCh {
			timer.Start()

			err := w.callback(event)
			if err != nil {
				logger.Errorf("failed to handle event %s on %s. %+v", event.Type, w.resource.Name, err)
			}

			timer.Stop()
		}
	}()
	return <-errCh
}

// watch creates a go routine, and watches the custom resource at <name>.<group> starting at
// the given watch version. It emits events on the resources through the returned
// event chan. Errors will be reported through the returned error chan. The go routine
// exits on any error.
func (w *ResourceWatcher) watch() (<-chan *RawEvent, <-chan error) {
	eventCh := make(chan *RawEvent)
	// On unexpected error case, the operator should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			err := w.watchOuterResource(eventCh, errCh)
			if err != nil {
				errCh <- fmt.Errorf("failed to watch %s resource. %+v", w.resource.Name, err)
				return
			}
		}
	}()

	return eventCh, errCh
}

func (w *ResourceWatcher) watchOuterResource(eventCh chan *RawEvent, errCh chan error) error {
	resp, err := watchResource(w.context, w.resource, w.namespace, w.watchVersion)
	if err != nil {
		errCh <- err
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid status code: %s", resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		ev, st, err := pollEvent(decoder)
		done, err := w.handlePollEventResult(st, err, errCh)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		logger.Debugf("rook pool event: %+v", ev)

		// Extract the resource version from the raw json
		meta := &v1.ObjectMeta{}
		err = json.Unmarshal(ev.Object, meta)
		if err != nil {
			return fmt.Errorf("fail to unmarshal metadata from body %s: %v", resp.Body, err)
		}
		w.watchVersion = meta.ResourceVersion
		eventCh <- ev
	}
}

func (w *ResourceWatcher) handlePollEventResult(status *metav1.Status, errIn error, errCh chan error) (done bool, err error) {
	if errIn != nil {
		if errIn == io.EOF { // apiserver will close stream periodically
			logger.Debug("apiserver closed stream")
			done = true
			return
		}

		err = errIn
		errCh <- fmt.Errorf("received invalid event from API server: %v", err)
		return
	}

	if status != nil {

		if status.Code == http.StatusGone {
			// event history is outdated.
			// if nothing has changed, we can go back to watch again.
			var resourceVersion string
			resourceVersion, err = w.checkStaleCache()
			if err == nil && resourceVersion != "" {
				// we were able to recover from the cache, so we update the watch version to the latest
				// resource version from the cache
				w.watchVersion = resourceVersion
				done = true
				return
			}

			// if anything has changed (or error on relist), we have to rebuild the state.
			// go to recovery path
			err = ErrVersionOutdated
			errCh <- ErrVersionOutdated
			return
		}

		logger.Errorf("unexpected status response from API server: %v", status.Message)
		done = true
		return
	}
	return
}

func pollEvent(decoder *json.Decoder) (*RawEvent, *metav1.Status, error) {
	re := &RawEvent{}
	err := decoder.Decode(re)
	if err != nil {
		if err == io.EOF {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("fail to decode raw event from apiserver (%v)", err)
	}

	if re.Type == kwatch.Error {
		status := &metav1.Status{}
		err = json.Unmarshal(re.Object, status)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode (%s) into metav1.Status (%v)", re.Object, err)
		}
		logger.Infof("returning pollEvent status %+v", status)
		return nil, status, nil
	}

	return re, nil, nil
}
