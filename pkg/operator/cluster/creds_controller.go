/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

package cluster

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/api/core/v1"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/rook/rook/pkg/clusterd"
)

var (
	// controllerResyncPeriod potentially can be part of rook Operator configuration.
	controllerResyncPeriod = 120 * time.Second
	// controllerRetry is number of retries to process an event
	controllerRetry = 5
)

// PVCController implements Kube controller that monitors rook PVC events.
// The controller ensures all kube namespaces have a set of credentials
// to manage lifeCycle of ceph primitives.
type PVCController struct {
	queue            workqueue.RateLimitingInterface
	indexer          cache.Indexer
	informer         cache.Controller
	clusterContext   *clusterd.Context
	clusterNamespace string
}

func newCredsController(context *clusterd.Context, ns string) *PVCController {
	return &PVCController{
		clusterNamespace: ns,
		clusterContext:   context,
		queue:            workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
}

func (c *PVCController) pvcHandler(key interface{}) error {
	obj, _, err := c.indexer.GetByKey(key.(string))
	if err != nil {
		return err
	}

	pvc := obj.(*v1.PersistentVolumeClaim)

	// Validating pvc class, only acting on rook primitives
	if !strings.Contains(pvc.Annotations["volume.beta.kubernetes.io/storage-provisioner"], "rook") {
		return nil
	}

	cephUser := newCephUser(
		c.clusterContext,
		c.clusterNamespace,
		pvc.ObjectMeta.Namespace)
	if _, err := cephUser.create(); err != nil {
		return err
	}

	if err = cephUser.setKubeSecret(); err != nil {
		return err
	}

	return nil
}

func (c *PVCController) run(stopCh chan struct{}) {
	eventList := cache.NewListWatchFromClient(
		c.clusterContext.Clientset.Core().RESTClient(),
		"persistentvolumeclaims",
		v1.NamespaceAll,
		fields.Everything(),
	)

	c.indexer, c.informer = cache.NewIndexerInformer(
		eventList,
		&v1.PersistentVolumeClaim{},
		controllerResyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		},
		cache.Indexers{},
	)

	defer runtime.HandleCrash()
	defer c.queue.ShutDown()
	logger.Info("Starting PVC controller (managing ceph creds).")

	go c.informer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
	logger.Info("Stopping PVC (ceph creds) controller.")
}

func (c *PVCController) runWorker() {
	for c.processNextItem() {
	}
}

func (c *PVCController) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	defer c.queue.Done(key)

	err := c.pvcHandler(key)
	c.handleErr(err, key)
	return true
}

func (c *PVCController) onAdd(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err == nil {
		c.queue.Add(key)
	}
}

func (c *PVCController) onUpdate(obj, oldObj interface{}) {
	logger.Debug("Not implemented. Checking Ceph Creds on pvc UPDATE events.")
}

func (c *PVCController) onDelete(obj interface{}) {
	logger.Debug("Not implemented. Checking Ceph Creds on pvc DELETE events.")
}

func (c *PVCController) handleErr(err error, obj interface{}) {
	if err == nil {
		c.queue.Forget(obj)
		return
	}

	if c.queue.NumRequeues(obj) < controllerRetry {
		logger.Errorf("Ceph Creds controller error: %+v", err)
		c.queue.AddRateLimited(obj)
		return
	}

	logger.Errorf("Gave up on pvc event: %v, %v", obj, err)
	c.queue.Forget(obj)
	runtime.HandleError(err)
}
