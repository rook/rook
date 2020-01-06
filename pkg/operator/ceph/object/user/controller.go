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
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	appName = object.AppName
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-object")

// ObjectStoreUserResource represents the object store user custom resource
var ObjectStoreUserResource = k8sutil.CustomResource{
	Name:    "cephobjectstoreuser",
	Plural:  "cephobjectstoreusers",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Kind:    reflect.TypeOf(cephv1.CephObjectStoreUser{}).Name(),
}

// ObjectStoreUserController represents a controller object for object store user custom resources
type ObjectStoreUserController struct {
	context     *clusterd.Context
	ownerRef    metav1.OwnerReference
	clusterSpec *cephv1.ClusterSpec
	namespace   string
}

// NewObjectStoreUserController create controller for watching object store user custom resources created
func NewObjectStoreUserController(context *clusterd.Context, clusterSpec *cephv1.ClusterSpec, namespace string, ownerRef metav1.OwnerReference) *ObjectStoreUserController {
	return &ObjectStoreUserController{
		context:     context,
		ownerRef:    ownerRef,
		clusterSpec: clusterSpec,
		namespace:   namespace,
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
	go k8sutil.WatchCR(ObjectStoreUserResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient(), &cephv1.CephObjectStoreUser{}, stopCh)

	return nil
}

func (c *ObjectStoreUserController) onAdd(obj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Creating object store user for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	user, err := getObjectStoreUserObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstoreuser object. %v", err)
		return
	}
	updateCephObjectStoreUserStatus(user.GetName(), user.GetNamespace(), k8sutil.ProcessingStatus, c.context)

	if err = c.createUser(c.context, user); err != nil {
		logger.Errorf("failed to create object store user %q. %v", user.Name, err)
		updateCephObjectStoreUserStatus(user.GetName(), user.GetNamespace(), k8sutil.FailedStatus, c.context)

	}
	updateCephObjectStoreUserStatus(user.GetName(), user.GetNamespace(), k8sutil.ReadyStatus, c.context)
}

func (c *ObjectStoreUserController) onUpdate(oldObj, newObj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Updating object store user for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}
	// TODO: Add update code here after features are added which require updates.
}

func (c *ObjectStoreUserController) onDelete(obj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Deleting object store user for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	user, err := getObjectStoreUserObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstoreuser object. %v", err)
		return
	}

	if err = deleteUser(c.context, user); err != nil {
		logger.Errorf("failed to delete object store user %q. %v", user.Name, err)
	}
}

// ParentClusterChanged determines wether or not a CR update has been sent
func (c *ObjectStoreUserController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo, isUpgrade bool) {
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
	return nil, errors.Errorf("not a known objectstoreuser object %+v", obj)
}

// Create the user
func (c *ObjectStoreUserController) createUser(context *clusterd.Context, u *cephv1.CephObjectStoreUser) error {
	// validate the user settings
	if err := ValidateUser(context, u); err != nil {
		return errors.Wrapf(err, "invalid user %s arguments", u.Name)
	}
	// Set DisplayName to match Name if DisplayName is not set
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

	initialized, err := objectStoreInitialized(objContext)
	if err != nil {
		logger.Errorf("failed to detect if object store is initialized. %v", err)
	} else if !initialized {
		err := wait.Poll(time.Second*15, time.Minute*5, func() (ok bool, err error) {
			initialized, err := objectStoreInitialized(objContext)
			if err != nil {
				return true, err
			}
			return initialized, nil
		})
		if err != nil {
			logger.Errorf("err or timed out while waiting for objectstore %q to be ready. %v", u.Spec.Store, err)
		}
	}

	user, rgwerr, err := object.CreateUser(objContext, userConfig)
	if err != nil {
		pollErr := wait.Poll(time.Second*15, time.Minute*5, func() (ok bool, err error) {
			user, rgwerr, err = object.CreateUser(objContext, userConfig)
			if err != nil {
				if rgwerr == object.RGWErrorBadData {
					return true, errors.Wrapf(err, "failed to create rgw user %q. error code %d", u.Name, rgwerr)
				}
				return false, nil
			}
			return true, nil
		})
		if pollErr != nil {
			return errors.Wrapf(pollErr, "errored or timed out while waiting for objectuser %q to be created", u.Name)
		}
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
		return errors.Wrapf(err, "failed to save user %s secret", u.Name)
	}
	logger.Infof("created user %s", u.Name)
	return nil
}

func objectStoreInitialized(context *object.Context) (bool, error) {
	// check if CephObjectStore CR is created
	_, err := context.Context.RookClientset.CephV1().CephObjectStores(context.ClusterName).Get(context.Name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Warningf("CephObjectStore %s could not be found. %v", context.Name, err)
			return false, nil
		}
		return false, err
	}

	// check if ObjectStore is initialized
	// rook does this by starting the RGW pod(s)
	selector := fmt.Sprintf("%s=%s,%s=%s",
		"rgw", context.Name,
		k8sutil.AppAttr, appName,
	)
	pods, err := context.Context.Clientset.CoreV1().Pods(context.ClusterName).List(metav1.ListOptions{LabelSelector: selector, FieldSelector: "status.phase=Running"})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	// check if at least one pod is running
	if pods != nil {
		logger.Infof("CephObjectStore %s is running", context.Name)
		return true, nil
	}
	logger.Infof("CephObjectStore %s is not running", context.Name)
	return false, nil
}

// Delete the user
func deleteUser(context *clusterd.Context, u *cephv1.CephObjectStoreUser) error {
	objContext := object.NewContext(context, u.Spec.Store, u.Namespace)
	_, rgwerr, err := object.DeleteUser(objContext, u.Name)
	if err != nil {
		if rgwerr == 3 {
			logger.Infof("user %s does not exist in store %s", u.Name, u.Spec.Store)
		} else {
			return errors.Wrapf(err, "failed to delete user %q", u.Name)
		}
	}

	err = context.Clientset.CoreV1().Secrets(u.Namespace).Delete(fmt.Sprintf("rook-ceph-object-user-%s-%s", u.Spec.Store, u.Name), &metav1.DeleteOptions{})
	if err != nil {
		logger.Warningf("failed to delete user %s secret. %v", fmt.Sprintf("rook-ceph-object-user-%s-%s", u.Spec.Store, u.Name), err)
	}

	logger.Infof("user %s deleted successfully", u.Name)
	return nil
}

// ValidateUser validates the user arguments
func ValidateUser(context *clusterd.Context, u *cephv1.CephObjectStoreUser) error {
	if u.Name == "" {
		return errors.New("missing name")
	}
	if u.Namespace == "" {
		return errors.New("missing namespace")
	}
	if u.Spec.Store == "" {
		return errors.New("missing store")
	}
	return nil
}

func updateCephObjectStoreUserStatus(name, namespace, status string, context *clusterd.Context) {
	updatedCephObjectStoreUser, err := context.RookClientset.CephV1().CephObjectStoreUsers(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Unable to update the cephObjectStoreUser %s status %v", updatedCephObjectStoreUser.GetName(), err)
		return
	}
	if updatedCephObjectStoreUser.Status == nil {
		updatedCephObjectStoreUser.Status = &cephv1.Status{}
	} else if updatedCephObjectStoreUser.Status.Phase == status {
		return
	}
	updatedCephObjectStoreUser.Status.Phase = status
	_, err = context.RookClientset.CephV1().CephObjectStoreUsers(updatedCephObjectStoreUser.Namespace).Update(updatedCephObjectStoreUser)
	if err != nil {
		logger.Errorf("Unable to update the cephObjectStoreUser %s status %v", updatedCephObjectStoreUser.GetName(), err)
		return
	}
}
