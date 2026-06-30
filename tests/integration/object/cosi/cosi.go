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

package cosi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cosiv1 "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephcosi "github.com/rook/rook/pkg/operator/ceph/object/cosi"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-cosi"

// cosiInstallKustomize installs the upstream COSI CRDs and central controller.
// It is pinned to the same release as the typed client in go.mod so the
// installed CRDs match the Go types this test builds. The repo root
// kustomization aggregates the CRDs (client/config/crd) and the controller.
const cosiInstallKustomize = "github.com/kubernetes-sigs/container-object-storage-interface?ref=v0.2.2"

func TestCephCOSIDriver(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName       = Namespace
		objectStore       = store.ObjectStore()
		adminClient       = store.AdminClient()
		operatorNamespace = installer.SystemNamespace(objectStore.Namespace)

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		// the COSI driver provisions buckets and users on behalf of bucket
		// claims, so it needs a privileged object store user
		osu = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user",
				Namespace: ns.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
				Capabilities: &cephv1.ObjectUserCapSpec{
					User:   "*",
					Bucket: "*",
				},
			},
		}
		userSecretName = fmt.Sprintf("rook-ceph-object-user-%s-%s", objectStore.Name, osu.Name)

		cephCOSIDriver = cephv1.CephCOSIDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cephcosi.CephCOSIDriverName,
				Namespace: operatorNamespace,
			},
			Spec: cephv1.CephCOSIDriverSpec{
				DeploymentStrategy: cephv1.COSIDeploymentStrategyAuto,
			},
		}

		bucketClass = cosiv1.BucketClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName + "-bucketclass",
			},
			DriverName:     cephcosi.CephCOSIDriverPrefix + ".ceph.objectstorage.k8s.io",
			DeletionPolicy: cosiv1.DeletionPolicyDelete,
			Parameters: map[string]string{
				"objectStoreUserSecretName":      userSecretName,
				"objectStoreUserSecretNamespace": ns.Name,
			},
		}

		bucketClaim = cosiv1.BucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-bucketclaim",
				Namespace: ns.Name,
			},
			Spec: cosiv1.BucketClaimSpec{
				BucketClassName: bucketClass.Name,
				Protocols:       []cosiv1.Protocol{cosiv1.ProtocolS3},
			},
		}

		osuClient         = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)
		driverClient      = k8sh.RookClientset.CephV1().CephCOSIDrivers(operatorNamespace)
		deployClient      = k8sh.Clientset.AppsV1().Deployments(operatorNamespace)
		bucketClassClient = k8sh.COSIClientset.ObjectstorageV1alpha1().BucketClasses()
		bucketClaimClient = k8sh.COSIClientset.ObjectstorageV1alpha1().BucketClaims(ns.Name)
		bucketClient      = k8sh.COSIClientset.ObjectstorageV1alpha1().Buckets()
	)

	t.Run("COSI driver", func(t *testing.T) {
		if store.TLSEnabled() {
			// The ceph-cosi driver is deployed without any RGW CA trust (see
			// pkg/operator/ceph/object/cosi/spec.go), so it cannot provision
			// against a TLS-only object store endpoint.
			t.Skip("ceph-cosi driver does not support a TLS object store endpoint")
		}

		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)
		requireCOSIInstall(t, k8sh)

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, osuClient, &osu, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("create CephCOSIDriver %q", cephCOSIDriver.Name), func(t *testing.T) {
			_, err := driverClient.Create(ctx, &cephCOSIDriver, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("ceph-cosi-driver Deployment %q becomes ready", cephCOSIDriver.Name), func(t *testing.T) {
			// the driver pulls its container images on first reconcile, so allow
			// more time than a routine reconcile wait
			wait4.RequireCondition(ctx, t, deployClient, cephCOSIDriver.Name, deploymentReady, 3*time.Minute)
		})

		t.Run(fmt.Sprintf("create BucketClass %q", bucketClass.Name), func(t *testing.T) {
			_, err := bucketClassClient.Create(ctx, &bucketClass, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create BucketClaim %q", bucketClaim.Name), func(t *testing.T) {
			_, err := bucketClaimClient.Create(ctx, &bucketClaim, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		var backendBucket string
		t.Run(fmt.Sprintf("BucketClaim %q becomes ready", bucketClaim.Name), func(t *testing.T) {
			liveClaim := wait4.RequireCondition(ctx, t, bucketClaimClient, bucketClaim.Name,
				func(bc *cosiv1.BucketClaim) bool {
					return bc.Status.BucketReady && bc.Status.BucketName != ""
				},
				wait4.TimeoutLong)
			backendBucket = liveClaim.Status.BucketName
		})

		t.Run("provisioned Bucket becomes ready", func(t *testing.T) {
			require.NotEmpty(t, backendBucket, "BucketClaim did not report a bucket name")
			wait4.RequireCondition(ctx, t, bucketClient, backendBucket,
				func(b *cosiv1.Bucket) bool { return b.Status.BucketReady },
				wait4.TimeoutLong)
		})

		t.Run("bucket exists in backend ceph", func(t *testing.T) {
			require.NotEmpty(t, backendBucket, "BucketClaim did not report a bucket name")
			info, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: backendBucket})
			require.NoError(t, err)
			assert.Equal(t, backendBucket, info.Bucket)
		})

		t.Run(fmt.Sprintf("delete BucketClaim %q", bucketClaim.Name), func(t *testing.T) {
			// the backing Bucket is garbage-collected by the COSI controller
			wait4.AssertDelete(ctx, t, bucketClaimClient, bucketClaim.Name, wait4.TimeoutLong)

			wait4.AssertAbsent(ctx, t, bucketClient, backendBucket, wait4.TimeoutShort)
		})

		t.Run("bucket removed from backend ceph", func(t *testing.T) {
			// deletionPolicy Delete means deleting the claim deprovisions the bucket
			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, "backend bucket is deleted", func(ctx context.Context) error {
				_, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: backendBucket})
				if err == nil {
					return fmt.Errorf("bucket %q still exists in backend", backendBucket)
				}
				if !errors.Is(err, admin.ErrNoSuchBucket) {
					return fmt.Errorf("checking backend bucket %q: %w", backendBucket, err)
				}
				return nil
			})
		})

		t.Run(fmt.Sprintf("delete BucketClass %q", bucketClass.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, bucketClassClient, bucketClass.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, osuClient, osu.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete CephCOSIDriver %q", cephCOSIDriver.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, driverClient, cephCOSIDriver.Name, wait4.TimeoutShort)
		})
	})
}

// requireCOSIInstall installs the upstream COSI CRDs and central controller and
// registers their removal via t.Cleanup. rook only deploys the ceph-cosi driver
// (from the CephCOSIDriver CR); the CRDs and the central controller that
// reconciles BucketClaims into Buckets come from upstream.
func requireCOSIInstall(t *testing.T, k8sh *utils.K8sHelper) {
	t.Helper()

	_, err := k8sh.Kubectl("create", "-k", cosiInstallKustomize)
	require.NoError(t, err, "failed to install COSI CRDs and controller")

	t.Cleanup(func() {
		if _, err := k8sh.Kubectl("delete", "-k", cosiInstallKustomize, "--ignore-not-found=true"); err != nil {
			t.Errorf("failed to uninstall COSI CRDs and controller: %v", err)
		}
	})
}

// deploymentReady reports whether a Deployment has at least one ready replica.
func deploymentReady(d *appsv1.Deployment) bool {
	return d.Status.ReadyReplicas >= 1
}
