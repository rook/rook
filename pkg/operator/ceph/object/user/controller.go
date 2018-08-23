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

// Package objectuser to manage a rook object store user.
package objectuser

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	cephrgw "github.com/rook/rook/pkg/daemon/ceph/rgw"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName       = "objectstoreuser"
	customResourceNamePlural = "objectstoreusers"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-object")

// ObjectStoreResource represents the object store user custom resource
var ObjectStoreUserResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   cephv1beta1.CustomResourceGroup,
	Version: cephv1beta1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1beta1.ObjectStoreUser{}).Name(),
}

// ObjectStoreUserController represents a controller object for object store user custom resources
type ObjectStoreUserController struct {
	context     *clusterd.Context
	rookImage   string
	hostNetwork bool
	ownerRef    metav1.OwnerReference
}

// NewObjectStoreUserController create controller for watching object store user custom resources created
func NewObjectStoreUserController(context *clusterd.Context, rookImage string, hostNetwork bool, ownerRef metav1.OwnerReference) *ObjectStoreUserController {
	return &ObjectStoreUserController{
		context:     context,
		rookImage:   rookImage,
		hostNetwork: hostNetwork,
		ownerRef:    ownerRef,
	}
}

// StartWatch watches for instances of ObjectStoreUser custom resources and acts on them
func (c *ObjectStoreUserController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc: c.onAdd,
		// UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store user resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(ObjectStoreUserResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1beta1().RESTClient())
	go watcher.Watch(&cephv1beta1.ObjectStoreUser{}, stopCh)

	return nil
}

func (c *ObjectStoreUserController) onAdd(obj interface{}) {
	user, err := getObjectStoreUserObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstoreuser object: %+v", err)
		return
	}

	if err = createUser(c.context, user); err != nil {
		logger.Errorf("failed to create object store user %s. %+v", user.Name, err)
	}
}

// TODO: Need to think through how to do updates, leaving out for now.

// func (c *ObjectStoreUserController) onUpdate(oldObj, newObj interface{}) {
// 	// if the object store user spec is modified, update the object store user
// 	oldStoreUser, err := getObjectStoreUserObject(oldObj)
// 	if err != nil {
// 		logger.Errorf("failed to get old objectstoreuser object: %+v", err)
// 		return
// 	}
// 	newStoreUser, err := getObjectStoreUserObject(newObj)
// 	if err != nil {
// 		logger.Errorf("failed to get new objectstoreuser object: %+v", err)
// 		return
// 	}
//
// 	if !storeUserChanged(oldStoreUser.Spec, newStoreUser.Spec) {
// 		logger.Debugf("object store user %s did not change", newStoreUser.Name)
// 		return
// 	}
//
// 	logger.Infof("applying object store user %s changes", newStoreUser.Name)
// 	if err = UpdateStoreUser(c.context, *newStoreUser, c.rookImage, c.hostNetwork, c.storeUserOwners(newStoreUser)); err != nil {
// 		logger.Errorf("failed to create (modify) object store user %s. %+v", newStoreUser.Name, err)
// 	}
// }

func (c *ObjectStoreUserController) onDelete(obj interface{}) {
	user, err := getObjectStoreUserObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstoreuser object: %+v", err)
		return
	}

	if err = deleteUser(c.context, user); err != nil {
		logger.Errorf("failed to delete object store user %s. %+v", user.Name, err)
	}
}

func (c *ObjectStoreUserController) storeUserOwners(store *cephv1beta1.ObjectStoreUser) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the object store user resources.
	// If the object store user crd is deleted, the operator will explicitly remove the object store user resources.
	// If the object store user crd still exists when the cluster crd is deleted, this will make sure the object store user
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func getObjectStoreUserObject(obj interface{}) (objectstoreuser *cephv1beta1.ObjectStoreUser, err error) {
	var ok bool
	objectstoreuser, ok = obj.(*cephv1beta1.ObjectStoreUser)
	if ok {
		// the objectstoreuser object is of the latest type, simply return it
		return objectstoreuser.DeepCopy(), nil
	}
	return nil, fmt.Errorf("not a known objectstoreuser object: %+v", obj)
}

// Create the user
func createUser(context *clusterd.Context, u *cephv1beta1.ObjectStoreUser) error {
	// validate the user settings
	if err := ValidateUser(context, u); err != nil {
		return fmt.Errorf("invalid user %s arguments. %+v", u.Name, err)
	}

	// create the user
	logger.Infof("creating user %s in namespace %s", u.Name, u.Namespace)
	userConfig := cephrgw.ObjectUser{
		UserID:      u.Name,
		DisplayName: &u.Name,
	}
	objContext := cephrgw.NewContext(context, u.Spec.Store, u.Namespace)
	if user, rgwerr, err := cephrgw.CreateUser(objContext, userConfig); err != nil {
		return fmt.Errorf("failed to create user %s. RadosGW returned error %d: %+v", u.Name, rgwerr, err)
	} else {
		logger.Infof("user accessKey: %s", user.AccessKey)
		logger.Infof("user accessKey: %s", user.SecretKey)
		logger.Infof("created user %s", u.Name)
	}

	return nil
}

// Delete the user
func deleteUser(context *clusterd.Context, u *cephv1beta1.ObjectStoreUser) error {
	objContext := cephrgw.NewContext(context, u.Spec.Store, u.Namespace)
	if result, rgwerr, err := cephrgw.DeleteUser(objContext, u.Name); err != nil {
		return fmt.Errorf("failed to delete user '%s'. RadosGW returned error %d: %+v", u.Name, rgwerr, err)
	} else if result != "" {
		logger.Infof("Result of user delete is: %s", result)
	}

	return nil
}

// Validate the user arguments
func ValidateUser(context *clusterd.Context, u *cephv1beta1.ObjectStoreUser) error {
	if u.Name == "" {
		return fmt.Errorf("missing name")
	}
	if u.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if u.Spec.Store == "" {
		return fmt.Errorf("missing store")
	}
	return nil
}
