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

package versioned

import (
	glog "github.com/golang/glog"
	cephv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/ceph/v1alpha1"
	cockroachdbv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/cockroachdb/v1alpha1"
	rookv1alpha1 "github.com/rook/rook/pkg/client/clientset/versioned/typed/rook/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/client/clientset/versioned/typed/rook/v1alpha2"
	discovery "k8s.io/client-go/discovery"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	CephV1alpha1() cephv1alpha1.CephV1alpha1Interface
	// Deprecated: please explicitly pick a version if possible.
	Ceph() cephv1alpha1.CephV1alpha1Interface
	CockroachdbV1alpha1() cockroachdbv1alpha1.CockroachdbV1alpha1Interface
	// Deprecated: please explicitly pick a version if possible.
	Cockroachdb() cockroachdbv1alpha1.CockroachdbV1alpha1Interface
	RookV1alpha1() rookv1alpha1.RookV1alpha1Interface
	// Deprecated: please explicitly pick a version if possible.
	Rook() rookv1alpha1.RookV1alpha1Interface
	RookV1alpha2() rookv1alpha2.RookV1alpha2Interface
}

// Clientset contains the clients for groups. Each group has exactly one
// version included in a Clientset.
type Clientset struct {
	*discovery.DiscoveryClient
	cephV1alpha1        *cephv1alpha1.CephV1alpha1Client
	cockroachdbV1alpha1 *cockroachdbv1alpha1.CockroachdbV1alpha1Client
	rookV1alpha1        *rookv1alpha1.RookV1alpha1Client
	rookV1alpha2        *rookv1alpha2.RookV1alpha2Client
}

// CephV1alpha1 retrieves the CephV1alpha1Client
func (c *Clientset) CephV1alpha1() cephv1alpha1.CephV1alpha1Interface {
	return c.cephV1alpha1
}

// Deprecated: Ceph retrieves the default version of CephClient.
// Please explicitly pick a version.
func (c *Clientset) Ceph() cephv1alpha1.CephV1alpha1Interface {
	return c.cephV1alpha1
}

// CockroachdbV1alpha1 retrieves the CockroachdbV1alpha1Client
func (c *Clientset) CockroachdbV1alpha1() cockroachdbv1alpha1.CockroachdbV1alpha1Interface {
	return c.cockroachdbV1alpha1
}

// Deprecated: Cockroachdb retrieves the default version of CockroachdbClient.
// Please explicitly pick a version.
func (c *Clientset) Cockroachdb() cockroachdbv1alpha1.CockroachdbV1alpha1Interface {
	return c.cockroachdbV1alpha1
}

// RookV1alpha1 retrieves the RookV1alpha1Client
func (c *Clientset) RookV1alpha1() rookv1alpha1.RookV1alpha1Interface {
	return c.rookV1alpha1
}

// Deprecated: Rook retrieves the default version of RookClient.
// Please explicitly pick a version.
func (c *Clientset) Rook() rookv1alpha1.RookV1alpha1Interface {
	return c.rookV1alpha1
}

// RookV1alpha2 retrieves the RookV1alpha2Client
func (c *Clientset) RookV1alpha2() rookv1alpha2.RookV1alpha2Interface {
	return c.rookV1alpha2
}

// Discovery retrieves the DiscoveryClient
func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

// NewForConfig creates a new Clientset for the given config.
func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}
	var cs Clientset
	var err error
	cs.cephV1alpha1, err = cephv1alpha1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	cs.cockroachdbV1alpha1, err = cockroachdbv1alpha1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	cs.rookV1alpha1, err = rookv1alpha1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	cs.rookV1alpha2, err = rookv1alpha2.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}

	cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfig(&configShallowCopy)
	if err != nil {
		glog.Errorf("failed to create the DiscoveryClient: %v", err)
		return nil, err
	}
	return &cs, nil
}

// NewForConfigOrDie creates a new Clientset for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *Clientset {
	var cs Clientset
	cs.cephV1alpha1 = cephv1alpha1.NewForConfigOrDie(c)
	cs.cockroachdbV1alpha1 = cockroachdbv1alpha1.NewForConfigOrDie(c)
	cs.rookV1alpha1 = rookv1alpha1.NewForConfigOrDie(c)
	cs.rookV1alpha2 = rookv1alpha2.NewForConfigOrDie(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClientForConfigOrDie(c)
	return &cs
}

// New creates a new Clientset for the given RESTClient.
func New(c rest.Interface) *Clientset {
	var cs Clientset
	cs.cephV1alpha1 = cephv1alpha1.New(c)
	cs.cockroachdbV1alpha1 = cockroachdbv1alpha1.New(c)
	cs.rookV1alpha1 = rookv1alpha1.New(c)
	cs.rookV1alpha2 = rookv1alpha2.New(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClient(c)
	return &cs
}
