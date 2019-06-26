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
	"errors"
	"fmt"
	"strings"
	"time"

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
	command, args := FinalizeCephCommandArgs("ceph", args, context.ConfigDir, clusterName)

	output, err := context.Executor.ExecuteCommandWithOutput(false, "", command, args...)
	if err != nil {
		return "", fmt.Errorf("failed to run 'ceph version'. %+v", err)
	}
	logger.Debug(output)

	return output, nil
}

func getAllCephDaemonVersionsString(context *clusterd.Context, clusterName string) (string, error) {
	args := []string{"versions"}
	command, args := FinalizeCephCommandArgs("ceph", args, context.ConfigDir, clusterName)

	output, err := context.Executor.ExecuteCommandWithOutput(false, "", command, args...)
	if err != nil {
		return "", fmt.Errorf("failed to run 'ceph versions'. %+v", err)
	}
	logger.Debug(output)

	return output, nil
}

func getCephDaemonVersionString(context *clusterd.Context, deployment, clusterName string) (string, error) {
	daemonName, err := findDaemonName(deployment)
	if err != nil {
		return "", fmt.Errorf("%+v", err)
	}
	daemonID := findDaemonID(deployment)
	daemon := daemonName + "." + daemonID

	args := []string{"tell", daemon, "version"}
	command, args := FinalizeCephCommandArgs("ceph", args, context.ConfigDir, clusterName)
	output, err := context.Executor.ExecuteCommandWithOutput(false, "", command, args...)
	if err != nil {
		return "", fmt.Errorf("failed to run ceph tell. %+v", err)
	}
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
		return nil, fmt.Errorf("failed to extract ceph version. %+v", err)
	}

	return v, nil
}

// GetCephDaemonVersion reports the Ceph version of a particular daemon
func GetCephDaemonVersion(context *clusterd.Context, deployment, clusterName string) (*cephver.CephVersion, error) {
	output, err := getCephDaemonVersionString(context, deployment, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to run ceph tell. %+v", err)
	}
	logger.Debug(output)

	v, err := cephver.ExtractCephVersion(output)
	if err != nil {
		return nil, fmt.Errorf("failed to extract ceph version. %+v", err)
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
		return nil, fmt.Errorf("failed to retrieve ceph versions results. %+v", err)
	}

	return &cephVersionsResult, nil
}

// EnableMessenger2 enable the messenger 2 protocol on Nautilus clusters
func EnableMessenger2(context *clusterd.Context) error {
	_, err := context.Executor.ExecuteCommandWithOutput(false, "", "ceph", "mon", "enable-msgr2")
	if err != nil {
		return fmt.Errorf("failed to enable msgr2 protocol. %+v", err)
	}
	logger.Infof("successfully enabled msgr2 protocol")

	return nil
}

// EnableNautilusOSD disallows pre-Nautilus OSDs and enables all new Nautilus-only functionality
func EnableNautilusOSD(context *clusterd.Context) error {
	_, err := context.Executor.ExecuteCommandWithOutput(false, "", "ceph", "osd", "require-osd-release", "nautilus")
	if err != nil {
		return fmt.Errorf("failed to disallow pre-nautilus osds and enable all new nautilus-only functionality: %+v", err)
	}
	logger.Infof("successfully disallowed pre-nautilus osds and enabled all new nautilus-only functionality")
	return nil
}

// OkToStop determines if it's ok to stop an upgrade
func OkToStop(context *clusterd.Context, namespace, deployment, clusterName string, cephVersion cephver.CephVersion) error {
	daemonName, err := findDaemonName(deployment)
	if err != nil {
		logger.Warningf("%+v", err)
		return nil
	}

	// The ok-to-stop command for mon and mds landed on 14.2.1
	// so we return nil if that Ceph version is not satisfied
	if !cephVersion.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 1}) {
		if daemonName != "osd" {
			return nil
		}
	}

	versions, err := GetAllCephDaemonVersions(context, clusterName)
	if err != nil {
		return fmt.Errorf("failed to get ceph daemons versions. %+v", err)
	}

	switch daemonName {
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
		// if len(versions.Osd) > 1, this means we have different Ceph versions osd(s)
		// This is fine, we can run the upgrade checks
		if len(versions.Osd) == 1 {
			// now trying to parse and find how many osds are presents
			// if we have less than 3 osds we skip the check and do best-effort
			for _, osdCount := range versions.Osd {
				if osdCount < 3 {
					logger.Infof("the cluster has less than 3 OSDs, not performing upgrade check, running in best-effort")
					return nil
				}
			}
		}
	}
	// we don't implement any checks for mon, rgw and rbdmirror since:
	//  - mon: the is done in the monitor code since it ensures all the mons are always in quorum before continuing
	//  - rgw: the pod spec has a liveness probe so if the pod successfully start
	//  - rbdmirror: you can chain as many as you want like mdss but there is no ok-to-stop logic yet
	err = okToStopDaemon(context, deployment, clusterName)
	if err != nil {
		return fmt.Errorf("failed to check if %s was ok to stop. %+v", deployment, err)
	}

	return nil
}

// OkToContinue determines if it's ok to continue an upgrade
func OkToContinue(context *clusterd.Context, namespace, deployment string) error {
	daemonName, err := findDaemonName(deployment)
	if err != nil {
		logger.Warningf("%+v", err)
		return nil
	}

	// the mon case is handled directly in the deployment where the mon checks for quorum
	switch daemonName {
	case "osd":
		err := okToContinueOSDDaemon(context, namespace)
		if err != nil {
			return fmt.Errorf("failed to check if %s was ok to continue. %+v", deployment, err)
		}
	case "mds":
		err := okToContinueMDSDaemon(context, namespace, deployment)
		if err != nil {
			return fmt.Errorf("failed to check if %s was ok to continue. %+v", deployment, err)
		}
	}

	return nil
}

func okToStopDaemon(context *clusterd.Context, deployment, clusterName string) error {
	daemonID := findDaemonID(deployment)
	daemonName, err := findDaemonName(deployment)
	if err != nil {
		logger.Warningf("%+v", err)
		return nil
	}

	if !stringInSlice(daemonName, daemonNoCheck) {
		args := []string{daemonName, "ok-to-stop", daemonID}
		command, args := FinalizeCephCommandArgs("ceph", args, context.ConfigDir, clusterName)

		output, err := context.Executor.ExecuteCommandWithOutput(false, "", command, args...)
		if err != nil {
			return fmt.Errorf("deployment %s cannot be stopped. %+v", deployment, err)
		}
		logger.Debugf("deployment %s is ok to be updated. %s", deployment, output)
	}

	logger.Debugf("deployment %s is ok to be updated.", deployment)

	return nil
}

// okToContinueOSDDaemon determines whether it's fine to go to the next osd during an upgrade
// This basically makes sure all the PGs have settled
func okToContinueOSDDaemon(context *clusterd.Context, namespace string) error {
	// Reconciliating PGs should not take too long so let's wait up to 10 minutes
	err := util.Retry(10, 60*time.Second, func() error {
		return IsClusterClean(context, namespace)
	})
	if err != nil {
		return err
	}

	return nil
}

// okToContinueMDSDaemon determines whether it's fine to go to the next mds during an upgrade
// mostly a placeholder function for the future but since we have standby mds this shouldn't be needed
func okToContinueMDSDaemon(context *clusterd.Context, namespace, deployment string) error {
	// wait for the MDS to be active again or in standby-replay
	err := util.Retry(10, 15*time.Second, func() error {
		return MdsActiveOrStandbyReplay(context, namespace, findFSName(deployment, namespace))
	})
	if err != nil {
		return err
	}

	return nil
}

func findFSName(deployment, namespace string) string {
	mdsID := findDaemonID(deployment)
	fsNameTrimSuffix := strings.TrimSuffix(deployment, "-"+mdsID)
	return strings.TrimPrefix(fsNameTrimSuffix, namespace+"-mds-")
}

func findDaemonID(deployment string) string {
	daemonTrimPrefixSplit := strings.Split(deployment, "-")
	return daemonTrimPrefixSplit[len(daemonTrimPrefixSplit)-1]
}

func findDaemonName(deployment string) (string, error) {
	err := errors.New("could not find daemon name, is this a new daemon?")

	if strings.Contains(deployment, "mds") {
		return "mds", nil
	}
	if strings.Contains(deployment, "rgw") {
		return "rgw", nil
	}
	if strings.Contains(deployment, "mon") {
		return "mon", nil
	}
	if strings.Contains(deployment, "osd") {
		return "osd", nil
	}
	if strings.Contains(deployment, "mgr") {
		return "mgr", nil
	}
	if strings.Contains(deployment, "nfs") {
		return "nfs", nil
	}
	if strings.Contains(deployment, "rbd-mirror") {
		return "rbd-mirror", nil
	}

	return "", fmt.Errorf("%+v from deployment %s", err, deployment)
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
		logger.Warningf("failed to get ceph daemons versions. %+v, this likely means there is no cluster yet.", err)
	} else {
		r, err = daemonMapEntry(versions, daemonType)
		if err != nil {
			return vv, fmt.Errorf("failed to find daemon map entry %+v", err)
		}
		for v := range r {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				return vv, fmt.Errorf("failed to extract ceph version. %+v", err)
			}
			vv = *version
			// break right after the first iteration
			// the first one is always the least up-to-date
			break
		}
	}

	return vv, nil
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

	return nil, fmt.Errorf("invalid daemonType %s", daemonType)
}
