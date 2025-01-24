/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package client

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	defaultMaxRetries    = 10
	defaultRetryDelay    = 60 * time.Second
	defaultOSDRetryDelay = 10 * time.Second
)

var (
	// we don't perform any checks on these daemons
	// they don't have any "ok-to-stop" command implemented
	daemonNoCheck    = []string{"mgr", "rgw", "rbd-mirror", "nfs", "fs-mirror"}
	errNoHostInCRUSH = errors.New("no host in crush map yet?")
)

func getCephMonVersionString(context *clusterd.Context, clusterInfo *ClusterInfo) (string, error) {
	args := []string{"version"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed to run 'ceph version'. %s", string(buf))
	}
	output := string(buf)
	logger.Debug(output)

	return output, nil
}

func getAllCephDaemonVersionsString(context *clusterd.Context, clusterInfo *ClusterInfo) (string, error) {
	args := []string{"versions"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed to run 'ceph versions'. %s", string(buf))
	}
	output := string(buf)
	logger.Debug(output)

	return output, nil
}

// GetCephMonVersion reports the Ceph version of all the monitors, or at least a majority with quorum
func GetCephMonVersion(context *clusterd.Context, clusterInfo *ClusterInfo) (*cephver.CephVersion, error) {
	output, err := getCephMonVersionString(context, clusterInfo)
	if err != nil {
		return nil, err
	}
	logger.Debug(output)

	v, err := cephver.ExtractCephVersion(output)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract ceph version")
	}

	return v, nil
}

// GetAllCephDaemonVersions reports the Ceph version of each daemon in the cluster
func GetAllCephDaemonVersions(context *clusterd.Context, clusterInfo *ClusterInfo) (*cephv1.CephDaemonsVersions, error) {
	output, err := getAllCephDaemonVersionsString(context, clusterInfo)
	if err != nil {
		return nil, err
	}
	logger.Debug(output)

	var cephVersionsResult cephv1.CephDaemonsVersions
	err = json.Unmarshal([]byte(output), &cephVersionsResult)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve ceph versions results")
	}

	return &cephVersionsResult, nil
}

// EnableReleaseOSDFunctionality disallows pre-Nautilus OSDs and enables all new Nautilus-only functionality
func EnableReleaseOSDFunctionality(context *clusterd.Context, clusterInfo *ClusterInfo, release string) error {
	args := []string{"osd", "require-osd-release", release}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to disallow pre-%s osds and enable all new %s-only functionality", release, release)
	}
	output := string(buf)
	logger.Debug(output)
	logger.Infof("successfully disallowed pre-%s osds and enabled all new %s-only functionality", release, release)

	return nil
}

// OkToStop determines if it's ok to stop an upgrade
func OkToStop(context *clusterd.Context, clusterInfo *ClusterInfo, deployment, daemonType, daemonName string) error {
	okToStopRetries, okToStopDelay := getRetryConfig(clusterInfo, daemonType)
	versions, err := GetAllCephDaemonVersions(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get ceph daemons versions")
	}

	switch daemonType {
	// Trying to handle the case where a **single** mon is deployed and an upgrade is called
	case "mon":
		// if len(versions.Mon) > 1, this means we have different Ceph versions for some monitor(s).
		// This is fine, we can run the upgrade checks
		if len(versions.Mon) == 1 {
			// now trying to parse and find how many mons are presents
			// if we have less than 3 mons we skip the check and do best-effort
			// we do less than 3 because during the initial bootstrap the mon sequence is updated too
			// so running the check on 2/3 mon fails
			// versions.Mon looks like this map[ceph version 19.0.0-12-g6c8fb92 (6c8fb920cb1d862f36ee852ed849a15f9a50bd68) squid (dev):1]
			// now looping over a single element since we can't address the key directly (we don't know its name)
			for _, monCount := range versions.Mon {
				if monCount < 3 {
					logger.Infof("the cluster has fewer than 3 monitors, not performing upgrade check, running in best-effort")
					return nil
				}
			}
		}
	// Trying to handle the case where a **single** osd is deployed and an upgrade is called
	case "osd":
		if osdDoNothing(context, clusterInfo) {
			return nil
		}
	}

	// we don't implement any checks for mon, rgw and rbdmirror since:
	//  - mon: the is done in the monitor code since it ensures all the mons are always in quorum before continuing
	//  - rgw: the pod spec has a liveness probe so if the pod successfully start
	//  - rbdmirror: you can chain as many as you want like mdss but there is no ok-to-stop logic yet
	err = util.Retry(okToStopRetries, okToStopDelay, func() error {
		return okToStopDaemon(context, clusterInfo, deployment, daemonType, daemonName)
	})
	if err != nil {
		return errors.Wrapf(err, "failed to check if %s was ok to stop", deployment)
	}

	return nil
}

// OkToContinue determines if it's ok to continue an upgrade
func OkToContinue(context *clusterd.Context, clusterInfo *ClusterInfo, deployment, daemonType, daemonName string) error {
	// the mon case is handled directly in the deployment where the mon checks for quorum
	switch daemonType {
	case "mds":
		err := okToContinueMDSDaemon(context, clusterInfo, deployment, daemonType, daemonName)
		if err != nil {
			return errors.Wrapf(err, "failed to check if %s was ok to continue", deployment)
		}
	}

	return nil
}

func okToStopDaemon(context *clusterd.Context, clusterInfo *ClusterInfo, deployment, daemonType, daemonName string) error {
	if !sets.NewString(daemonNoCheck...).Has(daemonType) {
		args := []string{daemonType, "ok-to-stop", daemonName}
		buf, err := NewCephCommand(context, clusterInfo, args).Run()
		if err != nil {
			return errors.Wrapf(err, "deployment %s cannot be stopped. %s", deployment, string(buf))
		}
		output := string(buf)
		logger.Debugf("deployment %s is ok to be updated. %s", deployment, output)
	}

	// At this point, we can't tell if the daemon is unknown or if
	// but it's not a problem since perhaps it has no "ok-to-stop" call
	// It's fine to return nil here
	logger.Debugf("deployment %s is ok to be updated.", deployment)

	return nil
}

// okToContinueMDSDaemon determines whether it's fine to go to the next mds during an upgrade
// mostly a placeholder function for the future but since we have standby mds this shouldn't be needed
func okToContinueMDSDaemon(context *clusterd.Context, clusterInfo *ClusterInfo, deployment, daemonType, daemonName string) error {
	// wait for the MDS to be active again or in standby-replay
	retries, delay := getRetryConfig(clusterInfo, "mds")
	err := util.Retry(retries, delay, func() error {
		return MdsActiveOrStandbyReplay(context, clusterInfo, findFSName(deployment))
	})
	if err != nil {
		return err
	}

	return nil
}

// LeastUptodateDaemonVersion returns the ceph version of the least updated daemon type
// So if we invoke this method function with "mon", it will look for the least recent version
// Assume the following:
//
//	"mon": {
//	    "ceph version 18.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) reef (stable)": 2,
//	    "ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 1
//	}
//
// In the case we will pick: "ceph version 18.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) reef (stable)": 2,
// And eventually return 18.2.5
func LeastUptodateDaemonVersion(context *clusterd.Context, clusterInfo *ClusterInfo, daemonType string) (cephver.CephVersion, error) {
	var r map[string]int
	var vv cephver.CephVersion

	// Always invoke ceph version before an upgrade so we are sure to be up-to-date
	versions, err := GetAllCephDaemonVersions(context, clusterInfo)
	if err != nil {
		return vv, errors.Wrap(err, "failed to get ceph daemons versions")
	}

	r, err = daemonMapEntry(versions, daemonType)
	if err != nil {
		return vv, errors.Wrap(err, "failed to find daemon map entry")
	}
	for v := range r {
		version, err := cephver.ExtractCephVersion(v)
		if err != nil {
			return vv, errors.Wrap(err, "failed to extract ceph version")
		}
		vv = *version
		// break right after the first iteration
		// the first one is always the least up-to-date
		break
	}

	return vv, nil
}

func findFSName(deployment string) string {
	return strings.TrimPrefix(deployment, "rook-ceph-mds-")
}

func daemonMapEntry(versions *cephv1.CephDaemonsVersions, daemonType string) (map[string]int, error) {
	switch daemonType {
	case "mon":
		return versions.Mon, nil
	case "mgr":
		return versions.Mgr, nil
	case "mds":
		return versions.Mds, nil
	case "osd":
		return versions.Osd, nil
	case "rgw":
		return versions.Rgw, nil
	case "mirror":
		return versions.RbdMirror, nil
	}

	return nil, errors.Errorf("invalid daemonType %s", daemonType)
}

func allOSDsSameHost(context *clusterd.Context, clusterInfo *ClusterInfo) (bool, error) {
	tree, err := HostTree(context, clusterInfo)
	if err != nil {
		return false, errors.Wrap(err, "failed to get the osd tree")
	}

	osds, err := OsdListNum(context, clusterInfo)
	if err != nil {
		return false, errors.Wrap(err, "failed to get the osd list")
	}

	hostOsdTree, err := buildHostListFromTree(tree)
	if err != nil {
		return false, errors.Wrap(err, "failed to build osd tree")
	}

	hostOsdNodes := len(hostOsdTree.Nodes)
	if hostOsdNodes == 0 {
		return false, errNoHostInCRUSH
	}

	// If the number of OSD node is 1, chances are this is simple setup with all OSDs on it
	if hostOsdNodes == 1 {
		// number of OSDs on that host
		hostOsdNum := len(hostOsdTree.Nodes[0].Children)
		// we take the total number of OSDs and remove the OSDs that are out of the CRUSH map
		osdUp := len(osds) - len(tree.Stray)
		// If the number of children of that host (basically OSDs) is equal to the total number of OSDs
		// We can assume that all OSDs are running on the same machine
		if hostOsdNum == osdUp {
			return true, nil
		}
	}

	return false, nil
}

func buildHostListFromTree(tree OsdTree) (OsdTree, error) {
	var osdList OsdTree

	if tree.Nodes == nil {
		return osdList, errors.New("osd tree not populated, missing 'nodes' field")
	}

	for _, t := range tree.Nodes {
		if t.Type == "host" {
			osdList.Nodes = append(osdList.Nodes, t)
		}
	}

	return osdList, nil
}

// OSDUpdateShouldCheckOkToStop returns true if Rook should check ok-to-stop for OSDs when doing
// OSD daemon updates. It will return false if it should not perform ok-to-stop checks, for example,
// when there are fewer than 3 OSDs
func OSDUpdateShouldCheckOkToStop(context *clusterd.Context, clusterInfo *ClusterInfo) bool {
	userIntervention := "the user will likely need to set continueUpgradeAfterChecksEvenIfNotHealthy to allow OSD updates to proceed"

	osds, err := OsdListNum(context, clusterInfo)
	if err != nil {
		// If calling osd list fails, we assume there are more than 3 OSDs and we check if ok-to-stop
		// If there are less than 3 OSDs, the ok-to-stop call will fail
		// this can still be controlled by setting continueUpgradeAfterChecksEvenIfNotHealthy
		// At least this will happen for a single OSD only, which means 2 OSDs will restart in a small interval
		logger.Warningf("failed to determine the total number of osds. will check if OSDs are ok-to-stop. if there are fewer than 3 OSDs %s. %v", userIntervention, err)
		return true
	}
	if len(osds) < 3 {
		logger.Warningf("the cluster has fewer than 3 osds. not performing upgrade check. running in best-effort")
		return false
	}

	// aio means all in one
	aio, err := allOSDsSameHost(context, clusterInfo)
	if err != nil {
		if errors.Is(err, errNoHostInCRUSH) {
			logger.Warning("the CRUSH map has no 'host' entries so not performing ok-to-stop checks")
			return false
		}
		logger.Warningf("failed to determine if all osds are running on the same host. will check if OSDs are ok-to-stop. if all OSDs are running on one host %s. %v", userIntervention, err)
		return true
	}
	if aio {
		logger.Warningf("all OSDs are running on the same host. not performing upgrade check. running in best-effort")
		return false
	}

	return true
}

// osdDoNothing determines whether we should perform upgrade pre-check and post-checks for the OSD daemon
// it checks for various cluster info like number of OSD and their placement
// it returns 'true' if we need to do nothing and false and we should pre-check/post-check
func osdDoNothing(context *clusterd.Context, clusterInfo *ClusterInfo) bool {
	osds, err := OsdListNum(context, clusterInfo)
	if err != nil {
		logger.Warningf("failed to determine the total number of osds. will check if the osd is ok-to-stop anyways. %v", err)
		// If calling osd list fails, we assume there are more than 3 OSDs and we check if ok-to-stop
		// If there are less than 3 OSDs, the ok-to-stop call will fail
		// this can still be controlled by setting continueUpgradeAfterChecksEvenIfNotHealthy
		// At least this will happen for a single OSD only, which means 2 OSDs will restart in a small interval
		return false
	}
	if len(osds) < 3 {
		logger.Warningf("the cluster has fewer than 3 osds, not performing upgrade check, running in best-effort")
		return true
	}

	// aio means all in one
	aio, err := allOSDsSameHost(context, clusterInfo)
	if err != nil {
		// We return true so that we can continue without a retry and subsequently not test if the
		// osd can be stopped This handles the scenario where the OSDs have been created but not yet
		// started due to a wrong CR configuration For instance, when OSDs are encrypted and Vault
		// is used to store encryption keys, if the KV version is incorrect during the cluster
		// initialization the OSDs will fail to start and stay in CLBO until the CR is updated again
		// with the correct KV version so that it can start For this scenario we don't need to go
		// through the path where the check whether the OSD can be stopped or not, so it will always
		// fail and make us wait for nothing
		if errors.Is(err, errNoHostInCRUSH) {
			logger.Warning("the CRUSH map has no 'host' entries so not performing ok-to-stop checks")
			return true
		}
		logger.Warningf("failed to determine if all osds are running on the same host, performing upgrade check anyways. %v", err)
		return false
	}

	if aio {
		logger.Warningf("all OSDs are running on the same host, not performing upgrade check, running in best-effort")
		return true
	}

	return false
}

func getRetryConfig(clusterInfo *ClusterInfo, daemonType string) (int, time.Duration) {
	switch daemonType {
	case "osd":
		return int(clusterInfo.OsdUpgradeTimeout / defaultOSDRetryDelay), defaultOSDRetryDelay
	case "mds":
		return defaultMaxRetries, 15 * time.Second
	}

	return defaultMaxRetries, defaultRetryDelay
}
