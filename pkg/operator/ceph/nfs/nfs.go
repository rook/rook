/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package nfs manages NFS ganesha servers for Ceph
package nfs

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ganeshaRadosGraceCmd = "ganesha-rados-grace"
)

var updateDeploymentAndWait = opmon.UpdateCephDeploymentAndWait

type daemonConfig struct {
	ID              string              // letter ID of daemon (e.g., a, b, c, ...)
	ConfigConfigMap string              // name of configmap holding config
	DataPathMap     *config.DataPathMap // location to store data in container
}

// Create the ganesha server
func (c *CephNFSController) upCephNFS(n cephv1.CephNFS, oldActive int) error {
	if err := validateGanesha(c.context, n); err != nil {
		return err
	}

	logger.Infof("Starting cephNFS %s(%d-%d)", n.Name, oldActive,
		n.Spec.Server.Active-1)

	for i := oldActive; i < n.Spec.Server.Active; i++ {
		id := k8sutil.IndexToName(i)

		configName, err := c.generateConfig(n, id)
		if err != nil {
			return errors.Wrapf(err, "failed to create config")
		}

		err = c.addRADOSConfigFile(n, id)
		if err != nil {
			return errors.Wrapf(err, "failed to create RADOS config object")
		}

		cfg := daemonConfig{
			ID:              id,
			ConfigConfigMap: configName,
			DataPathMap: &config.DataPathMap{
				HostDataDir:        "",                          // nfs daemon does not store data on host, ...
				ContainerDataDir:   cephconfig.DefaultConfigDir, // does share data in containers using emptyDir, ...
				HostLogAndCrashDir: "",                          // and does not log to /var/log/ceph dir
			},
		}

		// start the deployment
		deployment := c.makeDeployment(n, cfg)
		_, err = c.context.Clientset.AppsV1().Deployments(n.Namespace).Create(deployment)
		if err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create ganesha deployment")
			}
			logger.Infof("ganesha deployment %s already exists. updating if needed", deployment.Name)
			// We don't invoke ceph versions here since nfs do not show up in the service map (yet?)
			if err := updateDeploymentAndWait(c.context, deployment, n.Namespace, "nfs", id, c.clusterInfo.CephVersion, c.isUpgrade, c.clusterSpec.SkipUpgradeChecks, false); err != nil {
				return errors.Wrapf(err, "failed to update ganesha deployment %s", deployment.Name)
			}
		} else {
			logger.Infof("ganesha deployment %s started", deployment.Name)
		}

		// create a service
		err = c.createCephNFSService(n, cfg)
		if err != nil {
			return errors.Wrapf(err, "failed to create ganesha service")
		}

		c.addServerToDatabase(n, id)
	}

	return nil
}

// Create empty config file for new ganesha server
func (c *CephNFSController) addRADOSConfigFile(n cephv1.CephNFS, name string) error {
	nodeID := getNFSNodeID(n, name)
	config := getGaneshaConfigObject(nodeID)
	cmd := "rados"
	args := []string{
		"--pool", n.Spec.RADOS.Pool,
		"--namespace", n.Spec.RADOS.Namespace,
		"--conf", cephclient.CephConfFilePath(c.context.ConfigDir, c.namespace),
	}
	moniker := "rados stat " + config
	err := c.context.Executor.ExecuteCommand(false, moniker, cmd, append(args, "stat", config)...)
	if err == nil {
		// If stat works then we assume it's present already
		return nil
	}
	// try to create it
	moniker = "rados create " + config
	return c.context.Executor.ExecuteCommand(false, moniker, cmd, append(args, "create", config)...)
}

func (c *CephNFSController) addServerToDatabase(nfs cephv1.CephNFS, name string) {
	logger.Infof("Adding ganesha %s to grace db", name)

	if err := c.runGaneshaRadosGrace(nfs, name, "add"); err != nil {
		logger.Errorf("failed to add %q to grace db. %v", name, err)
	}
}

func (c *CephNFSController) removeServerFromDatabase(nfs cephv1.CephNFS, name string) {
	logger.Infof("Removing ganesha %q from grace db", name)

	if err := c.runGaneshaRadosGrace(nfs, name, "remove"); err != nil {
		logger.Errorf("failed to remove %q from grace db. %v", name, err)
	}
}

func (c *CephNFSController) runGaneshaRadosGrace(nfs cephv1.CephNFS, name, action string) error {
	nodeID := getNFSNodeID(nfs, name)
	cmd := ganeshaRadosGraceCmd
	args := []string{"--pool", nfs.Spec.RADOS.Pool, "--ns", nfs.Spec.RADOS.Namespace, action, nodeID}
	moniker := ganeshaRadosGraceCmd + " " + action + " " + nodeID
	// Need to run a command with CEPH_CONF env var set, so don't use c.context.Executor
	x := exec.Command(cmd, args...)
	x.Env = []string{fmt.Sprintf("CEPH_CONF=%s", cephclient.CephConfFilePath(c.context.ConfigDir, c.namespace))}
	var b bytes.Buffer
	x.Stdout = &b
	x.Stderr = &b
	logger.Infof("Running command: %s %s %s", x.Env[0], cmd, strings.Join(args, " "))
	if err := x.Run(); err != nil {
		return errors.Wrapf(err, `failed to execute %q
stdout: %s
stderr: %s`, moniker, x.Stdout, x.Stderr)
	}
	return nil
	// The below can be used when/if `ganesha-rados-grace` supports a `--cephconf` or similar option
	// return c.context.Executor.ExecuteCommand(false, moniker, cmd, args...)
}

func (c *CephNFSController) generateConfig(n cephv1.CephNFS, name string) (string, error) {

	data := map[string]string{
		"config": getGaneshaConfig(n, name),
	}
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", appName, n.Name, name),
			Namespace: n.Namespace,
			Labels:    getLabels(n, name),
		},
		Data: data,
	}
	if _, err := c.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Create(configMap); err != nil {
		if kerrors.IsAlreadyExists(err) {
			if _, err := c.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Update(configMap); err != nil {
				return "", errors.Wrapf(err, "failed to update ganesha config")
			}
			return configMap.Name, nil
		}
		return "", errors.Wrapf(err, "failed to create ganesha config")
	}
	return configMap.Name, nil
}

// Delete the ganesha server
func (c *CephNFSController) downCephNFS(n cephv1.CephNFS, newActive int) error {
	for i := n.Spec.Server.Active - 1; i >= newActive; i-- {
		name := k8sutil.IndexToName(i)

		// Remove from grace db
		c.removeServerFromDatabase(n, name)
	}

	return nil
}

func instanceName(n cephv1.CephNFS, name string) string {
	return fmt.Sprintf("%s-%s-%s", appName, n.Name, name)
}

func validateGanesha(context *clusterd.Context, n cephv1.CephNFS) error {
	// core properties
	if n.Name == "" {
		return errors.New("missing name")
	}
	if n.Namespace == "" {
		return errors.New("missing namespace")
	}

	// Client recovery properties
	if n.Spec.RADOS.Pool == "" {
		return errors.New("missing RADOS.pool")
	}
	if n.Spec.RADOS.Namespace == "" {
		return errors.New("missing RADOS.namespace")
	}

	// Ganesha server properties
	if n.Spec.Server.Active == 0 {
		return errors.New("at least one active server required")
	}

	// We cannot run an NFS server if no MDS is running
	// The existence of the pool provided in n.Spec.RADOS.Pool is necessary otherwise addRADOSConfigFile() will fail
	_, err := client.GetPoolDetails(context, n.Namespace, n.Spec.RADOS.Pool)
	if err != nil {
		return errors.Wrapf(err, "pool %s not found, did the filesystem cr successfully complete?", n.Spec.RADOS.Pool)
	}

	return nil
}
