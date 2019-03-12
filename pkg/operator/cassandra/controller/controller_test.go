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
	"time"

	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	rookScheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	rookinformers "github.com/rook/rook/pkg/client/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const informerResyncPeriod = time.Millisecond

// newFakeClusterController returns a ClusterController with fake clientsets
// and informers.
// The kubeObjects and rookObjects given as input are injected into the informers' cache.
func newFakeClusterController(kubeObjects []runtime.Object, rookObjects []runtime.Object) *ClusterController {

	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	rookScheme.AddToScheme(scheme.Scheme)

	kubeClient := kubefake.NewSimpleClientset(kubeObjects...)
	rookClient := rookfake.NewSimpleClientset(rookObjects...)

	kubeSharedInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, informerResyncPeriod)
	rookSharedInformerFactory := rookinformers.NewSharedInformerFactory(rookClient, informerResyncPeriod)
	stopCh := make(chan struct{})

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName})

	cc := &ClusterController{
		rookImage:  "",
		kubeClient: kubeClient,
		rookClient: rookClient,

		clusterLister:           rookSharedInformerFactory.Cassandra().V1alpha1().Clusters().Lister(),
		clusterListerSynced:     rookSharedInformerFactory.Cassandra().V1alpha1().Clusters().Informer().HasSynced,
		statefulSetLister:       kubeSharedInformerFactory.Apps().V1().StatefulSets().Lister(),
		statefulSetListerSynced: kubeSharedInformerFactory.Apps().V1().StatefulSets().Informer().HasSynced,
		podLister:               kubeSharedInformerFactory.Core().V1().Pods().Lister(),
		podListerSynced:         kubeSharedInformerFactory.Core().V1().Pods().Informer().HasSynced,
		serviceLister:           kubeSharedInformerFactory.Core().V1().Services().Lister(),
		serviceListerSynced:     kubeSharedInformerFactory.Core().V1().Services().Informer().HasSynced,

		queue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), clusterQueueName),
		recorder: recorder,
	}

	kubeSharedInformerFactory.Start(stopCh)
	rookSharedInformerFactory.Start(stopCh)

	cache.WaitForCacheSync(
		stopCh,
		cc.clusterListerSynced,
		cc.statefulSetListerSynced,
		cc.serviceListerSynced,
		cc.podListerSynced,
	)

	return cc
}
