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
	"fmt"
	"regexp"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/exec"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Context holds the context for the object store.
type Context struct {
	Context               *clusterd.Context
	clusterInfo           *cephclient.ClusterInfo
	Name                  string
	UID                   string
	Endpoint              string
	Realm                 string
	ZoneGroup             string
	Zone                  string
	adminOpsUserAccessKey string
	adminOpsUserSecretKey string
	adminOpsClient        *admin.API
}

const (
	// RGWAdminOpsUserSecretName is the secret name of the admin ops user
	// #nosec G101 since this is not leaking any hardcoded credentials, it's just the secret name
	RGWAdminOpsUserSecretName = "rgw-admin-ops-user"
	rgwAdminOpsUserAccessKey  = "accessKey"
	rgwAdminOpsUserSecretKey  = "secretKey"
	rgwAdminOpsUserCaps       = "buckets=*;users=*;usage=read;metadata=read;zone=read"
)

var (
	rgwAdminOpsUserDisplayName = "RGW Admin Ops User"
)

// NewContext creates a new object store context.
func NewContext(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, name string) *Context {
	return &Context{Context: context, Name: name, clusterInfo: clusterInfo}
}

func NewMultisiteContext(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, store *cephv1.CephObjectStore) (*Context, error) {
	objContext := &Context{Context: context, Name: store.Name, clusterInfo: clusterInfo}
	realmName, zoneGroupName, zoneName, err := getMultisiteForObjectStore(context, store)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get realm/zone group/zone for object store %q", store.Name)
	}

	objContext.Realm = realmName
	objContext.ZoneGroup = zoneGroupName
	objContext.Zone = zoneName
	return objContext, nil
}

func extractJSON(output string) (string, error) {
	// `radosgw-admin` sometimes leaves logs to stderr even if it succeeds.
	// So we should skip them if parsing output as json.
	pattern := regexp.MustCompile(`(?ms)^{.*}$`)
	match := pattern.Find([]byte(output))
	if match == nil {
		return "", errors.Errorf("didn't contain json. %s", output)
	}
	return string(match), nil
}

// RunAdminCommandNoMultisite is for running radosgw-admin commands in scenarios where an object-store has not been created yet or for commands on the realm or zonegroup (ex: radosgw-admin zonegroup get)
// This function times out after a fixed interval if no response is received.
func RunAdminCommandNoMultisite(c *Context, expectJSON bool, args ...string) (string, error) {
	command, args := cephclient.FinalizeCephCommandArgs("radosgw-admin", c.clusterInfo, args, c.Context.ConfigDir)

	// start the rgw admin command
	output, err := c.Context.Executor.ExecuteCommandWithTimeout(cephclient.CephCommandTimeout, command, args...)
	if err != nil {
		return output, err
	}
	if expectJSON {
		match, err := extractJSON(output)
		if err != nil {
			return output, errors.Wrap(err, "failed to parse as JSON")
		}
		output = match
	}

	return output, nil
}

// This function is for running radosgw-admin commands in scenarios where an object-store has been created and the Context has been updated with the appropriate realm, zone group, and zone.
func runAdminCommand(c *Context, expectJSON bool, args ...string) (string, error) {
	// If the objectStoreName is not passed in the storage class
	// This means we are pointing to an external cluster so these commands are not needed
	// simply because the external cluster mode does not support that yet
	//
	// The following conditions tries to determine if the cluster is external
	// When connecting to an external cluster, the Ceph user is different than client.admin
	// This is not perfect though since "client.admin" is somehow supported...
	if c.Name != "" && c.clusterInfo.CephCred.Username == cephclient.AdminUsername {
		options := []string{
			fmt.Sprintf("--rgw-realm=%s", c.Realm),
			fmt.Sprintf("--rgw-zonegroup=%s", c.ZoneGroup),
			fmt.Sprintf("--rgw-zone=%s", c.Zone),
		}

		args = append(args, options...)
	}

	// work around FIFO file I/O issue when radosgw-admin is not compatible between version
	// installed in Rook operator and RGW version in Ceph cluster (#7573)
	result, err := RunAdminCommandNoMultisite(c, expectJSON, args...)
	if err != nil && isFifoFileIOError(err) {
		logger.Debug("retrying 'radosgw-admin' command with OMAP backend to work around FIFO file I/O issue")

		// We can either run 'ceph --version' to determine the Ceph version running in the operator
		// and then pick a flag to use, or we can just try to use both flags and return the one that
		// works. Same number of commands being run.
		retryArgs := append(args, "--rgw-data-log-backing=omap") // v16.2.0- in the operator
		retryResult, retryErr := RunAdminCommandNoMultisite(c, expectJSON, retryArgs...)
		if retryErr != nil && isInvalidFlagError(retryErr) {
			retryArgs = append(args, "--rgw-default-data-log-backing=omap") // v16.2.1+ in the operator
			retryResult, retryErr = RunAdminCommandNoMultisite(c, expectJSON, retryArgs...)
		}

		return retryResult, retryErr
	}

	return result, err
}

func isFifoFileIOError(err error) bool {
	exitCode, extractErr := exec.ExtractExitCode(err)
	if extractErr != nil {
		logger.Errorf("failed to determine return code of 'radosgw-admin' command. assuming this could be a FIFO file I/O issue. %#v", err)
		return true
	}
	// exit code 5 (EIO) is returned when there is a FIFO file I/O issue
	return exitCode == 5
}

func isInvalidFlagError(err error) bool {
	exitCode, extractErr := exec.ExtractExitCode(err)
	if extractErr != nil {
		logger.Errorf("failed to determine return code of 'radosgw-admin' command. assuming this could be an invalid flag error. %#v", err)
	}
	// exit code 22 (EINVAL) is returned when there is an invalid flag
	// it's also returned from some other failures, but this should be rare for Rook
	return exitCode == 22
}

func GetAdminOPSUserCredentials(ctx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, objContext *Context, cephObjectStore *cephv1.CephObjectStore) (string, string, error) {
	if cephObjectStore.Spec.IsExternal() {
		// Fetch the secret for admin ops user
		s := &v1.Secret{}
		err := ctx.Client.Get(context.TODO(), types.NamespacedName{Name: RGWAdminOpsUserSecretName, Namespace: cephObjectStore.Namespace}, s)
		if err != nil {
			return "", "", err
		}

		accessKey, ok := s.Data[rgwAdminOpsUserAccessKey]
		if !ok {
			return "", "", errors.Errorf("failed to find accessKey %q for rgw admin ops in secret %q", rgwAdminOpsUserAccessKey, RGWAdminOpsUserSecretName)
		}
		secretKey, ok := s.Data[rgwAdminOpsUserSecretKey]
		if !ok {
			return "", "", errors.Errorf("failed to find secretKey %q for rgw admin ops in secret %q", rgwAdminOpsUserSecretKey, RGWAdminOpsUserSecretName)
		}

		// Set the keys for further usage
		return string(accessKey), string(secretKey), nil
	}

	// Fetch the admin ops user locally
	userConfig := ObjectUser{
		UserID:       RGWAdminOpsUserSecretName,
		DisplayName:  &rgwAdminOpsUserDisplayName,
		AdminOpsUser: true,
	}
	logger.Debugf("creating s3 user object %q for object store %q", userConfig.UserID, cephObjectStore.Namespace)
	user, rgwerr, err := CreateUser(objContext, userConfig)
	if err != nil {
		if rgwerr == ErrorCodeFileExists {
			user, _, err = GetUser(objContext, userConfig.UserID)
			if err != nil {
				return "", "", errors.Wrapf(err, "failed to get details from ceph object user %q for object store %q", userConfig.UserID, cephObjectStore.Name)
			}
		} else {
			return "", "", errors.Wrapf(err, "failed to create object user %q. error code %d for object store %q", userConfig.UserID, rgwerr, cephObjectStore.Name)
		}
	}
	return *user.AccessKey, *user.SecretKey, nil
}
