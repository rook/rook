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
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/log"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// Default RADOS pool name after the NFS changes in Ceph
	nfsDefaultPoolName = ".nfs"

	// CephNFSNameLabelKey is the label key that contains the name of the CephNFS resource
	CephNFSNameLabelKey = "ceph_nfs"
)

var updateDeploymentAndWait = opmon.UpdateCephDeploymentAndWait

type daemonConfig struct {
	ID                  string              // letter ID of daemon (e.g., a, b, c, ...)
	ConfigConfigMap     string              // name of configmap holding config
	ConfigConfigMapHash string              // hash of configmap holding config
	DataPathMap         *config.DataPathMap // location to store data in container
}

// Create the ganesha server
func (r *ReconcileCephNFS) upCephNFS(n *cephv1.CephNFS) error {
	nsName := controller.NsName(n.Namespace, n.Name)
	nfsToSkipReconcile, err := controller.GetDaemonsToSkipReconcile(r.clusterInfo.Context, r.context, n.Namespace, config.NfsType, AppName)
	if err != nil {
		return errors.Wrap(err, "failed to check for NFS daemons to skip reconcile")
	}

	for i := 0; i < n.Spec.Server.Active; i++ {
		id := k8sutil.IndexToName(i)

		if nfsToSkipReconcile.Has(fmt.Sprintf("%s-%s", n.Name, id)) {
			log.NamedWarning(nsName, logger, "skipping reconcile of nfs daemon %q with label %q", id, cephv1.SkipReconcileLabelKey)
			continue
		}

		configName, configHash, err := r.createConfigMap(n, id)
		if err != nil {
			return errors.Wrap(err, "failed to create config")
		}

		err = r.addRADOSConfigFile(n)
		if err != nil {
			return errors.Wrap(err, "failed to create RADOS config object")
		}

		if err := r.setRadosConfig(n); err != nil {
			return errors.Wrap(err, "failed to set RADOS config options")
		}

		cfg := daemonConfig{
			ID:                  id,
			ConfigConfigMap:     configName,
			ConfigConfigMapHash: configHash,
			DataPathMap: &config.DataPathMap{
				HostDataDir:        "",                          // nfs daemon does not store data on host, ...
				ContainerDataDir:   cephclient.DefaultConfigDir, // does share data in containers using emptyDir, ...
				HostLogAndCrashDir: "",                          // and does not log to /var/log/ceph dir
			},
		}

		err = r.generateKeyring(n, id)
		if err != nil {
			return errors.Wrapf(err, "failed to generate keyring for %q", id)
		}

		// create the deployment
		deployment, err := r.makeDeployment(n, cfg)
		if err != nil {
			return errors.Wrap(err, "failed to set to create deployment")
		}
		// Set owner ref to cephNFS object
		err = controllerutil.SetControllerReference(n, deployment, r.scheme)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference for ceph nfs deployment %q", deployment.Name)
		}

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(deployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set annotation for deployment %q", deployment.Name)
		}

		// start the deployment
		_, err = r.context.Clientset.AppsV1().Deployments(n.Namespace).Create(r.opManagerContext, deployment, metav1.CreateOptions{})
		if err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "failed to create ceph nfs deployment")
			}
			log.NamedInfo(nsName, logger, "ceph nfs deployment %q already exists. updating if needed", deployment.Name)
			if err := updateDeploymentAndWait(r.context, r.clusterInfo, deployment, "nfs", id, r.cephClusterSpec.SkipUpgradeChecks, false); err != nil {
				return errors.Wrapf(err, "failed to update ceph nfs deployment %q", deployment.Name)
			}
		} else {
			log.NamedInfo(nsName, logger, "ceph nfs deployment %q started", deployment.Name)
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
func (r *ReconcileCephNFS) addRADOSConfigFile(n *cephv1.CephNFS) error {
	config := getGaneshaConfigObject(n)

	flags := []string{
		"--pool", n.Spec.RADOS.Pool,
		"--namespace", n.Spec.RADOS.Namespace,
	}

	cmd := cephclient.NewRadosCommand(r.context, r.clusterInfo, append(flags, "stat", config))
	_, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err == nil {
		// If stat works then we assume it's present already
		return nil
	}

	// try to create it
	cmd = cephclient.NewRadosCommand(r.context, r.clusterInfo, append(flags, "create", config))
	_, err = cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrap(err, "failed to create initial rados config object")
	}
	return nil
}

func (r *ReconcileCephNFS) addServerToDatabase(nfs *cephv1.CephNFS, name string) error {
	nsName := controller.NsName(nfs.Namespace, nfs.Name)
	log.NamedInfo(nsName, logger, "adding ganesha %q to grace db", name)

	if err := r.runGaneshaRadosGrace(nfs, name, "add"); err != nil {
		return errors.Wrapf(err, "failed to add %q to grace db", name)
	}

	return nil
}

func (r *ReconcileCephNFS) removeServerFromDatabase(nfs *cephv1.CephNFS, name string) {
	nsName := controller.NsName(nfs.Namespace, nfs.Name)
	log.NamedInfo(nsName, logger, "removing ganesha %q from grace db", name)

	if err := r.runGaneshaRadosGrace(nfs, name, "remove"); err != nil {
		log.NamedError(nsName, logger, "failed to remove %q from grace db. %v", name, err)
	}
}

func (r *ReconcileCephNFS) runGaneshaRadosGrace(nfs *cephv1.CephNFS, name, action string) error {
	nodeID := getNFSNodeID(nfs, name)
	args := []string{"--pool", nfs.Spec.RADOS.Pool, "--ns", nfs.Spec.RADOS.Namespace, action, nodeID}
	cmd := cephclient.NewGaneshaRadosGraceCommand(r.context, r.clusterInfo, args)
	_, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	return err
}

func (r *ReconcileCephNFS) generateConfigMap(n *cephv1.CephNFS, name string) *v1.ConfigMap {
	data := map[string]string{
		"config": getGaneshaConfig(n, r.clusterInfo.CephVersion, name),
	}
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(n, name),
			Namespace: n.Namespace,
			Labels:    getLabels(n, name, true),
		},
		Data: data,
	}

	return configMap
}

// return the name of the configmap, plus a hash of the data
func (r *ReconcileCephNFS) createConfigMap(n *cephv1.CephNFS, name string) (string, string, error) {
	// Generate configMap
	configMap := r.generateConfigMap(n, name)
	nsName := controller.NsName(n.Namespace, n.Name)

	// Set owner reference
	err := controllerutil.SetControllerReference(n, configMap, r.scheme)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to set owner reference for ceph ganesha configmap %q", configMap.Name)
	}

	if _, err := r.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Create(r.opManagerContext, configMap, metav1.CreateOptions{}); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return "", "", errors.Wrap(err, "failed to create ganesha config map")
		}

		log.NamedDebug(nsName, logger, "updating config map %q that already exists", configMap.Name)
		if _, err = r.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Update(r.opManagerContext, configMap, metav1.UpdateOptions{}); err != nil {
			return "", "", errors.Wrap(err, "failed to update ganesha config map")
		}
	}

	return configMap.Name, k8sutil.Hash(fmt.Sprintf("%v", configMap.Data)), nil
}

// Down scale the ganesha server
func (r *ReconcileCephNFS) downCephNFS(n *cephv1.CephNFS, nfsServerListNum int) error {
	for i := nfsServerListNum - 1; i >= n.Spec.Server.Active; i-- {
		idToRemove := i

		name := k8sutil.IndexToName(idToRemove)
		resourceName := instanceName(n, name) // shared by deployment, service, and configmap

		// Remove service
		err := r.context.Clientset.CoreV1().Services(n.Namespace).Delete(r.opManagerContext, resourceName, metav1.DeleteOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete ceph nfs service %q", resourceName)
			}
		}

		// Remove configmap
		err = r.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Delete(r.opManagerContext, resourceName, metav1.DeleteOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete ceph nfs config map %q", resourceName)
			}
		}

		// Remove from grace db
		r.removeServerFromDatabase(n, name)

		// Remove deployment
		// since we list deployments to determine what to remove, have to remove deployment last
		err = r.context.Clientset.AppsV1().Deployments(n.Namespace).Delete(r.opManagerContext, resourceName, metav1.DeleteOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete ceph nfs deployment %q", resourceName)
			}
		}
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

func validateGanesha(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, n *cephv1.CephNFS) error {
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

	return nil
}

// create and enable default RADOS pool
func (r *ReconcileCephNFS) configureNFSPool(n *cephv1.CephNFS) error {
	poolName := n.Spec.RADOS.Pool
	nsName := controller.NsName(n.Namespace, n.Name)
	log.NamedInfo(nsName, logger, "configuring pool %q for nfs", poolName)

	args := []string{"osd", "pool", "create", poolName, "--yes-i-really-mean-it"}

	output, err := cephclient.NewCephCommand(r.context, r.clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create default NFS pool %q. %s", poolName, string(output))
	}

	args = []string{"osd", "pool", "application", "enable", poolName, "nfs", "--yes-i-really-mean-it"}
	_, err = cephclient.NewCephCommand(r.context, r.clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable application 'nfs' on pool %q", poolName)
	}

	log.NamedInfo(nsName, logger, "set pool %q for the application nfs", poolName)
	return nil
}
