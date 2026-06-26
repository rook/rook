/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package keys

import (
	"context"
	"fmt"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/secrets"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// generate the secret name the operator is expected to generate for the CephObjectStoreUser
func generateObjectStoreUserSecretName(osu cephv1.CephObjectStoreUser) string {
	return "rook-ceph-object-user-" + osu.Spec.Store + "-" + osu.Name
}

// awsKeySecret returns a Secret holding an s3 access/secret key pair under the
// conventional AWS env var names.
func awsKeySecret(ns, name, accessKey, secretKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte(accessKey),
			"AWS_SECRET_ACCESS_KEY": []byte(secretKey),
		},
	}
}

// objectUserKey returns an ObjectUserKey referencing the access/secret key
// pair stored in the named awsKeySecret secret.
func objectUserKey(secretName string) cephv1.ObjectUserKey {
	return cephv1.ObjectUserKey{
		AccessKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: secretName,
			},
			Key: "AWS_ACCESS_KEY_ID",
		},
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: secretName,
			},
			Key: "AWS_SECRET_ACCESS_KEY",
		},
	}
}

// find a UserKeySpec by AccessKey value
func findUserKeySpec(keys []admin.UserKeySpec, accessKey string) (admin.UserKeySpec, error) {
	for _, k := range keys {
		if k.AccessKey == accessKey {
			return k, nil
		}
	}
	return admin.UserKeySpec{}, fmt.Errorf("UserKeySpec with AccessKey %q not found", accessKey)
}

func checkStatusKeys(t *testing.T, k8sh *utils.K8sHelper, osu cephv1.CephObjectStoreUser, expected ...*corev1.Secret) {
	secrets.RequireStatusRefs(t, k8sh,
		k8sh.RookClientset.CephV1().CephObjectStoreUsers(osu.Namespace), osu.Name,
		fmt.Sprintf("cephObjectStoreUser %q has .status.keys set", osu.Name),
		func(u *cephv1.CephObjectStoreUser) []cephv1.SecretReference {
			if u.Status == nil {
				return nil
			}
			return u.Status.Keys
		},
		expected...)
}

// requireRgwUserKeys verifies every expected key on the rgw user — reporting
// all divergences rather than stopping at the first — and then aborts the
// caller if any check failed: the steps that follow assume the keys are in
// sync, so continuing would only cascade noise.
func requireRgwUserKeys(t *testing.T, adminClient *admin.API, osu cephv1.CephObjectStoreUser, accessKeyName, secretKeyName string, expected ...*corev1.Secret) {
	t.Helper()

	if !t.Run(fmt.Sprintf("rgw user %q has keys set", osu.Name), func(t *testing.T) {
		ctx := t.Context()

		for _, secret := range expected {
			// assume that the .Phase doesn't change when updating keys
			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, fmt.Sprintf("rgw user key in sync with secret %q", secret.Name), func(ctx context.Context) error {
				liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu.Name})
				if err != nil {
					return err
				}
				keySpec, err := findUserKeySpec(liveUser.Keys, string(secret.Data[accessKeyName]))
				if err != nil {
					return err
				}
				if keySpec.SecretKey != string(secret.Data[secretKeyName]) {
					return fmt.Errorf("secret key for %q not yet in sync", secret.Name)
				}
				return nil
			})
		}

		// check that no extra keys are present
		liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu.Name})
		require.NoError(t, err)
		assert.Equal(t, len(expected), len(liveUser.Keys))
	}) {
		t.FailNow()
	}
}

const Namespace = "test-userkeys"

func TestObjectStoreUserKeys(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()
		adminClient = store.AdminClient()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		secret1 = awsKeySecret(ns.Name, defaultName+"-secret1", "P1ANF2BP4K2LPGOR5SBB", "yfCHPhhOed0vJkqZsGEODyJrdKqHD09OMTWCnwjX")
		secret2 = awsKeySecret(ns.Name, defaultName+"-secret2", "2I6RPUTQFMNCSYEXZ6VM", "uY066SWPfaOVlDeYc7GJyOTfkDejyDdXrqehS6wx")
		secret3 = awsKeySecret(ns.Name, defaultName+"-secret3", "J4D0P20F3EDR51OSND7Y", "jn89OpkXNoDlIHVVQ23mE2DZgPmuDK3WVH5ExOvQ")
		secret4 = awsKeySecret(ns.Name, defaultName+"-secret4", "MPZX7DG5WJWQ6VPCSLYT", "phh7DIxnLPeSD2V6FUouhmnWrKlKRD5dBykyXozX")
		secret5 = awsKeySecret(ns.Name, defaultName+"-secret5", "7TNSSANCO5KXK23IPT91", "HksEDf0hEh3PtTvl7s9x6CyXfkWuY8eAMYAcvH5l")

		osu1 = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user1",
				Namespace: ns.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
				Keys: []cephv1.ObjectUserKey{
					objectUserKey(secret1.Name),
					objectUserKey(secret2.Name),
					objectUserKey(secret3.Name),
				},
			},
		}

		osuClient    = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)
		secretClient = k8sh.Clientset.CoreV1().Secrets(ns.Name)
	)
	t.Run("ObjectStoreUser keys", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		for _, secret := range []*corev1.Secret{secret1, secret2, secret3, secret4, secret5} {
			t.Run(fmt.Sprintf("create secret %q", secret.Name), func(t *testing.T) {
				_, err := secretClient.Create(ctx, secret, metav1.CreateOptions{})
				require.NoError(t, err)
			})
		}

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, osuClient, &osu1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		requireRgwUserKeys(t, adminClient, osu1, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", secret1, secret2, secret3)
		checkStatusKeys(t, k8sh, osu1, secret1, secret2, secret3)

		t.Run(fmt.Sprintf("update keys on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveOsu, err := osuClient.Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.Keys = []cephv1.ObjectUserKey{
				objectUserKey(secret4.Name),
				objectUserKey(secret5.Name),
			}

			_, err = osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		requireRgwUserKeys(t, adminClient, osu1, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", secret4, secret5)
		checkStatusKeys(t, k8sh, osu1, secret4, secret5)

		// test transition from explicit keys -> automatic secret creation

		// when all explicit keys are removed from CephObjectStoreUser, one should
		// be left in place and the operator should [still] create a k8s secret for it
		t.Run(fmt.Sprintf("remove all keys set on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveOsu, err := osuClient.Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.Keys = nil

			_, err = osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)

			wait4.RequireEventually(ctx, t, wait4.TimeoutShort, "rgw user has exactly 1 key", func(ctx context.Context) error {
				liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
				if err != nil {
					return err
				}
				if len(liveUser.Keys) != 1 {
					return fmt.Errorf("rgw user has %d keys, want 1", len(liveUser.Keys))
				}
				return nil
			})
		})

		// fetch the automatic secret; it should hold the only key set on the rgw user
		autoSecret, err := secretClient.Get(ctx, generateObjectStoreUserSecretName(osu1), metav1.GetOptions{})
		require.NoError(t, err)

		requireRgwUserKeys(t, adminClient, osu1, "AccessKey", "SecretKey", autoSecret)

		t.Run(fmt.Sprintf("cephObjectStoreUser %q .status.keys is unset", osu1.Name), func(t *testing.T) {
			liveOsu, err := osuClient.Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			require.NotNil(t, liveOsu.Status)
			assert.Len(t, liveOsu.Status.Keys, 0)
		})

		// test transition automatic secret creation -> explicit keys
		t.Run(fmt.Sprintf("add keys to CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveOsu, err := osuClient.Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.Keys = []cephv1.ObjectUserKey{
				objectUserKey(secret1.Name),
				objectUserKey(secret2.Name),
				objectUserKey(secret3.Name),
				objectUserKey(secret4.Name),
				objectUserKey(secret5.Name),
			}

			_, err = osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		requireRgwUserKeys(t, adminClient, osu1, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", secret1, secret2, secret3, secret4, secret5)
		checkStatusKeys(t, k8sh, osu1, secret1, secret2, secret3, secret4, secret5)

		// updating a secret already referenced by a CephObjectStoreUser should trigger a reconcile
		t.Run(fmt.Sprintf("update secret %q data", secret1.Name), func(t *testing.T) {
			liveSecret, err := secretClient.Get(ctx, secret1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveSecret.Data = map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("foo"),
				"AWS_SECRET_ACCESS_KEY": []byte("bar"),
			}

			_, err = secretClient.Update(ctx, liveSecret, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		updatedSecret1, err := secretClient.Get(ctx, secret1.Name, metav1.GetOptions{})
		require.NoError(t, err)

		requireRgwUserKeys(t, adminClient, osu1, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", updatedSecret1, secret2, secret3, secret4, secret5)
		checkStatusKeys(t, k8sh, osu1, updatedSecret1, secret2, secret3, secret4, secret5)

		// deleting a secret referenced by a CephObjectStoreUser should trigger a reconcile (which fails)
		t.Run(fmt.Sprintf("delete secret %q", secret1.Name), func(t *testing.T) {
			err := secretClient.Delete(ctx, secret1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("CephObjectStoreUser %q has phase ReconcileFailed", osu1.Name), func(t *testing.T) {
			wait4.RequireCondition(ctx, t, osuClient, osu1.Name, wait4.ObjectStoreUserPhase(string(cephv1.ReconcileFailed)), wait4.TimeoutShort)
		})

		t.Run("missing referenced secret should not block CephObjectStoreUser deletion", func(t *testing.T) {
			t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
				wait4.AssertDelete(ctx, t, osuClient, osu1.Name, wait4.TimeoutShort)
			})
		})

		t.Run(fmt.Sprintf("no CephObjectStoreUsers in ns %q", ns.Name), func(t *testing.T) {
			osus, err := osuClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, osus.Items, 0)
		})

		t.Run(fmt.Sprintf("user %q does not exist", osu1.Name), func(t *testing.T) {
			_, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.ErrorIs(t, err, admin.ErrNoSuchUser)
		})

		// check that CephObjectStoreUser removal did not delete any secret resources

		// secret1 was deleted in a previous step
		for _, secret := range []*corev1.Secret{secret2, secret3, secret4, secret5} {
			t.Run(fmt.Sprintf("secret %q still exists", secret.Name), func(t *testing.T) {
				_, err := secretClient.Get(ctx, secret.Name, metav1.GetOptions{})
				require.NoError(t, err)
			})

			// cleanup the secrets created for the test
			t.Run(fmt.Sprintf("delete secret %q", secret.Name), func(t *testing.T) {
				err := secretClient.Delete(ctx, secret.Name, metav1.DeleteOptions{})
				require.NoError(t, err)
			})
		}

		t.Run(fmt.Sprintf("no secrets in ns %q", ns.Name), func(t *testing.T) {
			secrets, err := secretClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, secrets.Items, 0)
		})
	})
}
