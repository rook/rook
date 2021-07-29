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

package notification

import (
	"context"
	"strings"

	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object/topic"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	notificationLabelPrefix = "bucket-notification-"
)

// ReconcileOBCLabels reconciles a ObjectBucketClaim labels
type ReconcileOBCLabels struct {
	client           client.Client
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	clusterSpec      *cephv1.ClusterSpec
	opManagerContext context.Context
}

func addOBCLabelReconciler(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the OBC CRD object
	err = c.Watch(&source.Kind{Type: &bktv1alpha1.ObjectBucketClaim{}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a ObjectBucketClaim object and makes changes based on the state read
// and the ObjectBucketClaim labels
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileOBCLabels) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileOBCLabels) reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger.Debugf("reconciling ObjectBucketClaim %v labels for bucket notifications", request.NamespacedName.String())
	// Fetch the ObjectBucketClaim instance
	obc := bktv1alpha1.ObjectBucketClaim{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, &obc)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("ObjectBucketClaim %q resource not found. Ignoring since resource must be deleted.", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve ObjectBucketClaim %q", request.NamespacedName)
	}

	// reschedule if ObjectBucket was not created yet
	if obc.Spec.ObjectBucketName == "" {
		logger.Infof("ObjectBucketClaim %q resource did not create the bucket yet. will retry", request.NamespacedName)
		return waitForRequeueIfObjectBucketNotReady, nil
	}

	// get the ObjectBucket
	ob := bktv1alpha1.ObjectBucket{}
	bucketName := types.NamespacedName{Namespace: obc.Namespace, Name: obc.Spec.ObjectBucketName}
	if err := r.client.Get(r.opManagerContext, bucketName, &ob); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve ObjectBucket %q", bucketName)
	}
	objectStoreName, err := getCephObjectStoreName(ob)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to get object store from ObjectBucket %q", bucketName)
	}

	// find the namespace for the ceph cluster (may be different than the namespace of the topic CR)
	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(
		r.opManagerContext,
		r.client,
		types.NamespacedName{Namespace: objectStoreName.Namespace},
		controllerName,
	)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		if !obc.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		logger.Debug("Ceph cluster not yet present")
		return reconcileResponse, nil
	}
	r.clusterSpec = &cephCluster.Spec

	// DELETE: the CR was deleted
	if !obc.GetDeletionTimestamp().IsZero() {
		logger.Debugf("ObjectBucketClaim %q was deleted", request.NamespacedName)
		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = mon.LoadClusterInfo(r.context, r.opManagerContext, cephCluster.Namespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}

	// delete all existing notifications
	provisioner := Provisioner{
		Client:           r.client,
		Context:          r.context,
		ClusterInfo:      r.clusterInfo,
		ClusterSpec:      r.clusterSpec,
		opManagerContext: r.opManagerContext,
	}
	session, err := provisioner.createSession(
		ob.Spec.AdditionalState["cephUser"],
		objectStoreName,
	)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create session for bucket notification provisioning for ObjectBucketClaim %q", bucketName)
	}
	err = provisioner.DeleteAll(&ob, session)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed delete all bucket notifications from ObjectbucketClaim %q", bucketName)
	}

	// looking for notifications in the labels
	for labelKey, labelValue := range obc.Labels {
		notifyLabels := strings.SplitAfterN(labelKey, notificationLabelPrefix, 2)
		if len(notifyLabels) > 1 && notifyLabels[1] != "" {
			if labelValue != notifyLabels[1] {
				logger.Warningf("bucket notification label mismatch. ignoring key %q value %q", labelKey, labelValue)
				continue
			}
			// for each notification label fetch the bucket notification CRD
			logger.Debugf("bucket notification label %q found on ObjectbucketClaim %q", labelValue, bucketName)
			notification := &cephv1.CephBucketNotification{}
			bnName := types.NamespacedName{Namespace: obc.Namespace, Name: labelValue}
			if err := r.client.Get(r.opManagerContext, bnName, notification); err != nil {
				if kerrors.IsNotFound(err) {
					logger.Infof("CephBucketNotification %q not found", bnName)
					return waitForRequeueIfNotificationNotReady, nil
				}
				return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve CephBucketNotification %q", bnName)
			}

			// verify that the associated topic configuration exists with an ARN
			bucketTopic := &cephv1.CephBucketTopic{}
			topicName := types.NamespacedName{Namespace: obc.Namespace, Name: notification.Spec.Topic}
			if err := r.client.Get(r.opManagerContext, topicName, bucketTopic); err != nil {
				if kerrors.IsNotFound(err) {
					logger.Infof("CephBucketTopic %q not found", topicName)
					return waitForRequeueIfTopicNotReady, nil
				}
				return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve CephBucketTopic %q", topicName)
			}
			topicARN, err := topic.GetARN(bucketTopic)
			if err != nil {
				return waitForRequeueIfTopicNotReady, errors.Wrapf(err, "CephBucketTopic %q not provisioned", topicName)
			}

			if err = validateObjectStoreName(bucketTopic, objectStoreName); err != nil {
				return reconcile.Result{}, err
			}

			// provision the notification
			err = provisioner.Create(&ob, topicARN, notification, session)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to provision CephBucketNotification %q", bnName)
			}
			logger.Infof("provisioned CephBucketNotification %q", bnName)
		}
	}

	return reconcile.Result{}, nil
}
