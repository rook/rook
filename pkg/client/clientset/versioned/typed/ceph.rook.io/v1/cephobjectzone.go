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

// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	"context"
	"time"

	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	scheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// CephObjectZonesGetter has a method to return a CephObjectZoneInterface.
// A group's client should implement this interface.
type CephObjectZonesGetter interface {
	CephObjectZones(namespace string) CephObjectZoneInterface
}

// CephObjectZoneInterface has methods to work with CephObjectZone resources.
type CephObjectZoneInterface interface {
	Create(ctx context.Context, cephObjectZone *v1.CephObjectZone, opts metav1.CreateOptions) (*v1.CephObjectZone, error)
	Update(ctx context.Context, cephObjectZone *v1.CephObjectZone, opts metav1.UpdateOptions) (*v1.CephObjectZone, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.CephObjectZone, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.CephObjectZoneList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.CephObjectZone, err error)
	CephObjectZoneExpansion
}

// cephObjectZones implements CephObjectZoneInterface
type cephObjectZones struct {
	client rest.Interface
	ns     string
}

// newCephObjectZones returns a CephObjectZones
func newCephObjectZones(c *CephV1Client, namespace string) *cephObjectZones {
	return &cephObjectZones{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the cephObjectZone, and returns the corresponding cephObjectZone object, and an error if there is any.
func (c *cephObjectZones) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.CephObjectZone, err error) {
	result = &v1.CephObjectZone{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("cephobjectzones").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of CephObjectZones that match those selectors.
func (c *cephObjectZones) List(ctx context.Context, opts metav1.ListOptions) (result *v1.CephObjectZoneList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.CephObjectZoneList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("cephobjectzones").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested cephObjectZones.
func (c *cephObjectZones) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("cephobjectzones").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a cephObjectZone and creates it.  Returns the server's representation of the cephObjectZone, and an error, if there is any.
func (c *cephObjectZones) Create(ctx context.Context, cephObjectZone *v1.CephObjectZone, opts metav1.CreateOptions) (result *v1.CephObjectZone, err error) {
	result = &v1.CephObjectZone{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("cephobjectzones").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(cephObjectZone).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a cephObjectZone and updates it. Returns the server's representation of the cephObjectZone, and an error, if there is any.
func (c *cephObjectZones) Update(ctx context.Context, cephObjectZone *v1.CephObjectZone, opts metav1.UpdateOptions) (result *v1.CephObjectZone, err error) {
	result = &v1.CephObjectZone{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("cephobjectzones").
		Name(cephObjectZone.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(cephObjectZone).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the cephObjectZone and deletes it. Returns an error if one occurs.
func (c *cephObjectZones) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("cephobjectzones").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *cephObjectZones) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("cephobjectzones").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched cephObjectZone.
func (c *cephObjectZones) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.CephObjectZone, err error) {
	result = &v1.CephObjectZone{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("cephobjectzones").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
