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
	clientset "github.com/rook/rook/pkg/client/clientset/versioned"
	cephv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/ceph/v1alpha1"
	fakecephv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/ceph/v1alpha1/fake"
	cephv1beta1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/ceph/v1beta1"
	fakecephv1beta1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/ceph/v1beta1/fake"
	cockroachdbv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/cockroachdb/v1alpha1"
	fakecockroachdbv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/cockroachdb/v1alpha1/fake"
	miniov1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/minio/v1alpha1"
	fakeminiov1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/minio/v1alpha1/fake"
	rookv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/rook/v1alpha1"
	fakerookv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/rook/v1alpha1/fake"
	rookv1alpha2 "github.com/rook/rook/pkg/client/clientset/versioned/typed/rook/v1alpha2"
	fakerookv1alpha2 "github.com/rook/rook/pkg/client/clientset/versioned/typed/rook/v1alpha2/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/testing"
)

// NewSimpleClientset returns a clientset that will respond with the provided objects.
// It's backed by a very simple object tracker that processes creates, updates and deletions as-is,
// without applying any validations and/or defaults. It shouldn't be considered a replacement
// for a real clientset and is mostly useful in simple unit tests.
func NewSimpleClientset(objects ...runtime.Object) *Clientset {
	o := testing.NewObjectTracker(scheme, codecs.UniversalDecoder())
	for _, obj := range objects {
		if err := o.Add(obj); err != nil {
			panic(err)
		}
	}

	cs := &Clientset{}
	cs.Fake.AddReactor("*", "*", testing.ObjectReaction(o))
	cs.Fake.AddWatchReactor("*", testing.DefaultWatchReactor(watch.NewFake(), nil))

	cs.discovery = &fakediscovery.FakeDiscovery{Fake: &cs.Fake}
	return cs
}

// Clientset implements clientset.Interface. Meant to be embedded into a
// struct to get a default implementation. This makes faking out just the method
// you want to test easier.
type Clientset struct {
	testing.Fake
	discovery *fakediscovery.FakeDiscovery
}

func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

var _ clientset.Interface = &Clientset{}

// CephV1alpha1 retrieves the CephV1alpha1Client
func (c *Clientset) CephV1alpha1() cephv1alpha1.CephV1alpha1Interface {
	return &fakecephv1alpha1.FakeCephV1alpha1{Fake: &c.Fake}
}

// CephV1beta1 retrieves the CephV1beta1Client
func (c *Clientset) CephV1beta1() cephv1beta1.CephV1beta1Interface {
	return &fakecephv1beta1.FakeCephV1beta1{Fake: &c.Fake}
}

// Ceph retrieves the CephV1beta1Client
func (c *Clientset) Ceph() cephv1beta1.CephV1beta1Interface {
	return &fakecephv1beta1.FakeCephV1beta1{Fake: &c.Fake}
}

// CockroachdbV1alpha1 retrieves the CockroachdbV1alpha1Client
func (c *Clientset) CockroachdbV1alpha1() cockroachdbv1alpha1.CockroachdbV1alpha1Interface {
	return &fakecockroachdbv1alpha1.FakeCockroachdbV1alpha1{Fake: &c.Fake}
}

// Cockroachdb retrieves the CockroachdbV1alpha1Client
func (c *Clientset) Cockroachdb() cockroachdbv1alpha1.CockroachdbV1alpha1Interface {
	return &fakecockroachdbv1alpha1.FakeCockroachdbV1alpha1{Fake: &c.Fake}
}

// MinioV1alpha1 retrieves the MinioV1alpha1Client
func (c *Clientset) MinioV1alpha1() miniov1alpha1.MinioV1alpha1Interface {
	return &fakeminiov1alpha1.FakeMinioV1alpha1{Fake: &c.Fake}
}

// Minio retrieves the MinioV1alpha1Client
func (c *Clientset) Minio() miniov1alpha1.MinioV1alpha1Interface {
	return &fakeminiov1alpha1.FakeMinioV1alpha1{Fake: &c.Fake}
}

// RookV1alpha1 retrieves the RookV1alpha1Client
func (c *Clientset) RookV1alpha1() rookv1alpha1.RookV1alpha1Interface {
	return &fakerookv1alpha1.FakeRookV1alpha1{Fake: &c.Fake}
}

// Rook retrieves the RookV1alpha1Client
func (c *Clientset) Rook() rookv1alpha1.RookV1alpha1Interface {
	return &fakerookv1alpha1.FakeRookV1alpha1{Fake: &c.Fake}
}

// RookV1alpha2 retrieves the RookV1alpha2Client
func (c *Clientset) RookV1alpha2() rookv1alpha2.RookV1alpha2Interface {
	return &fakerookv1alpha2.FakeRookV1alpha2{Fake: &c.Fake}
}
