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

package object

import (
	"context"
	"fmt"
	"strings"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// GetAccount retrieves account information from RGW using the admin ops API.
func GetAccount(ctx context.Context, adminOpsContext *AdminOpsContext, accountID string) (admin.Account, error) {
	if accountID == "" {
		return admin.Account{}, errors.New("account ID cannot be empty")
	}

	account, err := adminOpsContext.AdminOpsClient.GetAccount(ctx, accountID)
	if err != nil {
		return admin.Account{}, errors.Wrapf(err, "failed to get account %q", accountID)
	}

	return account, nil
}

// CreateAccount creates a new RGW account using the admin ops API.
// If account.ID is empty, Ceph will auto-generate the account ID.
func CreateAccount(ctx context.Context, adminOpsContext *AdminOpsContext, account admin.Account) (admin.Account, error) {
	if account.Name == "" {
		return admin.Account{}, errors.New("account name cannot be empty")
	}

	createdAccount, err := adminOpsContext.AdminOpsClient.CreateAccount(ctx, account)
	if err != nil {
		return admin.Account{}, errors.Wrapf(err, "failed to create account %q", account.Name)
	}

	return createdAccount, nil
}

// ModifyAccount modifies an existing RGW account.
func ModifyAccount(ctx context.Context, adminOpsContext *AdminOpsContext, account admin.Account) (admin.Account, error) {
	if account.ID == "" {
		return admin.Account{}, errors.New("account ID cannot be empty")
	}

	modifiedAccount, err := adminOpsContext.AdminOpsClient.ModifyAccount(ctx, account)
	if err != nil {
		return admin.Account{}, errors.Wrapf(err, "failed to modify account %q", account.ID)
	}

	return modifiedAccount, nil
}

// DeleteAccount removes an RGW account using the admin ops API.
func DeleteAccount(nsName types.NamespacedName, ctx context.Context, adminOpsContext *AdminOpsContext, accountID string) error {
	if accountID == "" {
		return errors.New("account ID cannot be empty")
	}

	err := adminOpsContext.AdminOpsClient.DeleteAccount(ctx, accountID)
	if err != nil {
		// If account doesn't exist, consider it successful (idempotent)
		if errors.Is(err, admin.ErrNoSuchKey) {
			log.NamedInfo(nsName, logger, "account %q not found, considering deletion successful", accountID)
			return nil
		}
		return errors.Wrapf(err, "failed to delete account %q", accountID)
	}

	return nil
}

// ForceDeleteAccount removes an RGW account by creating a Kubernetes Job that runs
// "radosgw-admin account rm --purge-data". A Job is used because purging large
// amounts of data can far exceed the operator's 15-second command timeout.
// The Job retries on failure until the purge completes or the account no longer exists.
func ForceDeleteAccount(uid types.UID, nsName types.NamespacedName, adminOpsContext *AdminOpsContext, accountID string, cephClusterSpec *cephv1.ClusterSpec) (bool, error) {
	if accountID == "" {
		return false, errors.New("account ID cannot be empty")
	}
	if adminOpsContext == nil {
		return false, errors.New("adminOpsContext cannot be nil")
	}

	clusterInfo := adminOpsContext.clusterInfo
	job := makeAccountPurgeJob(uid, nsName, adminOpsContext, accountID, clusterInfo, cephClusterSpec)

	if err := clusterInfo.OwnerInfo.SetControllerReference(job); err != nil {
		return false, errors.Wrapf(err, "failed to set owner reference on purge job for account %q", accountID)
	}

	log.NamedInfo(nsName, logger, "creating purge-data job %q for account %q", job.Name, accountID)
	if err := k8sutil.RunJob(clusterInfo.Context, adminOpsContext.Context.Context.Clientset, job); err != nil {
		return false, errors.Wrapf(err, "failed to create purge-data job for account %q", accountID)
	}

	// check for the job to complete with a re-qeueue loop, to not block other reconcile operations
	jobComplete, err := k8sutil.CheckIfJobIsCompleted(clusterInfo.Context, adminOpsContext.Context.Context.Clientset, job)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if purge-data job %q for account %q is completed", job.Name, accountID)
	}
	if !jobComplete {
		log.NamedInfo(nsName, logger, "purge-data job %q for account %q is still running, requeuing reconcile", job.Name, accountID)
		return true, nil
	}

	// if the job completed successfully, the account should be gone, but check to be sure
	_, err = GetAccount(clusterInfo.Context, adminOpsContext, accountID)
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchKey) {
			log.NamedInfo(nsName, logger, "account %q successfully purged", accountID)
			// delete the job now that the account is gone
			if err := k8sutil.DeleteBatchJob(clusterInfo.Context, adminOpsContext.Context.Context.Clientset, job.Namespace, job.Name, true); err != nil {
				return false, errors.Wrapf(err, "failed to delete purge-data job %q for account %q", job.Name, accountID)
			}
			// this function will take exit from here
			return false, nil
		}
	}

	// do not delete the job if the account still exists, so we can check the job logs for errors
	return false, errors.Wrapf(err, "failed to verify account %q was purged", accountID)
}

func makeAccountPurgeJob(uid types.UID, nsName types.NamespacedName, adminOpsContext *AdminOpsContext, accountID string, clusterInfo *cephclient.ClusterInfo, cephClusterSpec *cephv1.ClusterSpec) *batch.Job {
	// Build the radosgw-admin command args
	args := []string{
		"account", "rm",
		"--purge-data",
		fmt.Sprintf("--account-id=%s", accountID),
	}

	// Add multisite flags when applicable
	if adminOpsContext.Name != "" && clusterInfo.CephCred.Username == cephclient.AdminUsername {
		args = append(args,
			fmt.Sprintf("--rgw-realm=%s", adminOpsContext.Realm),
			fmt.Sprintf("--rgw-zonegroup=%s", adminOpsContext.ZoneGroup),
			fmt.Sprintf("--rgw-zone=%s", adminOpsContext.Zone),
		)
	}
	keyringFile := fmt.Sprintf("%s.keyring", clusterInfo.CephCred.Username)
	keyringFilePath := fmt.Sprintf("%s/%s/%s", cephClusterSpec.DataDirHostPath, nsName.Namespace, keyringFile)
	configFile := fmt.Sprintf("%s.config", clusterInfo.Namespace)
	configFilePath := fmt.Sprintf("%s/%s/%s", cephClusterSpec.DataDirHostPath, nsName.Namespace, configFile)

	args = append(args,
		fmt.Sprintf("--keyring=%s", keyringFilePath),
		fmt.Sprintf("--conf=%s", configFilePath),
		fmt.Sprintf("--name=%s", clusterInfo.CephCred.Username),
	)

	// Wrap in a shell to treat ENOENT (exit code 2) as success
	shellCmd := fmt.Sprintf(`
		radosgw-admin %s;
		rc=$?;
		if [ $rc -eq 0 ]; then
    		echo 'account purge succeeded';
  		elif [ $rc -eq 2 ]; then
    		echo 'account not found, nothing to purge';
    		exit 0;
  		else
    		echo "account purge failed with exit code $rc";
  		fi;
  		exit $rc
		`, strings.Join(args, " "),
	)

	volumes := []corev1.Volume{{Name: "account-cleanup", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: cephClusterSpec.DataDirHostPath}}}}
	volumeMounts := []corev1.VolumeMount{{Name: "account-cleanup", MountPath: cephClusterSpec.DataDirHostPath}}
	jobName := k8sutil.TruncateNodeNameForJob("rgw-account-purge-%s", fmt.Sprintf("%s-%s-%s", nsName.Name, nsName.Namespace, uid))

	labels := map[string]string{
		k8sutil.AppAttr:     "rgw-account-purge",
		k8sutil.ClusterAttr: clusterInfo.Namespace,
	}
	timeToLive := int32(86400) // 24 hours

	job := getJobTemplate(jobName, nsName.Namespace, labels, volumes, volumeMounts, shellCmd, cephClusterSpec, timeToLive)

	k8sutil.AddRookVersionLabelToJob(job)
	return job
}

func getJobTemplate(jobName, namespace string, labels map[string]string, volumes []corev1.Volume, volumeMounts []corev1.VolumeMount, shellCmd string, cephClusterSpec *cephv1.ClusterSpec, timeToLive int32) *batch.Job {
	return &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: batch.JobSpec{
			TTLSecondsAfterFinished: &timeToLive,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "rgw-account-purge",
							Image:           cephClusterSpec.CephVersion.Image,
							ImagePullPolicy: opcontroller.GetContainerImagePullPolicy(cephClusterSpec.CephVersion.ImagePullPolicy),
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{shellCmd},
							VolumeMounts:    volumeMounts,
							SecurityContext: opcontroller.PrivilegedContext(true),
						},
					},
					Volumes:            volumes,
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: k8sutil.DefaultServiceAccount,
				},
			},
		},
	}
}

// CreateAccountRootUser creates a root user for the given RGW account using the admin ops API.
func CreateAccountRootUser(ctx context.Context, adminOpsContext *AdminOpsContext, user admin.User) (admin.User, error) {
	if user.ID == "" {
		return admin.User{}, errors.New("user ID cannot be empty")
	}
	if user.AccountID == "" {
		return admin.User{}, errors.New("account ID cannot be empty")
	}

	createdUser, err := adminOpsContext.AdminOpsClient.CreateUser(ctx, user)
	if err != nil {
		return admin.User{}, errors.Wrapf(err, "failed to create root user %q for account %q", user.ID, user.AccountID)
	}

	return createdUser, nil
}

// GetAccountRootUser retrieves a root user from RGW using the admin ops API.
func GetAccountRootUser(ctx context.Context, adminOpsContext *AdminOpsContext, userID string) (admin.User, error) {
	if userID == "" {
		return admin.User{}, errors.New("user ID cannot be empty")
	}

	user, err := adminOpsContext.AdminOpsClient.GetUser(ctx, admin.User{ID: userID})
	if err != nil {
		return admin.User{}, errors.Wrapf(err, "failed to get root user %q", userID)
	}

	return user, nil
}

// ModifyAccountRootUser modifies an existing root user in RGW using the admin ops API.
func ModifyAccountRootUser(ctx context.Context, adminOpsContext *AdminOpsContext, user admin.User) (admin.User, error) {
	if user.ID == "" {
		return admin.User{}, errors.New("user ID cannot be empty")
	}

	modifiedUser, err := adminOpsContext.AdminOpsClient.ModifyUser(ctx, user)
	if err != nil {
		return admin.User{}, errors.Wrapf(err, "failed to modify root user %q", user.ID)
	}

	return modifiedUser, nil
}

// DeleteAccountRootUser removes a root user from RGW using the admin ops API.
func DeleteAccountRootUser(ctx context.Context, adminOpsContext *AdminOpsContext, userID string) error {
	if userID == "" {
		return errors.New("user ID cannot be empty")
	}

	err := adminOpsContext.AdminOpsClient.RemoveUser(ctx, admin.User{ID: userID})
	if err != nil {
		return errors.Wrapf(err, "failed to delete root user %q", userID)
	}

	return nil
}
