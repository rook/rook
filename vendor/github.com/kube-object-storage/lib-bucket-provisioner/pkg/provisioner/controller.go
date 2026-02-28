/*
Copyright 2019 Red Hat Inc.

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

package provisioner

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/clientset/versioned"
	informers "github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/informers/externalversions/objectbucket.io/v1alpha1"
	listers "github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/listers/objectbucket.io/v1alpha1"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
)

type controller interface {
	Start(<-chan struct{}) error
	SetLabels(map[string]string)
}

// Provisioner is a CRD Controller responsible for executing the Reconcile() function
// in response to OBC events.
type obcController struct {
	clientset    kubernetes.Interface
	libClientset versioned.Interface
	obcLister    listers.ObjectBucketClaimLister
	obLister     listers.ObjectBucketLister
	obcInformer  informers.ObjectBucketClaimInformer
	obcHasSynced cache.InformerSynced
	obHasSynced  cache.InformerSynced
	queue        workqueue.RateLimitingInterface
	// static label containing provisioner name and provisioner-specific labels which are all added
	// to the OB, OBC, configmap and secret
	provisionerLabels map[string]string
	provisioner       api.Provisioner
	provisionerName   string
}

var _ controller = &obcController{}

func NewController(provisionerName string, provisioner api.Provisioner, clientset kubernetes.Interface, crdClientSet versioned.Interface, obcInformer informers.ObjectBucketClaimInformer, obInformer informers.ObjectBucketInformer) *obcController {
	ctrl := &obcController{
		clientset:    clientset,
		libClientset: crdClientSet,
		obcLister:    obcInformer.Lister(),
		obLister:     obInformer.Lister(),
		obcInformer:  obcInformer,
		obcHasSynced: obcInformer.Informer().HasSynced,
		obHasSynced:  obInformer.Informer().HasSynced,
		queue:        workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		provisionerLabels: map[string]string{
			provisionerLabelKey: labelValue(provisionerName),
		},
		provisionerName: provisionerName,
		provisioner:     provisioner,
	}

	obcInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.enqueueOBC,
		UpdateFunc: func(old, new interface{}) {
			oldObc := old.(*v1alpha1.ObjectBucketClaim)
			newObc := new.(*v1alpha1.ObjectBucketClaim)
			if newObc.ResourceVersion == oldObc.ResourceVersion {
				// periodic re-sync can be ignored
				return
			}
			// if old and new both have deletionTimestamps we can also ignore the
			// update since these events are occurring on an obc marked for deletion,
			// eg. extra finalizers being added and deleted.
			if newObc.ObjectMeta.DeletionTimestamp != nil && oldObc.ObjectMeta.DeletionTimestamp != nil {
				return
			}

			if !updateSupported(oldObc, newObc) {
				return
			}

			// handle this update
			ctrl.enqueueOBC(new)
		},
		DeleteFunc: func(obj interface{}) {
			// Since a finalizer is added to the obc and thus the obc will remain
			// visible, we do not need to handle delete events here. Instead, obc
			// deletes are indicated by the deletionTimestamp being non-nil.
			return
		},
	})
	return ctrl
}

func (c *obcController) Start(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	if !cache.WaitForCacheSync(stopCh, c.obcHasSynced, c.obHasSynced) {
		return fmt.Errorf("failed to wait for caches to sync ")
	}
	count := 1
	if threadiness, set := os.LookupEnv("LIB_BUCKET_PROVISIONER_THREADS"); set {
		count, _ = strconv.Atoi(threadiness)
	}
	for i := 0; i < count; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	<-stopCh
	return nil
}

// add provisioner-specific labels to the existing static label in the obcController struct.
func (c *obcController) SetLabels(labels map[string]string) {
	for k, v := range labels {
		c.provisionerLabels[k] = v
	}
}

func (c *obcController) enqueueOBC(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.queue.AddRateLimited(key)
}

func (c *obcController) runWorker() {
	for c.processNextItemInQueue() {
	}
}

func (c *obcController) processNextItemInQueue() bool {
	obj, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.queue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date than when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.queue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.queue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.queue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
	}
	return true
}

// Reconcile implements the Reconciler interface. This function contains the business logic
// of the OBC obcController.
// Note: the obc obtained from the key is not expected to be nil. In other words, this func is
// not called when informers detect an object is missing and trigger a formal delete event.
// Instead, delete is indicated by the deletionTimestamp being non-nil on an update event.
func (c *obcController) syncHandler(key string) error {

	setLoggersWithRequest(key)
	logD.Info("reconciling claim")

	obc, err := claimForKey(key, c.libClientset)
	if err != nil {
		//      The OBC was deleted immediately after creation, before it could be processed by
		//      handleProvisionClaim.  As a finalizer is immediately applied to the OBC before processing,
		//      if it does not have a finalizer, it was not processed, and no artifacts were created.
		//      Therefore, it is safe to assume nothing needs to be done.
		if errors.IsNotFound(err) {
			log.Info("OBC vanished, assuming it was deleted")
			return nil
		}
		return fmt.Errorf("could not sync OBC %s: %v", key, err)
	}

	class, err := storageClassForClaim(c.clientset, obc)
	if err != nil {
		return err
	}
	if !c.supportedProvisioner(class.Provisioner) {
		log.Info("unsupported provisioner", "got", class.Provisioner)
		return nil
	}

	// ***********************
	// Delete or Revoke Bucket
	// ***********************
	if obc.ObjectMeta.DeletionTimestamp != nil {
		log.Info("OBC deleted, proceeding with cleanup")
		return c.handleDeleteClaim(key, obc)
	}

	if obc.Status.Phase == "" {
		// update the OBC's status to pending before any provisioning related errors can occur
		obc, err = updateObjectBucketClaimPhase(
			c.libClientset,
			obc,
			v1alpha1.ObjectBucketClaimStatusPhasePending)
		if err != nil {
			return fmt.Errorf("error updating OBC status: %s", err)
		}
	}

	// idempotent provisioner
	err = c.handleProvisionClaim(key, obc, class)

	// If the handler above errors, the request will be re-queued. In the distant future, we will
	// likely want some ignorable error types in order to skip re-queuing
	return err
}

func (c *obcController) handleProvisionClaim(key string, obc *v1alpha1.ObjectBucketClaim, class *storagev1.StorageClass) error {

	log.Info("syncing obc creation")

	var (
		ob  *v1alpha1.ObjectBucket
		err error
	)

	// set finalizer in OBC so that resources cleaned up is controlled when the obc is deleted
	if obc, err = c.setOBCMetaFields(obc); err != nil {
		return err
	}

	ob, err = getObFromKey(key, c.libClientset) // ob may be nil here
	if err != nil {
		return fmt.Errorf("failed to find ob associated with obc %q", obc.Name)
	}

	// on an operator restart, the event will be an add event, and we should check if the obc has
	// been updated in comparison to the ob, since we don't have an old OBC to compare to
	if err = errIfObcConfigHasBeenModified(ob, obc); err != nil {
		return err
	}

	// If a storage class contains a non-nil value for the "bucketName" key, it is assumed
	// to be a Grant request to the given bucket (brownfield).  If the value is nil or the
	// key is undefined, it is assumed to be a provisioning request.  This allows administrators
	// to control access to static buckets via RBAC rules on storage classes.
	isDynamicProvisioning := isNewBucketByStorageClass(class)

	bucketName := class.Parameters[v1alpha1.StorageClassBucket]
	if isDynamicProvisioning {
		bucketName, err = composeBucketName(obc)
		if err != nil {
			return fmt.Errorf("error composing bucket name: %v", err)
		}
	}
	if len(bucketName) == 0 {
		return fmt.Errorf("bucket name missing")
	}

	// In the case where a bucket name is being generated, generate the name and store it in the OBC
	// spec before doing any Provisioning so that any crashes encountered in this code will not
	// result in multiple buckets being generated for the same OBC. bucketName takes precedence over
	// generateBucketName if both are present.
	if obc.Spec.BucketName == "" {
		obc.Spec.BucketName = bucketName
		obc, err = updateClaim(
			c.libClientset,
			obc)
		if err != nil {
			return fmt.Errorf("error updating OBC %q with bucket name: %v", key, err)
		}
	}

	userID, err := c.provisioner.GenerateUserID(obc, ob)
	if err != nil {
		return fmt.Errorf("failed to generate user id for use as idempotency key: %v", err)
	}

	options := &api.BucketOptions{
		ReclaimPolicy:     class.ReclaimPolicy,
		BucketName:        bucketName,
		UserID:            userID,
		ObjectBucketClaim: obc.DeepCopy(),
		Parameters:        class.Parameters,
	}

	verb := "provisioning"
	if !isDynamicProvisioning {
		verb = "granting access to"
	}
	logD.Info(verb, "bucket", options.BucketName)

	if isDynamicProvisioning {
		ob, err = c.provisioner.Provision(options)
	} else {
		ob, err = c.provisioner.Grant(options)
	}

	// The k8s code generator does not generate equality methods, and golang's native
	// reflect.DeepEqual panics at unexported k8s struct fields, so must use apiequality lib.
	emptyBucket := (ob == nil || apiequality.Semantic.DeepEqual(*ob, v1alpha1.ObjectBucket{}))

	if err != nil {
		return fmt.Errorf("error %s bucket: %v", verb, err)
	} else if emptyBucket {
		return fmt.Errorf("provisioner returned empty object bucket")
	}

	// Create/Update auth secret and endpoint configmap
	err = createOrUpdateSecret(
		obc,
		ob.Spec.Authentication,
		c.provisionerLabels,
		c.clientset)
	if err != nil {
		return fmt.Errorf("error creating secret for OBC: %v", err)
	}
	err = createOrUpdateConfigMap(
		obc,
		ob.Spec.Endpoint,
		c.provisionerLabels,
		c.clientset)
	if err != nil {
		return fmt.Errorf("error creating configmap for OBC: %v", err)
	}

	// Create/Update OB
	setObjectBucketName(ob, key)
	ob.Spec.StorageClassName = obc.Spec.StorageClassName
	if ob.Spec.ReclaimPolicy == nil || *ob.Spec.ReclaimPolicy == corev1.PersistentVolumeReclaimPolicy("") {
		// Do not blindly overwrite the reclaim policy. The provisioner might have reason to
		// specify a reclaim policy that is  different from the storage class.
		ob.Spec.ReclaimPolicy = options.ReclaimPolicy
	}
	addLabels(ob, c.provisionerLabels)
	addFinalizers(ob, []string{finalizer})
	ob.Spec.ClaimRef, err = claimRefForKey(key, c.libClientset)
	if err != nil {
		return fmt.Errorf("error getting reference to OBC: %v", err)
	}
	ob, err = createOrUpdateObjectBucket(
		ob,
		c.libClientset)
	if err != nil {
		return fmt.Errorf("error creating or updating OB %q: %v", ob.Name, err)
	}

	// Status must be set/updated separately from OB spec
	ob.Status.Phase = v1alpha1.ObjectBucketStatusPhaseBound
	ob, err = c.libClientset.ObjectbucketV1alpha1().ObjectBuckets().UpdateStatus(context.TODO(), ob, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("error updating OB %q status to %q", ob.Name, ob.Status.Phase)
	}

	// update OBC
	obc.Spec.ObjectBucketName = ob.Name
	obc.Spec.BucketName = bucketName
	obc, err = updateClaim(
		c.libClientset,
		obc)
	if err != nil {
		return fmt.Errorf("error updating OBC: %v", err)
	}
	obc, err = updateObjectBucketClaimPhase(
		c.libClientset,
		obc,
		v1alpha1.ObjectBucketClaimStatusPhaseBound)
	if err != nil {
		return fmt.Errorf("error updating OBC %q's status to %q: %v", obc.Name, v1alpha1.ObjectBucketClaimStatusPhaseBound, err)
	}

	return nil
}

// Delete or Revoke access to bucket defined by passed-in key and obc.
func (c *obcController) handleDeleteClaim(key string, obc *v1alpha1.ObjectBucketClaim) error {
	// Call `Delete` for new (greenfield) buckets with reclaimPolicy == "Delete".
	// Call `Revoke` for new buckets with reclaimPolicy != "Delete".
	// Call `Revoke` for existing (brownfield) buckets regardless of reclaimPolicy.

	log.Info("syncing obc deletion")

	ob, cm, secret, errs := c.getExistingResourcesFromKey(key)
	if len(errs) > 0 {
		return fmt.Errorf("error getting resources: %v", errs)
	}

	// Delete/Revoke cannot be called if the ob is nil; however, if the secret
	// and/or cm != nil we can delete them
	if ob == nil {
		log.Error(nil, "nil ObjectBucket, assuming it has been deleted")
		return c.deleteResources(nil, cm, secret, obc)
	}

	if ob.Spec.ReclaimPolicy == nil {
		log.Error(nil, "missing reclaimPolicy", "ob", ob.Name)
		return nil
	}

	// call Delete or Revoke and then delete generated k8s resources
	// Note: if Delete or Revoke return err then we do not try to delete resources
	ob, err := updateObjectBucketPhase(c.libClientset, ob, v1alpha1.ObjectBucketClaimStatusPhaseReleased)
	if err != nil {
		return err
	}

	// decide whether Delete or Revoke is called
	if isNewBucketByObjectBucket(c.clientset, ob) && *ob.Spec.ReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
		if err = c.provisioner.Delete(ob); err != nil {
			// Do not proceed to deleting the ObjectBucket if the deprovisioning fails for bookkeeping purposes
			return fmt.Errorf("provisioner error deleting bucket %v", err)
		}
	} else {
		if err = c.provisioner.Revoke(ob); err != nil {
			return fmt.Errorf("provisioner error revoking access to bucket %v", err)
		}
	}

	return c.deleteResources(ob, cm, secret, obc)
}

func (c *obcController) supportedProvisioner(provisioner string) bool {
	return provisioner == c.provisionerName
}

// trim the errors resulting from objects not being found
func (c *obcController) getExistingResourcesFromKey(key string) (*v1alpha1.ObjectBucket, *corev1.ConfigMap, *corev1.Secret, []error) {
	ob, cm, secret, errs := c.getResourcesFromKey(key)
	for i := len(errs) - 1; i >= 0; i-- {
		if errors.IsNotFound(errs[i]) {
			errs = append(errs[:i], errs[i+1:]...)
		}
	}
	return ob, cm, secret, errs
}

// Gathers resources by names derived from key.
// Returns pointers to those resources if they exist, nil otherwise and an slice of errors who's
// len() == n errors. If no errors occur, len() is 0.
func (c *obcController) getResourcesFromKey(key string) (ob *v1alpha1.ObjectBucket, cm *corev1.ConfigMap, sec *corev1.Secret, errs []error) {

	var err error
	// The cap(errs) must be large enough to encapsulate errors returned by all 3 *ForClaimKey funcs
	errs = make([]error, 0, 3)
	groupErrors := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	ob, err = c.objectBucketForClaimKey(key)
	groupErrors(err)
	cm, err = configMapForClaimKey(key, c.clientset)
	groupErrors(err)
	sec, err = secretForClaimKey(key, c.clientset)
	groupErrors(err)

	return
}

// Deleting the resources generated by a Provision or Grant call is triggered by the delete of
// the OBC. However, a finalizer is added to the OBC so that we can cleanup up the other resources
// created by a Provision or Grant call. Since the secret and configmap's ownerReference is the OBC
// they will be garbage collected once their finalizers are removed. The OB must be explicitly
// deleted since it is a global resource and cannot have a namespaced ownerReference. The last step
// is to remove the finalizer on the OBC so it too will be garbage collected.
// Returns err if we can't delete one or more of the resources, the final returned error being
// somewhat arbitrary.
func (c *obcController) deleteResources(ob *v1alpha1.ObjectBucket, cm *corev1.ConfigMap, s *corev1.Secret, obc *v1alpha1.ObjectBucketClaim) (err error) {

	if delErr := deleteObjectBucket(ob, c.libClientset); delErr != nil {
		log.Error(delErr, "error deleting objectBucket", ob.Name)
		err = delErr
	}
	if delErr := releaseSecret(s, c.clientset); delErr != nil {
		log.Error(delErr, "error releasing secret")
		err = delErr
	}
	if delErr := releaseConfigMap(cm, c.clientset); delErr != nil {
		log.Error(delErr, "error releasing configMap")
		err = delErr
	}
	if delErr := releaseOBC(obc, c.libClientset); delErr != nil {
		log.Error(delErr, "error releasing obc")
		err = delErr
	}
	return err
}

// Add finalizer and labels to the OBC.
func (c *obcController) setOBCMetaFields(obc *v1alpha1.ObjectBucketClaim) (*v1alpha1.ObjectBucketClaim, error) {
	clib := c.libClientset

	// Do not make changes directly to the obc used as input. If the update fails, we should return
	// the obc given as input as it was given so code that comes after can't assume obc is at the
	// new phase.
	updateOBC := obc.DeepCopy()

	addFinalizers(updateOBC, []string{finalizer})
	addLabels(updateOBC, c.provisionerLabels)

	logD.Info("updating OBC metadata")
	obcUpdated, err := updateClaim(clib, updateOBC)
	if err != nil {
		return obc, fmt.Errorf("error configuring obc metadata: %v", err)
	}

	return obcUpdated, nil
}

func (c *obcController) objectBucketForClaimKey(key string) (*v1alpha1.ObjectBucket, error) {
	logD.Info("getting objectBucket for key", "key", key)
	name, err := objectBucketNameFromClaimKey(key)
	if err != nil {
		return nil, err
	}
	ob, err := c.libClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return ob, nil
}

func updateSupported(old, new *v1alpha1.ObjectBucketClaim) bool {

	// Deletiong stamp is set, so return true so that it will be added
	// to queue for deletion
	if new.ObjectMeta.DeletionTimestamp != nil {
		return true
	}

	// The only field supported for update is obc.spec.additionalConfig
	if reflect.DeepEqual(new.Spec, old.Spec) {
		return false
	}
	// create copy of old spec, and set the new spec's additionalConfig on it
	oldspec := old.Spec.DeepCopy()
	oldspec.AdditionalConfig = new.Spec.AdditionalConfig
	if !reflect.DeepEqual(*oldspec, new.Spec) {
		// new OBC spec has changed something other than additionalConfig
		log.Error(nil, "invalid changes to OBC. only additionalConfig can be updated")
		return false
	}
	return true
}
