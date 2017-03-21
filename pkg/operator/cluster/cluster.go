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
package cluster

import (
	"fmt"
	"time"

	"k8s.io/client-go/pkg/api/errors"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/cephmgr/client"
	cephmon "github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/api"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/osd"
	"github.com/rook/rook/pkg/operator/rgw"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	defaultPool = "rook"
)

var (
	healthCheckInterval = 10 * time.Second
	logger              = capnslog.NewPackageLogger("github.com/rook/rook-operator", "op-cluster")
)

type Cluster struct {
	factory   client.ConnectionFactory
	clientset kubernetes.Interface
	Metadata  v1.ObjectMeta `json:"metadata,omitempty"`
	Spec      `json:"spec"`
	dataDir   string
	mons      *mon.Cluster
	osds      *osd.Cluster
	apis      *api.Cluster
	rgws      *rgw.Cluster
}

func New(spec Spec, factory client.ConnectionFactory, clientset kubernetes.Interface) *Cluster {
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
	c.mons = mon.New(c.clientset, c.factory, c.Spec.Namespace, c.Spec.DataDirHostPath, c.Spec.Version)
	clusterInfo, err := c.mons.Start()
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	c.apis = api.New(c.clientset, c.Spec.Namespace, c.Spec.Version)
	err = c.apis.Start()
	if err != nil {
		return fmt.Errorf("failed to start the REST api. %+v", err)
	}

	// Start the OSDs
	c.osds = osd.New(c.clientset, c.Spec.Namespace, c.Spec.Version, c.Spec.DeviceFilter, c.Spec.DataDirHostPath, c.Spec.UseAllDevices)
	err = c.osds.Start()
	if err != nil {
		return fmt.Errorf("failed to start the osds. %+v", err)
	}

	err = c.createClientAccess(clusterInfo)
	if err != nil {
		return fmt.Errorf("failed to create client access. %+v", err)
	}

	logger.Infof("Done creating rook instance in namespace %s", c.Spec.Namespace)
	return nil
}

func (c *Cluster) Monitor(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			logger.Infof("Stopping monitoring of cluster %s", c.Spec.Namespace)
			return

		case <-time.After(healthCheckInterval):
			logger.Debugf("checking health of mons")
			err := c.mons.CheckHealth()
			if err != nil {
				logger.Infof("failed to check mon health. %+v", err)
			}
		}
	}
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
