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

// FakeFilesystems implements FilesystemInterface
type FakeFilesystems struct {
	Fake *FakeCephV1alpha1
	ns   string
}

var filesystemsResource = schema.GroupVersionResource{Group: "ceph.rook.io", Version: "v1alpha1", Resource: "filesystems"}

var filesystemsKind = schema.GroupVersionKind{Group: "ceph.rook.io", Version: "v1alpha1", Kind: "Filesystem"}

// Get takes name of the filesystem, and returns the corresponding filesystem object, and an error if there is any.
func (c *FakeFilesystems) Get(name string, options v1.GetOptions) (result *v1alpha1.Filesystem, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(filesystemsResource, c.ns, name), &v1alpha1.Filesystem{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Filesystem), err
}

// List takes label and field selectors, and returns the list of Filesystems that match those selectors.
func (c *FakeFilesystems) List(opts v1.ListOptions) (result *v1alpha1.FilesystemList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(filesystemsResource, filesystemsKind, c.ns, opts), &v1alpha1.FilesystemList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.FilesystemList{}
	for _, item := range obj.(*v1alpha1.FilesystemList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested filesystems.
func (c *FakeFilesystems) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(filesystemsResource, c.ns, opts))

}

// Create takes the representation of a filesystem and creates it.  Returns the server's representation of the filesystem, and an error, if there is any.
func (c *FakeFilesystems) Create(filesystem *v1alpha1.Filesystem) (result *v1alpha1.Filesystem, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(filesystemsResource, c.ns, filesystem), &v1alpha1.Filesystem{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Filesystem), err
}

// Update takes the representation of a filesystem and updates it. Returns the server's representation of the filesystem, and an error, if there is any.
func (c *FakeFilesystems) Update(filesystem *v1alpha1.Filesystem) (result *v1alpha1.Filesystem, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(filesystemsResource, c.ns, filesystem), &v1alpha1.Filesystem{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Filesystem), err
}

// Delete takes name of the filesystem and deletes it. Returns an error if one occurs.
func (c *FakeFilesystems) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(filesystemsResource, c.ns, name), &v1alpha1.Filesystem{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeFilesystems) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(filesystemsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.FilesystemList{})
	return err
}

// Patch applies the patch and returns the patched filesystem.
func (c *FakeFilesystems) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Filesystem, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(filesystemsResource, c.ns, name, data, subresources...), &v1alpha1.Filesystem{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Filesystem), err
}
