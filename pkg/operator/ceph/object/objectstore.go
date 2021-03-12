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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rootPool = ".rgw.root"

	// AppName is the name Rook uses for the object store's application
	AppName               = "rook-ceph-rgw"
	bucketProvisionerName = "ceph.rook.io/bucket"
	AccessKeyName         = "access-key"
	SecretKeyName         = "secret-key"
	svcDNSSuffix          = "svc"

	// Timeout for setting the dashboard access key
	applyDashboardKeyTimeout = 15 * time.Second
)

var (
	metadataPools = []string{
		// .rgw.root (rootPool) is appended to this slice where needed
		"rgw.control",
		"rgw.meta",
		"rgw.log",
		"rgw.buckets.index",
		"rgw.buckets.non-ec",
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
	IsMaster     string     `json:"is_master"`
	Zones        []zoneType `json:"zones"`
}

type zoneType struct {
	Name      string   `json:"name"`
	Endpoints []string `json:"endpoints"`
}

type realmType struct {
	Realms []string `json:"realms"`
}

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
	zoneEndpointsList, err := getZoneEndpoints(objContext, objContext.Endpoint)
	if err != nil {
		return err
	}

	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneEndpoints := strings.Join(zoneEndpointsList, ",")
	endpointArg := fmt.Sprintf("--endpoints=%s", zoneEndpoints)

	zoneIsMaster, err := checkZoneIsMaster(objContext)
	if err != nil {
		return errors.Wrap(err, "failed to find out zone in Master")
	}

	zoneGroupIsMaster := false
	if zoneIsMaster {
		_, err = RunAdminCommandNoMultisite(objContext, false, "zonegroup", "modify", realmArg, zoneGroupArg, endpointArg)
		if err != nil {
			return errors.Wrapf(err, "failed to remove object store %q endpoint from rgw zone group %q", objContext.Name, objContext.ZoneGroup)
		}
		logger.Debugf("endpoint %q was removed from zone group %q. the remaining endpoints in the zone group are %q", objContext.Endpoint, objContext.ZoneGroup, zoneEndpoints)

		// check if zone group is master only if zone is master for creating the system user
		zoneGroupIsMaster, err = checkZoneGroupIsMaster(objContext)
		if err != nil {
			return errors.Wrapf(err, "failed to find out whether zone group %q in is the master zone group", objContext.ZoneGroup)
		}
	}

	_, err = runAdminCommand(objContext, false, "zone", "modify", endpointArg)
	if err != nil {
		return errors.Wrapf(err, "failed to remove object store %q endpoint from rgw zone %q", objContext.Name, spec.Zone.Name)
	}
	logger.Debugf("endpoint %q was removed from zone %q. the remaining endpoints in the zone are %q", objContext.Endpoint, objContext.Zone, zoneEndpoints)

	if zoneIsMaster && zoneGroupIsMaster && zoneEndpoints == "" {
		logger.Infof("WARNING: No other zone in realm %q can commit to the period or pull the realm until you create another object-store in zone %q", objContext.Realm, objContext.Zone)
	}

	// the period will help notify other zones of changes if there are multi-zones
	_, err = runAdminCommand(objContext, false, "period", "update", "--commit")
	if err != nil {
		return errors.Wrap(err, "failed to update period after removing an endpoint from the zone")
	}
	logger.Infof("successfully updated period for realm %v after removal of object-store %v", objContext.Realm, objContext.Name)

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
		err = deletePools(objContext, spec, lastStore)
		if err != nil {
			return errors.Wrap(err, "failed to delete object store pools")
		}
	} else {
		logger.Infof("PreservePoolsOnDelete is set in object store %s. Pools not deleted", objContext.Name)
	}

	return nil
}

// This is used for quickly getting the name of the realm, zone group, and zone for an object-store to pass into a Context
func getMultisiteForObjectStore(clusterdContext *clusterd.Context, store *cephv1.CephObjectStore) (string, string, string, error) {
	ctx := context.TODO()
	if store.Spec.IsMultisite() {
		zone, err := clusterdContext.RookClientset.CephV1().CephObjectZones(store.Namespace).Get(ctx, store.Spec.Zone.Name, metav1.GetOptions{})
		if err != nil {
			return "", "", "", errors.Wrapf(err, "failed to find zone for object-store %q", store.Name)
		}

		zonegroup, err := clusterdContext.RookClientset.CephV1().CephObjectZoneGroups(store.Namespace).Get(ctx, zone.Spec.ZoneGroup, metav1.GetOptions{})
		if err != nil {
			return "", "", "", errors.Wrapf(err, "failed to find zone group for object-store %q", store.Name)
		}

		realm, err := clusterdContext.RookClientset.CephV1().CephObjectRealms(store.Namespace).Get(ctx, zonegroup.Spec.Realm, metav1.GetOptions{})
		if err != nil {
			return "", "", "", errors.Wrapf(err, "failed to find realm for object-store %q", store.Name)
		}

		return realm.Name, zonegroup.Name, zone.Name, nil
	}

	return store.Name, store.Name, store.Name, nil
}

func checkZoneIsMaster(objContext *Context) (bool, error) {
	logger.Debugf("checking if zone %v is the master zone", objContext.Zone)
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	zoneGroupJson, err := RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		return false, errors.Wrap(err, "failed to get rgw zone group")
	}
	zoneGroupOutput, err := DecodeZoneGroupConfig(zoneGroupJson)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse zonegroup get json")
	}
	logger.Debugf("got master zone ID for zone group %v", objContext.ZoneGroup)

	zoneOutput, err := RunAdminCommandNoMultisite(objContext, true, "zone", "get", realmArg, zoneGroupArg, zoneArg)
	if err != nil {
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
		return false, errors.Wrap(err, "failed to get rgw zone group")
	}

	zoneGroupJson, err := DecodeZoneGroupConfig(zoneGroupOutput)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse master zone id")
	}

	zoneGroupIsMaster, err := strconv.ParseBool(zoneGroupJson.IsMaster)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse is_master from zone group json into bool")
	}

	return zoneGroupIsMaster, nil
}

func DecodeSecret(secret *v1.Secret, keyName string) (string, error) {
	realmKey, ok := secret.Data[keyName]

	if !ok {
		return "", errors.New("key was not in secret data")
	}

	return string(realmKey), nil
}

func GetRealmKeyArgs(clusterdContext *clusterd.Context, realmName, namespace string) (string, string, error) {
	ctx := context.TODO()
	logger.Debugf("getting keys for realm %v", realmName)
	// get realm's access and secret keys
	realmSecretName := realmName + "-keys"
	realmSecret, err := clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(ctx, realmSecretName, metav1.GetOptions{})
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to get realm %q keys secret", realmName)
	}
	logger.Debugf("found keys secret for realm %v", realmName)

	accessKey, err := DecodeSecret(realmSecret, AccessKeyName)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to decode realm %q access key", realmName)
	}
	secretKey, err := DecodeSecret(realmSecret, SecretKeyName)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to decode realm %q access key", realmName)
	}
	logger.Debugf("decoded keys for realm %v", realmName)

	accessKeyArg := fmt.Sprintf("--access-key=%s", accessKey)
	secretKeyArg := fmt.Sprintf("--secret-key=%s", secretKey)

	return accessKeyArg, secretKeyArg, nil
}

func getZoneEndpoints(objContext *Context, serviceEndpoint string) ([]string, error) {
	logger.Debugf("getting current endpoints for zone %v", objContext.Zone)
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)

	zoneGroupOutput, err := RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		return []string{}, errors.Wrap(err, "failed to get rgw zone group")
	}
	zoneGroupJson, err := DecodeZoneGroupConfig(zoneGroupOutput)
	if err != nil {
		return []string{}, errors.Wrap(err, "failed to parse zones list")
	}

	zoneEndpointsList := []string{}
	for _, zone := range zoneGroupJson.Zones {
		if zone.Name == objContext.Zone {
			for _, endpoint := range zone.Endpoints {
				// in case object-store operator code is rereconciled, zone modify could get run again with serviceEndpoint added again
				if endpoint != serviceEndpoint {
					zoneEndpointsList = append(zoneEndpointsList, endpoint)
				}
			}
			break
		}
	}

	return zoneEndpointsList, nil
}

func createMultisite(objContext *Context, endpointArg string) error {
	logger.Debugf("creating realm, zone group, zone for object-store %v", objContext.Name)

	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)

	updatePeriod := false
	// create the realm if it doesn't exist yet
	output, err := RunAdminCommandNoMultisite(objContext, true, "realm", "get", realmArg)
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			updatePeriod = true
			output, err = RunAdminCommandNoMultisite(objContext, false, "realm", "create", realmArg)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph realm %q, for reason %q", objContext.ZoneGroup, output)
			}
			logger.Debugf("created realm %v", objContext.Realm)
		} else {
			return errors.Wrapf(err, "radosgw-admin realm get failed with code %d, for reason %q", code, output)
		}
	}

	// create the zonegroup if it doesn't exist yet
	output, err = RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			updatePeriod = true
			output, err = RunAdminCommandNoMultisite(objContext, false, "zonegroup", "create", "--master", realmArg, zoneGroupArg, endpointArg)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph zone group %q, for reason %q", objContext.ZoneGroup, output)
			}
			logger.Debugf("created zone group %v", objContext.ZoneGroup)
		} else {
			return errors.Wrapf(err, "radosgw-admin zonegroup get failed with code %d, for reason %q", code, output)
		}
	}

	// create the zone if it doesn't exist yet
	output, err = runAdminCommand(objContext, true, "zone", "get")
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			updatePeriod = true
			output, err = runAdminCommand(objContext, false, "zone", "create", "--master", endpointArg)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph zone %q, for reason %q", objContext.Zone, output)
			}
			logger.Debugf("created zone %v", objContext.Zone)
		} else {
			return errors.Wrapf(err, "radosgw-admin zone get failed with code %d, for reason %q", code, output)
		}
	}

	if updatePeriod {
		// the period will help notify other zones of changes if there are multi-zones
		_, err := runAdminCommand(objContext, false, "period", "update", "--commit")
		if err != nil {
			return errors.Wrap(err, "failed to update period")
		}
		logger.Debugf("updated period for realm %v", objContext.Realm)
	}

	logger.Infof("Multisite for object-store: realm=%s, zonegroup=%s, zone=%s", objContext.Realm, objContext.ZoneGroup, objContext.Zone)

	return nil
}

func joinMultisite(objContext *Context, endpointArg, zoneEndpoints, namespace string) error {
	logger.Debugf("joining zone %v", objContext.Zone)
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	zoneIsMaster, err := checkZoneIsMaster(objContext)
	if err != nil {
		return err
	}
	zoneGroupIsMaster := false

	if zoneIsMaster {
		// endpoints that are part of a master zone are supposed to be the endpoints for a zone group
		_, err := RunAdminCommandNoMultisite(objContext, false, "zonegroup", "modify", realmArg, zoneGroupArg, endpointArg)
		if err != nil {
			return errors.Wrapf(err, "failed to add object store %q in rgw zone group %q", objContext.Name, objContext.ZoneGroup)
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
		return errors.Wrapf(err, "failed to add object store %q in rgw zone %q", objContext.Name, objContext.Zone)
	}
	logger.Debugf("endpoints for zone %q are now %q", objContext.Zone, zoneEndpoints)

	// the period will help notify other zones of changes if there are multi-zones
	_, err = RunAdminCommandNoMultisite(objContext, false, "period", "update", "--commit", realmArg, zoneGroupArg, zoneArg)
	if err != nil {
		return errors.Wrap(err, "failed to update period")
	}
	logger.Infof("added object store %q to realm %q, zonegroup %q, zone %q", objContext.Name, objContext.Realm, objContext.ZoneGroup, objContext.Zone)

	// create system user for realm for master zone in master zonegorup for multisite scenario
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

	output, err := RunAdminCommandNoMultisite(objContext, false, "user", "info", uidArg)
	if err == nil {
		logger.Debugf("realm system user %q has already been created", uid)
		return nil
	}

	if code, ok := exec.ExitStatus(err); ok && code == int(syscall.EINVAL) {
		logger.Debugf("realm system user %q not found, running `radosgw-admin user create`", uid)
		accessKeyArg, secretKeyArg, err := GetRealmKeyArgs(objContext.Context, objContext.Realm, namespace)
		if err != nil {
			return errors.Wrap(err, "failed to get keys for realm")
		}
		logger.Debugf("found keys to create realm system user %v", uid)
		systemArg := "--system"
		displayNameArg := fmt.Sprintf("--display-name=%s.user", objContext.Realm)
		output, err = RunAdminCommandNoMultisite(objContext, false, "user", "create", realmArg, zoneGroupArg, zoneArg, uidArg, displayNameArg, accessKeyArg, secretKeyArg, systemArg)
		if err != nil {
			return errors.Wrapf(err, "failed to create realm system user %q for reason: %q", uid, output)
		}
		logger.Debugf("created realm system user %v", uid)
	} else {
		return errors.Wrapf(err, "radosgw-admin user info for system user failed with code %d and output %q", code, output)
	}

	return nil
}

func setMultisite(objContext *Context, store *cephv1.CephObjectStore, serviceIP string) error {
	logger.Debugf("setting multisite configuration for object-store %v", store.Name)
	serviceEndpoint := fmt.Sprintf("http://%s:%d", serviceIP, store.Spec.Gateway.Port)
	if store.Spec.Gateway.SecurePort != 0 {
		serviceEndpoint = fmt.Sprintf("https://%s:%d", serviceIP, store.Spec.Gateway.SecurePort)
	}

	if store.Spec.IsMultisite() {
		zoneEndpointsList, err := getZoneEndpoints(objContext, serviceEndpoint)
		if err != nil {
			return err
		}
		zoneEndpointsList = append(zoneEndpointsList, serviceEndpoint)

		zoneEndpoints := strings.Join(zoneEndpointsList, ",")
		logger.Debugf("Endpoints for zone %q are: %q", objContext.Zone, zoneEndpoints)
		endpointArg := fmt.Sprintf("--endpoints=%s", zoneEndpoints)

		err = joinMultisite(objContext, endpointArg, zoneEndpoints, store.Namespace)
		if err != nil {
			return errors.Wrapf(err, "failed join ceph multisite in zone %q", objContext.Zone)
		}
	} else {
		endpointArg := fmt.Sprintf("--endpoints=%s", serviceEndpoint)
		err := createMultisite(objContext, endpointArg)
		if err != nil {
			return errors.Wrapf(err, "failed create ceph multisite for object-store %q", objContext.Name)
		}
	}

	logger.Infof("multisite configuration for object-store %v is complete", store.Name)
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

func deletePools(context *Context, spec cephv1.ObjectStoreSpec, lastStore bool) error {
	if emptyPool(spec.DataPool) && emptyPool(spec.MetadataPool) {
		logger.Info("skipping removal of pools since not specified in the object store")
		return nil
	}

	pools := append(metadataPools, dataPoolName)
	if lastStore {
		pools = append(pools, rootPool)
	}

	for _, pool := range pools {
		name := poolName(context.Name, pool)
		if err := ceph.DeletePool(context.Context, context.clusterInfo, name); err != nil {
			logger.Warningf("failed to delete pool %q. %v", name, err)
		}
	}

	// Delete erasure code profile if any
	erasureCodes, err := ceph.ListErasureCodeProfiles(context.Context, context.clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to list erasure code profiles for cluster %s", context.clusterInfo.Namespace)
	}
	// cleans up the EC profile for the data pool only. Metadata pools don't support EC (only replication is supported).
	ecProfileName := client.GetErasureCodeProfileForPool(context.Name)
	for i := range erasureCodes {
		if erasureCodes[i] == ecProfileName {
			if err := ceph.DeleteErasureCodeProfile(context.Context, context.clusterInfo, ecProfileName); err != nil {
				return errors.Wrapf(err, "failed to delete erasure code profile %s for object store %s", ecProfileName, context.Name)
			}
			break
		}
	}

	return nil
}

func CreatePools(context *Context, clusterSpec *cephv1.ClusterSpec, metadataPool, dataPool cephv1.PoolSpec) error {
	if emptyPool(dataPool) && emptyPool(metadataPool) {
		logger.Info("no pools specified for the CR, checking for their existence...")
		pools := append(metadataPools, dataPoolName)
		pools = append(pools, rootPool)
		var missingPools []string
		for _, pool := range pools {
			poolName := poolName(context.Name, pool)
			_, err := ceph.GetPoolDetails(context.Context, context.clusterInfo, poolName)
			if err != nil {
				logger.Debugf("failed to find pool %q. %v", poolName, err)
				missingPools = append(missingPools, poolName)
			}
		}
		if len(missingPools) > 0 {
			return fmt.Errorf("CR store pools are missing: %v", missingPools)
		}
	}

	// get the default PG count for rgw metadata pools
	metadataPoolPGs, err := config.GetMonStore(context.Context, context.clusterInfo).Get("mon.", "rgw_rados_pool_pg_num_min")
	if err != nil {
		logger.Warningf("failed to adjust the PG count for rgw metadata pools. using the general default. %v", err)
		metadataPoolPGs = ceph.DefaultPGCount
	}

	if err := createSimilarPools(context, append(metadataPools, rootPool), clusterSpec, metadataPool, metadataPoolPGs, ""); err != nil {
		return errors.Wrap(err, "failed to create metadata pools")
	}

	ecProfileName := ""
	if dataPool.IsErasureCoded() {
		ecProfileName = client.GetErasureCodeProfileForPool(context.Name)
		// create a new erasure code profile for the data pool
		if err := ceph.CreateErasureCodeProfile(context.Context, context.clusterInfo, ecProfileName, dataPool); err != nil {
			return errors.Wrap(err, "failed to create erasure code profile")
		}
	}

	if err := createSimilarPools(context, []string{dataPoolName}, clusterSpec, dataPool, ceph.DefaultPGCount, ecProfileName); err != nil {
		return errors.Wrap(err, "failed to create data pool")
	}

	return nil
}

func createSimilarPools(context *Context, pools []string, clusterSpec *cephv1.ClusterSpec, poolSpec cephv1.PoolSpec, pgCount, ecProfileName string) error {
	for _, pool := range pools {
		// create the pool if it doesn't exist yet
		name := poolName(context.Name, pool)
		if poolDetails, err := ceph.GetPoolDetails(context.Context, context.clusterInfo, name); err != nil {
			// If the ceph config has an EC profile, an EC pool must be created. Otherwise, it's necessary
			// to create a replicated pool.
			var err error
			if poolSpec.IsErasureCoded() {
				// An EC pool backing an object store does not need to enable EC overwrites, so the pool is
				// created with that property disabled to avoid unnecessary performance impact.
				err = ceph.CreateECPoolForApp(context.Context, context.clusterInfo, name, ecProfileName, poolSpec, pgCount, AppName, false /* enableECOverwrite */)
			} else {
				err = ceph.CreateReplicatedPoolForApp(context.Context, context.clusterInfo, clusterSpec, name, poolSpec, pgCount, AppName)
			}
			if err != nil {
				return errors.Wrapf(err, "failed to create pool %s for object store %s.", name, context.Name)
			}
		} else {
			// pools already exist
			if !poolSpec.IsErasureCoded() {
				// detect if the replication is different from the pool details
				if poolDetails.Size != poolSpec.Replicated.Size {
					logger.Infof("pool size is changed from %d to %d", poolDetails.Size, poolSpec.Replicated.Size)
					if err := ceph.SetPoolReplicatedSizeProperty(context.Context, context.clusterInfo, poolDetails.Name, strconv.FormatUint(uint64(poolSpec.Replicated.Size), 10)); err != nil {
						return errors.Wrapf(err, "failed to set size property to replicated pool %q to %d", poolDetails.Name, poolSpec.Replicated.Size)
					}
				}
			}
		}
		// Set the pg_num_min if not the default so the autoscaler won't immediately increase the pg count
		if pgCount != ceph.DefaultPGCount {
			if err := ceph.SetPoolProperty(context.Context, context.clusterInfo, name, "pg_num_min", pgCount); err != nil {
				return errors.Wrapf(err, "failed to set pg_num_min on pool %q to %q", name, pgCount)
			}
		}
	}
	return nil
}

func poolName(storeName, poolName string) string {
	if strings.HasPrefix(poolName, ".") {
		return poolName
	}
	// the name of the pool is <instance>.<name>, except for the pool ".rgw.root" that spans object stores
	return fmt.Sprintf("%s.%s", storeName, poolName)
}

// GetObjectBucketProvisioner returns the bucket provisioner name appended with operator namespace if OBC is watching on it
func GetObjectBucketProvisioner(c *clusterd.Context, namespace string) string {
	provName := bucketProvisionerName
	obcWatchOnNamespace, err := k8sutil.GetOperatorSetting(c.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_OBC_WATCH_OPERATOR_NAMESPACE", "false")
	if err != nil {
		logger.Warning("failed to verify if obc should watch the operator namespace or all of them, watching all")
	} else {
		if strings.EqualFold(obcWatchOnNamespace, "true") {
			provName = fmt.Sprintf("%s.%s", namespace, bucketProvisionerName)
		}
	}
	return provName
}

// CheckDashboardUser returns true if the user is configure else return false
func checkDashboardUser(context *Context) (bool, error) {
	args := []string{"dashboard", "get-rgw-api-access-key"}
	cephCmd := ceph.NewCephCommand(context.Context, context.clusterInfo, args)
	out, err := cephCmd.Run()

	if string(out) != "" {
		return true, err
	}

	return false, err
}

func enableRGWDashboard(context *Context) error {
	logger.Info("enabling rgw dashboard")
	checkDashboard, err := checkDashboardUser(context)
	if err != nil {
		logger.Debug("Unable to fetch dashboard user key for RGW, hence skipping")
		return nil
	}
	if checkDashboard {
		logger.Debug("RGW Dashboard is already enabled")
		return nil
	}
	user := ObjectUser{
		UserID:      DashboardUser,
		DisplayName: &DashboardUser,
		SystemUser:  true,
	}
	u, errCode, err := CreateUser(context, user)
	if err != nil || errCode != 0 {
		return errors.Wrapf(err, "failed to create user %q", DashboardUser)
	}
	args := []string{"dashboard", "set-rgw-api-access-key", *u.AccessKey}
	cephCmd := ceph.NewCephCommand(context.Context, context.clusterInfo, args)
	_, err = cephCmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set user %q accesskey", DashboardUser)
	}

	args = []string{"dashboard", "set-rgw-api-secret-key", *u.SecretKey}
	cephCmd = ceph.NewCephCommand(context.Context, context.clusterInfo, args)
	go func() {
		// Setting the dashboard api secret started hanging in some clusters
		// starting in ceph v15.2.8. We run it in a goroutine until the fix
		// is found. We expect the ceph command to timeout so at least the goroutine exits.
		logger.Info("setting the dashboard api secret key")
		_, err = cephCmd.RunWithTimeout(applyDashboardKeyTimeout)
		if err != nil {
			logger.Errorf("failed to set user %q secretkey. %v", DashboardUser, err)
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
	cephCmd := ceph.NewCephCommand(context.Context, context.clusterInfo, args)
	_, err = cephCmd.RunWithTimeout(applyDashboardKeyTimeout)
	if err != nil {
		logger.Warningf("failed to reset user accesskey for user %q. %v", DashboardUser, err)
	}

	args = []string{"dashboard", "reset-rgw-api-secret-key"}
	cephCmd = ceph.NewCephCommand(context.Context, context.clusterInfo, args)
	_, err = cephCmd.RunWithTimeout(applyDashboardKeyTimeout)
	if err != nil {
		logger.Warningf("failed to reset user secretkey for user %q. %v", DashboardUser, err)
	}
	logger.Info("done disabling the dashboard api secret key")
}
