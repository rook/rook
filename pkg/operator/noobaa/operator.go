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

// Package noobaa implements noobaa operator.
package noobaa

import (
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/cmd/rook/rook"
	rook_client "github.com/rook/rook/pkg/client/clientset/versioned"
	rook_scheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	rook_informers "github.com/rook/rook/pkg/client/informers/externalversions"
	nb_informers "github.com/rook/rook/pkg/client/informers/externalversions/noobaa.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiext "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	kube_informers "k8s.io/client-go/informers"
	apps_informers "k8s.io/client-go/informers/apps/v1"
	core_informers "k8s.io/client-go/informers/core/v1"
	rbac_informers "k8s.io/client-go/informers/rbac/v1"
	kube_client "k8s.io/client-go/kubernetes"
	kube_scheme "k8s.io/client-go/kubernetes/scheme"
	corev1_typed "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "noobaa-operator")

// Operator is managing the noobaa resources
// It runs a controller per CRD to operate noobaa systems and buckets.
type Operator struct {
	DevMode                bool
	StopChan               chan struct{}
	EventRecorder          record.EventRecorder
	KubeClient             kube_client.Interface
	RookClient             rook_client.Interface
	APIExtClient           apiext.Interface
	InformersSyncedList    []cache.InformerSynced
	KubeInformerFactory    kube_informers.SharedInformerFactory
	KubeInformerFactoryAny kube_informers.SharedInformerFactory
	RookInformerFactory    rook_informers.SharedInformerFactory
	PodInformer            core_informers.PodInformer
	NodeInformer           core_informers.NodeInformer
	SecretInformer         core_informers.SecretInformer
	ServiceInformer        core_informers.ServiceInformer
	ServiceAccountInformer core_informers.ServiceAccountInformer
	StatefulSetInformer    apps_informers.StatefulSetInformer
	RoleInformer           rbac_informers.RoleInformer
	RoleBindingInformer    rbac_informers.RoleBindingInformer
	NooBaaSystemInformer   nb_informers.NooBaaSystemInformer
	SystemController       *SystemController
}

// NewOperator initializes a new operator
func NewOperator(context *clusterd.Context) *Operator {

	kubeClient := context.Clientset
	rookClient := context.RookClientset
	apiExtClient := context.APIExtensionClientset

	// Prepare an event recorder
	// Add rook types to the default Kubernetes Scheme so we can log events
	logger.Infof("Creating event broadcaster...")
	rook_scheme.AddToScheme(kube_scheme.Scheme)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logger.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1_typed.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(
		kube_scheme.Scheme,
		corev1.EventSource{Component: "noobaa-operator"},
	)

	// Prepare informer factories
	resyncPeriod := 10 * time.Second
	rookInformerFactory := rook_informers.NewSharedInformerFactory(
		rookClient,
		resyncPeriod,
	)
	kubeInformerFactory := kube_informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		resyncPeriod,
		kube_informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			// Watch only kubernetes resources relevant to our app
			options.LabelSelector = "app=noobaa"
		}),
	)
	kubeInformerFactoryAny := kube_informers.NewSharedInformerFactory(
		kubeClient,
		resyncPeriod,
	)

	o := &Operator{
		DevMode:                os.Getenv("DEV") == "true",
		StopChan:               make(chan struct{}),
		EventRecorder:          eventRecorder,
		KubeClient:             kubeClient,
		RookClient:             rookClient,
		APIExtClient:           apiExtClient,
		InformersSyncedList:    make([]cache.InformerSynced, 0, 10),
		KubeInformerFactory:    kubeInformerFactory,
		KubeInformerFactoryAny: kubeInformerFactoryAny,
		RookInformerFactory:    rookInformerFactory,
		PodInformer:            kubeInformerFactory.Core().V1().Pods(),
		NodeInformer:           kubeInformerFactoryAny.Core().V1().Nodes(),
		SecretInformer:         kubeInformerFactory.Core().V1().Secrets(),
		ServiceInformer:        kubeInformerFactory.Core().V1().Services(),
		ServiceAccountInformer: kubeInformerFactory.Core().V1().ServiceAccounts(),
		StatefulSetInformer:    kubeInformerFactory.Apps().V1().StatefulSets(),
		RoleInformer:           kubeInformerFactory.Rbac().V1().Roles(),
		RoleBindingInformer:    kubeInformerFactory.Rbac().V1().RoleBindings(),
		NooBaaSystemInformer:   rookInformerFactory.Noobaa().V1alpha1().NooBaaSystems(),
	}
	o.SystemController = NewSystemController(o)
	return o
}

// Run starts informers watching for changes and runs the controllers
func (o *Operator) Run() {

	logger.Info("Operator: Create CRDs ...")
	// o.MustCreateCRDFromFile(&nbv1.NooBaaSystem{}, "noobaa-system-crd.yaml")
	// o.MustCreateCRDFromFile(&nbv1.NooBaaBackingStore{}, "noobaa-backing-store-crd.yaml")
	// o.MustCreateCRDFromFile(&nbv1.NooBaaBucketClass{}, "noobaa-bucket-class-crd.yaml")

	logger.Info("Operator: Start informers ...")
	o.RookInformerFactory.Start(o.StopChan)
	o.KubeInformerFactory.Start(o.StopChan)
	o.KubeInformerFactoryAny.Start(o.StopChan)

	logger.Info("Operator: Wait for cache sync ...")
	synced := cache.WaitForCacheSync(o.StopChan, o.InformersSyncedList...)
	if !synced {
		// means that StopChan already stopped us while waiting for initial cache sync
		return
	}

	logger.Info("Operator: Running controllers ...")
	o.SystemController.Run()
}

// Stop allows Run() to exit by stopping the informers which are repeatedly watching for changes,
// and also stop the controllers workers waiting on their work queue.
// This method is currently not used for production, since it is simpler to just kill (signal)
// the operator process to have a single failure path for crashes and maintenance.
// But we might want this for unit tests, and to make a graceful exit -
// which means not in the middle of a single control loop, in that case we'll call Stop().
func (o *Operator) Stop() {
	close(o.StopChan)
	o.SystemController.Stop()
}

// AddEventHandler is a convenient helper that adds the handler to the informer, and also
// maintains the list of informers that need to be synched before we start the controllers.
func (o *Operator) AddEventHandler(informer cache.SharedIndexInformer, handler cache.ResourceEventHandler) {
	informer.AddEventHandler(handler)
	o.InformersSyncedList = append(o.InformersSyncedList, informer.HasSynced)
}

// GetControllerOwner will take any resource implementing metav1.Object and attempt
// to find the resource that 'owns' it and marked as controller.
// Returns nil if the object does not have an appropriate OwnerReference.
func (o *Operator) GetControllerOwner(obj interface{}) (*metav1.OwnerReference, metav1.Object) {
	metaObject, err := meta.Accessor(obj)
	if err != nil {
		deleted, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			logger.Errorf("Error decoding object, invalid type %+v", reflect.TypeOf(obj))
			return nil, nil
		}
		metaObject, err := meta.Accessor(deleted.Obj)
		if err != nil {
			logger.Errorf("Error decoding deleted object, invalid type")
			return nil, nil
		}
		logger.Infof("Recovered deleted object: %s", metaObject.GetSelfLink())
	}
	owner := metav1.GetControllerOf(metaObject)
	return owner, metaObject
}

// NewObjectEventHandler returns a watch handler based on a simple handler func
// with general handling of updates to same resource version.
func (o *Operator) NewObjectEventHandler(objectHandler func(obj interface{})) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    objectHandler,
		DeleteFunc: objectHandler,
		UpdateFunc: func(old, obj interface{}) {
			newMeta, err := meta.Accessor(obj)
			if err != nil {
				logger.Tracef("Operator: Ignore non meta object event %+v", reflect.TypeOf(obj))
				return
			}
			oldMeta, err := meta.Accessor(old)
			if err != nil {
				logger.Tracef("Operator: Ignore non meta old object event %+v", reflect.TypeOf(old))
				return
			}
			if newMeta.GetResourceVersion() == oldMeta.GetResourceVersion() {
				logger.Tracef("Operator: Ignore cached ResourceVersion=%s for %s",
					newMeta.GetResourceVersion(), newMeta.GetSelfLink())
				return
			}
			objectHandler(obj)
		},
	}
}

// MustCreateCRDFromFile reads a yaml/json file with CRD spec and creates it if it doesn't exist.
// It will retry while respecting the operator's stop channel close events.
// Stopping here is probably less relevant for a running operator, mostly for testing.
func (o *Operator) MustCreateCRDFromFile(objType runtime.Object, crdFileName string) {
	crd := &apiextv1.CustomResourceDefinition{}
	crdFilePath := crdFileName
	if o.DevMode {
		crdFilePath = "cluster/examples/kubernetes/noobaa/" + crdFileName
	}
	err := ReadObjectFromFile(crdFilePath, crd)
	rook.TerminateOnError(err, "Operator: Failed loading resource from file "+crdFilePath)

	err = wait.PollImmediateUntil(time.Second, func() (bool, error) { return o.CreateCRD(crd) }, o.StopChan)
	rook.TerminateOnError(err, fmt.Sprintf("Operator: %s CRD could not be installed", crd.Kind))

}

// CreateCRD uses APIExtClient to create the CRD if it doesn't exist.
// Returns true if created/exists, false to retry, error on unrecoverable error.
func (o *Operator) CreateCRD(crd *apiextv1.CustomResourceDefinition) (bool, error) {
	crdClient := o.APIExtClient.ApiextensionsV1beta1().CustomResourceDefinitions()
	crdReply, err := crdClient.Get(crd.GetName(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = crdClient.Create(crd)
		crdReply, err = crdClient.Get(crd.GetName(), metav1.GetOptions{})
	}
	if err == nil {
		for _, cond := range crdReply.Status.Conditions {
			switch cond.Type {
			case apiextv1.Established:
				if cond.Status == apiextv1.ConditionTrue {
					return true, nil
				}
			case apiextv1.NamesAccepted:
				if cond.Status == apiextv1.ConditionFalse {
					return false, fmt.Errorf("CRD name conflict: %v", cond.Reason)
				}
			}
		}
	}
	return false, nil
}

// ReadObjectFromFile reads a yaml/json kubernetes object from file.
func ReadObjectFromFile(filePath string, obj interface{}) error {

	file, err := os.Open(filePath)
	if err != nil {
		logger.Errorf("Operator: failed openning file: %s %+v", filePath, err)
		return err
	}

	err = yaml.NewYAMLOrJSONDecoder(file, 4096).Decode(obj)
	if err != nil {
		logger.Errorf("Operator: failed decode file: %s %+v", filePath, err)
		return err
	}

	return nil
}
