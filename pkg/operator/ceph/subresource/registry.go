/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package subresource

import "github.com/pkg/errors"

var (
	// CephClusterRegistry is the subresource registry for all resources which can be dependents of
	// a CephCluster.
	CephClusterRegistry = NewRegistry()
)

type Registry struct {
	registry map[string]Subresource
}

// NewRegistry creates a new, empty Registry for a namespace/cluster.
func NewRegistry() Registry {
	return Registry{
		registry: make(map[string]Subresource),
	}
}

func (r *Registry) Register(s Subresource) error {
	kind := s.Kind()
	if _, ok := r.registry[kind]; ok {
		return errors.Errorf("subresource %q is already registered", kind)
	}
	r.registry[kind] = s
	return nil
}

func (r *Registry) List() []Subresource {
	srs := make([]Subresource, 0, len(r.registry))
	for _, v := range r.registry {
		srs = append(srs, v)
	}
	return srs // not in any particular order
}

func (r *Registry) Get(kind string) (Subresource, error) {
	sr, ok := r.registry[kind]
	if !ok {
		return nil, errors.Errorf("subresource %q has not been registered", kind)
	}
	return sr, nil
}
