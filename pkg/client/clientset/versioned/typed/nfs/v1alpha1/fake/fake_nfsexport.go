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

// FakeNFSExports implements NFSExportInterface
type FakeNFSExports struct {
	Fake *FakeNfsV1alpha1
	ns   string
}

var nfsexportsResource = schema.GroupVersionResource{Group: "nfs.rook.io", Version: "v1alpha1", Resource: "nfsexports"}

var nfsexportsKind = schema.GroupVersionKind{Group: "nfs.rook.io", Version: "v1alpha1", Kind: "NFSExport"}

// Get takes name of the nFSExport, and returns the corresponding nFSExport object, and an error if there is any.
func (c *FakeNFSExports) Get(name string, options v1.GetOptions) (result *v1alpha1.NFSExport, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(nfsexportsResource, c.ns, name), &v1alpha1.NFSExport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NFSExport), err
}

// List takes label and field selectors, and returns the list of NFSExports that match those selectors.
func (c *FakeNFSExports) List(opts v1.ListOptions) (result *v1alpha1.NFSExportList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(nfsexportsResource, nfsexportsKind, c.ns, opts), &v1alpha1.NFSExportList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.NFSExportList{}
	for _, item := range obj.(*v1alpha1.NFSExportList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested nFSExports.
func (c *FakeNFSExports) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(nfsexportsResource, c.ns, opts))

}

// Create takes the representation of a nFSExport and creates it.  Returns the server's representation of the nFSExport, and an error, if there is any.
func (c *FakeNFSExports) Create(nFSExport *v1alpha1.NFSExport) (result *v1alpha1.NFSExport, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(nfsexportsResource, c.ns, nFSExport), &v1alpha1.NFSExport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NFSExport), err
}

// Update takes the representation of a nFSExport and updates it. Returns the server's representation of the nFSExport, and an error, if there is any.
func (c *FakeNFSExports) Update(nFSExport *v1alpha1.NFSExport) (result *v1alpha1.NFSExport, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(nfsexportsResource, c.ns, nFSExport), &v1alpha1.NFSExport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NFSExport), err
}

// Delete takes name of the nFSExport and deletes it. Returns an error if one occurs.
func (c *FakeNFSExports) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(nfsexportsResource, c.ns, name), &v1alpha1.NFSExport{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeNFSExports) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(nfsexportsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.NFSExportList{})
	return err
}

// Patch applies the patch and returns the patched nFSExport.
func (c *FakeNFSExports) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.NFSExport, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(nfsexportsResource, c.ns, name, data, subresources...), &v1alpha1.NFSExport{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.NFSExport), err
}
