/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package noobaa

import (
	"strings"
	"sync"
	"time"

	semver "github.com/hashicorp/go-version"
	nbv1 "github.com/rook/rook/pkg/apis/noobaa.rook.io/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	ContainerImageOrg              = "noobaa"
	ContainerImageRepo             = "noobaa-core"
	ContainerImageTag              = "4.0"
	ContainerImageConstraintSemver = ">=4, <5"
	ContainerImageName             = ContainerImageOrg + "/" + ContainerImageRepo
	ContainerImage                 = ContainerImageName + ":" + ContainerImageTag
)

var (
	ContainerImageConstraint, _ = semver.NewConstraint(ContainerImageConstraintSemver)
)

// SystemController represents a controller object for noobaa system
type SystemController struct {
	Operator *Operator

	// queue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	queue workqueue.RateLimitingInterface
}

// NewSystemController create controller for watching noobaa custom resources created
func NewSystemController(operator *Operator) *SystemController {

	c := &SystemController{
		Operator: operator,
		queue:    workqueue.NewNamedRateLimitingQueue(NewCustomRateLimiter(), "SystemController"),
	}

	// We need to register our informer handlers here before Run()
	// so that the informers can sync the cache before the controller runs
	logger.Info("System: Register controller handlers for informers ...")
	commonHandler := c.Operator.NewObjectEventHandler(c.HandleOwnedObject)
	c.Operator.AddEventHandler(c.Operator.PodInformer.Informer(), commonHandler)
	c.Operator.AddEventHandler(c.Operator.NodeInformer.Informer(), cache.ResourceEventHandlerFuncs{})
	c.Operator.AddEventHandler(c.Operator.SecretInformer.Informer(), commonHandler)
	c.Operator.AddEventHandler(c.Operator.ServiceInformer.Informer(), commonHandler)
	c.Operator.AddEventHandler(c.Operator.ServiceAccountInformer.Informer(), commonHandler)
	c.Operator.AddEventHandler(c.Operator.RoleInformer.Informer(), commonHandler)
	c.Operator.AddEventHandler(c.Operator.RoleBindingInformer.Informer(), commonHandler)
	c.Operator.AddEventHandler(c.Operator.StatefulSetInformer.Informer(), c.statefulSetHandler())
	c.Operator.AddEventHandler(c.Operator.NooBaaSystemInformer.Informer(), c.systemHandler())
	return c
}

// Run works on the controller queue and for each system runs sync.
// It will return the system to the queue on failure.
func (c *SystemController) Run() {
	for {
		item, shutdown := c.queue.Get()
		if shutdown {
			return
		}
		fullname := item.(string)
		err := c.SyncSystem(fullname)
		if err != nil {
			logger.Infof("System: Requeueing system %s", fullname)
			c.queue.AddRateLimited(fullname)
		} else {
			c.queue.Forget(fullname)
		}
		c.queue.Done(fullname)
	}
}

// Stop shutdowns the work queue which will not accept new items.
func (c *SystemController) Stop() {
	c.queue.ShutDown()
}

// SyncSystem finds the system in the informer's cache and calls sync on it.
func (c *SystemController) SyncSystem(fullname string) error {

	spl := strings.Split(fullname, "/")
	namespace, name := spl[0], spl[1]
	sys, err := c.Operator.NooBaaSystemInformer.Lister().NooBaaSystems(namespace).Get(name)
	if errors.IsNotFound(err) {
		// The system may no longer exist, in which case we just stop processing it
		logger.Infof("System: SyncSystem() Queued system no longer exists %s", fullname)
		return nil
	}
	if err != nil {
		logger.Errorf("System: SyncSystem() Unexpected error getting system %s", fullname)
		return err
	}
	if sys.APIVersion == "" || sys.Kind == "" {
		sys.GetObjectKind().SetGroupVersionKind(nbv1.SchemeGroupVersion.WithKind(nbv1.NooBaaSystemKind))
		logger.Tracef("System: SyncSystem() Update system with empty APIVersion/Kind %s", fullname)
	}

	logger.Infof("System: SyncSystem() Starting to sync: %s", fullname)
	state := NewSystemState(c.Operator, sys)
	err = state.Sync()
	if err != nil {
		logger.Errorf("❌ System: SyncSystem Error: %s %s", fullname, err)
		if state.ErrorSuppressed {
			return nil
		}
		return err
	}

	logger.Infof("✅ System: SyncSystem Done: %s", fullname)

	return nil
}

// EnqueueSystem takes a system namespace and name and adds it to the work queue.
func (c *SystemController) EnqueueSystem(namespace, name string) {
	fullname := namespace + "/" + name
	c.queue.AddRateLimited(fullname)
}

// HandleOwnedObject will take any resource implementing metav1.Object and attempt
// to find the syste  resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that system to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *SystemController) HandleOwnedObject(obj interface{}) {

	owner, metaObject := c.Operator.GetControllerOwner(obj)
	if owner == nil {
		logger.Warningf("System: Ignore event - no owner %+v", metaObject)
		return
	}

	fullname := metaObject.GetNamespace() + "/" + metaObject.GetName()
	logger.Infof("System: Event on: %s", fullname)

	if owner.Kind != nbv1.NooBaaSystemKind {
		logger.Tracef("System: Ignore event on %s - Unexpected kind of owner %+v", fullname, owner)
		return
	}

	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		logger.Tracef("System: Ignore event on %s - Bad APIVersion %+v", fullname, owner)
		return
	}

	if gv != nbv1.SchemeGroupVersion {
		logger.Tracef("System: Ignore event on %s - with unexpected APIVersion %+v", fullname, owner)
		return
	}

	sys, err := c.Operator.NooBaaSystemInformer.Lister().NooBaaSystems(metaObject.GetNamespace()).Get(owner.Name)
	if err != nil {
		logger.Tracef("System: Ignore event on %s - system not found %+v", fullname, owner)
		return
	}

	if sys.UID != owner.UID {
		logger.Tracef("System: Ignore event on %s - UID mismatch %+v system %+v", fullname, owner, sys.ObjectMeta)
		return
	}

	c.EnqueueSystem(sys.Namespace, sys.Name)
}

func (c *SystemController) systemHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			sys := obj.(*nbv1.NooBaaSystem)
			c.EnqueueSystem(sys.Namespace, sys.Name)
		},
		DeleteFunc: func(obj interface{}) {
			// TODO: handled by GC, any need to do explicit?
		},
		UpdateFunc: func(old, obj interface{}) {
			sys := obj.(*nbv1.NooBaaSystem)
			oldsys := old.(*nbv1.NooBaaSystem)
			if sys.ResourceVersion == oldsys.ResourceVersion {
				logger.Tracef("System: Ignore cached ResourceVersion=%s for %s", sys.ResourceVersion, sys.Name)
				return
			}
			if equality.Semantic.DeepEqual(sys.Spec, oldsys.Spec) {
				return
			}
			c.EnqueueSystem(sys.Namespace, sys.Name)
		},
	}
}

func (c *SystemController) statefulSetHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    c.HandleOwnedObject,
		DeleteFunc: c.HandleOwnedObject,
		UpdateFunc: func(old, obj interface{}) {
			oldStatefulSet := old.(*appsv1.StatefulSet)
			newStatefulSet := obj.(*appsv1.StatefulSet)
			if newStatefulSet.ResourceVersion == oldStatefulSet.ResourceVersion {
				return
			}
			// If ObservedGeneration != Generation, it means that the StatefulSet controller
			// has not yet processed the current StatefulSet object.
			// That means its Status is stale and we don't want to queue it.
			if newStatefulSet.Status.ObservedGeneration != newStatefulSet.Generation {
				return
			}
			c.HandleOwnedObject(obj)
		},
	}
}

// CustomRateLimiter implements our custom backoff policy for failed syncs
type CustomRateLimiter struct {
	lock  sync.Mutex
	items map[interface{}]CustomRateLimiterItemInfo
}

type CustomRateLimiterItemInfo struct {
	Requeues int
}

func NewCustomRateLimiter() *CustomRateLimiter {
	return &CustomRateLimiter{
		items: make(map[interface{}]CustomRateLimiterItemInfo),
	}
}

func (r *CustomRateLimiter) When(item interface{}) time.Duration {
	r.lock.Lock()
	defer r.lock.Unlock()
	info := r.items[item]
	info.Requeues++
	r.items[item] = info
	n := info.Requeues

	switch {
	case n == 1:
		return time.Millisecond * 200
	case n <= 60:
		return time.Second
	case n <= 120:
		return time.Second * 10
	default:
		return time.Minute
	}
}

func (r *CustomRateLimiter) Forget(item interface{}) {
	r.lock.Lock()
	defer r.lock.Unlock()
	delete(r.items, item)
}

func (r *CustomRateLimiter) NumRequeues(item interface{}) int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.items[item].Requeues
}

var _ workqueue.RateLimiter = &CustomRateLimiter{}
