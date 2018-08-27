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

package fake

import (
	v1alpha1 "github.com/rook/operator-kit/sample-operator/pkg/apis/myproject/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeSamples implements SampleInterface
type FakeSamples struct {
	Fake *FakeMyprojectV1alpha1
	ns   string
}

var samplesResource = schema.GroupVersionResource{Group: "myproject", Version: "v1alpha1", Resource: "samples"}

var samplesKind = schema.GroupVersionKind{Group: "myproject", Version: "v1alpha1", Kind: "Sample"}

// Get takes name of the sample, and returns the corresponding sample object, and an error if there is any.
func (c *FakeSamples) Get(name string, options v1.GetOptions) (result *v1alpha1.Sample, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(samplesResource, c.ns, name), &v1alpha1.Sample{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Sample), err
}

// List takes label and field selectors, and returns the list of Samples that match those selectors.
func (c *FakeSamples) List(opts v1.ListOptions) (result *v1alpha1.SampleList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(samplesResource, samplesKind, c.ns, opts), &v1alpha1.SampleList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.SampleList{}
	for _, item := range obj.(*v1alpha1.SampleList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested samples.
func (c *FakeSamples) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(samplesResource, c.ns, opts))

}

// Create takes the representation of a sample and creates it.  Returns the server's representation of the sample, and an error, if there is any.
func (c *FakeSamples) Create(sample *v1alpha1.Sample) (result *v1alpha1.Sample, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(samplesResource, c.ns, sample), &v1alpha1.Sample{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Sample), err
}

// Update takes the representation of a sample and updates it. Returns the server's representation of the sample, and an error, if there is any.
func (c *FakeSamples) Update(sample *v1alpha1.Sample) (result *v1alpha1.Sample, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(samplesResource, c.ns, sample), &v1alpha1.Sample{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Sample), err
}

// Delete takes name of the sample and deletes it. Returns an error if one occurs.
func (c *FakeSamples) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(samplesResource, c.ns, name), &v1alpha1.Sample{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeSamples) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(samplesResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.SampleList{})
	return err
}

// Patch applies the patch and returns the patched sample.
func (c *FakeSamples) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Sample, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(samplesResource, c.ns, name, data, subresources...), &v1alpha1.Sample{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Sample), err
}
