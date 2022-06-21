/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"syscall"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
)

// CreateRadosNamespace create a rados namespace in a pool.
// poolName is the name of the ceph block pool, the same as the CephBlockPool CR name.
func CreateRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, namespaceName string) error {
	logger.Infof("creating rados namespace %s/%s in k8s namespace %q", poolName, namespaceName, clusterInfo.Namespace)
	//  rbd namespace create pool-name/namespace-name
	args := []string{"namespace", "create", "--pool", poolName, "--namespace", namespaceName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	output, err := cmd.Run()
	if err != nil {
		code, ok := exec.ExitStatus(err)
		if ok && code == int(syscall.EEXIST) {
			logger.Debugf("rados namespace %s/%s in k8s namespace %q already exists", poolName, namespaceName, clusterInfo.Namespace)
			return nil
		}
		return errors.Wrapf(err, "failed to create rados namespace %s/%s. %s", poolName, namespaceName, output)

	}

	logger.Infof("successfully created rados namespace %s/%s in k8s namespace %q", poolName, namespaceName, clusterInfo.Namespace)
	return nil
}

func getRadosNamespaceStatistics(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, namespaceName string) (*PoolStatistics, error) {
	var poolStats PoolStatistics

	args := []string{"pool", "stats", "--pool", poolName, "--namespace", namespaceName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true
	output, err := cmd.Run()
	if err != nil {
		code, ok := exec.ExitStatus(err)
		if ok && code == int(syscall.ENOENT) {
			return &poolStats, nil
		}
		return nil, errors.Wrapf(err, "failed to get pool stats. %s", string(output))
	}
	if err := json.Unmarshal(output, &poolStats); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal pool stats response")
	}

	return &poolStats, nil
}

func checkForImagesInRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, namespaceName string) error {
	logger.Debugf("checking any images/snapshots present in pool %s/%s in k8s namespace %q", poolName, namespaceName, clusterInfo.Namespace)
	stats, err := getRadosNamespaceStatistics(context, clusterInfo, poolName, namespaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to list images/snapshots in pool %s/%s", poolName, namespaceName)
	}
	if stats.Images.Count == 0 && stats.Images.SnapCount == 0 {
		logger.Infof("no images/snapshots present in pool %s/%s in k8s namespace %q", poolName, namespaceName, clusterInfo.Namespace)
		return nil
	}

	return errors.Errorf("pool %s/%s contains %d images and %d snapshots", poolName, namespaceName, stats.Images.Count, stats.Images.SnapCount)
}

// DeleteRadosNamespace delete a rados namespace.
func DeleteRadosNamespace(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, namespaceName string) error {
	err := checkForImagesInRadosNamespace(context, clusterInfo, poolName, namespaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to check if pool %s/%s has rbd images", poolName, namespaceName)
	}
	logger.Infof("deleting rados namespace %s/%s in k8s namespace %q", poolName, namespaceName, clusterInfo.Namespace)
	args := []string{"namespace", "remove", "--pool", poolName, "--namespace", namespaceName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	output, err := cmd.Run()
	if err != nil {
		code, ok := exec.ExitStatus(err)
		if !ok || code != int(syscall.ENOENT) {
			return errors.Wrapf(err, "failed to delete rados namespace %s/%s. %s", poolName, namespaceName, output)
		}
	}

	logger.Infof("successfully deleted rados namespace %s/%s in k8s namespace %q", poolName, namespaceName, clusterInfo.Namespace)
	return nil
}
