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

package object

import (
	"fmt"

	"github.com/pkg/errors"
	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/dependents"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const bucketDependentType = "buckets in the object store (could be from ObjectBucketClaims or COSI Buckets)"

// CephObjectStoreDependents returns the buckets which exist in the object store that should block
// deletion.
// TODO: need unit tests for this - need to be able to fake the admin ops API (nontrivial)
func CephObjectStoreDependents(
	clusterdCtx *clusterd.Context,
	clusterInfo *client.ClusterInfo,
	store *v1.CephObjectStore,
	objCtx *Context,
	opsCtx *AdminOpsContext,
) (*dependents.DependentList, error) {
	nsName := fmt.Sprintf("%s/%s", store.Namespace, store.Name)
	baseErrMsg := fmt.Sprintf("failed to get dependents of CephObjectStore %q", nsName)

	deps := dependents.NewDependentList()

	// NOTE: we should still check for buckets when the RGW connection is external since we have no
	// way of knowing if the bucket was created due to an ObjectBucketClaim or COSI Bucket.
	err := getBucketDependents(deps, clusterdCtx, clusterInfo, store, objCtx, opsCtx)
	if err != nil {
		return deps, errors.Wrapf(err, baseErrMsg)
	}

	// CephObjectStoreUsers
	users, err := clusterdCtx.RookClientset.CephV1().CephObjectStoreUsers(store.Namespace).List(clusterInfo.Context, metav1.ListOptions{})
	if err != nil {
		return deps, errors.Wrapf(err, "%s. failed to list CephObjectStoreUsers for CephObjectStore %q", baseErrMsg, nsName)
	}
	for _, user := range users.Items {
		if user.Spec.Store == store.Name {
			deps.Add("CephObjectStoreUsers", user.Name)
		}
		logger.Debugf("found CephObjectStoreUser %q that does not depend on CephObjectStore %q", user.Name, nsName)
	}

	return deps, nil
}

// adds bucket dependents to the given dependents list
func getBucketDependents(
	deps *dependents.DependentList,
	clusterdCtx *clusterd.Context,
	clusterInfo *client.ClusterInfo,
	store *v1.CephObjectStore,
	objCtx *Context,
	opsCtx *AdminOpsContext,
) error {
	nsName := fmt.Sprintf("%s/%s", store.Namespace, store.Name)

	missingPools, err := missingPools(objCtx)
	if err != nil {
		return errors.Wrapf(err, "failed to check for object buckets")
	}
	if len(missingPools) > 0 {
		// this may be an external object store that does not have all the necessary pools.
		// this may also be a Rook-created store that did not finish deleting all pools before the
		// Rook operator restarted.
		// in either case, we cannot get a successful connection to RGW(s) to check for buckets, and
		// we can assume it is safe for deletion to proceed
		logger.Infof("skipping check for bucket dependents of CephObjectStore %q. some pools are missing: %v", nsName, missingPools)
		return nil
	}

	// buckets (including lib-bucket-provisioner buckets and COSI buckets)
	buckets, err := opsCtx.AdminOpsClient.ListBuckets(clusterInfo.Context)
	if err != nil {
		return errors.Wrapf(err, "failed to list buckets in CephObjectStore %q", nsName)
	}
	healthCheckBucket := genHealthCheckerBucketName(string(store.UID))
	for _, b := range buckets {
		if b == healthCheckBucket {
			continue // don't include the health checker bucket as a blocking dependent
		}
		deps.Add(bucketDependentType, b)
	}

	return nil
}
