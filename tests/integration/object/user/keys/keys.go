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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/secrets"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// generateObjectStoreUserSecretName returns the secret name the operator is expected to generate for the CephObjectStoreUser
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

// findUserKeySpec finds a UserKeySpec by AccessKey value
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

// checkNoS3Keys verifies that a CephObjectStoreUser with disableS3Keys set has
// finished reconciling its current spec into an rgw user without keys, with no
// Secret for the user and none named in its status.
func checkNoS3Keys(t *testing.T, k8sh *utils.K8sHelper, adminClient *admin.API, osu cephv1.CephObjectStoreUser) {
	t.Helper()

	t.Run(fmt.Sprintf("CephObjectStoreUser %q has no S3 keys", osu.Name), func(t *testing.T) {
		ctx := t.Context()
		osuClient := k8sh.RookClientset.CephV1().CephObjectStoreUsers(osu.Namespace)
		secretClient := k8sh.Clientset.CoreV1().Secrets(osu.Namespace)

		// Ready at the current generation is written only after the keys and
		// the Secret have been removed
		liveOsu := wait4.RequireCondition(ctx, t, osuClient, osu.Name, func(u *cephv1.CephObjectStoreUser) bool {
			return u.Spec.DisableS3Keys != nil && *u.Spec.DisableS3Keys && wait4.ObjectStoreUser(u) && u.Status.ObservedGeneration == u.Generation
		}, wait4.TimeoutShort)
		assert.NotContains(t, liveOsu.Status.Info, "secretName")
		assert.Empty(t, liveOsu.Status.Keys)

		liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu.Name})
		require.NoError(t, err)
		assert.Empty(t, liveUser.Keys)

		wait4.AssertAbsent(ctx, t, secretClient, generateObjectStoreUserSecretName(osu), wait4.TimeoutShort)
	})
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
		secret6 = awsKeySecret(ns.Name, defaultName+"-secret6", "K6QWZ3RNXH0B8T2LEJ5M", "Wq3n8ZfT0vLrB2sKdY7mXcP5hJ9gA1uE4oN6iR8t")

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

		// keyless from creation
		osu2 = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user2",
				Namespace: ns.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
				DisableS3Keys:    new(true),
			},
		}

		// created with a key, then made keyless, then given a key again
		osu3 = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user3",
				Namespace: ns.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
			},
		}
		osu3SecretName = generateObjectStoreUserSecretName(osu3)

		osuClient    = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)
		secretClient = k8sh.Clientset.CoreV1().Secrets(ns.Name)
	)
	t.Run("ObjectStoreUser keys", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		for _, secret := range []*corev1.Secret{secret1, secret2, secret3, secret4, secret5, secret6} {
			t.Run(fmt.Sprintf("create secret %q", secret.Name), func(t *testing.T) {
				_, err := secretClient.Create(ctx, secret, metav1.CreateOptions{})
				require.NoError(t, err)
			})
		}

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu2.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, osuClient, &osu2, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		checkNoS3Keys(t, k8sh, adminClient, osu2)

		t.Run(fmt.Sprintf("disableS3Keys with keys on CephObjectStoreUser %q is rejected", osu2.Name), func(t *testing.T) {
			patch, err := json.Marshal(map[string]any{"spec": map[string]any{
				"keys": []cephv1.ObjectUserKey{objectUserKey(secret6.Name)},
			}})
			require.NoError(t, err)

			_, err = osuClient.Patch(ctx, osu2.Name, types.MergePatchType, patch, metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}})
			require.Error(t, err)
			assert.True(t, kerrors.IsInvalid(err), err)
			assert.ErrorContains(t, err, "disableS3Keys cannot be combined with keys")
		})

		t.Run(fmt.Sprintf("disableS3Keys with an empty keys list on CephObjectStoreUser %q is accepted", osu2.Name), func(t *testing.T) {
			// the typed client omits an empty keys list, so the patch spells it out
			patch := []byte(`{"spec":{"keys":[]}}`)

			_, err := osuClient.Patch(ctx, osu2.Name, types.MergePatchType, patch, metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}})
			assert.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu3.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, osuClient, &osu3, wait4.ObjectStoreUser, wait4.TimeoutMedium)
		})

		osu3Secret := wait4.RequireCondition(ctx, t, secretClient, osu3SecretName, func(s *corev1.Secret) bool {
			return len(s.Data["AccessKey"]) > 0
		}, wait4.TimeoutShort)

		requireRgwUserKeys(t, adminClient, osu3, "AccessKey", "SecretKey", osu3Secret)

		t.Run(fmt.Sprintf("disable S3 keys on CephObjectStoreUser %q", osu3.Name), func(t *testing.T) {
			liveOsu, err := osuClient.Get(ctx, osu3.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.DisableS3Keys = new(true)

			_, err = osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		checkNoS3Keys(t, k8sh, adminClient, osu3)

		// turning the field off generates no key, so the way back to a key
		// published in the Secret is to declare one and then drop the declaration
		t.Run(fmt.Sprintf("declare key %q on CephObjectStoreUser %q", secret6.Name, osu3.Name), func(t *testing.T) {
			liveOsu, err := osuClient.Get(ctx, osu3.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.DisableS3Keys = new(false)
			liveOsu.Spec.Keys = []cephv1.ObjectUserKey{objectUserKey(secret6.Name)}

			_, err = osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		requireRgwUserKeys(t, adminClient, osu3, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", secret6)

		t.Run(fmt.Sprintf("remove all keys set on CephObjectStoreUser %q", osu3.Name), func(t *testing.T) {
			liveOsu, err := osuClient.Get(ctx, osu3.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.Keys = nil

			_, err = osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("secret %q holds the key declared from secret %q", osu3SecretName, secret6.Name), func(t *testing.T) {
			// the rgw user held only this key before the removal too, so only Ready at the
			// new generation shows that the removal has been reconciled
			wait4.RequireCondition(ctx, t, osuClient, osu3.Name, func(u *cephv1.CephObjectStoreUser) bool {
				return wait4.ObjectStoreUser(u) && u.Status.ObservedGeneration == u.Generation && len(u.Status.Keys) == 0
			}, wait4.TimeoutShort)

			secret, err := secretClient.Get(ctx, osu3SecretName, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, string(secret6.Data["AWS_ACCESS_KEY_ID"]), string(secret.Data["AccessKey"]))
			assert.Equal(t, string(secret6.Data["AWS_SECRET_ACCESS_KEY"]), string(secret.Data["SecretKey"]))
		})

		requireRgwUserKeys(t, adminClient, osu3, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", secret6)

		for _, osu := range []cephv1.CephObjectStoreUser{osu2, osu3} {
			t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu.Name), func(t *testing.T) {
				wait4.AssertDelete(ctx, t, osuClient, osu.Name, wait4.TimeoutShort)
			})
		}

		t.Run(fmt.Sprintf("secret %q is deleted with its user", osu3SecretName), func(t *testing.T) {
			wait4.AssertAbsent(ctx, t, secretClient, osu3SecretName, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
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

		// fetch the automatic secret; it should hold the only key set on the rgw
		// user. The controller writes the secret after reconciling the rgw keys,
		// so poll until it carries a key the rgw user has rather than snapshotting
		// it the instant the rgw user converges — that snapshot can predate the
		// write, and requireRgwUserKeys never re-reads it.
		var autoSecret *corev1.Secret
		wait4.RequireEventually(ctx, t, wait4.TimeoutShort, "automatic secret holds a key set on the rgw user", func(ctx context.Context) error {
			s, err := secretClient.Get(ctx, generateObjectStoreUserSecretName(osu1), metav1.GetOptions{})
			if err != nil {
				return err
			}
			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			if err != nil {
				return err
			}
			if _, err := findUserKeySpec(liveUser.Keys, string(s.Data["AccessKey"])); err != nil {
				return err
			}
			autoSecret = s
			return nil
		})

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
		for _, secret := range []*corev1.Secret{secret2, secret3, secret4, secret5, secret6} {
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
