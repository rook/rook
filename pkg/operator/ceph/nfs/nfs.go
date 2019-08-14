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
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
			return fmt.Errorf("failed to create config. %+v", err)
		}

		err = c.addRADOSConfigFile(n, id)
		if err != nil {
			return fmt.Errorf("failed to create RADOS config object. %+v", err)
		}

		cfg := daemonConfig{
			ID:              id,
			ConfigConfigMap: configName,
			DataPathMap: &config.DataPathMap{
				HostDataDir:      "",                          // nfs daemon does not store data on host, ...
				ContainerDataDir: cephconfig.DefaultConfigDir, // does share data in containers using emptyDir, ...
				HostLogDir:       "",                          // and does not log to /var/log/ceph dir
			},
		}

		// start the deployment
		deployment := c.makeDeployment(n, cfg)
		_, err = c.context.Clientset.AppsV1().Deployments(n.Namespace).Create(deployment)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create ganesha deployment. %+v", err)
			}
			logger.Infof("ganesha deployment %s already exists. updating if needed", deployment.Name)
			// We don't invoke ceph versions here since nfs do not show up in the service map (yet?)
			if err := updateDeploymentAndWait(c.context, deployment, n.Namespace, "nfs", id, c.clusterInfo.CephVersion); err != nil {
				return fmt.Errorf("failed to update ganesha deployment %s. %+v", deployment.Name, err)
			}
		} else {
			logger.Infof("ganesha deployment %s started", deployment.Name)
		}

		// create a service
		err = c.createCephNFSService(n, cfg)
		if err != nil {
			return fmt.Errorf("failed to create ganesha service. %+v", err)
		}

		c.addServerToDatabase(n, id)
	}

	return nil
}

// Create empty config file for new ganesha server
func (c *CephNFSController) addRADOSConfigFile(n cephv1.CephNFS, name string) error {
	nodeID := getNFSNodeID(n, name)
	config := getGaneshaConfigObject(nodeID)
	err := c.context.Executor.ExecuteCommand(false, "", "rados", "--pool", n.Spec.RADOS.Pool, "--namespace", n.Spec.RADOS.Namespace, "stat", config)
	if err == nil {
		// If stat works then we assume it's present already
		return nil
	}
	// try to create it
	return c.context.Executor.ExecuteCommand(false, "", "rados", "--pool", n.Spec.RADOS.Pool, "--namespace", n.Spec.RADOS.Namespace, "create", config)
}

func (c *CephNFSController) addServerToDatabase(nfs cephv1.CephNFS, name string) {
	logger.Infof("Adding ganesha %s to grace db", name)

	if err := c.runGaneshaRadosGrace(nfs, name, "add"); err != nil {
		logger.Errorf("failed to add %s to grace db. %+v", name, err)
	}
}

func (c *CephNFSController) removeServerFromDatabase(nfs cephv1.CephNFS, name string) {
	logger.Infof("Removing ganesha %s from grace db", name)

	if err := c.runGaneshaRadosGrace(nfs, name, "remove"); err != nil {
		logger.Errorf("failed to remove %s from grace db. %+v", name, err)
	}
}

func (c *CephNFSController) runGaneshaRadosGrace(nfs cephv1.CephNFS, name, action string) error {
	nodeID := getNFSNodeID(nfs, name)
	args := []string{"--pool", nfs.Spec.RADOS.Pool, "--ns", nfs.Spec.RADOS.Namespace, action, nodeID}
	return c.context.Executor.ExecuteCommand(false, "", ganeshaRadosGraceCmd, args...)
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
		if errors.IsAlreadyExists(err) {
			if _, err := c.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Update(configMap); err != nil {
				return "", fmt.Errorf("failed to update ganesha config. %+v", err)
			}
			return configMap.Name, nil
		}
		return "", fmt.Errorf("failed to create ganesha config. %+v", err)
	}
	return configMap.Name, nil
}

// Delete the ganesha server
func (c *CephNFSController) downCephNFS(n cephv1.CephNFS, newActive int) error {
	for i := n.Spec.Server.Active - 1; i >= newActive; i-- {
		name := k8sutil.IndexToName(i)

		// Remove from grace db
		c.removeServerFromDatabase(n, name)

		// Delete the mds deployment
		k8sutil.DeleteDeployment(c.context.Clientset, n.Namespace, instanceName(n, name))

		// Delete the ganesha service
		options := &metav1.DeleteOptions{}
		err := c.context.Clientset.CoreV1().Services(n.Namespace).Delete(instanceName(n, name), options)
		if err != nil && !errors.IsNotFound(err) {
			logger.Warningf("failed to delete ganesha service. %+v", err)
		}
	}

	return nil
}

func instanceName(n cephv1.CephNFS, name string) string {
	return fmt.Sprintf("%s-%s-%s", appName, n.Name, name)
}

func validateGanesha(context *clusterd.Context, n cephv1.CephNFS) error {
	// core properties
	if n.Name == "" {
		return fmt.Errorf("missing name")
	}
	if n.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}

	// Client recovery properties
	if n.Spec.RADOS.Pool == "" {
		return fmt.Errorf("missing RADOS.pool")
	}
	if n.Spec.RADOS.Namespace == "" {
		return fmt.Errorf("missing RADOS.namespace")
	}

	// Ganesha server properties
	if n.Spec.Server.Active == 0 {
		return fmt.Errorf("at least one active server required")
	}

	return nil
}
