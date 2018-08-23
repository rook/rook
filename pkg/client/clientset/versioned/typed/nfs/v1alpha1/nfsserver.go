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

// NFSServersGetter has a method to return a NFSServerInterface.
// A group's client should implement this interface.
type NFSServersGetter interface {
	NFSServers(namespace string) NFSServerInterface
}

// NFSServerInterface has methods to work with NFSServer resources.
type NFSServerInterface interface {
	Create(*v1alpha1.NFSServer) (*v1alpha1.NFSServer, error)
	Update(*v1alpha1.NFSServer) (*v1alpha1.NFSServer, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.NFSServer, error)
	List(opts v1.ListOptions) (*v1alpha1.NFSServerList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NFSServer, err error)
	NFSServerExpansion
}

// nFSServers implements NFSServerInterface
type nFSServers struct {
	client rest.Interface
	ns     string
}

// newNFSServers returns a NFSServers
func newNFSServers(c *NfsV1alpha1Client, namespace string) *nFSServers {
	return &nFSServers{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the nFSServer, and returns the corresponding nFSServer object, and an error if there is any.
func (c *nFSServers) Get(name string, options v1.GetOptions) (result *v1alpha1.NFSServer, err error) {
	result = &v1alpha1.NFSServer{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("nfsservers").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of NFSServers that match those selectors.
func (c *nFSServers) List(opts v1.ListOptions) (result *v1alpha1.NFSServerList, err error) {
	result = &v1alpha1.NFSServerList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("nfsservers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested nFSServers.
func (c *nFSServers) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("nfsservers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a nFSServer and creates it.  Returns the server's representation of the nFSServer, and an error, if there is any.
func (c *nFSServers) Create(nFSServer *v1alpha1.NFSServer) (result *v1alpha1.NFSServer, err error) {
	result = &v1alpha1.NFSServer{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("nfsservers").
		Body(nFSServer).
		Do().
		Into(result)
	return
}

// Update takes the representation of a nFSServer and updates it. Returns the server's representation of the nFSServer, and an error, if there is any.
func (c *nFSServers) Update(nFSServer *v1alpha1.NFSServer) (result *v1alpha1.NFSServer, err error) {
	result = &v1alpha1.NFSServer{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("nfsservers").
		Name(nFSServer.Name).
		Body(nFSServer).
		Do().
		Into(result)
	return
}

// Delete takes name of the nFSServer and deletes it. Returns an error if one occurs.
func (c *nFSServers) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("nfsservers").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *nFSServers) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("nfsservers").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched nFSServer.
func (c *nFSServers) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NFSServer, err error) {
	result = &v1alpha1.NFSServer{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("nfsservers").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
