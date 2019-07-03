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

// Package objectuser to manage a rook object store user.
package objectuser

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	appName = object.AppName
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-object")

// ObjectStoreUserResource represents the object store user custom resource
var ObjectStoreUserResource = opkit.CustomResource{
	Name:    "cephobjectstoreuser",
	Plural:  "cephobjectstoreusers",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1.CephObjectStoreUser{}).Name(),
}

// ObjectStoreUserController represents a controller object for object store user custom resources
type ObjectStoreUserController struct {
	context   *clusterd.Context
	ownerRef  metav1.OwnerReference
	namespace string
}

// NewObjectStoreUserController create controller for watching object store user custom resources created
func NewObjectStoreUserController(context *clusterd.Context, namespace string, ownerRef metav1.OwnerReference) *ObjectStoreUserController {
	return &ObjectStoreUserController{
		context:   context,
		ownerRef:  ownerRef,
		namespace: namespace,
	}
}

// StartWatch watches for instances of ObjectStoreUser custom resources and acts on them
func (c *ObjectStoreUserController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store user resources in namespace %s", c.namespace)
	watcher := opkit.NewWatcher(ObjectStoreUserResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient())
	go watcher.Watch(&cephv1.CephObjectStoreUser{}, stopCh)

	return nil
}

func (c *ObjectStoreUserController) onAdd(obj interface{}) {
	user, err := getObjectStoreUserObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstoreuser object: %+v", err)
		return
	}

	if err = c.createUser(c.context, user); err != nil {
		logger.Errorf("failed to create object store user %s. %+v", user.Name, err)
	}
}

func (c *ObjectStoreUserController) onUpdate(oldObj, newObj interface{}) {
	// TODO: Add update code here after features are added which require updates.
}

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

func (c *ObjectStoreUserController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo) {
	logger.Debugf("No need to update object store users after the parent cluster changed")
}

func (c *ObjectStoreUserController) storeUserOwners(store *cephv1.CephObjectStoreUser) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the object store user resources.
	// If the object store user crd is deleted, the operator will explicitly remove the object store user resources.
	// If the object store user crd still exists when the cluster crd is deleted, this will make sure the object store user
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func getObjectStoreUserObject(obj interface{}) (objectstoreuser *cephv1.CephObjectStoreUser, err error) {
	var ok bool
	objectstoreuser, ok = obj.(*cephv1.CephObjectStoreUser)
	if ok {
		// the objectstoreuser object is of the latest type, simply return it
		return objectstoreuser.DeepCopy(), nil
	}
	return nil, fmt.Errorf("not a known objectstoreuser object: %+v", obj)
}

// Create the user
func (c *ObjectStoreUserController) createUser(context *clusterd.Context, u *cephv1.CephObjectStoreUser) error {
	// validate the user settings
	if err := ValidateUser(context, u); err != nil {
		return fmt.Errorf("invalid user %s arguments. %+v", u.Name, err)
	}
	//Set DisplayName to match Name if DisplayName is not set
	displayName := u.Spec.DisplayName
	if len(displayName) == 0 {
		displayName = u.Name
	}

	// create the user
	logger.Infof("creating user %s in namespace %s", u.Name, u.Namespace)
	userConfig := object.ObjectUser{
		UserID:      u.Name,
		DisplayName: &displayName,
	}
	objContext := object.NewContext(context, u.Spec.Store, u.Namespace)

	user, rgwerr, err := object.CreateUser(objContext, userConfig)
	if err != nil {
		return fmt.Errorf("failed to create user %s. RadosGW returned error %d: %+v", u.Name, rgwerr, err)
	}

	// Store the keys in a secret
	secrets := map[string]string{
		"AccessKey": *user.AccessKey,
		"SecretKey": *user.SecretKey,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("rook-ceph-object-user-%s-%s", u.Spec.Store, u.Name),
			Namespace: u.Namespace,
			Labels: map[string]string{
				"app":               appName,
				"user":              u.Name,
				"rook_cluster":      u.Namespace,
				"rook_object_store": u.Spec.Store,
			},
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	k8sutil.SetOwnerRef(&secret.ObjectMeta, &c.ownerRef)

	_, err = context.Clientset.CoreV1().Secrets(u.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save user %s secret. %+v", u.Name, err)
	}
	logger.Infof("created user %s", u.Name)
	return nil
}

// Delete the user
func deleteUser(context *clusterd.Context, u *cephv1.CephObjectStoreUser) error {
	objContext := object.NewContext(context, u.Spec.Store, u.Namespace)
	_, rgwerr, err := object.DeleteUser(objContext, u.Name)
	if err != nil {
		if rgwerr == 3 {
			logger.Infof("user %s does not exist in store %s", u.Name, u.Spec.Store)
		} else {
			return fmt.Errorf("failed to delete user '%s': %+v", u.Name, err)
		}
	}

	err = context.Clientset.CoreV1().Secrets(u.Namespace).Delete(fmt.Sprintf("rook-ceph-object-user-%s-%s", u.Spec.Store, u.Name), &metav1.DeleteOptions{})
	if err != nil {
		logger.Warningf("failed to delete user %s secret. %+v", fmt.Sprintf("rook-ceph-object-user-%s-%s", u.Spec.Store, u.Name), err)
	}

	logger.Infof("user %s deleted successfully", u.Name)
	return nil
}

// ValidateUser validates the user arguments
func ValidateUser(context *clusterd.Context, u *cephv1.CephObjectStoreUser) error {
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
