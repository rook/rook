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
	"slices"
	"sort"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephBucketTopic{},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephBucketTopic]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephBucketTopic](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	// watch for kafka secrets
	// watch secrets referenced by CephBucketTopic.spec.endpoint.kafka.{userSecretRef,passwordSecretRef}
	const (
		// disable warning: G101: Potential hardcoded credentials (gosec)
		// nolint:gosec
		secretNameField = "spec.endpoint.kafka.secretNames"
	)

	err = mgr.GetFieldIndexer().IndexField(
		context.TODO(),
		&cephv1.CephBucketTopic{},
		secretNameField,
		func(obj client.Object) []string {
			var secretNames []string
			topic, ok := obj.(*cephv1.CephBucketTopic)
			if !ok {
				return nil
			}

			if topic.Spec.Endpoint.Kafka == nil {
				return nil
			}

			kafka := topic.Spec.Endpoint.Kafka

			if kafka.UserSecretRef != nil {
				secretNames = append(secretNames, kafka.UserSecretRef.Name)
			}

			if kafka.PasswordSecretRef != nil {
				secretNames = append(secretNames, kafka.PasswordSecretRef.Name)
			}

			slices.Sort(secretNames)
			return slices.Compact(secretNames)
		},
	)
	if err != nil {
		return errors.Wrapf(err, "failed to setup IndexField for CephBucketTopic.Spec.Endpoint.Kafka.{UserSecretRef,PasswordSecretRef}")
	}

	// Always trigger a reconcile when a secret is deleted. This will cause a
	// reconciliation failure to happen immediately in hopes of alerting the end
	// user to the configuration problem.
	changedOrDeleted := predicate.Or(
		predicate.TypedResourceVersionChangedPredicate[*corev1.Secret]{},
		predicate.TypedFuncs[*corev1.Secret]{
			DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Secret]) bool {
				return true
			},
		},
	)

	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Secret{},
			handler.TypedEnqueueRequestsFromMapFunc(
				func(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
					referencingTopics := &cephv1.CephBucketTopicList{}
					err := r.(*ReconcileBucketTopic).client.List(ctx, referencingTopics, &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector(secretNameField, secret.GetName()),
						Namespace:     secret.GetNamespace(),
					})
					if err != nil {
						logger.Errorf("failed to list CephBucketTopic(s) while handling event for secret %q in namespace %q. %v", secret.GetName(), secret.GetNamespace(), err)
						return []reconcile.Request{}
					}

					requests := make([]reconcile.Request, len(referencingTopics.Items))
					for i, item := range referencingTopics.Items {
						requests[i] = reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      item.GetName(),
								Namespace: item.GetNamespace(),
							},
						}
					}
					logger.Tracef("CephBucketTopic(s) referencing Secret %q in namespace %q: %v", secret.GetName(), secret.GetNamespace(), requests)
					return requests
				},
			),
			changedOrDeleted,
		),
	)
	if err != nil {
		return errors.Wrapf(err, "failed to configure watch for Secret(s)")
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephBucketTopic object and makes changes based on the state read
// and what is in the CephBucketTopic.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileBucketTopic) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus, nil, nil)
		log.NamedError(request.NamespacedName, logger, "failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileBucketTopic) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CephBucketTopic instance
	cephBucketTopic := &cephv1.CephBucketTopic{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephBucketTopic)
	if err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedDebug(request.NamespacedName, logger, "CephBucketTopic not found. Ignoring since resource must be deleted")
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
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephBucketTopic)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to add finalizer to CephBucketTopic %q", request.NamespacedName)
	}
	if generationUpdated {
		log.NamedInfo(request.NamespacedName, logger, "reconciling the object bucket topic after adding finalizer")
		return reconcile.Result{}, nil
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
		log.NamedDebug(request.NamespacedName, logger, "Ceph cluster not yet present, cannot create CephBucketTopic")
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
		log.NamedDebug(request.NamespacedName, logger, "deleting CephBucketTopic")
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
	r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcilingStatus, nil, nil)

	// create topic
	topicARN, referencedSecrets, err := r.createCephBucketTopic(cephBucketTopic)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "topic creation failed for CephBucketTopic %q", request.NamespacedName)
	}

	// update ObservedGeneration in status a the end of reconcile
	// Set Ready status, we are done reconciling
	r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.ReadyStatus, topicARN, referencedSecrets)

	// Return and do not requeue
	return reconcile.Result{}, nil
}

func (r *ReconcileBucketTopic) createCephBucketTopic(topic *cephv1.CephBucketTopic) (topicARN *string, referencedSecrets *map[types.UID]*corev1.Secret, err error) {
	topicARN, referencedSecrets, err = createTopicFunc(
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
func (r *ReconcileBucketTopic) updateStatus(observedGeneration int64, nsName types.NamespacedName, status string, topicARN *string, referencedSecrets *map[types.UID]*corev1.Secret) {
	topic := &cephv1.CephBucketTopic{}
	if err := r.client.Get(r.opManagerContext, nsName, topic); err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedDebug(nsName, logger, "CephBucketTopic %q not found. Ignoring since resource must be deleted", nsName)
			return
		}
		log.NamedWarning(nsName, logger, "failed to retrieve CephBucketTopic %q to update status to %q. error %v", nsName, status, err)
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

	log.NamedDebug(nsName, logger, "updating CephBucketTopic %q .status.secrets to %+v.", nsName, referencedSecrets)

	if referencedSecrets != nil {
		secretsStatus := []cephv1.SecretReference{}

		for _, secret := range *referencedSecrets {
			secretsStatus = append(secretsStatus, cephv1.SecretReference{
				SecretReference: corev1.SecretReference{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				},
				UID:             secret.UID,
				ResourceVersion: secret.ResourceVersion,
			})
		}

		// assume map key ordering is unstable between reconciles and sort the slice
		// by secret name
		sort.Slice(secretsStatus, func(i, j int) bool {
			return secretsStatus[i].Name < secretsStatus[j].Name
		})

		topic.Status.Secrets = secretsStatus
	}

	if err := reporting.UpdateStatus(r.client, topic); err != nil {
		log.NamedError(nsName, logger, "failed to set CephBucketTopic %q status to %q. error %v", nsName, status, err)
		return
	}
	log.NamedDebug(nsName, logger, "CephbucketTopic %q status updated to %q", nsName, status)
}
