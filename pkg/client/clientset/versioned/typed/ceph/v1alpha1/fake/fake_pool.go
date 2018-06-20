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

package fake

import (
	v1alpha1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakePools implements PoolInterface
type FakePools struct {
	Fake *FakeCephV1alpha1
	ns   string
}

var poolsResource = schema.GroupVersionResource{Group: "ceph.rook.io", Version: "v1alpha1", Resource: "pools"}

var poolsKind = schema.GroupVersionKind{Group: "ceph.rook.io", Version: "v1alpha1", Kind: "Pool"}

// Get takes name of the pool, and returns the corresponding pool object, and an error if there is any.
func (c *FakePools) Get(name string, options v1.GetOptions) (result *v1alpha1.Pool, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(poolsResource, c.ns, name), &v1alpha1.Pool{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Pool), err
}

// List takes label and field selectors, and returns the list of Pools that match those selectors.
func (c *FakePools) List(opts v1.ListOptions) (result *v1alpha1.PoolList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(poolsResource, poolsKind, c.ns, opts), &v1alpha1.PoolList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.PoolList{}
	for _, item := range obj.(*v1alpha1.PoolList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested pools.
func (c *FakePools) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(poolsResource, c.ns, opts))

}

// Create takes the representation of a pool and creates it.  Returns the server's representation of the pool, and an error, if there is any.
func (c *FakePools) Create(pool *v1alpha1.Pool) (result *v1alpha1.Pool, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(poolsResource, c.ns, pool), &v1alpha1.Pool{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Pool), err
}

// Update takes the representation of a pool and updates it. Returns the server's representation of the pool, and an error, if there is any.
func (c *FakePools) Update(pool *v1alpha1.Pool) (result *v1alpha1.Pool, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(poolsResource, c.ns, pool), &v1alpha1.Pool{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Pool), err
}

// Delete takes name of the pool and deletes it. Returns an error if one occurs.
func (c *FakePools) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(poolsResource, c.ns, name), &v1alpha1.Pool{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakePools) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(poolsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.PoolList{})
	return err
}

// Patch applies the patch and returns the patched pool.
func (c *FakePools) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Pool, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(poolsResource, c.ns, name, data, subresources...), &v1alpha1.Pool{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Pool), err
}
