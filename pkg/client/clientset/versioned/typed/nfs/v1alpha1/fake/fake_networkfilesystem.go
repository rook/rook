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
	v1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeNetworkFileSystems implements NetworkFileSystemInterface
type FakeNetworkFileSystems struct {
	Fake *FakeNfsV1alpha1
	ns   string
}

var networkfilesystemsResource = schema.GroupVersionResource{Group: "nfs.rook.io", Version: "v1alpha1", Resource: "networkfilesystems"}

var networkfilesystemsKind = schema.GroupVersionKind{Group: "nfs.rook.io", Version: "v1alpha1", Kind: "NetworkFileSystem"}

// Get takes name of the networkFileSystem, and returns the corresponding networkFileSystem object, and an error if there is any.
func (c *FakeNetworkFileSystems) Get(name string, options v1.GetOptions) (result *v1alpha1.NetworkFileSystem, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(networkfilesystemsResource, c.ns, name), &v1alpha1.NetworkFileSystem{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NetworkFileSystem), err
}

// List takes label and field selectors, and returns the list of NetworkFileSystems that match those selectors.
func (c *FakeNetworkFileSystems) List(opts v1.ListOptions) (result *v1alpha1.NetworkFileSystemList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(networkfilesystemsResource, networkfilesystemsKind, c.ns, opts), &v1alpha1.NetworkFileSystemList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.NetworkFileSystemList{}
	for _, item := range obj.(*v1alpha1.NetworkFileSystemList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested networkFileSystems.
func (c *FakeNetworkFileSystems) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(networkfilesystemsResource, c.ns, opts))

}

// Create takes the representation of a networkFileSystem and creates it.  Returns the server's representation of the networkFileSystem, and an error, if there is any.
func (c *FakeNetworkFileSystems) Create(networkFileSystem *v1alpha1.NetworkFileSystem) (result *v1alpha1.NetworkFileSystem, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(networkfilesystemsResource, c.ns, networkFileSystem), &v1alpha1.NetworkFileSystem{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NetworkFileSystem), err
}

// Update takes the representation of a networkFileSystem and updates it. Returns the server's representation of the networkFileSystem, and an error, if there is any.
func (c *FakeNetworkFileSystems) Update(networkFileSystem *v1alpha1.NetworkFileSystem) (result *v1alpha1.NetworkFileSystem, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(networkfilesystemsResource, c.ns, networkFileSystem), &v1alpha1.NetworkFileSystem{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NetworkFileSystem), err
}

// Delete takes name of the networkFileSystem and deletes it. Returns an error if one occurs.
func (c *FakeNetworkFileSystems) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(networkfilesystemsResource, c.ns, name), &v1alpha1.NetworkFileSystem{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeNetworkFileSystems) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(networkfilesystemsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.NetworkFileSystemList{})
	return err
}

// Patch applies the patch and returns the patched networkFileSystem.
func (c *FakeNetworkFileSystems) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NetworkFileSystem, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(networkfilesystemsResource, c.ns, name, data, subresources...), &v1alpha1.NetworkFileSystem{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NetworkFileSystem), err
}
