/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package wait4

import (
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

// ObjectStore reports whether a CephObjectStore has been reconciled to Ready.
func ObjectStore(os *cephv1.CephObjectStore) bool {
	return os.Status != nil && os.Status.Phase == cephv1.ConditionReady
}

// ObjectStoreDeletionBlocked reports whether a CephObjectStore's deletion is
// blocked on dependents.
func ObjectStoreDeletionBlocked(os *cephv1.CephObjectStore) bool {
	if os.Status == nil {
		return false
	}
	cond := cephv1.FindStatusCondition(os.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

// ObjectStoreUser reports whether a CephObjectStoreUser has been reconciled to
// Ready.
func ObjectStoreUser(u *cephv1.CephObjectStoreUser) bool {
	return ObjectStoreUserPhase(string(cephv1.ConditionReady))(u)
}

// ObjectStoreUserPhase returns a predicate reporting whether a
// CephObjectStoreUser's .status.phase matches phase. The parameter is a plain
// string because the cephv1 phase constants are a mix of ConditionType and
// ConditionReason (e.g. string(cephv1.ReconcileFailed)).
func ObjectStoreUserPhase(phase string) func(*cephv1.CephObjectStoreUser) bool {
	return func(u *cephv1.CephObjectStoreUser) bool {
		return u.Status != nil && u.Status.Phase == phase
	}
}

// BucketTopic reports whether a CephBucketTopic has been reconciled to Ready
// with its ARN populated. Note that .status.ARN is nil while .status.phase ==
// Reconciling, so the phase check alone is not sufficient.
func BucketTopic(bt *cephv1.CephBucketTopic) bool {
	return BucketTopicPhase(string(cephv1.ConditionReady))(bt) && bt.Status.ARN != nil
}

// BucketTopicPhase returns a predicate reporting whether a CephBucketTopic's
// .status.phase matches phase. The parameter is a plain string because the
// cephv1 phase constants are a mix of ConditionType and ConditionReason (e.g.
// string(cephv1.ReconcileFailed)).
func BucketTopicPhase(phase string) func(*cephv1.CephBucketTopic) bool {
	return func(bt *cephv1.CephBucketTopic) bool {
		return bt.Status != nil && bt.Status.Phase == phase
	}
}

// OBCBound reports whether an ObjectBucketClaim has reached the Bound phase.
func OBCBound(obc *bktv1alpha1.ObjectBucketClaim) bool {
	return obc.Status.Phase == bktv1alpha1.ObjectBucketClaimStatusPhaseBound
}

// OBBound reports whether an ObjectBucket has reached the Bound phase.
func OBBound(ob *bktv1alpha1.ObjectBucket) bool {
	return ob.Status.Phase == bktv1alpha1.ObjectBucketStatusPhaseBound
}
