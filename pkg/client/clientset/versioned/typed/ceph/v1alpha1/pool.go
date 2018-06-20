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

// PoolsGetter has a method to return a PoolInterface.
// A group's client should implement this interface.
type PoolsGetter interface {
	Pools(namespace string) PoolInterface
}

// PoolInterface has methods to work with Pool resources.
type PoolInterface interface {
	Create(*v1alpha1.Pool) (*v1alpha1.Pool, error)
	Update(*v1alpha1.Pool) (*v1alpha1.Pool, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.Pool, error)
	List(opts v1.ListOptions) (*v1alpha1.PoolList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Pool, err error)
	PoolExpansion
}

// pools implements PoolInterface
type pools struct {
	client rest.Interface
	ns     string
}

// newPools returns a Pools
func newPools(c *CephV1alpha1Client, namespace string) *pools {
	return &pools{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the pool, and returns the corresponding pool object, and an error if there is any.
func (c *pools) Get(name string, options v1.GetOptions) (result *v1alpha1.Pool, err error) {
	result = &v1alpha1.Pool{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("pools").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Pools that match those selectors.
func (c *pools) List(opts v1.ListOptions) (result *v1alpha1.PoolList, err error) {
	result = &v1alpha1.PoolList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("pools").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested pools.
func (c *pools) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("pools").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a pool and creates it.  Returns the server's representation of the pool, and an error, if there is any.
func (c *pools) Create(pool *v1alpha1.Pool) (result *v1alpha1.Pool, err error) {
	result = &v1alpha1.Pool{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("pools").
		Body(pool).
		Do().
		Into(result)
	return
}

// Update takes the representation of a pool and updates it. Returns the server's representation of the pool, and an error, if there is any.
func (c *pools) Update(pool *v1alpha1.Pool) (result *v1alpha1.Pool, err error) {
	result = &v1alpha1.Pool{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("pools").
		Name(pool.Name).
		Body(pool).
		Do().
		Into(result)
	return
}

// Delete takes name of the pool and deletes it. Returns an error if one occurs.
func (c *pools) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("pools").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *pools) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("pools").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched pool.
func (c *pools) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Pool, err error) {
	result = &v1alpha1.Pool{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("pools").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
