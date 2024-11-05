package object

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	defaultPlacementName           = "default"
	defaultPlacementCephConfigName = "default-placement"
	defaultPlacementStorageClass   = "STANDARD"
)

func IsNeedToCreateObjectStorePools(sharedPools cephv1.ObjectSharedPoolsSpec) bool {
	for _, pp := range sharedPools.PoolPlacements {
		if pp.Name == defaultPlacementName {
			// No need to create pools. External pools from default placement will be used
			return false
		}
	}
	if sharedPools.MetadataPoolName != "" && sharedPools.DataPoolName != "" {
		// No need to create pools. Shared pools will be used
		return false
	}
	return true
}

func validatePoolPlacements(placements []cephv1.PoolPlacementSpec) error {
	names := make(map[string]struct{}, len(placements))
	for _, p := range placements {
		if _, ok := names[p.Name]; ok {
			return fmt.Errorf("invalidObjStorePoolCofig: invalid placement %s: placement names must be unique", p.Name)
		}
		names[p.Name] = struct{}{}
		if err := validatePoolPlacementStorageClasses(p.StorageClasses); err != nil {
			return fmt.Errorf("invalidObjStorePoolCofig: invalid placement %s: %w", p.Name, err)
		}
	}
	return nil
}

func validatePoolPlacementStorageClasses(scList []cephv1.PlacementStorageClassSpec) error {
	names := make(map[string]struct{}, len(scList))
	for _, sc := range scList {
		if sc.Name == defaultPlacementStorageClass {
			return fmt.Errorf("invalid placement StorageClass %q: %q name is reserved", sc.Name, defaultPlacementStorageClass)
		}
		if _, ok := names[sc.Name]; ok {
			return fmt.Errorf("invalid placement StorageClass %q: name must be unique", sc.Name)
		}
		names[sc.Name] = struct{}{}
	}
	return nil
}

func adjustZonePlacementPools(zone map[string]interface{}, spec cephv1.ObjectSharedPoolsSpec) (map[string]interface{}, error) {
	name, err := getObjProperty[string](zone, "name")
	if err != nil {
		return nil, fmt.Errorf("unable to get zone name: %w", err)
	}

	//deep copy source zone
	zone, err = deepCopyJson(zone)
	if err != nil {
		return nil, fmt.Errorf("unable to deep copy config for zone %s: %w", name, err)
	}

	placements, err := getObjProperty[[]interface{}](zone, "placement_pools")
	if err != nil {
		return nil, fmt.Errorf("unable to get pool placements for zone %s: %w", name, err)
	}

	fromSpec := toZonePlacementPools(spec, name)

	inConfig := map[string]struct{}{}
	idxToRemove := map[int]struct{}{}
	for i, p := range placements {
		pObj, ok := p.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unable to cast pool placement to object for zone %s: %+v", name, p)
		}
		placementID, err := getObjProperty[string](pObj, "key")
		if err != nil {
			return nil, fmt.Errorf("unable to get pool placement name for zone %s: %w", name, err)
		}
		// check if placement should be removed
		if _, inSpec := fromSpec[placementID]; !inSpec && placementID != defaultPlacementCephConfigName {
			// remove placement if it is not in spec, but don't remove default placement
			idxToRemove[i] = struct{}{}
			continue
		}
		// update placement with values from spec:
		if pSpec, inSpec := fromSpec[placementID]; inSpec {
			_, err = updateObjProperty(pObj, pSpec.Val.IndexPool, "val", "index_pool")
			if err != nil {
				return nil, fmt.Errorf("unable to set index pool to pool placement %q for zone %q: %w", placementID, name, err)
			}
			_, err = updateObjProperty(pObj, pSpec.Val.DataExtraPool, "val", "data_extra_pool")
			if err != nil {
				return nil, fmt.Errorf("unable to set data extra pool to pool placement %q for zone %q: %w", placementID, name, err)
			}
			scObj, err := toObj(pSpec.Val.StorageClasses)
			if err != nil {
				return nil, fmt.Errorf("unable convert to pool placement %q storage class for zone %q: %w", placementID, name, err)
			}

			_, err = updateObjProperty(pObj, scObj, "val", "storage_classes")
			if err != nil {
				return nil, fmt.Errorf("unable to set storage classes to pool placement %q for zone %q: %w", placementID, name, err)
			}
			inConfig[placementID] = struct{}{}
		}
	}
	if len(idxToRemove) != 0 {
		//delete placements from slice
		updated := make([]interface{}, 0, len(placements)-len(idxToRemove))
		for i := range placements {
			if _, ok := idxToRemove[i]; ok {
				//remove
				continue
			}
			updated = append(updated, placements[i])
		}
		placements = updated
	}

	// add new placements from spec:
	for placementID, p := range fromSpec {
		if _, ok := inConfig[placementID]; ok {
			//already in config
			continue
		}
		pObj, err := toObj(p)
		if err != nil {
			return nil, fmt.Errorf("unable convert pool placement %q for zone %q: %w", placementID, name, err)
		}
		placements = append(placements, pObj)
	}

	_, err = updateObjProperty(zone, placements, "placement_pools")
	if err != nil {
		return nil, fmt.Errorf("unable to set pool placements for zone %q: %w", name, err)
	}
	return zone, nil
}

func getDefaultMetadataPool(spec cephv1.ObjectSharedPoolsSpec) string {
	for _, p := range spec.PoolPlacements {
		if p.Name == defaultPlacementName {
			return p.MetadataPoolName
		}
	}
	return spec.MetadataPoolName
}

// toZonePlacementPools converts pool placement CRD definition to zone config json format structures
func toZonePlacementPools(spec cephv1.ObjectSharedPoolsSpec, ns string) map[string]ZonePlacementPool {
	hasDefault := false
	res := make(map[string]ZonePlacementPool, len(spec.PoolPlacements)+1)
	for _, pp := range spec.PoolPlacements {
		name := pp.Name
		if pp.Name == defaultPlacementName {
			hasDefault = true
			name = defaultPlacementCephConfigName
		}
		res[name] = toZonePlacementPool(pp, ns)
	}
	if !hasDefault && spec.DataPoolName != "" && spec.MetadataPoolName != "" {
		// set shared pools as default if no default placement was provided
		res[defaultPlacementCephConfigName] = ZonePlacementPool{
			Key: defaultPlacementCephConfigName,
			Val: ZonePlacementPoolVal{
				// The extra pool is for omap data for multi-part uploads, so we use
				// the metadata pool instead of the data pool.
				DataExtraPool: spec.MetadataPoolName + ":" + ns + ".buckets.non-ec",
				IndexPool:     spec.MetadataPoolName + ":" + ns + ".buckets.index",
				StorageClasses: map[string]ZonePlacementStorageClass{
					defaultPlacementStorageClass: {
						DataPool: spec.DataPoolName + ":" + ns + ".buckets.data",
					},
				},
				// Workaround: radosgw-admin set zone json command sets incorrect default value for placement inline_data field.
				// So we should set default value (true) explicitly.
				// See: https://tracker.ceph.com/issues/67933
				InlineData: true,
			},
		}
	}
	return res
}

func toZonePlacementPool(spec cephv1.PoolPlacementSpec, ns string) ZonePlacementPool {
	placementNS := ns
	if spec.Name != defaultPlacementName {
		placementNS += "." + spec.Name
	}
	// The extra pool is for omap data for multi-part uploads, so we use
	// the metadata pool instead of the data pool.
	nonECPool := spec.MetadataPoolName + ":" + placementNS + ".data.non-ec"
	if spec.DataNonECPoolName != "" {
		nonECPool = spec.DataNonECPoolName + ":" + placementNS + ".data.non-ec"
	}

	res := ZonePlacementPool{
		Key: spec.Name,
		Val: ZonePlacementPoolVal{
			DataExtraPool: nonECPool,
			IndexPool:     spec.MetadataPoolName + ":" + placementNS + ".index",
			StorageClasses: map[string]ZonePlacementStorageClass{
				defaultPlacementStorageClass: {
					DataPool: spec.DataPoolName + ":" + placementNS + ".data",
				},
			},
			// Workaround: radosgw-admin set zone json command sets incorrect default value for placement inline_data field.
			// So we should set default value (true) explicitly.
			// See: https://tracker.ceph.com/issues/67933
			InlineData: true,
		},
	}
	if res.Key == defaultPlacementName {
		res.Key = defaultPlacementCephConfigName
	}
	for _, v := range spec.StorageClasses {
		res.Val.StorageClasses[v.Name] = ZonePlacementStorageClass{
			DataPool: v.DataPoolName + ":" + ns + "." + v.Name,
		}
	}
	return res
}

func adjustZoneGroupPlacementTargets(group, zone map[string]interface{}) (map[string]interface{}, error) {
	name, err := getObjProperty[string](group, "name")
	if err != nil {
		return nil, fmt.Errorf("unable to get zonegroup name: %w", err)
	}

	//deep copy source group
	group, err = deepCopyJson(group)
	if err != nil {
		return nil, fmt.Errorf("unable to deep copy config for zonegroup %s: %w", name, err)
	}

	_, err = updateObjProperty(group, defaultPlacementCephConfigName, "default_placement")
	if err != nil {
		return nil, fmt.Errorf("unable to set default_placement for zonegroup %s: %w", name, err)
	}

	desiredTargets, err := createPlacementTargetsFromZonePoolPlacements(zone)
	if err != nil {
		return nil, fmt.Errorf("unable to create targets from placements for zonegroup %q: %w", name, err)
	}
	currentTargets, err := getObjProperty[[]interface{}](group, "placement_targets")
	if err != nil {
		return nil, fmt.Errorf("unable to get targets from placements for zonegroup %q: %w", name, err)
	}

	applied := map[string]struct{}{}
	idxToRemove := map[int]struct{}{}
	for i, target := range currentTargets {
		tObj, ok := target.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unable to cast placement target to object for zonegroup %q: %+v", name, target)
		}
		tName, err := getObjProperty[string](tObj, "name")
		if err != nil {
			return nil, fmt.Errorf("unable to get placement target name for zonegroup %q: %w", name, err)
		}
		// update target:
		if desired, ok := desiredTargets[tName]; ok {
			sc := []interface{}{}
			ok = castJson(desired.StorageClasses, &sc)
			if ok {
				_, err = updateObjProperty(tObj, sc, "storage_classes")
			} else {
				_, err = updateObjProperty(tObj, desired.StorageClasses, "storage_classes")
			}
			if err != nil {
				return nil, fmt.Errorf("unable to set storage classes to pool placement target %q for zonegroup %q: %w", tName, name, err)
			}
			applied[tName] = struct{}{}
		} else {
			// remove target
			idxToRemove[i] = struct{}{}
			continue
		}
	}
	if len(idxToRemove) != 0 {
		//delete targets from slice
		updated := make([]interface{}, 0, len(currentTargets)-len(idxToRemove))
		for i := range currentTargets {
			if _, ok := idxToRemove[i]; ok {
				//remove
				continue
			}
			updated = append(updated, currentTargets[i])
		}
		currentTargets = updated
	}

	// add new targets:
	for targetName, target := range desiredTargets {
		if _, ok := applied[targetName]; ok {
			//already in config
			continue
		}
		tObj, err := toObj(target)
		if err != nil {
			return nil, fmt.Errorf("unable convert placement target %q for zonegroup %q: %w", targetName, name, err)
		}
		currentTargets = append(currentTargets, tObj)
	}

	_, err = updateObjProperty(group, currentTargets, "placement_targets")
	if err != nil {
		return nil, fmt.Errorf("unable to set placement targets for zonegroup %q: %w", name, err)
	}

	return group, nil
}

func createPlacementTargetsFromZonePoolPlacements(zone map[string]interface{}) (map[string]ZonegroupPlacementTarget, error) {
	zoneName, err := getObjProperty[string](zone, "name")
	if err != nil {
		return nil, fmt.Errorf("unable to get zone name: %w", err)
	}

	zonePoolPlacements, err := getObjProperty[[]interface{}](zone, "placement_pools")
	if err != nil {
		return nil, fmt.Errorf("unable to get pool placements for zone %q: %w", zoneName, err)
	}

	res := make(map[string]ZonegroupPlacementTarget, len(zonePoolPlacements))
	for _, pp := range zonePoolPlacements {
		ppObj, ok := pp.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unable to cast zone pool placement to json obj for zone %q: %+v", zoneName, pp)
		}
		name, err := getObjProperty[string](ppObj, "key")
		if err != nil {
			return nil, fmt.Errorf("unable to get pool placement key for zone %q: %w", zoneName, err)
		}
		storClasses, err := getObjProperty[map[string]interface{}](ppObj, "val", "storage_classes")
		if err != nil {
			return nil, fmt.Errorf("unable to get pool placement storage classes for zone %q: %w", zoneName, err)
		}
		target := ZonegroupPlacementTarget{
			Name: name,
		}
		for sc := range storClasses {
			target.StorageClasses = append(target.StorageClasses, sc)
		}
		sort.Strings(target.StorageClasses)
		res[name] = target
	}
	return res, nil
}

func getZoneJSON(objContext *Context) (map[string]interface{}, error) {
	if objContext.Realm == "" {
		return nil, fmt.Errorf("get zone: object store realm is missing from context")
	}
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)

	if objContext.Zone == "" {
		return nil, fmt.Errorf("get zone: object store zone is missing from context")
	}
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	logger.Debugf("get zone: rgw-realm=%s, rgw-zone=%s", objContext.Realm, objContext.Zone)

	jsonStr, err := RunAdminCommandNoMultisite(objContext, true, "zone", "get", realmArg, zoneArg)
	if err != nil {
		// This handles the case where the pod we use to exec command (act as a proxy) is not found/ready yet
		// The caller can nicely handle the error and not overflow the op logs with misleading error messages
		if kerrors.IsNotFound(err) {
			return nil, err
		}
		return nil, errors.Wrap(err, "failed to get rgw zone group")
	}
	logger.Debugf("get zone success: rgw-realm=%s, rgw-zone=%s, res=%s", objContext.Realm, objContext.Zone, jsonStr)
	res := map[string]interface{}{}
	return res, json.Unmarshal([]byte(jsonStr), &res)
}

func getZoneGroupJSON(objContext *Context) (map[string]interface{}, error) {
	if objContext.Realm == "" {
		return nil, fmt.Errorf("get zonegroup: object store realm is missing from context")
	}
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)

	if objContext.Zone == "" {
		return nil, fmt.Errorf("get zonegroup: object store zone is missing from context")
	}
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	if objContext.ZoneGroup == "" {
		return nil, fmt.Errorf("get zonegroup: object store zonegroup is missing from context")
	}
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)

	logger.Debugf("get zonegroup: rgw-realm=%s, rgw-zone=%s, rgw-zonegroup=%s", objContext.Realm, objContext.Zone, objContext.ZoneGroup)
	jsonStr, err := RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg, zoneArg)
	if err != nil {
		// This handles the case where the pod we use to exec command (act as a proxy) is not found/ready yet
		// The caller can nicely handle the error and not overflow the op logs with misleading error messages
		if kerrors.IsNotFound(err) {
			return nil, err
		}
		return nil, errors.Wrap(err, "failed to get rgw zone group")
	}
	logger.Debugf("get zonegroup success: rgw-realm=%s, rgw-zone=%s, rgw-zonegroup=%s, res=%s", objContext.Realm, objContext.Zone, objContext.ZoneGroup, jsonStr)
	res := map[string]interface{}{}
	return res, json.Unmarshal([]byte(jsonStr), &res)
}

func updateZoneJSON(objContext *Context, zone map[string]interface{}) (map[string]interface{}, error) {
	if objContext.Realm == "" {
		return nil, fmt.Errorf("update zone: object store realm is missing from context")
	}
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)

	if objContext.Zone == "" {
		return nil, fmt.Errorf("update zone: object store zone is missing from context")
	}
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	configBytes, err := json.Marshal(zone)
	if err != nil {
		return nil, err
	}
	configFilename := path.Join(objContext.Context.ConfigDir, objContext.Name+".zonecfg")
	if err := os.WriteFile(configFilename, configBytes, 0600); err != nil {
		return nil, errors.Wrap(err, "failed to write zone config file")
	}
	defer os.Remove(configFilename)

	args := []string{"zone", "set", zoneArg, "--infile=" + configFilename, realmArg}
	updatedBytes, err := RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set zone config")
	}
	logger.Debugf("update zone: %s json config updated value from %q to %q", objContext.Zone, string(configBytes), string(updatedBytes))
	updated := map[string]interface{}{}
	err = json.Unmarshal([]byte(updatedBytes), &updated)
	return updated, err
}

func updateZoneGroupJSON(objContext *Context, group map[string]interface{}) (map[string]interface{}, error) {
	if objContext.Realm == "" {
		return nil, fmt.Errorf("update zonegroup: object store realm is missing from context")
	}
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)

	if objContext.Zone == "" {
		return nil, fmt.Errorf("update zonegroup: object store zone is missing from context")
	}
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	if objContext.ZoneGroup == "" {
		return nil, fmt.Errorf("update zonegroup: object store zonegroup is missing from context")
	}
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)

	configBytes, err := json.Marshal(group)
	if err != nil {
		return nil, err
	}
	configFilename := path.Join(objContext.Context.ConfigDir, objContext.Name+".zonegroupcfg")
	if err := os.WriteFile(configFilename, configBytes, 0600); err != nil {
		return nil, errors.Wrap(err, "failed to write zonegroup config file")
	}
	defer os.Remove(configFilename)

	args := []string{"zonegroup", "set", zoneArg, "--infile=" + configFilename, realmArg, zoneGroupArg}
	updatedBytes, err := RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set zone config")
	}
	updated := map[string]interface{}{}
	err = json.Unmarshal([]byte(updatedBytes), &updated)
	return updated, err
}

type ZonegroupPlacementTarget struct {
	Name           string   `json:"name"`
	StorageClasses []string `json:"storage_classes"`
}

type ZonePlacementPool struct {
	Key string               `json:"key"`
	Val ZonePlacementPoolVal `json:"val"`
}

type ZonePlacementPoolVal struct {
	DataExtraPool  string                               `json:"data_extra_pool"`
	IndexPool      string                               `json:"index_pool"`
	StorageClasses map[string]ZonePlacementStorageClass `json:"storage_classes"`
	InlineData     bool                                 `json:"inline_data"`
}

type ZonePlacementStorageClass struct {
	DataPool string `json:"data_pool"`
}
