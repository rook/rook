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
	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util"
)

// CephDaemonsVersions is a structure that can be used to parsed the output of the 'ceph versions' command
type CephDaemonsVersions struct {
	Mon       map[string]int `json:"mon,omitempty"`
	Mgr       map[string]int `json:"mgr,omitempty"`
	Osd       map[string]int `json:"osd,omitempty"`
	Rgw       map[string]int `json:"rgw,omitempty"`
	Mds       map[string]int `json:"mds,omitempty"`
	RbdMirror map[string]int `json:"rbd-mirror,omitempty"`
	Overall   map[string]int `json:"overall,omitempty"`
}

var (
	// we don't perform any checks on these daemons
	// they don't have any "ok-to-stop" command implemented
	daemonNoCheck = []string{"mgr", "rgw", "rbd-mirror", "nfs"}
)

func getCephMonVersionString(context *clusterd.Context, clusterName string) (string, error) {
	args := []string{"version"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed to run 'ceph version")
	}
	output := string(buf)
	logger.Debug(output)

	return output, nil
}

func getAllCephDaemonVersionsString(context *clusterd.Context, clusterName string) (string, error) {
	args := []string{"versions"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed to run 'ceph versions")
	}
	output := string(buf)
	logger.Debug(output)

	return output, nil
}

// GetCephMonVersion reports the Ceph version of all the monitors, or at least a majority with quorum
func GetCephMonVersion(context *clusterd.Context, clusterName string) (*cephver.CephVersion, error) {
	output, err := getCephMonVersionString(context, clusterName)
	if err != nil {
		return nil, err
	}
	logger.Debug(output)

	v, err := cephver.ExtractCephVersion(output)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract ceph version")
	}

	return v, nil
}

// GetAllCephDaemonVersions reports the Ceph version of each daemon in the cluster
func GetAllCephDaemonVersions(context *clusterd.Context, clusterName string) (*CephDaemonsVersions, error) {
	output, err := getAllCephDaemonVersionsString(context, clusterName)
	if err != nil {
		return nil, err
	}
	logger.Debug(output)

	var cephVersionsResult CephDaemonsVersions
	err = json.Unmarshal([]byte(output), &cephVersionsResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ceph versions results")
	}

	return &cephVersionsResult, nil
}

// EnableMessenger2 enable the messenger 2 protocol on Nautilus clusters
func EnableMessenger2(context *clusterd.Context, clusterName string) error {
	args := []string{"mon", "enable-msgr2"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable msgr2 protocol")
	}
	output := string(buf)
	logger.Debug(output)
	logger.Infof("successfully enabled msgr2 protocol")

	return nil
}

// EnableNautilusOSD disallows pre-Nautilus OSDs and enables all new Nautilus-only functionality
func EnableNautilusOSD(context *clusterd.Context, clusterName string) error {
	args := []string{"osd", "require-osd-release", "nautilus"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to disallow pre-nautilus osds and enable all new nautilus-only functionality")
	}
	output := string(buf)
	logger.Debug(output)
	logger.Infof("successfully disallowed pre-nautilus osds and enabled all new nautilus-only functionality")

	return nil
}

// OkToStop determines if it's ok to stop an upgrade
func OkToStop(context *clusterd.Context, namespace, deployment, daemonType, daemonName string, cephVersion cephver.CephVersion) error {
	// The ok-to-stop command for mon and mds landed on 14.2.1
	// so we return nil if that Ceph version is not satisfied
	if !cephVersion.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 1}) {
		if daemonType != "osd" {
			return nil
		}
	}

	versions, err := GetAllCephDaemonVersions(context, namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get ceph daemons versions")
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
			// so running running the check on 2/3 mon fails
			// versions.Mon looks like this map[ceph version 15.0.0-12-g6c8fb92 (6c8fb920cb1d862f36ee852ed849a15f9a50bd68) octopus (dev):1]
			// now looping over a single element since we can't address the key directly (we don't know its name)
			for _, monCount := range versions.Mon {
				if monCount < 3 {
					logger.Infof("the cluster has less than 3 monitors, not performing upgrade check, running in best-effort")
					return nil
				}
			}
		}
	// Trying to handle the case where a **single** osd is deployed and an upgrade is called
	case "osd":
		if osdDoNothing(context, namespace) {
			return nil
		}
	}
	// we don't implement any checks for mon, rgw and rbdmirror since:
	//  - mon: the is done in the monitor code since it ensures all the mons are always in quorum before continuing
	//  - rgw: the pod spec has a liveness probe so if the pod successfully start
	//  - rbdmirror: you can chain as many as you want like mdss but there is no ok-to-stop logic yet
	err = okToStopDaemon(context, deployment, namespace, daemonType, daemonName)
	if err != nil {
		return errors.Wrapf(err, "failed to check if %s was ok to stop", deployment)
	}

	return nil
}

// OkToContinue determines if it's ok to continue an upgrade
func OkToContinue(context *clusterd.Context, namespace, deployment, daemonType, daemonName string) error {
	// the mon case is handled directly in the deployment where the mon checks for quorum
	switch daemonType {
	case "osd":
		if osdDoNothing(context, namespace) {
			return nil
		}
		err := okToContinueOSDDaemon(context, namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to check if %s was ok to continue", deployment)
		}
	case "mds":
		err := okToContinueMDSDaemon(context, namespace, deployment, daemonType, daemonName)
		if err != nil {
			return errors.Wrapf(err, "failed to check if %s was ok to continue", deployment)
		}
	}

	return nil
}

func okToStopDaemon(context *clusterd.Context, deployment, clusterName, daemonType, daemonName string) error {
	if !stringInSlice(daemonType, daemonNoCheck) {
		args := []string{daemonType, "ok-to-stop", daemonName}
		buf, err := NewCephCommand(context, clusterName, args).Run()
		if err != nil {
			return errors.Wrapf(err, "deployment %s cannot be stopped", deployment)
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

// okToContinueOSDDaemon determines whether it's fine to go to the next osd during an upgrade
// This basically makes sure all the PGs have settled
func okToContinueOSDDaemon(context *clusterd.Context, namespace string) error {
	// Reconciliating PGs should not take too long so let's wait up to 10 minutes
	err := util.Retry(10, 60*time.Second, func() error {
		return IsClusterCleanError(context, namespace)
	})
	if err != nil {
		return err
	}

	return nil
}

// okToContinueMDSDaemon determines whether it's fine to go to the next mds during an upgrade
// mostly a placeholder function for the future but since we have standby mds this shouldn't be needed
func okToContinueMDSDaemon(context *clusterd.Context, namespace, deployment, daemonType, daemonName string) error {
	// wait for the MDS to be active again or in standby-replay
	err := util.Retry(10, 15*time.Second, func() error {
		return MdsActiveOrStandbyReplay(context, namespace, findFSName(deployment))
	})
	if err != nil {
		return err
	}

	return nil
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// LeastUptodateDaemonVersion returns the ceph version of the least updated daemon type
// So if we invoke this method function with "mon", it will look for the least recent version
// Assume the following:
//
// "mon": {
//     "ceph version 13.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) mimic (stable)": 1,
//     "ceph version 14.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) nautilus (stable)": 2
// }
//
// In the case we will pick: "ceph version 13.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) mimic (stable)": 1,
// And eventually return 13.2.5
func LeastUptodateDaemonVersion(context *clusterd.Context, clusterName, daemonType string) (cephver.CephVersion, error) {
	var r map[string]int
	var vv cephver.CephVersion

	// Always invoke ceph version before an upgrade so we are sure to be up-to-date
	versions, err := GetAllCephDaemonVersions(context, clusterName)
	if err != nil {
		logger.Warningf("failed to get ceph daemons versions, this likely means there is no cluster yet. %v", err)
	} else {
		r, err = daemonMapEntry(versions, daemonType)
		if err != nil {
			return vv, errors.Wrapf(err, "failed to find daemon map entry")
		}
		for v := range r {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				return vv, errors.Wrapf(err, "failed to extract ceph version")
			}
			vv = *version
			// break right after the first iteration
			// the first one is always the least up-to-date
			break
		}
	}

	return vv, nil
}

func findFSName(deployment string) string {
	return strings.TrimPrefix(deployment, "rook-ceph-mds-")
}

func daemonMapEntry(versions *CephDaemonsVersions, daemonType string) (map[string]int, error) {
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

func allOSDsSameHost(context *clusterd.Context, clusterName string) (bool, error) {
	tree, err := HostTree(context, clusterName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get the osd tree")
	}

	osds, err := OsdListNum(context, clusterName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get the osd list")
	}

	hostOsdTree, err := buildHostListFromTree(tree)
	if err != nil {
		return false, errors.Wrapf(err, "failed to build osd tree")
	}

	hostOsdNodes := len(hostOsdTree.Nodes)
	if hostOsdNodes == 0 {
		return false, errors.New("no host in crush map yet?")
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

// osdDoNothing determines wether we should perfom upgrade pre-check and post-checks for the OSD daemon
// it checks for various cluster info like number of OSD and their placement
// it returns 'true' if we need to do nothing and false and we should pre-check/post-check
func osdDoNothing(context *clusterd.Context, clusterName string) bool {
	versions, err := GetAllCephDaemonVersions(context, clusterName)
	if err != nil {
		logger.Warningf("failed to get ceph daemons versions, this likely means there is no cluster yet. %v", err)
		return true
	}

	if len(versions.Osd) == 1 {
		// now trying to parse and find how many osds are presents
		// if we have less than 3 osds we skip the check and do best-effort
		for _, osdCount := range versions.Osd {
			if osdCount < 3 {
				logger.Warningf("the cluster has less than 3 OSDs, not performing upgrade check, running in best-effort")
				return true
			}
		}
	}

	// aio means all in one
	aio, err := allOSDsSameHost(context, clusterName)
	if err != nil {
		// That's tricky, we are about to perform an update so it's difficult to break the update for this
		// let's consider this is not a problem but log what happened
		logger.Warningf("not able to determine if all OSDs are running on the same host, not performing upgrade check, running in best-effort")
		return true
	}

	if aio {
		logger.Warningf("it looks like all OSDs are running on the same host, not performing upgrade check, running in best-effort")
		return true
	}

	return false
}
