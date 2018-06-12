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
	v1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	scheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// NetworkFileSystemsGetter has a method to return a NetworkFileSystemInterface.
// A group's client should implement this interface.
type NetworkFileSystemsGetter interface {
	NetworkFileSystems(namespace string) NetworkFileSystemInterface
}

// NetworkFileSystemInterface has methods to work with NetworkFileSystem resources.
type NetworkFileSystemInterface interface {
	Create(*v1alpha1.NetworkFileSystem) (*v1alpha1.NetworkFileSystem, error)
	Update(*v1alpha1.NetworkFileSystem) (*v1alpha1.NetworkFileSystem, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.NetworkFileSystem, error)
	List(opts v1.ListOptions) (*v1alpha1.NetworkFileSystemList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NetworkFileSystem, err error)
	NetworkFileSystemExpansion
}

// networkFileSystems implements NetworkFileSystemInterface
type networkFileSystems struct {
	client rest.Interface
	ns     string
}

// newNetworkFileSystems returns a NetworkFileSystems
func newNetworkFileSystems(c *NfsV1alpha1Client, namespace string) *networkFileSystems {
	return &networkFileSystems{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the networkFileSystem, and returns the corresponding networkFileSystem object, and an error if there is any.
func (c *networkFileSystems) Get(name string, options v1.GetOptions) (result *v1alpha1.NetworkFileSystem, err error) {
	result = &v1alpha1.NetworkFileSystem{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("networkfilesystems").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of NetworkFileSystems that match those selectors.
func (c *networkFileSystems) List(opts v1.ListOptions) (result *v1alpha1.NetworkFileSystemList, err error) {
	result = &v1alpha1.NetworkFileSystemList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("networkfilesystems").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested networkFileSystems.
func (c *networkFileSystems) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("networkfilesystems").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a networkFileSystem and creates it.  Returns the server's representation of the networkFileSystem, and an error, if there is any.
func (c *networkFileSystems) Create(networkFileSystem *v1alpha1.NetworkFileSystem) (result *v1alpha1.NetworkFileSystem, err error) {
	result = &v1alpha1.NetworkFileSystem{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("networkfilesystems").
		Body(networkFileSystem).
		Do().
		Into(result)
	return
}

// Update takes the representation of a networkFileSystem and updates it. Returns the server's representation of the networkFileSystem, and an error, if there is any.
func (c *networkFileSystems) Update(networkFileSystem *v1alpha1.NetworkFileSystem) (result *v1alpha1.NetworkFileSystem, err error) {
	result = &v1alpha1.NetworkFileSystem{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("networkfilesystems").
		Name(networkFileSystem.Name).
		Body(networkFileSystem).
		Do().
		Into(result)
	return
}

// Delete takes name of the networkFileSystem and deletes it. Returns an error if one occurs.
func (c *networkFileSystems) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("networkfilesystems").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *networkFileSystems) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("networkfilesystems").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched networkFileSystem.
func (c *networkFileSystems) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NetworkFileSystem, err error) {
	result = &v1alpha1.NetworkFileSystem{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("networkfilesystems").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
