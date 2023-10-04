/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"syscall"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	RGWErrorNone = iota
	RGWErrorUnknown
	RGWErrorNotFound
	RGWErrorBadData
	RGWErrorParse
	ErrorCodeFileExists = 17
)

// An ObjectUser defines the details of an object store user.
type ObjectUser struct {
	UserID       string              `json:"userId"`
	DisplayName  *string             `json:"displayName"`
	Email        *string             `json:"email"`
	AccessKey    *string             `json:"accessKey"`
	SecretKey    *string             `json:"secretKey"`
	SystemUser   bool                `json:"systemuser"`
	AdminOpsUser bool                `json:"adminopsuser"`
	MaxBuckets   int                 `json:"max_buckets"`
	UserQuota    admin.QuotaSpec     `json:"user_quota"`
	Caps         []admin.UserCapSpec `json:"caps"`
}

// func decodeUser(data string) (*ObjectUser, int, error) {
func decodeUser(data string) (*ObjectUser, int, error) {
	var user admin.User
	err := json.Unmarshal([]byte(data), &user)
	if err != nil {
		return nil, RGWErrorParse, errors.Wrapf(err, "failed to unmarshal json. %s", data)
	}

	rookUser := ObjectUser{UserID: user.ID, DisplayName: &user.DisplayName, Email: &user.Email}

	if len(user.Caps) > 0 {
		rookUser.Caps = user.Caps
	}

	if user.MaxBuckets != nil {
		rookUser.MaxBuckets = *user.MaxBuckets
	}

	if user.UserQuota.Enabled != nil {
		rookUser.UserQuota = user.UserQuota
	}

	if len(user.Keys) > 0 {
		rookUser.AccessKey = &user.Keys[0].AccessKey
		rookUser.SecretKey = &user.Keys[0].SecretKey
	} else {
		return nil, RGWErrorBadData, errors.New("AccessKey and SecretKey are missing")
	}

	return &rookUser, RGWErrorNone, nil
}

// GetUser returns the user with the given ID.
// The function is used **ONCE** only to provision so the RGW Admin Ops User
// Subsequent interaction with the API will be done with the created user
func GetUser(c *Context, id string) (*ObjectUser, int, error) {
	logger.Debugf("getting s3 user %q", id)
	// note: err is set for non-existent user but result output is also empty
	result, err := runAdminCommand(c, false, "user", "info", "--uid", id)
	if strings.Contains(result, "no user info saved") {
		return nil, RGWErrorNotFound, errors.New("warn: s3 user not found")
	}
	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrapf(err, "radosgw-admin command err. %s", result)
	}
	match, err := extractJSON(result)
	if err != nil {
		return nil, RGWErrorParse, errors.Wrap(err, "failed to get json")
	}
	return decodeUser(match)
}

// CreateUser creates a new user with the information given.
// The function is used **ONCE** only to provision so the RGW Admin Ops User
// Subsequent interaction with the API will be done with the created user
func CreateUser(c *Context, user ObjectUser, force bool) (*ObjectUser, int, error) {
	logger.Debugf("creating s3 user %q", user.UserID)

	if strings.TrimSpace(user.UserID) == "" {
		return nil, RGWErrorBadData, errors.New("userId cannot be empty")
	}

	if user.DisplayName == nil {
		return nil, RGWErrorBadData, errors.New("displayName is required")
	}

	args := []string{
		"user",
		"create",
		"--uid", user.UserID,
		"--display-name", *user.DisplayName,
	}

	if user.Email != nil {
		args = append(args, "--email", *user.Email)
	}

	if user.SystemUser {
		args = append(args, "--system")
	}

	if user.AdminOpsUser {
		args = append(args, "--caps", rgwAdminOpsUserCaps)
	}

	if user.AccessKey != nil {
		args = append(args, "--access-key", *user.AccessKey)
	}

	if user.SecretKey != nil {
		args = append(args, "--secret", *user.SecretKey)
	}

	if force {
		args = append(args, "--yes-i-really-mean-it")
	}

	result, err := runAdminCommand(c, true, args...)
	if err != nil {
		if code, err := exec.ExtractExitCode(err); err == nil && code == int(syscall.EEXIST) {
			return nil, ErrorCodeFileExists, errors.New("s3 user already exists")
		}

		if strings.Contains(result, "could not create user: unable to create user, user: ") {
			return nil, ErrorCodeFileExists, errors.New("s3 user already exists")
		}

		if strings.Contains(result, "could not create user: unable to create user, email: ") && strings.Contains(result, " is the email address an existing user") {
			return nil, RGWErrorBadData, errors.New("email already in use")
		}

		if strings.Contains(result, "global_init: unable to open config file from search list") {
			return nil, RGWErrorUnknown, errors.New("skipping reconcile since operator is still initializing")
		}

		// We don't know what happened
		return nil, RGWErrorUnknown, errors.Wrapf(err, "failed to create s3 user. %s", result)
	}
	return decodeUser(result)
}

// CreateOrRecreateUserIfExists if the user doesn't exist, it is created, should it already exist it is deleted and re-created
// It is called from the rgw dashboard setup logic.
func CreateOrRecreateUserIfExists(c *Context, user ObjectUser, force bool) (*ObjectUser, int, error) {
	objUser, errCode, err := CreateUser(c, user, force)
	if err != nil || (errCode != ErrorCodeFileExists && errCode != RGWErrorNone) {
		return nil, errCode, err
	}

	if errCode == RGWErrorNone {
		return objUser, errCode, err
	} else if errCode == ErrorCodeFileExists {
		// If the user already exists, delete and re-create it
		_, err := DeleteUser(c, user.UserID)
		if err != nil {
			return nil, RGWErrorUnknown, err
		}
	}

	return CreateUser(c, user, force)
}

func ListUserBuckets(c *Context, id string, opts ...string) (string, error) {

	args := []string{"bucket", "list", "--uid", id}
	if opts != nil {
		args = append(args, opts...)
	}

	result, err := runAdminCommand(c, false, args...)

	return result, errors.Wrapf(err, "failed to list buckets for user uid=%q", id)
}

// DeleteUser deletes the user with the given ID.
// Even though we should be using the Admin Ops API, we keep this on purpose until the entire migration is completed
// Used for the dashboard user
func DeleteUser(c *Context, id string, opts ...string) (string, error) {
	args := []string{"user", "rm", "--uid", id}
	if opts != nil {
		args = append(args, opts...)
	}
	result, err := runAdminCommand(c, false, args...)
	if err != nil {
		// If User does not exist return success
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			return result, nil
		}

		res, innerErr := ListUserBuckets(c, id)
		if innerErr == nil && res != "" && res != "[]" {
			return result, errors.Wrapf(err, "s3 user uid=%q have following buckets %q", id, res)
		}
	}

	return result, errors.Wrapf(err, "failed to delete s3 user uid=%q", id)
}

func GenerateCephUserSecretName(store, username string) string {
	return fmt.Sprintf("rook-ceph-object-user-%s-%s", store, username)
}

func generateCephUserSecret(userConfig *admin.User, endpoint, namespace, storeName, tlsSecretName string) *corev1.Secret {
	secretName := GenerateCephUserSecretName(storeName, userConfig.ID)
	// Store the keys in a secret
	secrets := map[string]string{
		"AccessKey": userConfig.Keys[0].AccessKey,
		"SecretKey": userConfig.Keys[0].SecretKey,
		"Endpoint":  endpoint,
	}
	if tlsSecretName != "" {
		secrets["SSLCertSecretName"] = tlsSecretName
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":               AppName,
				"user":              userConfig.ID,
				"rook_cluster":      namespace,
				"rook_object_store": storeName,
			},
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	return secret
}

func ReconcileCephUserSecret(ctx context.Context, k8sclient client.Client, scheme *runtime.Scheme, ownerRef metav1.Object, userConfig *admin.User, endpoint, namespace, storeName, tlsSecretName string) (reconcile.Result, error) {
	// Generate Kubernetes Secret
	secret := generateCephUserSecret(userConfig, endpoint, namespace, storeName, tlsSecretName)

	// Set owner ref to the object store user object
	err := controllerutil.SetControllerReference(ownerRef, secret, scheme)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to set owner reference of ceph object user secret %q", secret.Name)
	}

	// Create Kubernetes Secret
	err = opcontroller.CreateOrUpdateObject(ctx, k8sclient, secret)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create or update ceph object user %q secret", secret.Name)
	}
	return reconcile.Result{}, nil
}
