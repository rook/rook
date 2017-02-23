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
*/
package operator

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"

	"github.com/rook/rook/pkg/cephmgr/client"
	cephmon "github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/api"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/osd"
	"github.com/rook/rook/pkg/operator/rgw"
)

const (
	defaultPool = "rook"
)

type Operator struct {
	Namespace        string
	MasterHost       string
	containerVersion string
	clientset        *kubernetes.Clientset
	waitCluster      sync.WaitGroup
	factory          client.ConnectionFactory
	useAllDevices    bool
}

func New(namespace string, factory client.ConnectionFactory, clientset *kubernetes.Clientset, containerVersion string, useAllDevices bool) *Operator {
	return &Operator{
		Namespace:        namespace,
		factory:          factory,
		clientset:        clientset,
		containerVersion: containerVersion,
		useAllDevices:    useAllDevices,
	}
}

func (o *Operator) Run() error {

	// Start the mon pods
	m := mon.New(o.Namespace, o.factory, o.containerVersion)
	cluster, err := m.Start(o.clientset)
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	a := api.New(o.Namespace, o.containerVersion)
	err = a.Start(o.clientset, cluster)
	if err != nil {
		return fmt.Errorf("failed to start the REST api. %+v", err)
	}

	// Start the OSDs
	osds := osd.New(o.Namespace, o.containerVersion, o.useAllDevices)
	err = osds.Start(o.clientset, cluster)
	if err != nil {
		return fmt.Errorf("failed to start the osds. %+v", err)
	}

	// Start the object store
	r := rgw.New(o.Namespace, o.containerVersion, o.factory)
	err = r.Start(o.clientset, cluster)
	if err != nil {
		return fmt.Errorf("failed to start rgw. %+v", err)
	}

	err = o.createClientAccess(cluster)
	if err != nil {
		return fmt.Errorf("failed to create client access. %+v", err)
	}

	logger.Infof("DONE!")
	<-time.After(1000000 * time.Second)

	return nil
}

func (o *Operator) createClientAccess(clusterInfo *cephmon.ClusterInfo) error {
	context := &clusterd.Context{}
	conn, err := cephmon.ConnectToClusterAsAdmin(context, o.factory, clusterInfo)
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
	secrets := map[string]string{
		"key": rbdKey,
	}
	secret := &v1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: "rook-rbd-user"},
		StringData: secrets,
		Type:       k8sutil.RbdType,
	}
	_, err = o.clientset.Secrets(k8sutil.DefaultNamespace).Create(secret)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return fmt.Errorf("failed to save rook-rbd-user secret. %+v", err)
		}

		// update the secret in case we have a new cluster
		_, err = o.clientset.Secrets(k8sutil.DefaultNamespace).Update(secret)
		if err != nil {
			return fmt.Errorf("failed to update rook-rbd-user secret. %+v", err)
		}
		logger.Infof("updated existing rook-rbd-user secret")
	} else {
		logger.Infof("saved rook-rbd-user secret")
	}

	return nil
}
