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
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	validation "k8s.io/apimachinery/pkg/util/validation"
)

const (
	rootPool = ".rgw.root"

	// AppName is the name Rook uses for the object store's application
	AppName               = "rook-ceph-rgw"
	bucketProvisionerName = "ceph.rook.io/bucket"
	AccessKeyName         = "access-key"
	SecretKeyName         = "secret-key"
	svcDNSSuffix          = "svc"
	rgwRadosPoolPgNum     = "8"
	rgwApplication        = "rgw"
)

var (
	metadataPools = []string{
		// .rgw.root (rootPool) is appended to this slice where needed
		"rgw.control",
		"rgw.meta",
		"rgw.log",
		"rgw.buckets.index",
		"rgw.buckets.non-ec",
		"rgw.otp",
	}
	dataPoolName = "rgw.buckets.data"

	// An user with system privileges for dashboard service
	DashboardUser = "dashboard-admin"
)

type idType struct {
	ID string `json:"id"`
}

type zoneGroupType struct {
	MasterZoneID string     `json:"master_zone"`
	IsMaster     bool       `json:"is_master"`
	Zones        []zoneType `json:"zones"`
	Endpoints    []string   `json:"endpoints"`
}

type zoneType struct {
	Name      string   `json:"name"`
	Endpoints []string `json:"endpoints"`
}

type realmType struct {
	Realms []string `json:"realms"`
}

// allow commitConfigChanges to be overridden for unit testing
var commitConfigChanges = CommitConfigChanges

func deleteRealmAndPools(objContext *Context, spec cephv1.ObjectStoreSpec) error {
	if spec.IsMultisite() {
		// since pools for object store are created by the zone, the object store only needs to be removed from the zone
		err := removeObjectStoreFromMultisite(objContext, spec)
		if err != nil {
			return err
		}

		return nil
	}

	return deleteSingleSiteRealmAndPools(objContext, spec)
}

func removeObjectStoreFromMultisite(objContext *Context, spec cephv1.ObjectStoreSpec) error {
	// get list of endpoints not including the endpoint of the object-store for the zone
	zoneEndpointsList, isEndpointAlreadyExists, err := getZoneEndpoints(objContext, objContext.Endpoint)
	if err != nil {
		return err
	}

	// The endpoint is present in zone, hence remove it
	if isEndpointAlreadyExists {
		realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
		zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
		zoneEndpoints := strings.Join(zoneEndpointsList, ",")
		endpointArg := fmt.Sprintf("--endpoints=%s", zoneEndpoints)

		zoneIsMaster, err := CheckZoneIsMaster(objContext)
		if err != nil {
			return errors.Wrapf(err, "failed to determine if zone %q is master", objContext.Zone)
		}

		zoneGroupIsMaster := false
		if zoneIsMaster {
			_, err = RunAdminCommandNoMultisite(objContext, false, "zonegroup", "modify", realmArg, zoneGroupArg, endpointArg)
			if err != nil {
				if kerrors.IsNotFound(err) {
					return err
				}
				return errors.Wrapf(err, "failed to remove object store %q endpoint from rgw zone group %q", objContext.Name, objContext.ZoneGroup)
			}
			logger.Debugf("endpoint %q was removed from zone group %q. the remaining endpoints in the zone group are %q", objContext.Endpoint, objContext.ZoneGroup, zoneEndpoints)

			// check if zone group is master only if zone is master for creating the system user
			zoneGroupIsMaster, err = checkZoneGroupIsMaster(objContext)
			if err != nil {
				return errors.Wrapf(err, "failed to find out whether zone group %q is the master zone group", objContext.ZoneGroup)
			}
		}

		_, err = runAdminCommand(objContext, false, "zone", "modify", endpointArg)
		if err != nil {
			return errors.Wrapf(err, "failed to remove object store %q endpoint from rgw zone %q", objContext.Name, spec.Zone.Name)
		}
		logger.Debugf("endpoint %q was removed from zone %q. the remaining endpoints in the zone are %q", objContext.Endpoint, objContext.Zone, zoneEndpoints)

		if zoneIsMaster && zoneGroupIsMaster && zoneEndpoints == "" {
			logger.Warningf("WARNING: No other zone in realm %q can commit to the period or pull the realm until you create another object-store in zone %q", objContext.Realm, objContext.Zone)
		}

		// this will notify other zones of changes if there are multi-zones
		if err := commitConfigChanges(objContext); err != nil {
			nsName := fmt.Sprintf("%s/%s", objContext.clusterInfo.Namespace, objContext.Name)
			return errors.Wrapf(err, "failed to commit config changes after removing CephObjectStore %q from multi-site", nsName)
		}
	}
	return nil
}

func deleteSingleSiteRealmAndPools(objContext *Context, spec cephv1.ObjectStoreSpec) error {
	stores, err := getObjectStores(objContext)
	if err != nil {
		return errors.Wrap(err, "failed to detect object stores during deletion")
	}
	if len(stores) == 0 {
		logger.Infof("did not find object store %q, nothing to delete", objContext.Name)
		return nil
	}
	logger.Infof("Found stores %v when deleting store %s", stores, objContext.Name)

	err = deleteRealm(objContext)
	if err != nil {
		return errors.Wrap(err, "failed to delete realm")
	}

	lastStore := false
	if len(stores) == 1 && stores[0] == objContext.Name {
		lastStore = true
	}

	if !spec.PreservePoolsOnDelete {
		if EmptyPool(spec.DataPool) && EmptyPool(spec.MetadataPool) {
			logger.Info("skipping removal of pools since not specified in the object store")
			return nil
		}
		err = DeletePools(objContext, lastStore, objContext.Name)
		if err != nil {
			return errors.Wrap(err, "failed to delete object store pools")
		}
	} else {
		logger.Infof("PreservePoolsOnDelete is set in object store %s. Pools not deleted", objContext.Name)
	}

	return nil
}

// This is used for quickly getting the name of the realm, zone group, and zone for an object-store to pass into a Context
func getMultisiteForObjectStore(ctx context.Context, clusterdContext *clusterd.Context, spec *cephv1.ObjectStoreSpec, namespace, name string) (string, string, string, error) {
	if spec.IsExternal() {
		// In https://github.com/rook/rook/issues/6342, it was determined that
		// a multisite context isn't needed for external mode CephObjectStores.
		// The context is only needed for managing an object store, which isn't
		// happening in external mode.
		return "", "default", "default", nil
	}
	if spec.IsMultisite() {
		zone, err := clusterdContext.RookClientset.CephV1().CephObjectZones(namespace).Get(ctx, spec.Zone.Name, metav1.GetOptions{})
		if err != nil {
			return "", "", "", errors.Wrapf(err, "failed to find zone for object-store %q", name)
		}

		zonegroup, err := clusterdContext.RookClientset.CephV1().CephObjectZoneGroups(namespace).Get(ctx, zone.Spec.ZoneGroup, metav1.GetOptions{})
		if err != nil {
			return "", "", "", errors.Wrapf(err, "failed to find zone group for object-store %q", name)
		}

		realm, err := clusterdContext.RookClientset.CephV1().CephObjectRealms(namespace).Get(ctx, zonegroup.Spec.Realm, metav1.GetOptions{})
		if err != nil {
			return "", "", "", errors.Wrapf(err, "failed to find realm for object-store %q", name)
		}

		return realm.Name, zonegroup.Name, zone.Name, nil
	}

	return name, name, name, nil
}

func CheckZoneIsMaster(objContext *Context) (bool, error) {
	logger.Debugf("checking if zone %v is the master zone", objContext.Zone)
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	zoneGroupJson, err := RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		// This handles the case where the pod we use to exec command (act as a proxy) is not found/ready yet
		// The caller can nicely handle the error and not overflow the op logs with misleading error messages
		if kerrors.IsNotFound(err) {
			return false, err
		}
		return false, errors.Wrap(err, "failed to get rgw zone group")
	}
	zoneGroupOutput, err := DecodeZoneGroupConfig(zoneGroupJson)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse zonegroup get json")
	}
	logger.Debugf("got master zone ID for zone group %v", objContext.ZoneGroup)

	zoneOutput, err := RunAdminCommandNoMultisite(objContext, true, "zone", "get", realmArg, zoneGroupArg, zoneArg)
	if err != nil {
		// This handles the case where the pod we use to exec command (act as a proxy) is not found/ready yet
		// The caller can nicely handle the error and not overflow the op logs with misleading error messages
		if kerrors.IsNotFound(err) {
			return false, err
		}
		return false, errors.Wrap(err, "failed to get rgw zone")
	}
	zoneID, err := decodeID(zoneOutput)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse zone id")
	}
	logger.Debugf("got zone ID for zone %v", objContext.Zone)

	if zoneID == zoneGroupOutput.MasterZoneID {
		logger.Debugf("zone is master")
		return true, nil
	}

	logger.Debugf("zone is not master")
	return false, nil
}

func checkZoneGroupIsMaster(objContext *Context) (bool, error) {
	logger.Debugf("checking if zone group %v is the master zone group", objContext.ZoneGroup)
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)

	zoneGroupOutput, err := RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		// This handles the case where the pod we use to exec command (act as a proxy) is not found/ready yet
		// The caller can nicely handle the error and not overflow the op logs with misleading error messages
		if kerrors.IsNotFound(err) {
			return false, err
		}
		return false, errors.Wrap(err, "failed to get rgw zone group")
	}

	zoneGroupJson, err := DecodeZoneGroupConfig(zoneGroupOutput)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse master zone id")
	}

	return zoneGroupJson.IsMaster, nil
}

func DecodeSecret(secret *v1.Secret, keyName string) (string, error) {
	realmKey, ok := secret.Data[keyName]

	if !ok {
		return "", fmt.Errorf("failed to find key %q in secret %q data. user likely created or modified the secret manually and should add the missing key back into the secret", keyName, secret.Name)
	}

	return string(realmKey), nil
}

func GetRealmKeySecret(ctx context.Context, clusterdContext *clusterd.Context, realmName types.NamespacedName) (*v1.Secret, error) {
	realmSecretName := realmName.Name + "-keys"
	realmSecret, err := clusterdContext.Clientset.CoreV1().Secrets(realmName.Namespace).Get(ctx, realmSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get CephObjectRealm %q keys secret", realmName.String())
	}
	logger.Debugf("found keys secret for CephObjectRealm %q", realmName.String())
	return realmSecret, nil
}

func GetRealmKeyArgsFromSecret(realmSecret *v1.Secret, realmName types.NamespacedName) (string, string, error) {
	accessKey, err := DecodeSecret(realmSecret, AccessKeyName)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to decode CephObjectRealm %q access key from secret %q", realmName.String(), realmSecret.Name)
	}
	secretKey, err := DecodeSecret(realmSecret, SecretKeyName)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to decode CephObjectRealm %q secret key from secret %q", realmName.String(), realmSecret.Name)
	}
	logger.Debugf("decoded keys for realm %q", realmName.String())

	accessKeyArg := fmt.Sprintf("--access-key=%s", accessKey)
	secretKeyArg := fmt.Sprintf("--secret-key=%s", secretKey)

	return accessKeyArg, secretKeyArg, nil
}

func GetRealmKeyArgs(ctx context.Context, clusterdContext *clusterd.Context, realmName, namespace string) (string, string, error) {
	realmNsName := types.NamespacedName{Namespace: namespace, Name: realmName}
	logger.Debugf("getting keys for realm %q", realmNsName.String())

	secret, err := GetRealmKeySecret(ctx, clusterdContext, realmNsName)
	if err != nil {
		return "", "", err
	}

	return GetRealmKeyArgsFromSecret(secret, realmNsName)
}

func getZoneEndpoints(objContext *Context, serviceEndpoint string) ([]string, bool, error) {
	logger.Debugf("getting current endpoints for zone %v", objContext.Zone)
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	isEndpointAlreadyExists := false

	zoneGroupOutput, err := RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		// This handles the case where the pod we use to exec command (act as a proxy) is not found/ready yet
		// The caller can nicely handle the error and not overflow the op logs with misleading error messages
		return []string{}, isEndpointAlreadyExists, errorOrIsNotFound(err, "failed to get rgw zone group %q", objContext.Name)
	}
	zoneGroupJson, err := DecodeZoneGroupConfig(zoneGroupOutput)
	if err != nil {
		return []string{}, isEndpointAlreadyExists, errors.Wrap(err, "failed to parse zones list")
	}

	zoneEndpointsList := []string{}
	for _, zone := range zoneGroupJson.Zones {
		if zone.Name == objContext.Zone {
			for _, endpoint := range zone.Endpoints {
				// in case object-store operator code is rereconciled, zone modify could get run again with serviceEndpoint added again
				if endpoint != serviceEndpoint {
					zoneEndpointsList = append(zoneEndpointsList, endpoint)
				} else {
					isEndpointAlreadyExists = true
				}
			}
			break
		}
	}

	return zoneEndpointsList, isEndpointAlreadyExists, nil
}

func createMultisiteConfigurations(objContext *Context, store *cephv1.CephObjectStore, configType, configTypeArg string, args ...string) error {
	args = append([]string{configType}, args...)
	args = append(args, configTypeArg)
	// get the multisite config before creating
	configTypeArgs := []string{configType, "get", configTypeArg}
	if configType == "zonegroup" {
		configTypeArgs = append(configTypeArgs, fmt.Sprintf("--rgw-realm=%s", objContext.Realm))
	}
	output, getConfigErr := RunAdminCommandNoMultisite(objContext, true, configTypeArgs...)
	if getConfigErr == nil {
		return nil
	}

	if kerrors.IsNotFound(getConfigErr) {
		// the pod used to exec command (act as a proxy) is not found/ready yet
		// caller can nicely handle error and not overflow logs with misleading error messages
		return getConfigErr
	}

	code, err := exec.ExtractExitCode(getConfigErr)
	if err != nil {
		return errorOrIsNotFound(getConfigErr, "'radosgw-admin %q get' failed with code %q, for reason %q, error: (%v)", configType, strconv.Itoa(code), output, string(kerrors.ReasonForError(err)))
	}
	// ENOENT means "No such file or directory"
	if code != int(syscall.ENOENT) {
		code := strconv.Itoa(code)
		return errors.Wrapf(getConfigErr, "'radosgw-admin %q get' failed with code %q, for reason %q", configType, code, output)
	}

	// create the object if it doesn't exist yet
	if store.Spec.DefaultRealm && objContext.clusterInfo.CephVersion.IsAtLeast(cephver.Squid) {
		logger.Infof("marking object store %q as default realm", store.Namespace+"/"+store.Name)
		args = append(args, "--default")
	}
	output, err = RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errorOrIsNotFound(err, "failed to create ceph %q %q, for reason %q", configType, configTypeArg, output)
	}
	logger.Debugf("created %q %q", configType, configTypeArg)

	return nil
}

func createNonMultisiteStore(objContext *Context, endpointArg string, store *cephv1.CephObjectStore) error {
	logger.Debugf("creating realm, zone group, zone for object-store %v", objContext.Name)

	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	err := createMultisiteConfigurations(objContext, store, "realm", realmArg, "create")
	if err != nil {
		return err
	}

	err = createMultisiteConfigurations(objContext, store, "zonegroup", zoneGroupArg, "create", "--master", realmArg, endpointArg)
	if err != nil {
		return err
	}

	err = createMultisiteConfigurations(objContext, store, "zone", zoneArg, "create", "--master", endpointArg, realmArg, zoneGroupArg)
	if err != nil {
		return err
	}

	logger.Infof("Object store %q: realm=%s, zonegroup=%s, zone=%s", objContext.Name, objContext.Realm, objContext.ZoneGroup, objContext.Zone)

	// Configure the zone for RADOS namespaces
	err = ConfigureSharedPoolsForZone(objContext, store.Spec.SharedPools)
	if err != nil {
		return errors.Wrapf(err, "failed to configure rados namespaces for zone")
	}

	if err := commitConfigChanges(objContext); err != nil {
		nsName := fmt.Sprintf("%s/%s", objContext.clusterInfo.Namespace, objContext.Name)
		return errors.Wrapf(err, "failed to commit config changes after creating multisite config for CephObjectStore %q", nsName)
	}

	return nil
}

func JoinMultisite(objContext *Context, endpointArg, zoneEndpoints, namespace string) error {
	logger.Debugf("joining zone %v", objContext.Zone)
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	zoneIsMaster, err := CheckZoneIsMaster(objContext)
	if err != nil {
		return err
	}
	zoneGroupIsMaster := false

	if zoneIsMaster {
		// endpoints that are part of a master zone are supposed to be the endpoints for a zone group
		_, err := RunAdminCommandNoMultisite(objContext, false, "zonegroup", "modify", realmArg, zoneGroupArg, endpointArg)
		if err != nil {
			return errorOrIsNotFound(err, "failed to add object store %q in rgw zone group %q", objContext.Name, objContext.ZoneGroup)
		}
		logger.Debugf("endpoints for zonegroup %q are now %q", objContext.ZoneGroup, zoneEndpoints)

		// check if zone group is master only if zone is master for creating the system user
		zoneGroupIsMaster, err = checkZoneGroupIsMaster(objContext)
		if err != nil {
			return errors.Wrapf(err, "failed to find out whether zone group %q in is the master zone group", objContext.ZoneGroup)
		}
	}
	_, err = RunAdminCommandNoMultisite(objContext, false, "zone", "modify", realmArg, zoneGroupArg, zoneArg, endpointArg)
	if err != nil {
		return errorOrIsNotFound(err, "failed to add object store %q in rgw zone %q", objContext.Name, objContext.Zone)
	}
	logger.Debugf("endpoints for zone %q are now %q", objContext.Zone, zoneEndpoints)

	if err := commitConfigChanges(objContext); err != nil {
		nsName := fmt.Sprintf("%s/%s", objContext.clusterInfo.Namespace, objContext.Name)
		return errors.Wrapf(err, "failed to commit config changes for CephObjectStore %q when joining multisite ", nsName)
	}

	logger.Infof("added object store %q to realm %q, zonegroup %q, zone %q", objContext.Name, objContext.Realm, objContext.ZoneGroup, objContext.Zone)

	// create system user for realm for master zone in master zonegroup for multisite scenario
	if zoneIsMaster && zoneGroupIsMaster {
		err = createSystemUser(objContext, namespace)
		if err != nil {
			return err
		}
	}

	return nil
}

func createSystemUser(objContext *Context, namespace string) error {
	uid := objContext.Realm + "-system-user"
	uidArg := fmt.Sprintf("--uid=%s", uid)
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	output, err := RunAdminCommandNoMultisite(objContext, false, "user", "info", uidArg, realmArg, zoneGroupArg, zoneArg)
	if err == nil {
		logger.Debugf("realm system user %q has already been created", uid)
		return nil
	}

	if code, ok := exec.ExitStatus(err); ok && code == int(syscall.EINVAL) {
		logger.Debugf("realm system user %q not found, running `radosgw-admin user create`", uid)
		accessKeyArg, secretKeyArg, err := GetRealmKeyArgs(objContext.clusterInfo.Context, objContext.Context, objContext.Realm, namespace)
		if err != nil {
			return errors.Wrap(err, "failed to get keys for realm")
		}
		logger.Debugf("found keys to create realm system user %v", uid)
		systemArg := "--system"
		displayNameArg := fmt.Sprintf("--display-name=%s.user", objContext.Realm)
		output, err = RunAdminCommandNoMultisite(objContext, false, "user", "create", realmArg, zoneGroupArg, zoneArg, uidArg, displayNameArg, accessKeyArg, secretKeyArg, systemArg)
		if err != nil {
			return errorOrIsNotFound(err, "failed to create realm system user %q for reason: %q", uid, output)
		}
		logger.Debugf("created realm system user %v", uid)
	} else {
		return errorOrIsNotFound(err, "radosgw-admin user info for system user failed with code %d and output %q", strconv.Itoa(code), output)
	}

	return nil
}

func configureObjectStore(objContext *Context, store *cephv1.CephObjectStore, zone *cephv1.CephObjectZone) error {
	logger.Debugf("setting multisite configuration for object-store %v", store.Name)

	if store.Spec.IsMultisite() {
		if zone != nil && len(zone.Spec.CustomEndpoints) == 0 {
			// get list of endpoints not including the endpoint of the object-store for the zone
			zoneEndpointsList, isEndpointAlreadyExists, err := getZoneEndpoints(objContext, objContext.Endpoint)
			if err != nil {
				return err
			}

			// There is no need to update the Zone endpoints when:
			//  - the zone does not have the endpoint and the synchronization is disabled on the objectstore
			//  - the zone already have the endpoint and the synchronization is enabled
			if isEndpointAlreadyExists == store.Spec.Gateway.DisableMultisiteSyncTraffic {
				if !isEndpointAlreadyExists {
					zoneEndpointsList = append(zoneEndpointsList, objContext.Endpoint)
				}

				zoneEndpoints := strings.Join(zoneEndpointsList, ",")
				logger.Debugf("Endpoints for zone %q are: %q", objContext.Zone, zoneEndpoints)
				endpointArg := fmt.Sprintf("--endpoints=%s", zoneEndpoints)

				err = JoinMultisite(objContext, endpointArg, zoneEndpoints, store.Namespace)
				if err != nil {
					return errors.Wrapf(err, "failed join ceph multisite in zone %q", objContext.Zone)
				}
			}
		}
	} else {
		var endpointArg string
		if !store.Spec.Gateway.DisableMultisiteSyncTraffic {
			endpointArg = fmt.Sprintf("--endpoints=%s", objContext.Endpoint)
		}
		err := createNonMultisiteStore(objContext, endpointArg, store)
		if err != nil {
			return errorOrIsNotFound(err, "failed create ceph multisite for object-store %q", objContext.Name)
		}
	}

	logger.Infof("configuration for object-store %v is complete", store.Name)
	return nil
}

func deleteRealm(context *Context) error {
	//  <name>
	realmArg := fmt.Sprintf("--rgw-realm=%s", context.Name)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", context.Name)
	_, err := RunAdminCommandNoMultisite(context, false, "realm", "delete", realmArg)
	if err != nil {
		logger.Warningf("failed to delete rgw realm %q. %v", context.Name, err)
	}

	_, err = RunAdminCommandNoMultisite(context, false, "zonegroup", "delete", realmArg, zoneGroupArg)
	if err != nil {
		logger.Warningf("failed to delete rgw zonegroup %q. %v", context.Name, err)
	}

	_, err = runAdminCommand(context, false, "zone", "delete")
	if err != nil {
		logger.Warningf("failed to delete rgw zone %q. %v", context.Name, err)
	}

	return nil
}

func decodeID(data string) (string, error) {
	var id idType
	err := json.Unmarshal([]byte(data), &id)
	if err != nil {
		return "", errors.Wrap(err, "failed to unmarshal json")
	}

	return id.ID, err
}

func DecodeZoneGroupConfig(data string) (zoneGroupType, error) {
	var config zoneGroupType
	err := json.Unmarshal([]byte(data), &config)
	if err != nil {
		return config, errors.Wrap(err, "failed to unmarshal json")
	}

	return config, err
}

func getObjectStores(context *Context) ([]string, error) {
	output, err := RunAdminCommandNoMultisite(context, true, "realm", "list")
	if err != nil {
		// This handles the case where the pod we use to exec command (act as a proxy) is not found/ready yet
		// The caller can nicely handle the error and not overflow the op logs with misleading error messages
		if kerrors.IsNotFound(err) {
			return []string{}, err
		}
		// exit status 2 indicates the object store does not exist, so return nothing
		if strings.Index(err.Error(), "exit status 2") == 0 {
			return []string{}, nil
		}
		return nil, err
	}

	var r realmType
	err = json.Unmarshal([]byte(output), &r)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to unmarshal realms")
	}

	return r.Realms, nil
}

func DeletePools(ctx *Context, lastStore bool, poolPrefix string) error {
	pools := append(metadataPools, dataPoolName)
	if lastStore {
		pools = append(pools, rootPool)
	}

	if configurePoolsConcurrently() {
		waitGroup, _ := errgroup.WithContext(ctx.clusterInfo.Context)
		for _, pool := range pools {
			name := poolName(poolPrefix, pool)
			waitGroup.Go(func() error {
				if err := cephclient.DeletePool(ctx.Context, ctx.clusterInfo, name); err != nil {
					return errors.Wrapf(err, "failed to delete pool %q. ", name)
				}
				return nil
			},
			)
		}

		// Wait for all the pools to be deleted
		if err := waitGroup.Wait(); err != nil {
			logger.Warning(err)
		}

	} else {
		for _, pool := range pools {
			name := poolName(poolPrefix, pool)
			if err := cephclient.DeletePool(ctx.Context, ctx.clusterInfo, name); err != nil {
				logger.Warningf("failed to delete pool %q. %v", name, err)
			}
		}
	}

	// Delete erasure code profile if any
	erasureCodes, err := cephclient.ListErasureCodeProfiles(ctx.Context, ctx.clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to list erasure code profiles for cluster %s", ctx.clusterInfo.Namespace)
	}
	// cleans up the EC profile for the data pool only. Metadata pools don't support EC (only replication is supported).
	ecProfileName := cephclient.GetErasureCodeProfileForPool(ctx.Name)
	for i := range erasureCodes {
		if erasureCodes[i] == ecProfileName {
			if err := cephclient.DeleteErasureCodeProfile(ctx.Context, ctx.clusterInfo, ecProfileName); err != nil {
				return errors.Wrapf(err, "failed to delete erasure code profile %s for object store %s", ecProfileName, ctx.Name)
			}
			break
		}
	}

	return nil
}

func allObjectPools(storeName string) []string {
	baseObjPools := append(metadataPools, dataPoolName, rootPool)

	poolsForThisStore := make([]string, 0, len(baseObjPools))
	for _, p := range baseObjPools {
		poolsForThisStore = append(poolsForThisStore, poolName(storeName, p))
	}
	return poolsForThisStore
}

// Detect if there are pools that do not exist for this object store
func missingPools(context *Context) ([]string, error) {
	// list pools instead of querying each pool individually. querying each individually makes it
	// hard to determine if an error is because the pool does not exist or because of a connection
	// issue with ceph mons (or some other underlying issue). if listing pools fails, we can be sure
	// it is a connection issue and return an error.
	existingPoolSummaries, err := cephclient.ListPoolSummaries(context.Context, context.clusterInfo)
	if err != nil {
		return []string{}, errors.Wrapf(err, "failed to determine if pools are missing. failed to list pools")
	}
	existingPools := sets.New[string]()
	for _, summary := range existingPoolSummaries {
		existingPools.Insert(summary.Name)
	}

	missingPools := []string{}
	for _, objPool := range allObjectPools(context.Zone) {
		if !existingPools.Has(objPool) {
			missingPools = append(missingPools, objPool)
		}
	}

	return missingPools, nil
}

func CreateObjectStorePools(context *Context, cluster *cephv1.ClusterSpec, metadataPool, dataPool cephv1.PoolSpec) error {
	if EmptyPool(dataPool) || EmptyPool(metadataPool) {
		logger.Info("no pools specified for the CR, checking for their existence...")
		missingPools, err := missingPools(context)
		if err != nil {
			return err
		}
		if len(missingPools) > 0 {
			return fmt.Errorf("CR store pools are missing: %v", missingPools)
		}

		// pools exist, nothing to do
		return nil
	}

	if err := createSimilarPools(context, append(metadataPools, rootPool), cluster, metadataPool, rgwRadosPoolPgNum); err != nil {
		return errors.Wrap(err, "failed to create metadata pools")
	}

	if err := createSimilarPools(context, []string{dataPoolName}, cluster, dataPool, cephclient.DefaultPGCount); err != nil {
		return errors.Wrap(err, "failed to create data pool")
	}

	return nil
}

func ConfigureSharedPoolsForZone(objContext *Context, sharedPools cephv1.ObjectSharedPoolsSpec) error {
	if sharedPools.DataPoolName == "" && sharedPools.MetadataPoolName == "" && len(sharedPools.PoolPlacements) == 0 {
		logger.Debugf("no shared pools to configure for store %q", objContext.nsName())
		return nil
	}

	logger.Infof("configuring shared pools for object store %q", objContext.nsName())
	if err := sharedPoolsExist(objContext, sharedPools); err != nil {
		return errors.Wrapf(err, "object store cannot be configured until shared pools exist")
	}

	zoneConfig, err := getZoneJSON(objContext)
	if err != nil {
		return err
	}
	zoneUpdated, err := adjustZoneDefaultPools(zoneConfig, sharedPools)
	if err != nil {
		return err
	}
	zoneUpdated, err = adjustZonePlacementPools(zoneUpdated, sharedPools)
	if err != nil {
		return err
	}
	hasZoneChanged := !reflect.DeepEqual(zoneConfig, zoneUpdated)

	zoneGroupConfig, err := getZoneGroupJSON(objContext)
	if err != nil {
		return err
	}
	defaultPlacement := getDefaultPlacementName(sharedPools)
	zoneGroupUpdated, err := adjustZoneGroupPlacementTargets(zoneGroupConfig, zoneUpdated, defaultPlacement)
	if err != nil {
		return err
	}
	hasZoneGroupChanged := !reflect.DeepEqual(zoneGroupConfig, zoneGroupUpdated)

	// persist configuration updates:
	if hasZoneChanged {
		logger.Infof("zone config changed: performing zone config updates for %s", objContext.Zone)
		updatedZoneResult, err := updateZoneJSON(objContext, zoneUpdated)
		if err != nil {
			return fmt.Errorf("unable to persist zone config update for %s: %w", objContext.Zone, err)
		}
		if err = zoneUpdateWorkaround(objContext, zoneUpdated, updatedZoneResult); err != nil {
			return fmt.Errorf("failed to apply zone set workaround: %w", err)
		}
	}
	if hasZoneGroupChanged {
		logger.Infof("zonegroup config changed: performing zonegroup config updates for %s", objContext.ZoneGroup)
		_, err = updateZoneGroupJSON(objContext, zoneGroupUpdated)
		if err != nil {
			return fmt.Errorf("unable to persist zonegroup config update for %s: %w", objContext.ZoneGroup, err)
		}
	}

	return nil
}

func sharedPoolsExist(objContext *Context, sharedPools cephv1.ObjectSharedPoolsSpec) error {
	existingPools, err := cephclient.ListPoolSummaries(objContext.Context, objContext.clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to list pools")
	}
	existing := make(map[string]struct{}, len(existingPools))
	for _, pool := range existingPools {
		existing[pool.Name] = struct{}{}
	}
	// sharedPools.MetadataPoolName, DataPoolName, and sharedPools.PoolPlacements.DataNonECPoolName are optional.
	// ignore optional pools with empty name:
	existing[""] = struct{}{}

	if _, ok := existing[sharedPools.MetadataPoolName]; !ok {
		return fmt.Errorf("sharedPool do not exist: %s", sharedPools.MetadataPoolName)
	}
	if _, ok := existing[sharedPools.DataPoolName]; !ok {
		return fmt.Errorf("sharedPool do not exist: %s", sharedPools.DataPoolName)
	}

	for _, pp := range sharedPools.PoolPlacements {
		if _, ok := existing[pp.MetadataPoolName]; !ok {
			return fmt.Errorf("sharedPool does not exist: pool %s for placement %s", pp.MetadataPoolName, pp.Name)
		}
		if _, ok := existing[pp.DataPoolName]; !ok {
			return fmt.Errorf("sharedPool do not exist: pool %s for placement %s", pp.DataPoolName, pp.Name)
		}
		if _, ok := existing[pp.DataNonECPoolName]; !ok {
			return fmt.Errorf("sharedPool do not exist: pool %s for placement %s", pp.DataNonECPoolName, pp.Name)
		}
		for _, sc := range pp.StorageClasses {
			if _, ok := existing[sc.DataPoolName]; !ok {
				return fmt.Errorf("sharedPool do not exist: pool %s for StorageClass %s", sc.DataPoolName, sc.Name)
			}
		}
	}

	return nil
}

func adjustZoneDefaultPools(zone map[string]interface{}, spec cephv1.ObjectSharedPoolsSpec) (map[string]interface{}, error) {
	name, err := getObjProperty[string](zone, "name")
	if err != nil {
		return nil, fmt.Errorf("unable to get zone name: %w", err)
	}

	zone, err = deepCopyJson(zone)
	if err != nil {
		return nil, fmt.Errorf("unable to deep copy zone %s: %w", name, err)
	}

	defaultMetaPool := getDefaultMetadataPool(spec)
	if defaultMetaPool == "" {
		// default pool is not presented in shared pool spec
		return zone, nil
	}
	// add zone namespace to metadata pool to safely share accorss rgw instances or zones.
	// in non-multisite case zone name equals to rgw instance name
	defaultMetaPool = defaultMetaPool + ":" + name
	zonePoolNSSuffix := map[string]string{
		"domain_root":     ".meta.root",
		"control_pool":    ".control",
		"gc_pool":         ".log.gc",
		"lc_pool":         ".log.lc",
		"log_pool":        ".log",
		"intent_log_pool": ".log.intent",
		"usage_log_pool":  ".log.usage",
		"roles_pool":      ".meta.roles",
		"reshard_pool":    ".log.reshard",
		"user_keys_pool":  ".meta.users.keys",
		"user_email_pool": ".meta.users.email",
		"user_swift_pool": ".meta.users.swift",
		"user_uid_pool":   ".meta.users.uid",
		"otp_pool":        ".otp",
		"notif_pool":      ".log.notif",
		"topics_pool":     ".meta.topics",  // introduced in Ceph v19
		"account_pool":    ".meta.account", // introduced in Ceph v19
		"group_pool":      ".meta.group",   // introduced in Ceph v19
	}
	for pool, nsSuffix := range zonePoolNSSuffix {
		// replace rgw internal index pools with namespaced metadata pool
		namespacedPool := defaultMetaPool + nsSuffix
		prev, err := updateObjProperty(zone, namespacedPool, pool)
		if err != nil {
			logger.Infof("unable to apply rados namespace to shared pool: %v", err)
		}
		if namespacedPool != prev {
			logger.Debugf("update shared pool %s for zone %s: %s -> %s", pool, name, prev, namespacedPool)
		}
	}

	// check for unknown pool properties in zone json
	for field, val := range zone {
		if _, ok := val.(string); !ok {
			// not a string property
			continue
		}
		if !strings.HasSuffix(field, "_pool") {
			// not a pool property
			continue
		}
		if _, ok := zonePoolNSSuffix[field]; !ok {
			logger.Warningf("zone config %q contains unknown pool %q", name, field)
		}
	}

	return zone, nil
}

// There was a radosgw-admin bug that was preventing the RADOS namespace from being applied
// for the data pool. The fix is included in Reef v18.2.3 or newer, and v19.2.0.
// The workaround is to run a "radosgw-admin zone placement modify" command to apply
// the desired data pool config.
// After Reef (v18) support is removed, this method will be dead code.
func zoneUpdateWorkaround(objContext *Context, expectedZone, gotZone map[string]interface{}) error {
	// Update the necessary fields for RAODS namespaces
	// If the radosgw-admin fix is in the release, the data pool is already applied and we skip the workaround.
	expected, err := getObjProperty[[]interface{}](expectedZone, "placement_pools")
	if err != nil {
		return err
	}
	got, err := getObjProperty[[]interface{}](gotZone, "placement_pools")
	if err != nil {
		return err
	}
	if len(expected) != len(got) {
		// should not happen
		return fmt.Errorf("placements were not applied to zone config: expected %+v, got %+v", expected, got)
	}

	// update pool placements one-by-one if needed
	for i, expPl := range expected {
		expPoolObj, ok := expPl.(map[string]interface{})
		if !ok {
			return fmt.Errorf("unable to cast pool placement to object: %+v", expPl)
		}
		expPoolName, err := getObjProperty[string](expPoolObj, "key")
		if err != nil {
			return fmt.Errorf("unable to get pool placement name: %w", err)
		}

		gotPoolObj, ok := got[i].(map[string]interface{})
		if !ok {
			return fmt.Errorf("unable to cast pool placement to object: %+v", got[i])
		}
		gotPoolName, err := getObjProperty[string](gotPoolObj, "key")
		if err != nil {
			return fmt.Errorf("unable to get pool placement name: %w", err)
		}

		if expPoolName != gotPoolName {
			// should not happen
			return fmt.Errorf("placements were not applied to zone config: expected %+v, got %+v", expected, got)
		}
		err = zoneUpdatePlacementWorkaround(objContext, gotPoolName, expPoolObj, gotPoolObj)
		if err != nil {
			return fmt.Errorf("unable to do zone update workaround for placement %q: %w", gotPoolName, err)
		}
	}
	return nil
}

func zoneUpdatePlacementWorkaround(objContext *Context, placementID string, expect, got map[string]interface{}) error {
	args := []string{
		"zone", "placement", "modify",
		"--rgw-realm=" + objContext.Realm,
		"--rgw-zonegroup=" + objContext.ZoneGroup,
		"--rgw-zone=" + objContext.Zone,
		"--placement-id", placementID,
	}
	// check index and data pools
	needsWorkaround := false
	expPool, err := getObjProperty[string](expect, "val", "index_pool")
	if err != nil {
		return err
	}
	gotPool, err := getObjProperty[string](got, "val", "index_pool")
	if err != nil {
		return err
	}
	if expPool != gotPool {
		logger.Infof("do zone update workaround for zone %s, placement %s index pool: %s -> %s", objContext.Zone, placementID, gotPool, expPool)
		args = append(args, "--index-pool="+expPool)
		needsWorkaround = true
	}
	expPool, err = getObjProperty[string](expect, "val", "data_extra_pool")
	if err != nil {
		return err
	}
	gotPool, err = getObjProperty[string](got, "val", "data_extra_pool")
	if err != nil {
		return err
	}
	if expPool != gotPool {
		logger.Infof("do zone update workaround for zone %s, placement %s data extra pool: %s -> %s", objContext.Zone, placementID, gotPool, expPool)
		args = append(args, "--data-extra-pool="+expPool)
		needsWorkaround = true
	}

	if needsWorkaround {
		_, err = RunAdminCommandNoMultisite(objContext, false, args...)
		if err != nil {
			return errors.Wrap(err, "failed to set zone config")
		}
	}
	expSC, err := getObjProperty[map[string]interface{}](expect, "val", "storage_classes")
	if err != nil {
		return err
	}
	gotSC, err := getObjProperty[map[string]interface{}](got, "val", "storage_classes")
	if err != nil {
		return err
	}

	// check storage classes data pools
	for sc := range expSC {
		expDP, err := getObjProperty[string](expSC, sc, "data_pool")
		if err != nil {
			return err
		}
		gotDP, err := getObjProperty[string](gotSC, sc, "data_pool")
		if err != nil {
			return err
		}
		if expDP == gotDP {
			continue
		}
		logger.Infof("do zone update workaround for zone %s, placement %s storage-class %s pool: %s -> %s", objContext.Zone, placementID, sc, gotDP, expDP)
		args = []string{
			"zone", "placement", "modify",
			"--rgw-realm=" + objContext.Realm,
			"--rgw-zonegroup=" + objContext.ZoneGroup,
			"--rgw-zone=" + objContext.Zone,
			"--placement-id", placementID,
			"--storage-class", sc,
			"--data-pool=" + expDP,
		}
		output, err := RunAdminCommandNoMultisite(objContext, false, args...)
		if err != nil {
			return errors.Wrap(err, "failed to set zone config")
		}
		logger.Debugf("zone placement modify output=%s", output)
	}

	return nil
}

// configurePoolsConcurrently checks if operator pod resources are set or not
func configurePoolsConcurrently() bool {
	// if operator resources are specified return false as it will lead to operator pod killed due to resource limit
	// nolint S1008 (go-staticcheck), we can safely suppress this
	if os.Getenv("OPERATOR_RESOURCES_SPECIFIED") == "true" {
		return false
	}
	return true
}

func createSimilarPools(ctx *Context, pools []string, cluster *cephv1.ClusterSpec, poolSpec cephv1.PoolSpec, pgCount string) error {
	// We have concurrency
	if configurePoolsConcurrently() {
		waitGroup, _ := errgroup.WithContext(ctx.clusterInfo.Context)
		for _, pool := range pools {
			// Avoid the loop reusing the same value with a closure
			pool := pool

			waitGroup.Go(func() error { return createRGWPool(ctx, cluster, poolSpec, pgCount, pool) })
		}
		return waitGroup.Wait()
	}

	// No concurrency!
	for _, pool := range pools {
		err := createRGWPool(ctx, cluster, poolSpec, pgCount, pool)
		if err != nil {
			return err
		}
	}

	return nil
}

func createRGWPool(ctx *Context, cluster *cephv1.ClusterSpec, poolSpec cephv1.PoolSpec, pgCount, requestedName string) error {
	// create the pool if it doesn't exist yet
	poolSpec.Application = rgwApplication
	pool := cephv1.NamedPoolSpec{
		Name:     poolName(ctx.Name, requestedName),
		PoolSpec: poolSpec,
	}
	if err := cephclient.CreatePoolWithPGs(ctx.Context, ctx.clusterInfo, cluster, &pool, pgCount); err != nil {
		return errors.Wrapf(err, "failed to create pool %q", pool.Name)
	}
	// Set the pg_num_min if not the default so the autoscaler won't immediately increase the pg count
	if pgCount != cephclient.DefaultPGCount {
		if err := cephclient.SetPoolProperty(ctx.Context, ctx.clusterInfo, pool.Name, "pg_num_min", pgCount); err != nil {
			return errors.Wrapf(err, "failed to set pg_num_min on pool %q to %q", pool.Name, pgCount)
		}
	}

	return nil
}

func poolName(poolPrefix, poolName string) string {
	if strings.HasPrefix(poolName, ".") {
		return poolName
	}
	// the name of the pool is <instance>.<name>, except for the pool ".rgw.root" that spans object stores
	return fmt.Sprintf("%s.%s", poolPrefix, poolName)
}

// GetObjectBucketProvisioner returns the bucket provisioner name appended with operator namespace if OBC is watching on it
func GetObjectBucketProvisioner(namespace string) (string, error) {
	provName := bucketProvisionerName
	obcWatchOnNamespace := k8sutil.GetOperatorSetting("ROOK_OBC_WATCH_OPERATOR_NAMESPACE", "false")
	obcProvisionerNamePrefix := k8sutil.GetOperatorSetting("ROOK_OBC_PROVISIONER_NAME_PREFIX", "")
	if obcProvisionerNamePrefix != "" {
		errList := validation.IsDNS1123Label(obcProvisionerNamePrefix)
		if len(errList) > 0 {
			return "", errors.Errorf("invalid OBC provisioner name prefix %q. %v", obcProvisionerNamePrefix, errList)
		}
		provName = fmt.Sprintf("%s.%s", obcProvisionerNamePrefix, bucketProvisionerName)
	} else if obcWatchOnNamespace == "true" {
		provName = fmt.Sprintf("%s.%s", namespace, bucketProvisionerName)
	}
	return provName, nil
}

// CheckDashboardUser returns true if the dashboard user exists and has the same credentials as the given user, else return false
func checkDashboardUser(context *Context, user ObjectUser) (bool, error) {
	dUser, errId, err := GetUser(context, DashboardUser)

	// If not found or "none" error, all is good to not return the error
	switch errId {
	case RGWErrorNone:
		// If the access key or secret key is not the same as the given user, return false
		if user.AccessKey != nil && *user.AccessKey != *dUser.AccessKey {
			return false, nil
		}
		if user.SecretKey != nil && *user.SecretKey != *dUser.SecretKey {
			return false, nil
		}
		return true, nil
	case RGWErrorNotFound:
		return false, nil
	}
	return false, err
}

// retrieveDashboardAPICredentials Retrieves the dashboard's access and secret key and set it on the given ObjectUser
func retrieveDashboardAPICredentials(context *Context, user *ObjectUser) error {
	args := []string{"dashboard", "get-rgw-api-access-key"}
	cephCmd := cephclient.NewCephCommand(context.Context, context.clusterInfo, args)
	out, err := cephCmd.Run()
	if err != nil {
		return err
	}

	if string(out) != "" {
		accessKey := string(out)
		user.AccessKey = &accessKey
	}

	args = []string{"dashboard", "get-rgw-api-secret-key"}
	cephCmd = cephclient.NewCephCommand(context.Context, context.clusterInfo, args)
	out, err = cephCmd.Run()
	if err != nil {
		return err
	}

	if string(out) != "" {
		secretKey := string(out)
		user.SecretKey = &secretKey
	}

	return nil
}

func getDashboardUser(context *Context) (ObjectUser, error) {
	user := ObjectUser{
		UserID:      DashboardUser,
		DisplayName: &DashboardUser,
		SystemUser:  true,
	}

	// Retrieve RGW Dashboard credentials if some are already set
	if err := retrieveDashboardAPICredentials(context, &user); err != nil {
		return user, errors.Wrapf(err, "failed to retrieve RGW Dashboard credentials for %q user", DashboardUser)
	}

	return user, nil
}

func enableRGWDashboard(context *Context) error {
	logger.Info("enabling rgw dashboard")

	user, err := getDashboardUser(context)
	if err != nil {
		logger.Debug("failed to get current dashboard user")
		return err
	}

	checkDashboard, err := checkDashboardUser(context, user)
	if err != nil {
		logger.Debug("Unable to fetch dashboard user key for RGW, hence skipping")
		return nil
	}
	if checkDashboard {
		logger.Debug("RGW Dashboard is already enabled")
		return nil
	}

	// TODO:
	// Use admin ops user instead!
	// It's safe to create the user with the force flag regardless if the cluster's dashboard is
	// configured as a secondary rgw site. The creation will return the user already exists and we
	// will just fetch it (it has been created by the primary cluster)
	u, errCode, err := CreateOrRecreateUserIfExists(context, user, true)
	if err != nil || errCode != RGWErrorNone {
		// Handle already exists ErrorCodeFileExists
		return errors.Wrapf(err, "failed to create/ re-create user %q", DashboardUser)
	}

	var accessArgs, secretArgs []string
	var secretFile *os.File

	accessFile, err := util.CreateTempFile(*u.AccessKey)
	if err != nil {
		return errors.Wrap(err, "failed to create a temporary dashboard access-key file")
	}

	accessArgs = []string{"dashboard", "set-rgw-api-access-key", "-i", accessFile.Name()}
	defer func() {
		if err := os.Remove(accessFile.Name()); err != nil {
			logger.Errorf("failed to clean up dashboard access-key file. %v", err)
		}
	}()

	secretFile, err = util.CreateTempFile(*u.SecretKey)
	if err != nil {
		return errors.Wrap(err, "failed to create a temporary dashboard secret-key file")
	}

	secretArgs = []string{"dashboard", "set-rgw-api-secret-key", "-i", secretFile.Name()}

	cephCmd := cephclient.NewCephCommand(context.Context, context.clusterInfo, accessArgs)
	_, err = cephCmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set user %q accesskey", DashboardUser)
	}

	cephCmd = cephclient.NewCephCommand(context.Context, context.clusterInfo, secretArgs)
	go func() {
		// Setting the dashboard api secret started hanging in some clusters
		// starting in ceph v15.2.8. We run it in a goroutine until the fix
		// is found. We expect the ceph command to timeout so at least the goroutine exits.
		logger.Info("setting the dashboard api secret key")
		_, err = cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
		if err != nil {
			logger.Errorf("failed to set user %q secretkey. %v", DashboardUser, err)
		}
		if err := os.Remove(secretFile.Name()); err != nil {
			logger.Errorf("failed to clean up dashboard secret-key file. %v", err)
		}

		logger.Info("done setting the dashboard api secret key")
	}()

	return nil
}

func disableRGWDashboard(context *Context) {
	logger.Info("disabling the dashboard api user and secret key")

	_, _, err := GetUser(context, DashboardUser)
	if err != nil {
		logger.Infof("unable to fetch the user %q details from this objectstore %q", DashboardUser, context.Name)
	} else {
		logger.Info("deleting rgw dashboard user")
		_, err = DeleteUser(context, DashboardUser)
		if err != nil {
			logger.Warningf("failed to delete ceph user %q. %v", DashboardUser, err)
		}
	}

	args := []string{"dashboard", "reset-rgw-api-access-key"}
	cephCmd := cephclient.NewCephCommand(context.Context, context.clusterInfo, args)
	_, err = cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		logger.Warningf("failed to reset user accesskey for user %q. %v", DashboardUser, err)
	}

	args = []string{"dashboard", "reset-rgw-api-secret-key"}
	cephCmd = cephclient.NewCephCommand(context.Context, context.clusterInfo, args)
	_, err = cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		logger.Warningf("failed to reset user secretkey for user %q. %v", DashboardUser, err)
	}
	logger.Info("done disabling the dashboard api secret key")
}

func errorOrIsNotFound(err error, msg string, args ...string) error {
	// This handles the case where the pod we use to exec command (act as a proxy) is not found/ready yet
	// The caller can nicely handle the error and not overflow the op logs with misleading error messages
	if kerrors.IsNotFound(err) {
		return err
	}
	return errors.Wrapf(err, msg, args)
}

// ShouldUpdateZoneEndpointList checks whether zone endpoint list need to be updated or not
func ShouldUpdateZoneEndpointList(zones []zoneType, desiredEndpointList []string, zoneName string) (bool, error) {
	if zoneName == "" {
		return false, errors.Errorf("zone name can't be empty")
	}

	zoneExists, endpoints := findZoneEndpoints(zoneName, zones)
	if !zoneExists {
		return false, nil
	}

	return !listsAreEqual(desiredEndpointList, endpoints), nil
}

func findZoneEndpoints(targetZone string, zones []zoneType) (bool, []string) {
	for _, z := range zones {
		if z.Name == targetZone {
			return true, z.Endpoints
		}
	}
	return false, []string{}
}

func listsAreEqual(a, b []string) bool {
	as := make([]string, len(a))
	bs := make([]string, len(b))
	copy(as, a)
	copy(bs, b)
	sort.Strings(as)
	sort.Strings(bs)

	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func CheckIfZonePresentInZoneGroup(objContext *Context) (bool, error) {
	output, err := runAdminCommand(objContext, true, "zonegroup", "get")
	if err != nil {
		return false, err
	}
	zoneGroupJson, err := DecodeZoneGroupConfig(output)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse `radosgw-admin zonegroup get` output")
	}
	zoneExists, _ := findZoneEndpoints(objContext.Zone, zoneGroupJson.Zones)
	if zoneExists {
		return true, nil
	}
	return false, nil
}

// ValidateObjectStorePoolsConfig returns error if given ObjectStore pool configuration is inconsistent.
func ValidateObjectStorePoolsConfig(metadataPool, dataPool cephv1.PoolSpec, sharedPools cephv1.ObjectSharedPoolsSpec) error {
	if err := validatePoolPlacements(sharedPools.PoolPlacements); err != nil {
		return err
	}
	if !EmptyPool(dataPool) && sharedPools.DataPoolName != "" {
		return fmt.Errorf("invalidObjStorePoolConfig: object store dataPool and sharedPools.dataPool=%s are mutually exclusive. Only one of them can be set", sharedPools.DataPoolName)
	}
	if !EmptyPool(metadataPool) && sharedPools.MetadataPoolName != "" {
		return fmt.Errorf("invalidObjStorePoolConfig: object store metadataPool and sharedPools.metadataPool=%s are mutually exclusive. Only one of them can be set", sharedPools.MetadataPoolName)
	}
	return nil
}

func SetDefaultRealm(objContext *Context, realmName string) error {
	args := []string{"realm", "default", fmt.Sprintf("--rgw-realm=%s", realmName)}
	output, err := RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to set realm %q as default, reason: %q", realmName, output)
	}

	logger.Infof("successfully set realm %q as default", realmName+"/"+objContext.clusterInfo.Namespace)
	return nil
}
