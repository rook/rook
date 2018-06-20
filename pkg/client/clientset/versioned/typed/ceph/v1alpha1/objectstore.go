/*
Copyright 2018 The Kubernetes Authors.

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

package v1alpha1

import (
	v1alpha1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1alpha1"
	scheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ObjectStoresGetter has a method to return a ObjectStoreInterface.
// A group's client should implement this interface.
type ObjectStoresGetter interface {
	ObjectStores(namespace string) ObjectStoreInterface
}

// ObjectStoreInterface has methods to work with ObjectStore resources.
type ObjectStoreInterface interface {
	Create(*v1alpha1.ObjectStore) (*v1alpha1.ObjectStore, error)
	Update(*v1alpha1.ObjectStore) (*v1alpha1.ObjectStore, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.ObjectStore, error)
	List(opts v1.ListOptions) (*v1alpha1.ObjectStoreList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ObjectStore, err error)
	ObjectStoreExpansion
}

// objectStores implements ObjectStoreInterface
type objectStores struct {
	client rest.Interface
	ns     string
}

// newObjectStores returns a ObjectStores
func newObjectStores(c *CephV1alpha1Client, namespace string) *objectStores {
	return &objectStores{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the objectStore, and returns the corresponding objectStore object, and an error if there is any.
func (c *objectStores) Get(name string, options v1.GetOptions) (result *v1alpha1.ObjectStore, err error) {
	result = &v1alpha1.ObjectStore{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("objectstores").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ObjectStores that match those selectors.
func (c *objectStores) List(opts v1.ListOptions) (result *v1alpha1.ObjectStoreList, err error) {
	result = &v1alpha1.ObjectStoreList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("objectstores").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested objectStores.
func (c *objectStores) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("objectstores").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a objectStore and creates it.  Returns the server's representation of the objectStore, and an error, if there is any.
func (c *objectStores) Create(objectStore *v1alpha1.ObjectStore) (result *v1alpha1.ObjectStore, err error) {
	result = &v1alpha1.ObjectStore{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("objectstores").
		Body(objectStore).
		Do().
		Into(result)
	return
}

// Update takes the representation of a objectStore and updates it. Returns the server's representation of the objectStore, and an error, if there is any.
func (c *objectStores) Update(objectStore *v1alpha1.ObjectStore) (result *v1alpha1.ObjectStore, err error) {
	result = &v1alpha1.ObjectStore{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("objectstores").
		Name(objectStore.Name).
		Body(objectStore).
		Do().
		Into(result)
	return
}

// Delete takes name of the objectStore and deletes it. Returns an error if one occurs.
func (c *objectStores) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("objectstores").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *objectStores) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("objectstores").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched objectStore.
func (c *objectStores) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ObjectStore, err error) {
	result = &v1alpha1.ObjectStore{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("objectstores").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
