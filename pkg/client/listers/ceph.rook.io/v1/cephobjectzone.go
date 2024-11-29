/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// CephObjectZoneLister helps list CephObjectZones.
// All objects returned here must be treated as read-only.
type CephObjectZoneLister interface {
	// List lists all CephObjectZones in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.CephObjectZone, err error)
	// CephObjectZones returns an object that can list and get CephObjectZones.
	CephObjectZones(namespace string) CephObjectZoneNamespaceLister
	CephObjectZoneListerExpansion
}

// cephObjectZoneLister implements the CephObjectZoneLister interface.
type cephObjectZoneLister struct {
	indexer cache.Indexer
}

// NewCephObjectZoneLister returns a new CephObjectZoneLister.
func NewCephObjectZoneLister(indexer cache.Indexer) CephObjectZoneLister {
	return &cephObjectZoneLister{indexer: indexer}
}

// List lists all CephObjectZones in the indexer.
func (s *cephObjectZoneLister) List(selector labels.Selector) (ret []*v1.CephObjectZone, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.CephObjectZone))
	})
	return ret, err
}

// CephObjectZones returns an object that can list and get CephObjectZones.
func (s *cephObjectZoneLister) CephObjectZones(namespace string) CephObjectZoneNamespaceLister {
	return cephObjectZoneNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// CephObjectZoneNamespaceLister helps list and get CephObjectZones.
// All objects returned here must be treated as read-only.
type CephObjectZoneNamespaceLister interface {
	// List lists all CephObjectZones in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.CephObjectZone, err error)
	// Get retrieves the CephObjectZone from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.CephObjectZone, error)
	CephObjectZoneNamespaceListerExpansion
}

// cephObjectZoneNamespaceLister implements the CephObjectZoneNamespaceLister
// interface.
type cephObjectZoneNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all CephObjectZones in the indexer for a given namespace.
func (s cephObjectZoneNamespaceLister) List(selector labels.Selector) (ret []*v1.CephObjectZone, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.CephObjectZone))
	})
	return ret, err
}

// Get retrieves the CephObjectZone from the indexer for a given namespace and name.
func (s cephObjectZoneNamespaceLister) Get(name string) (*v1.CephObjectZone, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("cephobjectzone"), name)
	}
	return obj.(*v1.CephObjectZone), nil
}
