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

// Package ozonee to manage a Ozone object store.
package ozone

import (
	"bytes"
	"github.com/coreos/pkg/capnslog"
	flekszible "github.com/elek/flekszible/api"
	"github.com/elek/flekszible/api/data"
	"github.com/elek/flekszible/api/processor"
	opkit "github.com/rook/operator-kit"
	ozonev1alpha1 "github.com/rook/rook/pkg/apis/ozone.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/cache"
	"os"
	"reflect"
)

const (
	customResourceName       = "ozoneobjectstore"
	customResourceNamePlural = "ozoneobjectstores"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ozone-op-object")

// ObjectStoreResource represents the object store custom resource
var ObjectStoreResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   ozonev1alpha1.CustomResourceGroup,
	Version: ozonev1alpha1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(ozonev1alpha1.OzoneObjectStore{}).Name(),
}

// Controller represents a controller object for object store custom resources
type Controller struct {
	context     *clusterd.Context
	templateDir string
}

// NewController create controller for watching object store custom resources created
func NewController(context *clusterd.Context) *Controller {
	templateDir := os.Getenv("ROOK_OZONE_TEMPLATE_DIR")
	if templateDir == "" {
		templateDir = "/var/lib/ozone-templates"
	}

	return &Controller{
		context:     context,
		templateDir: templateDir,
	}
}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *Controller) StartWatch(namespace string, stopCh chan struct{}) error {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	namespaceString := namespace
	if namespaceString == corev1.NamespaceAll {
		namespaceString = "all namespace"
	}
	logger.Infof("start watching object store resources in %s", namespaceString)
	watcher := opkit.NewWatcher(ObjectStoreResource, namespace, resourceHandlerFuncs, c.context.RookClientset.OzoneV1alpha1().RESTClient())
	go watcher.Watch(&ozonev1alpha1.OzoneObjectStore{}, stopCh)

	return nil
}

//The main resource file generation logic from the CRD
func (c *Controller) generateResources(objectStore *ozonev1alpha1.OzoneObjectStore) error {
	_, err := flekszible.Initialize(c.templateDir)
	if err != nil {
		return err
	}

	processors := make([]processor.Processor, 0)

	processors = append(processors, &processor.Namespace{
		Namespace:          objectStore.Namespace,
		Force:              true,
		ClusterRoleSupport: true,
	})

	processors = append(processors, &processor.Image{
		Image: objectStore.Spec.OzoneVersion.Image,
	})

	emptyDir, err := processor.CreateTransformation("ozone/emptydir")
	if err != nil {
		return err
	}

	processors = append(processors, emptyDir)

	processors = append(processors, &processor.Replace{
		Path: data.NewPath("spec", "replicas"),
		Trigger: processor.Trigger{
			Definition: data.NodeFromPathValue(data.NewPath("metadata", "name"), "datanode"),
		},
		Value: objectStore.Spec.Storage.NodeCount,
	})

	resources, err := flekszible.Generate(c.templateDir, processors)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		switch kind := resource.Kind(); kind {
		case "StatefulSet":
			err = c.applyStatefulSet(resource, objectStore)
			if err != nil {
				logger.Errorf("Can't apply StatefulSet %s %s ", resource.Name(), err.Error())
			}
		case "Service":
			err = c.applyService(resource, objectStore)
			if err != nil {
				logger.Errorf("Can't apply Service %s %s ", resource.Name(), err.Error())
			}
		case "ConfigMap":
			err = c.applyConfigMap(resource, objectStore)
			if err != nil {
				logger.Errorf("Can't apply ConfigMap %s %s ", resource.Name(), err.Error())
			}
		default:
			logger.Errorf("Unhandled resource type: %s", resource.Kind())
		}
	}
	return nil
}

//helper function to set the owner for k8s resoures
func (c *Controller) setOwnerRef(objectStore *ozonev1alpha1.OzoneObjectStore, destination *metav1.ObjectMeta) {
	ownerRef := metav1.OwnerReference{
		APIVersion: ObjectStoreResource.Version,
		Kind:       ObjectStoreResource.Kind,
		Name:       objectStore.Name,
		UID:        types.UID(objectStore.ObjectMeta.UID),
	}
	k8sutil.SetOwnerRef(destination, &ownerRef)
}

//parse real kubernetes object from a generated resource
func (c *Controller) parseRenderedResource(resource *data.Resource,
	destinationPointer interface{}) error {
	resourceContent, err := resource.Content.ToString()
	if err != nil {
		return err
	}

	err = yaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(resourceContent), 1000).Decode(destinationPointer)
	if err != nil {
		return err
	}

	return nil
}

//Method to be called in case of the CRD is added
func (c *Controller) onAdd(obj interface{}) {
	objectstore := obj.(*ozonev1alpha1.OzoneObjectStore).DeepCopy()

	logger.Infof("Ozone object store object is added/updated  %s", objectstore.Name)

	err := c.generateResources(objectstore)
	if err != nil {
		logger.Errorf("Operation is failed for object store %s %s", objectstore.Name, err.Error())
	}

}

//Method to be called in case of the CRD is updated
func (c *Controller) onUpdate(oldObj, newObj interface{}) {
	newStore := newObj.(*ozonev1alpha1.OzoneObjectStore).DeepCopy()
	c.onAdd(newStore)
}

//Method to be called in case of the CRD is deleted
func (c *Controller) onDelete(obj interface{}) {
	objectstore := obj.(*ozonev1alpha1.OzoneObjectStore).DeepCopy()
	logger.Infof("Delete Ozone object store %s", objectstore.Name)
	// Cleanup is handled by the owner references set in 'onAdd' and the k8s garbage collector.
}

//helper function to generate and deploy Service after the custom transformations
func (c *Controller) applyService(resource *data.Resource,
	objectStore *ozonev1alpha1.OzoneObjectStore) error {
	logger.Infof("Applying Service %s", resource.Name())
	service := corev1.Service{}
	err := c.parseRenderedResource(resource, &service)
	if err != nil {
		return err
	}
	c.setOwnerRef(objectStore, &service.ObjectMeta)

	_, err = k8sutil.CreateOrUpdateService(c.context.Clientset, objectStore.Namespace, &service)

	if err != nil {
		return err
	}
	return nil
}

//helper function to generate and deploy ConfigMap after the custom transformations
func (c *Controller) applyConfigMap(resource *data.Resource,
	objectStore *ozonev1alpha1.OzoneObjectStore) error {
	logger.Infof("Applying ConfigMap %s", resource.Name())
	configMap := corev1.ConfigMap{}
	err := c.parseRenderedResource(resource, &configMap)

	if err != nil {
		return err
	}

	original, err := c.context.Clientset.CoreV1().ConfigMaps(objectStore.Namespace).Get(resource.Name(), metav1.GetOptions{})
	if err == nil {
		configMap.ResourceVersion = original.ResourceVersion
		_, err = c.context.Clientset.CoreV1().ConfigMaps(objectStore.Namespace).Update(&configMap)
	} else if errors.IsNotFound(err) {
		_, err = c.context.Clientset.CoreV1().ConfigMaps(objectStore.Namespace).Create(&configMap)
	}
	if err != nil {
		return err
	}
	return nil
}

//helper function to generate and deploy StatefulSets after the custom transformations
func (c *Controller) applyStatefulSet(resource *data.Resource,
	objectStore *ozonev1alpha1.OzoneObjectStore) error {
	logger.Infof("Applying StatefulSet %s", resource.Name())
	statefulSet := appsv1.StatefulSet{}
	err := c.parseRenderedResource(resource, &statefulSet)
	if err != nil {
		return err
	}
	c.setOwnerRef(objectStore, &statefulSet.ObjectMeta)
	original, err := c.context.Clientset.AppsV1().StatefulSets(objectStore.Namespace).Get(resource.Name(), metav1.GetOptions{})
	if err == nil {
		statefulSet.ResourceVersion = original.ResourceVersion
		_, err = c.context.Clientset.AppsV1().StatefulSets(objectStore.Namespace).Update(&statefulSet)
	} else if errors.IsNotFound(err) {
		_, err = c.context.Clientset.AppsV1().StatefulSets(objectStore.Namespace).Create(&statefulSet)
	}
	if err != nil {
		return err
	}
	return nil
}
