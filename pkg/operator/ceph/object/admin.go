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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strings"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	"github.com/google/go-cmp/cmp"
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
	Context     *clusterd.Context
	clusterInfo *cephclient.ClusterInfo
	Name        string
	UID         string
	Endpoint    string
	Realm       string
	ZoneGroup   string
	Zone        string
}

// AdminOpsContext holds the object store context as well as information for connecting to the admin
// ops API.
type AdminOpsContext struct {
	Context
	TlsCert               []byte
	AdminOpsUserAccessKey string
	AdminOpsUserSecretKey string
	AdminOpsClient        *admin.API
}

type debugHTTPClient struct {
	client admin.HTTPClient
	logger *capnslog.PackageLogger
}

// NewDebugHTTPClient helps us mutating the HTTP client to debug the request/response
func NewDebugHTTPClient(client admin.HTTPClient, logger *capnslog.PackageLogger) *debugHTTPClient {
	return &debugHTTPClient{client, logger}
}

func (c *debugHTTPClient) Do(req *http.Request) (*http.Response, error) {
	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		return nil, err
	}
	// this can leak credentials for making requests
	c.logger.Tracef("\n%s\n", string(dump))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dump, err = httputil.DumpResponse(resp, true)
	if err != nil {
		return nil, err
	}
	// this can leak any sensitive info like credentials in the response
	c.logger.Tracef("\n%s\n", string(dump))

	return resp, nil
}

const (
	// RGWAdminOpsUserSecretName is the secret name of the admin ops user
	//nolint:gosec // since this is not leaking any hardcoded credentials, it's just the secret name
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
	nsName := fmt.Sprintf("%s/%s", store.Namespace, store.Name)

	objContext := NewContext(context, clusterInfo, store.Name)
	objContext.UID = string(store.UID)

	if err := UpdateEndpointForAdminOps(objContext, store); err != nil {
		return nil, err
	}

	realmName, zoneGroupName, zoneName, err := getMultisiteForObjectStore(clusterInfo.Context, context, &store.Spec, store.Namespace, store.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get realm/zone group/zone for object store %q", nsName)
	}

	objContext.Realm = realmName
	objContext.ZoneGroup = zoneGroupName
	objContext.Zone = zoneName
	return objContext, nil
}

// GetAdminOpsEndpoint returns an endpoint that can be used to perform RGW admin ops
func GetAdminOpsEndpoint(s *cephv1.CephObjectStore) (string, error) {
	nsName := fmt.Sprintf("%s/%s", s.Namespace, s.Name)

	// advertise endpoint should be most likely to have a valid cert, so use it for admin ops
	endpoint, err := s.GetAdvertiseEndpointUrl()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get advertise endpoint for object store %q", nsName)
	}
	return endpoint, nil
}

// UpdateEndpointForAdminOps updates the object.Context endpoint with the latest admin ops endpoint
// for the CephObjectStore.
func UpdateEndpointForAdminOps(objContext *Context, store *cephv1.CephObjectStore) error {
	endpoint, err := GetAdminOpsEndpoint(store)
	if err != nil {
		return err
	}
	objContext.Endpoint = endpoint
	return nil
}

func NewMultisiteAdminOpsContext(
	objContext *Context,
	spec *cephv1.ObjectStoreSpec,
) (*AdminOpsContext, error) {
	accessKey, secretKey, err := GetAdminOPSUserCredentials(objContext, spec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or retrieve rgw admin ops user")
	}

	httpClient, tlsCert, err := genObjectStoreHTTPClientFunc(objContext, spec)
	if err != nil {
		return nil, err
	}

	// If DEBUG level is set we will mutate the HTTP client for printing request and response
	var client *admin.API
	if logger.LevelAt(capnslog.DEBUG) {
		client, err = admin.New(objContext.Endpoint, accessKey, secretKey, NewDebugHTTPClient(httpClient, logger))
		if err != nil {
			return nil, errors.Wrap(err, "failed to build admin ops API connection")
		}
	} else {
		client, err = admin.New(objContext.Endpoint, accessKey, secretKey, httpClient)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build admin ops API connection")
		}
	}

	return &AdminOpsContext{
		Context:               *objContext,
		TlsCert:               tlsCert,
		AdminOpsUserAccessKey: accessKey,
		AdminOpsUserSecretKey: secretKey,
		AdminOpsClient:        client,
	}, nil
}

func extractJSON(output string) (string, error) {
	// `radosgw-admin` sometimes leaves logs to stderr even if it succeeds.
	// So we should skip them if parsing output as json.
	// valid JSON can be an object (in braces) or an array (in brackets)
	arrayRegex := regexp.MustCompile(`(?ms)^\[.*\]$`)
	arrayMatch := arrayRegex.Find([]byte(output))
	objRegex := regexp.MustCompile(`(?ms)^{.*}$`)
	objMatch := objRegex.Find([]byte(output))
	if arrayMatch == nil && objMatch == nil {
		return "", errors.Errorf("didn't contain json. %s", output)
	}
	if arrayMatch == nil && objMatch != nil {
		return string(objMatch), nil
	}
	if arrayMatch != nil && objMatch == nil {
		return string(arrayMatch), nil
	}
	// if both object and array match, take the largest of the two matches
	if len(arrayMatch) > len(objMatch) {
		return string(arrayMatch), nil
	}
	return string(objMatch), nil
}

// RunAdminCommandNoMultisite is for running radosgw-admin commands in scenarios where an object-store has not been created yet or for commands on the realm or zonegroup (ex: radosgw-admin zonegroup get)
// This function times out after a fixed interval if no response is received.
// The function will return a Kubernetes error "NotFound" when exec fails when the pod does not exist
func RunAdminCommandNoMultisite(c *Context, expectJSON bool, args ...string) (string, error) {
	var output, stderr string
	var err error

	// If Multus is enabled we proxy all the command to the mgr sidecar
	if c.clusterInfo.NetworkSpec.IsMultus() {
		output, stderr, err = c.Context.RemoteExecutor.ExecCommandInContainerWithFullOutputWithTimeout(c.clusterInfo.Context, cephclient.ProxyAppLabel, cephclient.CommandProxyInitContainerName, c.clusterInfo.Namespace, append([]string{"radosgw-admin"}, args...)...)
	} else {
		command, args := cephclient.FinalizeCephCommandArgs("radosgw-admin", c.clusterInfo, args, c.Context.ConfigDir)
		output, err = c.Context.Executor.ExecuteCommandWithTimeout(exec.CephCommandsTimeout, command, args...)
	}

	if err != nil {
		return fmt.Sprintf("%s. %s", output, stderr), err
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
		logger.Debugf("retrying 'radosgw-admin' command with OMAP backend to work around FIFO file I/O issue. %v", result)

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
		logger.Errorf("failed to determine return code of 'radosgw-admin' command. assuming this could be a FIFO file I/O issue. %#v", extractErr)
		return true
	}
	// exit code 5 (EIO) is returned when there is a FIFO file I/O issue
	return exitCode == 5
}

func isInvalidFlagError(err error) bool {
	exitCode, extractErr := exec.ExtractExitCode(err)
	if extractErr != nil {
		logger.Errorf("failed to determine return code of 'radosgw-admin' command. assuming this could be an invalid flag error. %#v", extractErr)
	}
	// exit code 22 (EINVAL) is returned when there is an invalid flag
	// it's also returned from some other failures, but this should be rare for Rook
	return exitCode == 22
}

// CommitConfigChanges commits changes to RGW configs for realm/zonegroup/zone changes idempotently.
// Under the hood, this updates the RGW config period and commits the change if changes are detected.
func CommitConfigChanges(c *Context) error {
	currentPeriod, err := runAdminCommand(c, true, "period", "get")
	if err != nil {
		return errorOrIsNotFound(err, "failed to get the current RGW configuration period to see if it needs changed")
	}

	// this stages the current config changes and returns what the new period config will look like
	// without committing the changes
	stagedPeriod, err := runAdminCommand(c, true, "period", "update")
	if err != nil {
		return errorOrIsNotFound(err, "failed to stage the current RGW configuration period")
	}

	shouldCommit, err := periodWillChange(currentPeriod, stagedPeriod)
	if err != nil {
		return errors.Wrap(err, "failed to determine if the staged RGW configuration period is different from current")
	}

	// DO NOT MODIFY nsName here. It is part of the integration test checks noted below.
	nsName := fmt.Sprintf("%s/%s", c.clusterInfo.Namespace, c.Name)
	if !shouldCommit {
		// DO NOT MODIFY THE MESSAGE BELOW. It is checked in integration tests.
		logger.Infof("there are no changes to commit for RGW configuration period for CephObjectStore %q", nsName)
		return nil
	}
	// DO NOT MODIFY THE MESSAGE BELOW. It is checked in integration tests.
	logger.Infof("committing changes to RGW configuration period for CephObjectStore %q", nsName)
	// don't expect json output since we don't intend to use the output from the command
	_, err = runAdminCommand(c, false, "period", "update", "--commit")
	if err != nil {
		return errorOrIsNotFound(err, "failed to commit RGW configuration period changes")
	}

	return nil
}

// return true if the configuration period will change if the staged period is committed
func periodWillChange(current, staged string) (bool, error) {
	// Rook wants to check if there are any differences in the current period versus the period that
	// is staged to be applied/committed. If there are differences, then Rook should "commit" the
	// staged period changes to instruct RGWs to update their runtime configuration.
	//
	// For many RGW interactions, Rook often creates a typed struct to unmarshal RGW JSON output
	// into. In those cases Rook is able to opt in to only a small subset of specific fields it
	// needs. This keeps the coupling between Rook and RGW JSON output as loose as possible while
	// still being specific enough for Rook to operate.
	//
	// For this implementation, we could use a strongly-typed struct here to unmarshal into, and we
	// could use DisallowUnknownFields() to cause an error if the RGW JSON output changes to flag
	// when the existing implementation might be invalidated. This relies on an extremely tight
	// coupling between Rook and the JSON output from RGW. The failure mode of this implementation
	// is to return an error from the reconcile when there are unmarshalling errors, which results
	// in CephObjectStores that could not be updated if a version of Ceph changes the RGW output.
	//
	// In the chosen implementation, we unmarshal into "dumb" data structures that create a loose
	// coupling. With these, we must ignore the fields that we have observed to change between the
	// current and staged periods when we should *not* commit an un-changed period. The failure mode
	// of this implementation is that if the RGW output changes its structure, Rook may detect
	// differences where there are none. This would result in Rook committing the period more often
	// than necessary. Committing the period results in a short period of downtime while RGWs reload
	// their configuration, but we opt for this inconvenience in lieu of blocking reconciliation.
	//
	// For any implementation, if the RGW changes the behavior of its output but not the structure,
	// Rook could commit unnecessary period changes or fail to commit necessary period changes
	// depending on how the RGW output has changed. Rook cannot detect this class of failures, and
	// the behavior cannot be specifically known.
	var currentJSON map[string]interface{}
	var stagedJSON map[string]interface{}
	var err error

	err = json.Unmarshal([]byte(current), &currentJSON)
	if err != nil {
		return true, errors.Wrap(err, "failed to unmarshal current RGW configuration period")
	}
	err = json.Unmarshal([]byte(staged), &stagedJSON)
	if err != nil {
		return true, errors.Wrap(err, "failed to unmarshal staged RGW configuration period")
	}

	// There are some values in the periods that we don't care to diff because they are always
	// different in the staged period, even when no updates are needed. Sometimes, the values are
	// reported as different in the staging output but aren't actually changed upon commit.
	ignorePaths := cmp.FilterPath(func(path cmp.Path) bool {
		j := toJsonPath(path)
		switch j {
		case ".id": // {"epoch"}
			// "id" always changes in staged period, but not always when committed
			return true
		case ".predecessor_uuid":
			// "predecessor_uuid" always changes in staged period, but not always when committed
			return true
		case ".realm_epoch":
			// "realm_epoch" always increments in staged period, but not always when committed
			return true
		case ".epoch":
			// Strangely, "epoch" is not incremented in the staged period even though it is always
			// incremented upon an actual commit. It could be argued that this behavior is a bug.
			// Ignore this value to handle the possibility that the behavior changes in the future.
			return true
		default:
			return false
		}
	}, cmp.Ignore())

	diff := cmp.Diff(currentJSON, stagedJSON, ignorePaths)
	diff = strings.TrimSpace(diff)
	logger.Debugf("RGW config period diff:\n%s", diff)

	return (diff != ""), nil
}

// convert a cmp.Path to a path in json format like might be used with jq to query the item
// e.g., {"a": {"b": "c"}} returns .a.b
func toJsonPath(path cmp.Path) string {
	out := ""
	for _, step := range path {
		mi, ok := step.(cmp.MapIndex)
		if !ok {
			// because we use a dumb map[string]interface{}, we only need to process map indexes,
			// but the path has other node types because of how the json gets processed
			continue
		}
		out = out + "." + mi.Key().String()
	}
	return out
}

func GetAdminOPSUserCredentials(objContext *Context, spec *cephv1.ObjectStoreSpec) (string, string, error) {
	ns := objContext.clusterInfo.Namespace

	if spec.IsExternal() {
		// Fetch the secret for admin ops user
		s := &v1.Secret{}
		err := objContext.Context.Client.Get(objContext.clusterInfo.Context, types.NamespacedName{Name: RGWAdminOpsUserSecretName, Namespace: ns}, s)
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
	logger.Debugf("creating s3 user object %q for object store %q", userConfig.UserID, objContext.Name)

	forceUserCreation := false
	// If the cluster where we are running the rgw user create for the admin ops user is configured
	// as a secondary cluster the gateway will error out with:
	// 		Please run the command on master zone. Performing this operation on non-master zone leads to
	// 		inconsistent metadata between zones
	// It is safe to force it since the creation will return that the user already exists since it
	// has been created by the primary cluster. In this case, we simply read the user details.
	if spec.IsMultisite() {
		forceUserCreation = true
	}

	user, rgwerr, err := CreateUser(objContext, userConfig, forceUserCreation)
	if err != nil {
		if rgwerr == ErrorCodeFileExists {
			user, _, err = GetUser(objContext, userConfig.UserID)
			if err != nil {
				return "", "", errors.Wrapf(err, "failed to get details from ceph object user %q for object store %q", userConfig.UserID, objContext.Name)
			}
		} else {
			return "", "", errors.Wrapf(err, "failed to create object user %q. error code %d for object store %q", userConfig.UserID, rgwerr, objContext.Name)
		}
	}
	return *user.AccessKey, *user.SecretKey, nil
}
