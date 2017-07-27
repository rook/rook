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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package operator to manage Kubernetes storage.
package operator

import "sync"

type tprTracker struct {
	stopChMap  map[string]chan struct{}
	clusterRVs map[string]string
	sync.RWMutex
}

func newTPRTracker() *tprTracker {
	return &tprTracker{
		clusterRVs: make(map[string]string),
		stopChMap:  map[string]chan struct{}{},
	}
}

func (t *tprTracker) add(name, version string) {
	t.clusterRVs[name] = version
	if _, ok := t.stopChMap[name]; !ok {
		t.stopChMap[name] = make(chan struct{})
	}
}

func (t *tprTracker) remove(name string) {
	delete(t.clusterRVs, name)

	t.Lock()
	defer t.Unlock()
	if stopCh, ok := t.stopChMap[name]; ok {
		close(stopCh)
	}
	delete(t.stopChMap, name)
}

func (t *tprTracker) stop() {
	t.Lock()
	defer t.Unlock()
	for _, stop := range t.stopChMap {
		close(stop)
	}
	t.stopChMap = map[string]chan struct{}{}
}
