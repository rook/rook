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

package v1beta1

import (
	v1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	scheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ObjectstoreusersGetter has a method to return a ObjectstoreuserInterface.
// A group's client should implement this interface.
type ObjectstoreusersGetter interface {
	Objectstoreusers(namespace string) ObjectstoreuserInterface
}

// ObjectstoreuserInterface has methods to work with Objectstoreuser resources.
type ObjectstoreuserInterface interface {
	Create(*v1beta1.Objectstoreuser) (*v1beta1.Objectstoreuser, error)
	Update(*v1beta1.Objectstoreuser) (*v1beta1.Objectstoreuser, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1beta1.Objectstoreuser, error)
	List(opts v1.ListOptions) (*v1beta1.ObjectstoreuserList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.Objectstoreuser, err error)
	ObjectstoreuserExpansion
}

// objectstoreusers implements ObjectstoreuserInterface
type objectstoreusers struct {
	client rest.Interface
	ns     string
}

// newObjectstoreusers returns a Objectstoreusers
func newObjectstoreusers(c *CephV1beta1Client, namespace string) *objectstoreusers {
	return &objectstoreusers{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the objectstoreuser, and returns the corresponding objectstoreuser object, and an error if there is any.
func (c *objectstoreusers) Get(name string, options v1.GetOptions) (result *v1beta1.Objectstoreuser, err error) {
	result = &v1beta1.Objectstoreuser{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("objectstoreusers").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Objectstoreusers that match those selectors.
func (c *objectstoreusers) List(opts v1.ListOptions) (result *v1beta1.ObjectstoreuserList, err error) {
	result = &v1beta1.ObjectstoreuserList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("objectstoreusers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested objectstoreusers.
func (c *objectstoreusers) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("objectstoreusers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a objectstoreuser and creates it.  Returns the server's representation of the objectstoreuser, and an error, if there is any.
func (c *objectstoreusers) Create(objectstoreuser *v1beta1.Objectstoreuser) (result *v1beta1.Objectstoreuser, err error) {
	result = &v1beta1.Objectstoreuser{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("objectstoreusers").
		Body(objectstoreuser).
		Do().
		Into(result)
	return
}

// Update takes the representation of a objectstoreuser and updates it. Returns the server's representation of the objectstoreuser, and an error, if there is any.
func (c *objectstoreusers) Update(objectstoreuser *v1beta1.Objectstoreuser) (result *v1beta1.Objectstoreuser, err error) {
	result = &v1beta1.Objectstoreuser{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("objectstoreusers").
		Name(objectstoreuser.Name).
		Body(objectstoreuser).
		Do().
		Into(result)
	return
}

// Delete takes name of the objectstoreuser and deletes it. Returns an error if one occurs.
func (c *objectstoreusers) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("objectstoreusers").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *objectstoreusers) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("objectstoreusers").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched objectstoreuser.
func (c *objectstoreusers) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.Objectstoreuser, err error) {
	result = &v1beta1.Objectstoreuser{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("objectstoreusers").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
