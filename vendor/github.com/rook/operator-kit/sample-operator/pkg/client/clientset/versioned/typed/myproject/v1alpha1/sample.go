/*
Copyright 2017 The Kubernetes Authors All rights reserved.

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
	v1alpha1 "github.com/rook/operator-kit/sample-operator/pkg/apis/myproject/v1alpha1"
	scheme "github.com/rook/operator-kit/sample-operator/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// SamplesGetter has a method to return a SampleInterface.
// A group's client should implement this interface.
type SamplesGetter interface {
	Samples(namespace string) SampleInterface
}

// SampleInterface has methods to work with Sample resources.
type SampleInterface interface {
	Create(*v1alpha1.Sample) (*v1alpha1.Sample, error)
	Update(*v1alpha1.Sample) (*v1alpha1.Sample, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.Sample, error)
	List(opts v1.ListOptions) (*v1alpha1.SampleList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Sample, err error)
	SampleExpansion
}

// samples implements SampleInterface
type samples struct {
	client rest.Interface
	ns     string
}

// newSamples returns a Samples
func newSamples(c *MyprojectV1alpha1Client, namespace string) *samples {
	return &samples{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the sample, and returns the corresponding sample object, and an error if there is any.
func (c *samples) Get(name string, options v1.GetOptions) (result *v1alpha1.Sample, err error) {
	result = &v1alpha1.Sample{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("samples").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Samples that match those selectors.
func (c *samples) List(opts v1.ListOptions) (result *v1alpha1.SampleList, err error) {
	result = &v1alpha1.SampleList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("samples").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested samples.
func (c *samples) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("samples").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a sample and creates it.  Returns the server's representation of the sample, and an error, if there is any.
func (c *samples) Create(sample *v1alpha1.Sample) (result *v1alpha1.Sample, err error) {
	result = &v1alpha1.Sample{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("samples").
		Body(sample).
		Do().
		Into(result)
	return
}

// Update takes the representation of a sample and updates it. Returns the server's representation of the sample, and an error, if there is any.
func (c *samples) Update(sample *v1alpha1.Sample) (result *v1alpha1.Sample, err error) {
	result = &v1alpha1.Sample{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("samples").
		Name(sample.Name).
		Body(sample).
		Do().
		Into(result)
	return
}

// Delete takes name of the sample and deletes it. Returns an error if one occurs.
func (c *samples) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("samples").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *samples) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("samples").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched sample.
func (c *samples) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Sample, err error) {
	result = &v1alpha1.Sample{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("samples").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
