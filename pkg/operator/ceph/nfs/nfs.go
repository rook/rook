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

	"github.com/banzaicloud/k8s-objectmatcher/patch"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
func (r *ReconcileCephNFS) upCephNFS(n *cephv1.CephNFS, oldActive int) error {
	for i := oldActive; i < n.Spec.Server.Active; i++ {
		id := k8sutil.IndexToName(i)

		configName, err := r.createConfigMap(n, id)
		if err != nil {
			return errors.Wrap(err, "failed to create config")
		}

		err = r.addRADOSConfigFile(n, id)
		if err != nil {
			return errors.Wrap(err, "failed to create RADOS config object")
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

		// create the deployment
		deployment := r.makeDeployment(n, cfg)

		// Set owner ref to cephNFS object
		err = controllerutil.SetControllerReference(n, deployment, r.scheme)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference for ceph nfs %q secret", deployment.Name)
		}

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(deployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set annotation for deployment %q", deployment.Name)
		}

		// start the deployment
		_, err = r.context.Clientset.AppsV1().Deployments(n.Namespace).Create(deployment)
		if err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "failed to create ceph nfs deployment")
			}
			logger.Infof("ceph nfs deployment %q already exists. updating if needed", deployment.Name)
			if err := updateDeploymentAndWait(r.context, deployment, n.Namespace, "nfs", id, r.cephClusterSpec.SkipUpgradeChecks, false); err != nil {
				return errors.Wrapf(err, "failed to update ceph nfs deployment %q", deployment.Name)
			}
		} else {
			logger.Infof("ceph nfs deployment %q started", deployment.Name)
		}

		// create a service
		err = r.createCephNFSService(n, cfg)
		if err != nil {
			return errors.Wrap(err, "failed to create ceph nfs service")
		}

		// Add server to database
		err = r.addServerToDatabase(n, id)
		if err != nil {
			return errors.Wrapf(err, "failed to add server %q to database", id)
		}
	}

	return nil
}

// Create empty config file for new ganesha server
func (r *ReconcileCephNFS) addRADOSConfigFile(n *cephv1.CephNFS, name string) error {
	nodeID := getNFSNodeID(n, name)
	config := getGaneshaConfigObject(nodeID)
	cmd := "rados"
	args := []string{
		"--pool", n.Spec.RADOS.Pool,
		"--namespace", n.Spec.RADOS.Namespace,
		"--conf", cephclient.CephConfFilePath(r.context.ConfigDir, n.Namespace),
	}
	err := r.context.Executor.ExecuteCommand(cmd, append(args, "stat", config)...)
	if err == nil {
		// If stat works then we assume it's present already
		return nil
	}

	// try to create it
	return r.context.Executor.ExecuteCommand(cmd, append(args, "create", config)...)
}

func (r *ReconcileCephNFS) addServerToDatabase(nfs *cephv1.CephNFS, name string) error {
	logger.Infof("adding ganesha %q to grace db", name)

	if err := r.runGaneshaRadosGrace(nfs, name, "add"); err != nil {
		return errors.Wrapf(err, "failed to add %q to grace db", name)
	}

	return nil
}

func (r *ReconcileCephNFS) removeServerFromDatabase(nfs *cephv1.CephNFS, name string) {
	logger.Infof("removing ganesha %q from grace db", name)

	if err := r.runGaneshaRadosGrace(nfs, name, "remove"); err != nil {
		logger.Errorf("failed to remove %q from grace db. %v", name, err)
	}
}

func (r *ReconcileCephNFS) runGaneshaRadosGrace(nfs *cephv1.CephNFS, name, action string) error {
	nodeID := getNFSNodeID(nfs, name)
	cmd := ganeshaRadosGraceCmd
	args := []string{"--pool", nfs.Spec.RADOS.Pool, "--ns", nfs.Spec.RADOS.Namespace, action, nodeID}
	env := []string{fmt.Sprintf("CEPH_CONF=%s", cephclient.CephConfFilePath(r.context.ConfigDir, nfs.Namespace))}

	return r.context.Executor.ExecuteCommandWithEnv(env, cmd, args...)
}

func (r *ReconcileCephNFS) generateConfigMap(n *cephv1.CephNFS, name string) *v1.ConfigMap {

	data := map[string]string{
		"config": getGaneshaConfig(n, name),
	}
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", AppName, n.Name, name),
			Namespace: n.Namespace,
			Labels:    getLabels(n, name),
		},
		Data: data,
	}

	return configMap
}

func (r *ReconcileCephNFS) createConfigMap(n *cephv1.CephNFS, name string) (string, error) {
	// Generate configMap
	configMap := r.generateConfigMap(n, name)

	// Set owner reference
	err := controllerutil.SetControllerReference(n, configMap, r.scheme)
	if err != nil {
		return "", errors.Wrapf(err, "failed to set owner reference for ceph nfs %q config map", configMap.Name)
	}

	if _, err := r.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Create(configMap); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return "", errors.Wrap(err, "failed to create ganesha config map")
		}

		logger.Debugf("updating config map %q that already exists", configMap.Name)
		if _, err = r.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Update(configMap); err != nil {
			return "", errors.Wrap(err, "failed to update ganesha config map")
		}
	}

	return configMap.Name, nil
}

// Down scale the ganesha server
func (r *ReconcileCephNFS) downCephNFS(n *cephv1.CephNFS, nfsServerListNum int) error {
	diffCount := nfsServerListNum - n.Spec.Server.Active
	for i := 0; i < diffCount; {
		depIDToRemove := nfsServerListNum - 1

		name := k8sutil.IndexToName(depIDToRemove)
		depNameToRemove := instanceName(n, name)

		// Remove deployment
		logger.Infof("removing deployment %q", depNameToRemove)
		err := r.context.Clientset.AppsV1().Deployments(n.Namespace).Delete(depNameToRemove, &metav1.DeleteOptions{})
		if err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "failed to delete ceph nfs deployment")
			}
		}

		// Remove from grace db
		r.removeServerFromDatabase(n, name)

		nfsServerListNum = nfsServerListNum - 1
		i++
	}

	return nil
}

func (r *ReconcileCephNFS) removeServersFromDatabase(n *cephv1.CephNFS, newActive int) error {
	for i := n.Spec.Server.Active - 1; i >= newActive; i-- {
		name := k8sutil.IndexToName(i)
		r.removeServerFromDatabase(n, name)
	}

	return nil
}
func instanceName(n *cephv1.CephNFS, name string) string {
	return fmt.Sprintf("%s-%s-%s", AppName, n.Name, name)
}

func validateGanesha(context *clusterd.Context, n *cephv1.CephNFS) error {
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
		return errors.Wrapf(err, "pool %q not found, did the filesystem cr successfully complete?", n.Spec.RADOS.Pool)
	}

	return nil
}
