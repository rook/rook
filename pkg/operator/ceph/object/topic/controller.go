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

// Package topic to manage a rook bucket topics.
package topic

import (
	"context"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
	packageName    = "ceph-bucket-topic"
	controllerName = packageName + "-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", packageName)

// ReconcileBucketTopic reconciles a CephBucketTopic resource
type ReconcileBucketTopic struct {
	client           client.Client
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	clusterSpec      *cephv1.ClusterSpec
	opManagerContext context.Context
}

// Add creates a new CephBucketTopic Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, &ReconcileBucketTopic{
		client:           mgr.GetClient(),
		context:          context,
		opManagerContext: opManagerContext,
	})
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephBucketTopic CRD object
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &cephv1.CephBucketTopic{}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate()))
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephBucketTopic object and makes changes based on the state read
// and what is in the CephBucketTopic.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileBucketTopic) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus, nil)
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileBucketTopic) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CephBucketTopic instance
	cephBucketTopic := &cephv1.CephBucketTopic{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephBucketTopic)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("CephBucketTopic %q not found. Ignoring since resource must be deleted", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrapf(err, "failed to get CephBucketTopic %q", request.NamespacedName)
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephBucketTopic.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephBucketTopic)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to add finalizer to CephBucketTopic %q", request.NamespacedName)
	}

	// The CR was just created, initializing status fields
	if cephBucketTopic.Status == nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.EmptyStatus, nil)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(
		r.opManagerContext,
		r.client,
		types.NamespacedName{Namespace: cephBucketTopic.Spec.ObjectStoreNamespace},
		controllerName,
	)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		if !cephBucketTopic.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephBucketTopic)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to remove finalizer for CephBucketTopic %q", request.NamespacedName)
			}
			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		logger.Debugf("Ceph cluster not yet present, cannot create CephBucketTopic %q", request.NamespacedName)
		return reconcileResponse, nil
	}
	r.clusterSpec = &cephCluster.Spec

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, cephCluster.Namespace, r.clusterSpec)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}

	// DELETE: the CR was deleted
	if !cephBucketTopic.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting CephBucketTopic: %q", request.NamespacedName)
		err = r.deleteCephBucketTopic(cephBucketTopic)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete CephBucketTopic %q", request.NamespacedName)
		}
		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephBucketTopic)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to remove finalizer for CephBucketTopic %q", request.NamespacedName)
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the topic settings
	err = cephBucketTopic.ValidateTopicSpec()
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "invalid CephBucketTopic %q", request.NamespacedName)
	}

	// Start object reconciliation, updating status for this
	r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcilingStatus, nil)

	// create topic
	topicARN, err := r.createCephBucketTopic(cephBucketTopic)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "topic creation failed for CephBucketTopic %q", request.NamespacedName)
	}

	// update ObservedGeneration in status a the end of reconcile
	// Set Ready status, we are done reconciling
	r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.ReadyStatus, topicARN)

	// Return and do not requeue
	return reconcile.Result{}, nil
}

func (r *ReconcileBucketTopic) createCephBucketTopic(topic *cephv1.CephBucketTopic) (topicARN *string, err error) {
	topicARN, err = createTopicFunc(
		provisioner{
			client:           r.client,
			context:          r.context,
			clusterInfo:      r.clusterInfo,
			clusterSpec:      r.clusterSpec,
			opManagerContext: r.opManagerContext,
		},
		topic,
	)
	return
}

func (r *ReconcileBucketTopic) deleteCephBucketTopic(topic *cephv1.CephBucketTopic) error {
	return deleteTopicFunc(
		provisioner{
			client:           r.client,
			context:          r.context,
			clusterInfo:      r.clusterInfo,
			clusterSpec:      r.clusterSpec,
			opManagerContext: r.opManagerContext,
		},
		topic,
	)
}

// updateStatus updates the topic with a given status
func (r *ReconcileBucketTopic) updateStatus(observedGeneration int64, nsName types.NamespacedName, status string, topicARN *string) {
	topic := &cephv1.CephBucketTopic{}
	if err := r.client.Get(r.opManagerContext, nsName, topic); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("CephBucketTopic %q not found. Ignoring since resource must be deleted", nsName)
			return
		}
		logger.Warningf("failed to retrieve CephBucketTopic %q to update status to %q. error %v", nsName, status, err)
		return
	}
	if topic.Status == nil {
		topic.Status = &cephv1.BucketTopicStatus{}
	}

	topic.Status.ARN = topicARN
	topic.Status.Phase = status
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		topic.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, topic); err != nil {
		logger.Errorf("failed to set CephBucketTopic %q status to %q. error %v", nsName, status, err)
		return
	}
	logger.Debugf("CephbucketTopic %q status updated to %q", nsName, status)
}
