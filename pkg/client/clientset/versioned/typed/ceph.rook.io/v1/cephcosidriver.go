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

// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	"context"
<<<<<<< HEAD
=======
	"time"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	scheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
<<<<<<< HEAD
	gentype "k8s.io/client-go/gentype"
=======
	rest "k8s.io/client-go/rest"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
)

// CephCOSIDriversGetter has a method to return a CephCOSIDriverInterface.
// A group's client should implement this interface.
type CephCOSIDriversGetter interface {
	CephCOSIDrivers(namespace string) CephCOSIDriverInterface
}

// CephCOSIDriverInterface has methods to work with CephCOSIDriver resources.
type CephCOSIDriverInterface interface {
	Create(ctx context.Context, cephCOSIDriver *v1.CephCOSIDriver, opts metav1.CreateOptions) (*v1.CephCOSIDriver, error)
	Update(ctx context.Context, cephCOSIDriver *v1.CephCOSIDriver, opts metav1.UpdateOptions) (*v1.CephCOSIDriver, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.CephCOSIDriver, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.CephCOSIDriverList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.CephCOSIDriver, err error)
	CephCOSIDriverExpansion
}

// cephCOSIDrivers implements CephCOSIDriverInterface
type cephCOSIDrivers struct {
<<<<<<< HEAD
	*gentype.ClientWithList[*v1.CephCOSIDriver, *v1.CephCOSIDriverList]
=======
	client rest.Interface
	ns     string
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
}

// newCephCOSIDrivers returns a CephCOSIDrivers
func newCephCOSIDrivers(c *CephV1Client, namespace string) *cephCOSIDrivers {
	return &cephCOSIDrivers{
<<<<<<< HEAD
		gentype.NewClientWithList[*v1.CephCOSIDriver, *v1.CephCOSIDriverList](
			"cephcosidrivers",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *v1.CephCOSIDriver { return &v1.CephCOSIDriver{} },
			func() *v1.CephCOSIDriverList { return &v1.CephCOSIDriverList{} }),
	}
}
=======
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the cephCOSIDriver, and returns the corresponding cephCOSIDriver object, and an error if there is any.
func (c *cephCOSIDrivers) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.CephCOSIDriver, err error) {
	result = &v1.CephCOSIDriver{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("cephcosidrivers").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of CephCOSIDrivers that match those selectors.
func (c *cephCOSIDrivers) List(ctx context.Context, opts metav1.ListOptions) (result *v1.CephCOSIDriverList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.CephCOSIDriverList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("cephcosidrivers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested cephCOSIDrivers.
func (c *cephCOSIDrivers) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("cephcosidrivers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a cephCOSIDriver and creates it.  Returns the server's representation of the cephCOSIDriver, and an error, if there is any.
func (c *cephCOSIDrivers) Create(ctx context.Context, cephCOSIDriver *v1.CephCOSIDriver, opts metav1.CreateOptions) (result *v1.CephCOSIDriver, err error) {
	result = &v1.CephCOSIDriver{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("cephcosidrivers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(cephCOSIDriver).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a cephCOSIDriver and updates it. Returns the server's representation of the cephCOSIDriver, and an error, if there is any.
func (c *cephCOSIDrivers) Update(ctx context.Context, cephCOSIDriver *v1.CephCOSIDriver, opts metav1.UpdateOptions) (result *v1.CephCOSIDriver, err error) {
	result = &v1.CephCOSIDriver{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("cephcosidrivers").
		Name(cephCOSIDriver.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(cephCOSIDriver).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the cephCOSIDriver and deletes it. Returns an error if one occurs.
func (c *cephCOSIDrivers) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("cephcosidrivers").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *cephCOSIDrivers) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("cephcosidrivers").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched cephCOSIDriver.
func (c *cephCOSIDrivers) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.CephCOSIDriver, err error) {
	result = &v1.CephCOSIDriver{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("cephcosidrivers").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
