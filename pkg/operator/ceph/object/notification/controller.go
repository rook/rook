/*
Copyright 2021 The Rook Authors. All rights reserved.

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

// Package notification to manage a rook bucket notifications.
package notification

import (
	"context"
	"os"
	"time"

	"github.com/coreos/pkg/capnslog"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/object/bucket"
	"github.com/rook/rook/pkg/operator/ceph/object/topic"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	kapiv1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	packageName    = "ceph-bucket-notification"
	controllerName = packageName + "-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", packageName)

var (
	waitForRequeueIfTopicNotReady          = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
	waitForRequeueIfNotificationNotReady   = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
	waitForRequeueIfObjectBucketNotReady   = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
	waitForRequeueIfNotificationNotDeleted = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// ReconcileNotifications reconciles a CephbucketNotification
type ReconcileNotifications struct {
	client           client.Client
	context          *clusterd.Context
	opManagerContext context.Context
	recorder         record.EventRecorder
}

// Add creates a new CephBucketNotification controller and a new ObjectBucketClaim Controller and adds it to the Manager.
// The Manager will set fields on the Controller and start it when the Manager is started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	if os.Getenv(object.DisableOBCEnvVar) == "true" {
		logger.Info("skip running Object Bucket Notification controller")
		return nil
	}

	if err := addNotificationReconciler(mgr, &ReconcileNotifications{
		client:           mgr.GetClient(),
		context:          context,
		opManagerContext: opManagerContext,
		recorder:         mgr.GetEventRecorderFor(controllerName),
	}); err != nil {
		return err
	}

	return addOBCLabelReconciler(mgr, &ReconcileOBCLabels{
		client:           mgr.GetClient(),
		context:          context,
		opManagerContext: opManagerContext,
		recorder:         mgr.GetEventRecorderFor(controllerName),
	})
}

func addNotificationReconciler(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the OBC CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephBucketNotification{},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephBucketNotification]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephBucketNotification](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephBucketNotification object and makes changes based on the state read
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNotifications) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	reconcileResponse, notification, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reporting.ReportReconcileResult(logger, r.recorder, request, &notification, reconcileResponse, err)
}

func (r *ReconcileNotifications) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephBucketNotification, error) {
	// fetch the CephBucketNotification instance
	notification := &cephv1.CephBucketNotification{ObjectMeta: metav1.ObjectMeta{Name: request.Name, Namespace: request.Namespace}}
	bnName := request.NamespacedName
	r.recorder.Eventf(notification, kapiv1.EventTypeNormal, string(cephv1.ReconcileStarted), "Started reconciling CephBucketNotification %q", bnName)
	err := r.client.Get(r.opManagerContext, request.NamespacedName, notification)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("CephBucketNotification %q resource not found. Ignoring since resource must be deleted.", bnName)
			return reconcile.Result{}, *notification, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *notification, errors.Wrapf(err, "failed to retrieve CephBucketNotification %q", bnName)
	}

	// DELETE: the CR was deleted
	if !notification.GetDeletionTimestamp().IsZero() {
		logger.Debugf("CephBucketNotification %q was deleted", bnName)
		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *notification, nil
	}

	// get the topic associated with the notification, and make sure it is provisioned
	topicName := types.NamespacedName{Namespace: notification.Namespace, Name: notification.Spec.Topic}
	bucketTopic, err := topic.GetProvisioned(r.client, r.opManagerContext, topicName)
	if err != nil {
		return waitForRequeueIfTopicNotReady, *notification, errors.Wrapf(err, "topic %q not provisioned yet", topicName)
	}

	// Populate clusterInfo during each reconcile
	clusterInfo, clusterSpec, err := getReadyCluster(r.client, r.opManagerContext, *r.context, bucketTopic.Spec.ObjectStoreNamespace)
	if err != nil {
		return opcontroller.WaitForRequeueIfCephClusterNotReady, *notification, errors.Wrapf(err, "cluster is not ready")
	}
	if clusterInfo == nil || clusterSpec == nil {
		return opcontroller.WaitForRequeueIfCephClusterNotReady, *notification, errors.New("cluster is not ready")
	}

	// fetch all OBCs that has a label matching this CephBucketNotification
	namespaceListOpt := client.InNamespace(notification.Namespace)
	labelListOpt := client.MatchingLabels{
		notificationLabelPrefix + notification.Name: notification.Name,
	}
	obcList := &bktv1alpha1.ObjectBucketClaimList{}
	err = r.client.List(r.opManagerContext, obcList, namespaceListOpt, labelListOpt)
	if err != nil {
		return reconcile.Result{}, *notification, errors.Wrapf(err, "failed to list ObjectBucketClaims for CephBucketNotification %q", bnName)
	}
	if len(obcList.Items) == 0 {
		logger.Debugf("no ObjectbucketClaim associated with CephBucketNotification %q", bnName)
		return reconcile.Result{}, *notification, nil
	}

	// loop through all OBCs in the list and get their OBs
	for _, obc := range obcList.Items {
		if obc.Spec.ObjectBucketName == "" {
			return waitForRequeueIfObjectBucketNotReady, *notification, errors.Errorf("ObjectBucketClaim %q did not create the bucket yet",
				types.NamespacedName{Name: obc.Name, Namespace: obc.Namespace})
		}
		ob := bktv1alpha1.ObjectBucket{}
		bucketName := types.NamespacedName{Namespace: notification.Namespace, Name: obc.Spec.ObjectBucketName}
		if err := r.client.Get(r.opManagerContext, bucketName, &ob); err != nil {
			return reconcile.Result{}, *notification, errors.Wrapf(err, "failed to retrieve ObjectBucket %v", bucketName)
		}
		objectStoreName, err := getCephObjectStoreName(ob)
		if err != nil {
			return reconcile.Result{}, *notification, errors.Wrapf(err, "failed to get object store from ObjectBucket %q", bucketName)
		}
		if err = validateObjectStoreName(bucketTopic, objectStoreName); err != nil {
			return reconcile.Result{}, *notification, err
		}

		err = createNotificationFunc(
			provisioner{
				context:          r.context,
				clusterInfo:      clusterInfo,
				clusterSpec:      clusterSpec,
				opManagerContext: r.opManagerContext,
				owner:            ob.Spec.AdditionalState[bucket.CephUser],
				objectStoreName:  objectStoreName,
			},
			&ob,
			*bucketTopic.Status.ARN,
			notification,
		)
		if err != nil {
			return reconcile.Result{}, *notification, errors.Wrapf(err, "failed to provision notification for ObjectBucketClaims %q", bucketName)
		}
		logger.Infof("provisioned CephBucketNotification %q for ObjectBucketClaims %q", bnName, bucketName)
	}

	return reconcile.Result{}, *notification, nil
}

func getCephObjectStoreName(ob bktv1alpha1.ObjectBucket) (types.NamespacedName, error) {
	return bucket.GetObjectStoreNameFromBucket(&ob)
}

// verify that object store is configured correctly for OB, CephBucketNotification and CephBucketTopic
func validateObjectStoreName(topic *cephv1.CephBucketTopic, bucketStoreName types.NamespacedName) error {
	topicStoreName := types.NamespacedName{Name: topic.Spec.ObjectStoreName, Namespace: topic.Spec.ObjectStoreNamespace}
	if topicStoreName != bucketStoreName {
		return errors.Errorf("object store name mismatch between topic and bucket. %q != %q", topicStoreName, bucketStoreName)
	}
	return nil
}

// getReadyCluster get cluster info and spec if the cluster is ready
func getReadyCluster(client client.Client, opManagerContext context.Context, context clusterd.Context, objectStoreNamespace string) (*cephclient.ClusterInfo, *cephv1.ClusterSpec, error) {
	// find the namespace for the ceph cluster (may be different than the namespace of the notification CR)
	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, _ := opcontroller.IsReadyToReconcile(
		opManagerContext,
		client,
		types.NamespacedName{Namespace: objectStoreNamespace},
		controllerName,
	)
	if !isReadyToReconcile || !cephClusterExists {
		logger.Debug("Ceph cluster not yet present.")
		return nil, nil, nil
	}
	clusterInfo, _, _, err := opcontroller.LoadClusterInfo(&context, opManagerContext, cephCluster.Namespace, &cephCluster.Spec)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to populate cluster info")
	}
	return clusterInfo, &cephCluster.Spec, nil
}
