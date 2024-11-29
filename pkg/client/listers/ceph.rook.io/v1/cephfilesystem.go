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

// CephFilesystemLister helps list CephFilesystems.
// All objects returned here must be treated as read-only.
type CephFilesystemLister interface {
	// List lists all CephFilesystems in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.CephFilesystem, err error)
	// CephFilesystems returns an object that can list and get CephFilesystems.
	CephFilesystems(namespace string) CephFilesystemNamespaceLister
	CephFilesystemListerExpansion
}

// cephFilesystemLister implements the CephFilesystemLister interface.
type cephFilesystemLister struct {
	indexer cache.Indexer
}

// NewCephFilesystemLister returns a new CephFilesystemLister.
func NewCephFilesystemLister(indexer cache.Indexer) CephFilesystemLister {
	return &cephFilesystemLister{indexer: indexer}
}

// List lists all CephFilesystems in the indexer.
func (s *cephFilesystemLister) List(selector labels.Selector) (ret []*v1.CephFilesystem, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.CephFilesystem))
	})
	return ret, err
}

// CephFilesystems returns an object that can list and get CephFilesystems.
func (s *cephFilesystemLister) CephFilesystems(namespace string) CephFilesystemNamespaceLister {
	return cephFilesystemNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// CephFilesystemNamespaceLister helps list and get CephFilesystems.
// All objects returned here must be treated as read-only.
type CephFilesystemNamespaceLister interface {
	// List lists all CephFilesystems in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.CephFilesystem, err error)
	// Get retrieves the CephFilesystem from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.CephFilesystem, error)
	CephFilesystemNamespaceListerExpansion
}

// cephFilesystemNamespaceLister implements the CephFilesystemNamespaceLister
// interface.
type cephFilesystemNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all CephFilesystems in the indexer for a given namespace.
func (s cephFilesystemNamespaceLister) List(selector labels.Selector) (ret []*v1.CephFilesystem, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.CephFilesystem))
	})
	return ret, err
}

// Get retrieves the CephFilesystem from the indexer for a given namespace and name.
func (s cephFilesystemNamespaceLister) Get(name string) (*v1.CephFilesystem, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("cephfilesystem"), name)
	}
	return obj.(*v1.CephFilesystem), nil
}
