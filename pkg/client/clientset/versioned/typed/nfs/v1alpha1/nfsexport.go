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

// NFSExportsGetter has a method to return a NFSExportInterface.
// A group's client should implement this interface.
type NFSExportsGetter interface {
	NFSExports(namespace string) NFSExportInterface
}

// NFSExportInterface has methods to work with NFSExport resources.
type NFSExportInterface interface {
	Create(*v1alpha1.NFSExport) (*v1alpha1.NFSExport, error)
	Update(*v1alpha1.NFSExport) (*v1alpha1.NFSExport, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.NFSExport, error)
	List(opts v1.ListOptions) (*v1alpha1.NFSExportList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NFSExport, err error)
	NFSExportExpansion
}

// nFSExports implements NFSExportInterface
type nFSExports struct {
	client rest.Interface
	ns     string
}

// newNFSExports returns a NFSExports
func newNFSExports(c *NfsV1alpha1Client, namespace string) *nFSExports {
	return &nFSExports{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the nFSExport, and returns the corresponding nFSExport object, and an error if there is any.
func (c *nFSExports) Get(name string, options v1.GetOptions) (result *v1alpha1.NFSExport, err error) {
	result = &v1alpha1.NFSExport{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("nfsexports").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of NFSExports that match those selectors.
func (c *nFSExports) List(opts v1.ListOptions) (result *v1alpha1.NFSExportList, err error) {
	result = &v1alpha1.NFSExportList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("nfsexports").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested nFSExports.
func (c *nFSExports) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("nfsexports").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a nFSExport and creates it.  Returns the server's representation of the nFSExport, and an error, if there is any.
func (c *nFSExports) Create(nFSExport *v1alpha1.NFSExport) (result *v1alpha1.NFSExport, err error) {
	result = &v1alpha1.NFSExport{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("nfsexports").
		Body(nFSExport).
		Do().
		Into(result)
	return
}

// Update takes the representation of a nFSExport and updates it. Returns the server's representation of the nFSExport, and an error, if there is any.
func (c *nFSExports) Update(nFSExport *v1alpha1.NFSExport) (result *v1alpha1.NFSExport, err error) {
	result = &v1alpha1.NFSExport{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("nfsexports").
		Name(nFSExport.Name).
		Body(nFSExport).
		Do().
		Into(result)
	return
}

// Delete takes name of the nFSExport and deletes it. Returns an error if one occurs.
func (c *nFSExports) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("nfsexports").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *nFSExports) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("nfsexports").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched nFSExport.
func (c *nFSExports) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NFSExport, err error) {
	result = &v1alpha1.NFSExport{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("nfsexports").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
