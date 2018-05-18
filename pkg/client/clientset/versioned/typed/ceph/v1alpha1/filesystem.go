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

// FilesystemsGetter has a method to return a FilesystemInterface.
// A group's client should implement this interface.
type FilesystemsGetter interface {
	Filesystems(namespace string) FilesystemInterface
}

// FilesystemInterface has methods to work with Filesystem resources.
type FilesystemInterface interface {
	Create(*v1alpha1.Filesystem) (*v1alpha1.Filesystem, error)
	Update(*v1alpha1.Filesystem) (*v1alpha1.Filesystem, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.Filesystem, error)
	List(opts v1.ListOptions) (*v1alpha1.FilesystemList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Filesystem, err error)
	FilesystemExpansion
}

// filesystems implements FilesystemInterface
type filesystems struct {
	client rest.Interface
	ns     string
}

// newFilesystems returns a Filesystems
func newFilesystems(c *CephV1alpha1Client, namespace string) *filesystems {
	return &filesystems{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the filesystem, and returns the corresponding filesystem object, and an error if there is any.
func (c *filesystems) Get(name string, options v1.GetOptions) (result *v1alpha1.Filesystem, err error) {
	result = &v1alpha1.Filesystem{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("filesystems").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Filesystems that match those selectors.
func (c *filesystems) List(opts v1.ListOptions) (result *v1alpha1.FilesystemList, err error) {
	result = &v1alpha1.FilesystemList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("filesystems").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested filesystems.
func (c *filesystems) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("filesystems").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a filesystem and creates it.  Returns the server's representation of the filesystem, and an error, if there is any.
func (c *filesystems) Create(filesystem *v1alpha1.Filesystem) (result *v1alpha1.Filesystem, err error) {
	result = &v1alpha1.Filesystem{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("filesystems").
		Body(filesystem).
		Do().
		Into(result)
	return
}

// Update takes the representation of a filesystem and updates it. Returns the server's representation of the filesystem, and an error, if there is any.
func (c *filesystems) Update(filesystem *v1alpha1.Filesystem) (result *v1alpha1.Filesystem, err error) {
	result = &v1alpha1.Filesystem{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("filesystems").
		Name(filesystem.Name).
		Body(filesystem).
		Do().
		Into(result)
	return
}

// Delete takes name of the filesystem and deletes it. Returns an error if one occurs.
func (c *filesystems) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("filesystems").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *filesystems) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("filesystems").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched filesystem.
func (c *filesystems) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Filesystem, err error) {
	result = &v1alpha1.Filesystem{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("filesystems").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
