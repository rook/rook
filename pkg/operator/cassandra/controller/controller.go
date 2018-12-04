/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package controller

import (
	"fmt"
	"github.com/coreos/pkg/capnslog"
	"github.com/davecgh/go-spew/spew"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	rookClientset "github.com/rook/rook/pkg/client/clientset/versioned"
	rookScheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	informersv1alpha1 "github.com/rook/rook/pkg/client/informers/externalversions/cassandra.rook.io/v1alpha1"
	listersv1alpha1 "github.com/rook/rook/pkg/client/listers/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"reflect"
	"time"
)

const (
	controllerName   = "cassandra-controller"
	clusterQueueName = "cluster-queue"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cassandra-controller")

// ClusterController encapsulates all the tools the controller needs
// in order to talk to the Kubernetes API
type ClusterController struct {
	rookImage               string
	kubeClient              kubernetes.Interface
	rookClient              rookClientset.Interface
	clusterLister           listersv1alpha1.ClusterLister
	clusterListerSynced     cache.InformerSynced
	statefulSetLister       appslisters.StatefulSetLister
	statefulSetListerSynced cache.InformerSynced
	serviceLister           corelisters.ServiceLister
	serviceListerSynced     cache.InformerSynced
	podLister               corelisters.PodLister
	podListerSynced         cache.InformerSynced

	// queue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	queue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the Kubernetes API
	recorder record.EventRecorder
}

// New returns a new ClusterController
func New(
	rookImage string,
	kubeClient kubernetes.Interface,
	rookClient rookClientset.Interface,
	clusterInformer informersv1alpha1.ClusterInformer,
	statefulSetInformer appsinformers.StatefulSetInformer,
	serviceInformer coreinformers.ServiceInformer,
	podInformer coreinformers.PodInformer,
) *ClusterController {

	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	rookScheme.AddToScheme(scheme.Scheme)
	// Create event broadcaster
	logger.Infof("creating event broadcaster...")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logger.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName})

	cc := &ClusterController{
		rookImage:  rookImage,
		kubeClient: kubeClient,
		rookClient: rookClient,

		clusterLister:           clusterInformer.Lister(),
		clusterListerSynced:     clusterInformer.Informer().HasSynced,
		statefulSetLister:       statefulSetInformer.Lister(),
		statefulSetListerSynced: statefulSetInformer.Informer().HasSynced,
		podLister:               podInformer.Lister(),
		podListerSynced:         podInformer.Informer().HasSynced,
		serviceLister:           serviceInformer.Lister(),
		serviceListerSynced:     serviceInformer.Informer().HasSynced,

		queue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), clusterQueueName),
		recorder: recorder,
	}

	// Add event handling functions

	clusterInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newCluster := obj.(*cassandrav1alpha1.Cluster)
			cc.enqueueCluster(newCluster)
		},
		UpdateFunc: func(old, new interface{}) {
			newCluster := new.(*cassandrav1alpha1.Cluster)
			oldCluster := old.(*cassandrav1alpha1.Cluster)
			// If the Spec is the same as the one in our cache, there aren't
			// any changes we are interested in.
			if reflect.DeepEqual(newCluster.Spec, oldCluster.Spec) {
				return
			}
			cc.enqueueCluster(newCluster)
		},
		//Deletion handling:
		// Atm, the only thing left behind will be the state, ie
		// the PVCs that the StatefulSets don't erase.
		// This behaviour may actually be preferrable to deleting them,
		// since it ensures that no data will be lost if someone accidentally
		// deletes the cluster.
	})

	statefulSetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: cc.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newStatefulSet := new.(*appsv1.StatefulSet)
			oldStatefulSet := old.(*appsv1.StatefulSet)
			// If the StatefulSet is the same as the one in our cache, there
			// is no use adding it again.
			if newStatefulSet.ResourceVersion == oldStatefulSet.ResourceVersion {
				return
			}
			// If ObservedGeneration != Generation, it means that the StatefulSet controller
			// has not yet processed the current StatefulSet object.
			// That means its Status is stale and we don't want to queue it.
			if newStatefulSet.Status.ObservedGeneration != newStatefulSet.Generation {
				return
			}
			cc.handleObject(new)
		},
		DeleteFunc: cc.handleObject,
	})

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			service := obj.(*corev1.Service)
			if service.Spec.ClusterIP == corev1.ClusterIPNone {
				return
			}
			cc.handleObject(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			newService := new.(*corev1.Service)
			oldService := old.(*corev1.Service)
			if oldService.ResourceVersion == newService.ResourceVersion {
				return
			}
			cc.handleObject(new)
		},
		DeleteFunc: func(obj interface{}) {
			// TODO: investigate if further action needs to be taken
		},
	})

	return cc
}

// Run starts the ClusterController process loop
func (cc *ClusterController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer cc.queue.ShutDown()

	// 	Start the informer factories to begin populating the informer caches
	logger.Info("starting cassandra controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("waiting for informers caches to sync...")
	if ok := cache.WaitForCacheSync(
		stopCh,
		cc.clusterListerSynced,
		cc.statefulSetListerSynced,
		cc.podListerSynced,
		cc.serviceListerSynced,
	); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(cc.runWorker, time.Second, stopCh)
	}

	logger.Info("started workers")
	<-stopCh
	logger.Info("Shutting down cassandra controller workers")

	return nil
}

func (cc *ClusterController) runWorker() {
	for cc.processNextWorkItem() {
	}
}

func (cc *ClusterController) processNextWorkItem() bool {
	obj, shutdown := cc.queue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer cc.queue.Done(obj)
		key, ok := obj.(string)
		if !ok {
			cc.queue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in queue but got %#v", obj))
		}
		if err := cc.syncHandler(key); err != nil {
			cc.queue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s', requeueing: %s", key, err.Error())
		}
		cc.queue.Forget(obj)
		logger.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Cluster
// resource with the current status of the resource.
func (cc *ClusterController) syncHandler(key string) error {

	// Convert the namespace/name string into a distinct namespace and name.
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Cluster resource with this namespace/name
	cluster, err := cc.clusterLister.Clusters(namespace).Get(name)
	if err != nil {
		// The Cluster resource may no longer exist, in which case we stop processing.
		if apierrors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("cluster '%s' in work queue no longer exists", key))
			return nil
		}
		return fmt.Errorf("Unexpected error while getting cluster object: %s", err)
	}

	logger.Infof("handling cluster object: %+v", spew.Sdump(cluster))
	// Deepcopy here to ensure nobody messes with the cache.
	old, new := cluster, cluster.DeepCopy()
	// If sync was successful and Status has changed, update the Cluster.
	if err = cc.Sync(new); err == nil && !reflect.DeepEqual(old.Status, new.Status) {
		err = util.PatchClusterStatus(new, cc.rookClient)
	}

	return err
}

// enqueueCluster takes a Cluster resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should not be
// passed resources of any type other than Cluster.
func (cc *ClusterController) enqueueCluster(obj *cassandrav1alpha1.Cluster) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	cc.queue.AddRateLimited(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Cluster resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Cluster resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (cc *ClusterController) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		logger.Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	logger.Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If the object is not a Cluster or doesn't belong to our APIVersion, skip it.
		if ownerRef.Kind != "Cluster" || ownerRef.APIVersion != cassandrav1alpha1.APIVersion {
			return
		}

		cluster, err := cc.clusterLister.Clusters(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.Infof("ignoring orphaned object '%s' of cluster '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		cc.enqueueCluster(cluster)
		return
	}
}
