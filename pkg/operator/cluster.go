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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package operator

import (
	"fmt"

	"k8s.io/client-go/pkg/api/errors"

	"github.com/rook/rook/pkg/cephmgr/client"
	cephmon "github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/api"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/osd"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

type Cluster struct {
	factory   client.ConnectionFactory
	clientset kubernetes.Interface
	Metadata  v1.ObjectMeta `json:"metadata,omitempty"`
	Spec      ClusterSpec   `json:"spec"`
	dataDir   string
}

type ClusterSpec struct {
	// The namespace where the the rook resources will all be created.
	Namespace string `json:"namespace"`

	// Version is the expected version of the rook container to run in the cluster.
	// The rook-operator will eventually make the rook cluster version
	// equal to the expected version.
	Version string `json:"version"`

	// Paused is to pause the control of the operator for the rook cluster.
	Paused bool `json:"paused,omitempty"`

	// Whether to consume all the storage devices found on a machine
	UseAllDevices bool `json:"useAllDevices"`

	// A regular expression to allow more fine-grained selection of devices on nodes across the cluster
	DeviceFilter string `json:"deviceFilter"`

	// The path on the host where config and data can be persisted.
	DataDirHostPath string `json:"dataDirHostPath"`
}

func newCluster(spec ClusterSpec, factory client.ConnectionFactory, clientset kubernetes.Interface) *Cluster {
	return &Cluster{
		Spec:      spec,
		factory:   factory,
		clientset: clientset,
		dataDir:   k8sutil.DataDir,
	}
}

func (c *Cluster) CreateInstance() error {

	// Create the namespace if not already created
	ns := &v1.Namespace{ObjectMeta: v1.ObjectMeta{Name: c.Spec.Namespace}}
	_, err := c.clientset.CoreV1().Namespaces().Create(ns)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create namespace %s. %+v", c.Spec.Namespace, err)
		}
	}

	// Start the mon pods
	m := mon.New(c.clientset, c.factory, c.Spec.Namespace, c.Spec.DataDirHostPath, c.Spec.Version)
	cluster, err := m.Start()
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	a := api.New(c.clientset, c.Spec.Namespace, c.Spec.Version)
	err = a.Start(cluster)
	if err != nil {
		return fmt.Errorf("failed to start the REST api. %+v", err)
	}

	// Start the OSDs
	osds := osd.New(c.clientset, c.Spec.Namespace, c.Spec.Version, c.Spec.DeviceFilter, c.Spec.DataDirHostPath, c.Spec.UseAllDevices)
	err = osds.Start(cluster)
	if err != nil {
		return fmt.Errorf("failed to start the osds. %+v", err)
	}

	err = c.createClientAccess(cluster)
	if err != nil {
		return fmt.Errorf("failed to create client access. %+v", err)
	}

	logger.Infof("Done creating rook instance in namespace %s", c.Spec.Namespace)
	return nil
}

func (c *Cluster) createClientAccess(clusterInfo *cephmon.ClusterInfo) error {
	context := &clusterd.Context{ConfigDir: c.dataDir}
	conn, err := cephmon.ConnectToClusterAsAdmin(context, c.factory, clusterInfo)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster: %+v", err)
	}
	defer conn.Shutdown()

	// create the default rook pool
	pool := client.CephStoragePoolDetails{Name: defaultPool}
	_, err = client.CreatePool(conn, pool)
	if err != nil {
		return fmt.Errorf("failed to create default rook pool: %+v", err)
	}

	// create a user for rbd clients
	username := "client.rook-rbd-user"
	access := []string{"osd", "allow rwx", "mon", "allow r"}

	// get-or-create-key for the user account
	rbdKey, err := client.AuthGetOrCreateKey(conn, username, access)
	if err != nil {
		return fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	// store the secret for the rbd user in the default namespace
	name := fmt.Sprintf("%s-rbd-user", c.Spec.Namespace)
	secrets := map[string]string{
		"key": rbdKey,
	}
	secret := &v1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: name, Namespace: k8sutil.DefaultNamespace},
		StringData: secrets,
		Type:       k8sutil.RbdType,
	}
	_, err = c.clientset.CoreV1().Secrets(k8sutil.DefaultNamespace).Create(secret)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to save %s secret. %+v", name, err)
		}

		// update the secret in case we have a new cluster
		_, err = c.clientset.CoreV1().Secrets(k8sutil.DefaultNamespace).Update(secret)
		if err != nil {
			return fmt.Errorf("failed to update %s secret. %+v", name, err)
		}
		logger.Infof("updated existing %s secret", name)
	} else {
		logger.Infof("saved %s secret", name)
	}

	return nil
}
