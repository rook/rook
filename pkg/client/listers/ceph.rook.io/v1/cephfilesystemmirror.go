/*
<<<<<<< HEAD
Copyright 2018 The Rook Authors. All rights reserved.
=======
Copyright The Kubernetes Authors.
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

<<<<<<< HEAD
    http://www.apache.org/licenses/LICENSE-2.0
=======
    http://www.apache.org/licenses/LICENSE-2.0
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

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
<<<<<<< HEAD
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers"
=======
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	"k8s.io/client-go/tools/cache"
)

// CephFilesystemMirrorLister helps list CephFilesystemMirrors.
// All objects returned here must be treated as read-only.
type CephFilesystemMirrorLister interface {
	// List lists all CephFilesystemMirrors in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.CephFilesystemMirror, err error)
	// CephFilesystemMirrors returns an object that can list and get CephFilesystemMirrors.
	CephFilesystemMirrors(namespace string) CephFilesystemMirrorNamespaceLister
	CephFilesystemMirrorListerExpansion
}

// cephFilesystemMirrorLister implements the CephFilesystemMirrorLister interface.
type cephFilesystemMirrorLister struct {
<<<<<<< HEAD
	listers.ResourceIndexer[*v1.CephFilesystemMirror]
=======
	indexer cache.Indexer
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
}

// NewCephFilesystemMirrorLister returns a new CephFilesystemMirrorLister.
func NewCephFilesystemMirrorLister(indexer cache.Indexer) CephFilesystemMirrorLister {
<<<<<<< HEAD
	return &cephFilesystemMirrorLister{listers.New[*v1.CephFilesystemMirror](indexer, v1.Resource("cephfilesystemmirror"))}
=======
	return &cephFilesystemMirrorLister{indexer: indexer}
}

// List lists all CephFilesystemMirrors in the indexer.
func (s *cephFilesystemMirrorLister) List(selector labels.Selector) (ret []*v1.CephFilesystemMirror, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.CephFilesystemMirror))
	})
	return ret, err
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
}

// CephFilesystemMirrors returns an object that can list and get CephFilesystemMirrors.
func (s *cephFilesystemMirrorLister) CephFilesystemMirrors(namespace string) CephFilesystemMirrorNamespaceLister {
<<<<<<< HEAD
	return cephFilesystemMirrorNamespaceLister{listers.NewNamespaced[*v1.CephFilesystemMirror](s.ResourceIndexer, namespace)}
=======
	return cephFilesystemMirrorNamespaceLister{indexer: s.indexer, namespace: namespace}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
}

// CephFilesystemMirrorNamespaceLister helps list and get CephFilesystemMirrors.
// All objects returned here must be treated as read-only.
type CephFilesystemMirrorNamespaceLister interface {
	// List lists all CephFilesystemMirrors in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.CephFilesystemMirror, err error)
	// Get retrieves the CephFilesystemMirror from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.CephFilesystemMirror, error)
	CephFilesystemMirrorNamespaceListerExpansion
}

// cephFilesystemMirrorNamespaceLister implements the CephFilesystemMirrorNamespaceLister
// interface.
type cephFilesystemMirrorNamespaceLister struct {
<<<<<<< HEAD
	listers.ResourceIndexer[*v1.CephFilesystemMirror]
=======
	indexer   cache.Indexer
	namespace string
}

// List lists all CephFilesystemMirrors in the indexer for a given namespace.
func (s cephFilesystemMirrorNamespaceLister) List(selector labels.Selector) (ret []*v1.CephFilesystemMirror, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.CephFilesystemMirror))
	})
	return ret, err
}

// Get retrieves the CephFilesystemMirror from the indexer for a given namespace and name.
func (s cephFilesystemMirrorNamespaceLister) Get(name string) (*v1.CephFilesystemMirror, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("cephfilesystemmirror"), name)
	}
	return obj.(*v1.CephFilesystemMirror), nil
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
}
