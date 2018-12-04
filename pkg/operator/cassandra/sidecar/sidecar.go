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

package sidecar

import (
	"fmt"
	"github.com/coreos/pkg/capnslog"
	"github.com/davecgh/go-spew/spew"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	rookClientset "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	"github.com/yanniszark/go-nodetool/nodetool"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"time"
)

// MemberController encapsulates all the tools the sidecar needs to
// talk to the Kubernetes API
type MemberController struct {
	// Metadata of the specific Member
	name, namespace, ip       string
	cluster, datacenter, rack string
	mode                      cassandrav1alpha1.ClusterMode

	// Clients and listers to handle Kubernetes Objects
	kubeClient          kubernetes.Interface
	rookClient          rookClientset.Interface
	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced

	nodetool *nodetool.Nodetool
	queue    workqueue.RateLimitingInterface
	logger   *capnslog.PackageLogger
}

// New return a new MemberController
func New(
	name, namespace string,
	kubeClient kubernetes.Interface,
	rookClient rookClientset.Interface,
	serviceInformer coreinformers.ServiceInformer,
) (*MemberController, error) {

	logger := capnslog.NewPackageLogger("github.com/rook/rook", "sidecar")

	// Get the member's service
	var memberService *corev1.Service
	var err error
	for {
		memberService, err = kubeClient.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			logger.Infof("Something went wrong trying to get Member Service %s", name)

		} else if len(memberService.Spec.ClusterIP) > 0 {
			break
		}
		// If something went wrong, wait a little and retry
		time.Sleep(500 * time.Millisecond)
	}

	// Get the Member's metadata from the Pod's labels
	pod, err := kubeClient.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Create a new nodetool interface to talk to Cassandra
	url, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d/jolokia/", constants.JolokiaPort))
	if err != nil {
		return nil, err
	}
	nodetool := nodetool.NewFromURL(url)

	// Get the member's cluster
	cluster, err := rookClient.CassandraV1alpha1().Clusters(namespace).Get(pod.Labels[constants.ClusterNameLabel], metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	m := &MemberController{
		name:                name,
		namespace:           namespace,
		ip:                  memberService.Spec.ClusterIP,
		cluster:             pod.Labels[constants.ClusterNameLabel],
		datacenter:          pod.Labels[constants.DatacenterNameLabel],
		rack:                pod.Labels[constants.RackNameLabel],
		mode:                cluster.Spec.Mode,
		kubeClient:          kubeClient,
		rookClient:          rookClient,
		serviceLister:       serviceInformer.Lister(),
		serviceListerSynced: serviceInformer.Informer().HasSynced,
		nodetool:            nodetool,
		queue:               workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		logger:              logger,
	}

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			if svc.Name != m.name {
				logger.Errorf("Lister returned unexpected service %s", svc.Name)
				return
			}
			m.enqueueMemberService(svc)
		},
		UpdateFunc: func(old, new interface{}) {
			oldService := old.(*corev1.Service)
			newService := new.(*corev1.Service)
			if oldService.ResourceVersion == newService.ResourceVersion {
				return
			}
			if reflect.DeepEqual(oldService.Labels, newService.Labels) {
				return
			}
			logger.Infof("New event for my MemberService %s", newService.Name)
			m.enqueueMemberService(newService)
		},
		DeleteFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			if svc.Name == m.name {
				logger.Errorf("Unexpected deletion of MemberService %s", svc.Name)
			}
		},
	})

	return m, nil
}

// Run starts executing the sync loop for the sidecar
func (m *MemberController) Run(threadiness int, stopCh <-chan struct{}) error {

	defer runtime.HandleCrash()

	if ok := cache.WaitForCacheSync(stopCh, m.serviceListerSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	if err := m.onStartup(); err != nil {
		return fmt.Errorf("error on startup: %s", err.Error())
	}

	m.logger.Infof("Main event loop")
	go wait.Until(m.runWorker, time.Second, stopCh)

	<-stopCh
	m.logger.Info("Shutting down sidecar.")
	return nil

}

func (m *MemberController) runWorker() {
	for m.processNextWorkItem() {
	}
}

func (m *MemberController) processNextWorkItem() bool {
	obj, shutdown := m.queue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer m.queue.Done(obj)
		key, ok := obj.(string)
		if !ok {
			m.queue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in queue but got %#v", obj))
		}
		if err := m.syncHandler(key); err != nil {
			m.queue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s', requeueing: %s", key, err.Error())
		}
		m.queue.Forget(obj)
		m.logger.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

func (m *MemberController) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name.
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Cluster resource with this namespace/name
	svc, err := m.serviceLister.Services(namespace).Get(name)
	if err != nil {
		// The Cluster resource may no longer exist, in which case we stop processing.
		if apierrors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("member service '%s' in work queue no longer exists", key))
			return nil
		}
		return fmt.Errorf("unexpected error while getting member service object: %s", err)
	}

	m.logger.Infof("handling member service object: %+v", spew.Sdump(svc))
	err = m.Sync(svc)

	return err
}

// onStartup is executed before the MemberController starts
// its sync loop.
func (m *MemberController) onStartup() error {

	// Setup HTTP checks
	m.logger.Info("Setting up HTTP Checks...")
	go func() {
		err := m.setupHTTPChecks()
		m.logger.Fatalf("Error with HTTP Server: %s", err.Error())
		panic("Something went wrong with the HTTP Checks")
	}()

	// Prepare config files for Cassandra
	m.logger.Infof("Generating cassandra config files...")
	if err := m.generateConfigFiles(); err != nil {
		return fmt.Errorf("error generating config files: %s", err.Error())
	}

	// Start the database daemon
	cmd := exec.Command(entrypointPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		m.logger.Errorf("error starting database daemon: %s", err.Error())
		return err
	}

	return nil
}

func (m *MemberController) enqueueMemberService(obj metav1.Object) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	m.queue.AddRateLimited(key)
}
