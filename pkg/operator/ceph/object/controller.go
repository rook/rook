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
*/

// Package rgw to manage a rook object store.
package object

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName       = "objectstore"
	customResourceNamePlural = "objectstores"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-object")

// ObjectStoreResource represents the object store custom resource
var ObjectStoreResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   cephv1beta1.CustomResourceGroup,
	Version: cephv1beta1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1beta1.ObjectStore{}).Name(),
}

var ObjectStoreResourceRookLegacy = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   rookv1alpha1.CustomResourceGroup,
	Version: rookv1alpha1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(rookv1alpha1.ObjectStore{}).Name(),
}

// ObjectStoreController represents a controller object for object store custom resources
type ObjectStoreController struct {
	context     *clusterd.Context
	rookImage   string
	hostNetwork bool
	ownerRef    metav1.OwnerReference
}

// NewObjectStoreController create controller for watching object store custom resources created
func NewObjectStoreController(context *clusterd.Context, rookImage string, hostNetwork bool, ownerRef metav1.OwnerReference) *ObjectStoreController {
	return &ObjectStoreController{
		context:     context,
		rookImage:   rookImage,
		hostNetwork: hostNetwork,
		ownerRef:    ownerRef,
	}
}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *ObjectStoreController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(ObjectStoreResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1beta1().RESTClient())
	go watcher.Watch(&cephv1beta1.ObjectStore{}, stopCh)

	// watch for events on all legacy types too
	c.watchLegacyObjectStores(namespace, stopCh, resourceHandlerFuncs)

	return nil
}

func (c *ObjectStoreController) onAdd(obj interface{}) {
	objectstore, migrationNeeded, err := getObjectStoreObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstore object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migrateObjectStoreObject(objectstore, obj); err != nil {
			logger.Errorf("failed to migrate objectstore %s in namespace %s: %+v", objectstore.Name, objectstore.Namespace, err)
		}
		return
	}

	if err = CreateStore(c.context, *objectstore, c.rookImage, c.hostNetwork, c.storeOwners(objectstore)); err != nil {
		logger.Errorf("failed to create object store %s. %+v", objectstore.Name, err)
	}
}

func (c *ObjectStoreController) onUpdate(oldObj, newObj interface{}) {
	// if the object store spec is modified, update the object store
	oldStore, _, err := getObjectStoreObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old objectstore object: %+v", err)
		return
	}
	newStore, migrationNeeded, err := getObjectStoreObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new objectstore object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migrateObjectStoreObject(newStore, newObj); err != nil {
			logger.Errorf("failed to migrate objectstore %s in namespace %s: %+v", newStore.Name, newStore.Namespace, err)
		}
		return
	}

	if !storeChanged(oldStore.Spec, newStore.Spec) {
		logger.Debugf("object store %s did not change", newStore.Name)
		return
	}

	logger.Infof("applying object store %s changes", newStore.Name)
	if err = UpdateStore(c.context, *newStore, c.rookImage, c.hostNetwork, c.storeOwners(newStore)); err != nil {
		logger.Errorf("failed to create (modify) object store %s. %+v", newStore.Name, err)
	}
}

func (c *ObjectStoreController) onDelete(obj interface{}) {
	objectstore, migrationNeeded, err := getObjectStoreObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstore object: %+v", err)
		return
	}

	if migrationNeeded {
		logger.Infof("ignoring deletion of legacy objectstore %s in namespace %s", objectstore.Name, objectstore.Namespace)
		return
	}

	if err = DeleteStore(c.context, *objectstore); err != nil {
		logger.Errorf("failed to delete object store %s. %+v", objectstore.Name, err)
	}
}

func (c *ObjectStoreController) storeOwners(store *cephv1beta1.ObjectStore) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the object store resources.
	// If the object store crd is deleted, the operator will explicitly remove the object store resources.
	// If the object store crd still exists when the cluster crd is deleted, this will make sure the object store
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func storeChanged(oldStore, newStore cephv1beta1.ObjectStoreSpec) bool {
	if oldStore.DataPool.Replicated.Size != newStore.DataPool.Replicated.Size {
		logger.Infof("data pool replication changed from %d to %d", oldStore.DataPool.Replicated.Size, newStore.DataPool.Replicated.Size)
		return true
	}
	if oldStore.MetadataPool.Replicated.Size != newStore.MetadataPool.Replicated.Size {
		logger.Infof("metadata pool replication changed from %d to %d", oldStore.MetadataPool.Replicated.Size, newStore.MetadataPool.Replicated.Size)
		return true
	}
	if oldStore.Gateway.Instances != newStore.Gateway.Instances {
		logger.Infof("RGW instances changed from %d to %d", oldStore.Gateway.Instances, newStore.Gateway.Instances)
		return true
	}
	if oldStore.Gateway.Port != newStore.Gateway.Port {
		logger.Infof("Port changed from %d to %d", oldStore.Gateway.Port, newStore.Gateway.Port)
		return true
	}
	if oldStore.Gateway.SecurePort != newStore.Gateway.SecurePort {
		logger.Infof("SecurePort changed from %d to %d", oldStore.Gateway.SecurePort, newStore.Gateway.SecurePort)
		return true
	}
	if oldStore.Gateway.AllNodes != newStore.Gateway.AllNodes {
		logger.Infof("AllNodes changed from %t to %t", oldStore.Gateway.AllNodes, newStore.Gateway.AllNodes)
		return true
	}
	if oldStore.Gateway.SSLCertificateRef != newStore.Gateway.SSLCertificateRef {
		logger.Infof("SSLCertificateRef changed from %s to %s", oldStore.Gateway.SSLCertificateRef, newStore.Gateway.SSLCertificateRef)
		return true
	}
	return false
}

func (c *ObjectStoreController) watchLegacyObjectStores(namespace string, stopCh chan struct{}, resourceHandlerFuncs cache.ResourceEventHandlerFuncs) {
	// watch for objectstore.rook.io/v1alpha1 events if the CRD exists
	if _, err := c.context.RookClientset.RookV1alpha1().ObjectStores(namespace).List(metav1.ListOptions{}); err != nil {
		logger.Infof("skipping watching for legacy rook objectstore events (legacy objectstore CRD probably doesn't exist): %+v", err)
	} else {
		logger.Infof("start watching legacy rook objectstores in all namespaces")
		watcherLegacy := opkit.NewWatcher(ObjectStoreResourceRookLegacy, namespace, resourceHandlerFuncs, c.context.RookClientset.RookV1alpha1().RESTClient())
		go watcherLegacy.Watch(&rookv1alpha1.ObjectStore{}, stopCh)
	}
}

func getObjectStoreObject(obj interface{}) (objectstore *cephv1beta1.ObjectStore, migrationNeeded bool, err error) {
	var ok bool
	objectstore, ok = obj.(*cephv1beta1.ObjectStore)
	if ok {
		// the objectstore object is of the latest type, simply return it
		return objectstore.DeepCopy(), false, nil
	}

	// type assertion to current objectstore type failed, try instead asserting to the legacy objectstore types
	// then convert it to the current objectstore type
	objectStoreRookLegacy, ok := obj.(*rookv1alpha1.ObjectStore)
	if ok {
		return convertRookLegacyObjectStore(objectStoreRookLegacy.DeepCopy()), true, nil
	}

	return nil, false, fmt.Errorf("not a known objectstore object: %+v", obj)
}

func (c *ObjectStoreController) migrateObjectStoreObject(objectstoreToMigrate *cephv1beta1.ObjectStore, legacyObj interface{}) error {
	logger.Infof("migrating legacy objectstore %s in namespace %s", objectstoreToMigrate.Name, objectstoreToMigrate.Namespace)

	_, err := c.context.RookClientset.CephV1beta1().ObjectStores(objectstoreToMigrate.Namespace).Get(objectstoreToMigrate.Name, metav1.GetOptions{})
	if err == nil {
		// objectstore of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("objectstore object %s in namespace %s already exists, will not overwrite with migrated legacy objectstore.",
			objectstoreToMigrate.Name, objectstoreToMigrate.Namespace)
	} else {
		if !errors.IsNotFound(err) {
			return err
		}

		// objectstore of current type does not already exist, create it now to complete the migration
		_, err = c.context.RookClientset.CephV1beta1().ObjectStores(objectstoreToMigrate.Namespace).Create(objectstoreToMigrate)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy objectstore %s in namespace %s", objectstoreToMigrate.Name, objectstoreToMigrate.Namespace)
	}

	// delete the legacy objectstore instance, it should not be used anymore now that a migrated instance of the current type exists
	deletePropagation := metav1.DeletePropagationOrphan
	if _, ok := legacyObj.(*rookv1alpha1.ObjectStore); ok {
		logger.Infof("deleting legacy rook objectstore %s in namespace %s", objectstoreToMigrate.Name, objectstoreToMigrate.Namespace)
		return c.context.RookClientset.RookV1alpha1().ObjectStores(objectstoreToMigrate.Namespace).Delete(
			objectstoreToMigrate.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	}

	return fmt.Errorf("not a known objectstore object: %+v", legacyObj)
}

func convertRookLegacyObjectStore(legacyObjectStore *rookv1alpha1.ObjectStore) *cephv1beta1.ObjectStore {
	if legacyObjectStore == nil {
		return nil
	}

	legacySpec := legacyObjectStore.Spec

	objectStore := &cephv1beta1.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyObjectStore.Name,
			Namespace: legacyObjectStore.Namespace,
		},
		Spec: cephv1beta1.ObjectStoreSpec{
			MetadataPool: pool.ConvertRookLegacyPoolSpec(legacySpec.MetadataPool),
			DataPool:     pool.ConvertRookLegacyPoolSpec(legacySpec.DataPool),
			Gateway: cephv1beta1.GatewaySpec{
				Port:              legacySpec.Gateway.Port,
				SecurePort:        legacySpec.Gateway.SecurePort,
				Instances:         legacySpec.Gateway.Instances,
				AllNodes:          legacySpec.Gateway.AllNodes,
				SSLCertificateRef: legacySpec.Gateway.SSLCertificateRef,
				Placement:         rookv1alpha2.ConvertLegacyPlacement(legacySpec.Gateway.Placement),
				Resources:         legacySpec.Gateway.Resources,
			},
		},
	}

	return objectStore
}
