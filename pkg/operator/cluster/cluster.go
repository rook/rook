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
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/ceph/client"
	cephmon "github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/api"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/mgr"
	"github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/osd"
	"github.com/rook/rook/pkg/operator/rgw"
	rookclient "github.com/rook/rook/pkg/rook/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	crushConfigMapName = "crush-config"
	crushmapCreatedKey = "initialCrushMapCreated"
)

var (
	healthCheckInterval = 10 * time.Second
	clientTimeout       = 15 * time.Second
	logger              = capnslog.NewPackageLogger("github.com/rook/rook", "op-cluster")
)

type Cluster struct {
	context       *clusterd.Context
	v1.ObjectMeta `json:"metadata,omitempty"`
	Spec          `json:"spec"`
	mons          *mon.Cluster
	mgrs          *mgr.Cluster
	osds          *osd.Cluster
	apis          *api.Cluster
	rgws          *rgw.Cluster
	rclient       rookclient.RookRestClient
}

func (c *Cluster) Init(context *clusterd.Context) {
	c.context = context
}

func (c *Cluster) CreateInstance() error {

	// Create the namespace if not already created
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: c.Namespace}}
	_, err := c.context.Clientset.CoreV1().Namespaces().Create(ns)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create namespace %s. %+v", c.Namespace, err)
		}
	}

	// Create a configmap for overriding ceph config settings
	// These settings should only be modified by a user after they are initialized
	placeholderConfig := map[string]string{
		k8sutil.ConfigOverrideVal: "",
	}
	cm := &v1.ConfigMap{Data: placeholderConfig}
	cm.Name = k8sutil.ConfigOverrideName
	_, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create override configmap %s. %+v", c.Namespace, err)
	}

	// Start the mon pods
	c.mons = mon.New(c.context, c.Namespace, c.Spec.DataDirHostPath, c.Spec.VersionTag, c.Spec.Placement.GetMON())
	clusterInfo, err := c.mons.Start()
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	err = c.createInitialCrushMap()
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v", err)
	}

	c.mgrs = mgr.New(c.context, c.Namespace, c.Spec.VersionTag)
	err = c.mgrs.Start()
	if err != nil {
		return fmt.Errorf("failed to start the ceph mgr. %+v", err)
	}

	c.apis = api.New(c.context, c.Namespace, c.Spec.VersionTag, c.Spec.Placement.GetAPI())
	err = c.apis.Start()
	if err != nil {
		return fmt.Errorf("failed to start the REST api. %+v", err)
	}

	// Start the OSDs
	c.osds = osd.New(c.context, c.Namespace, c.Spec.VersionTag, c.Spec.Storage, c.Spec.DataDirHostPath, c.Spec.Placement.GetOSD())
	err = c.osds.Start()
	if err != nil {
		return fmt.Errorf("failed to start the osds. %+v", err)
	}

	err = c.createClientAccess(clusterInfo)
	if err != nil {
		return fmt.Errorf("failed to create client access. %+v", err)
	}

	logger.Infof("Done creating rook instance in namespace %s", c.Namespace)
	return nil
}

func (c *Cluster) Monitor(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			logger.Infof("Stopping monitoring of cluster in namespace %s", c.Namespace)
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

func (c *Cluster) createInitialCrushMap() error {
	configMapExists := false
	createCrushMap := false

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(crushConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// crush config map was not found, meaning we haven't created the initial crush map
		createCrushMap = true
	} else {
		// crush config map was found, look in it to verify we've created the initial crush map
		configMapExists = true
		val, ok := cm.Data[crushmapCreatedKey]
		if !ok {
			createCrushMap = true
		} else if val != "1" {
			createCrushMap = true
		}
	}

	if !createCrushMap {
		// no need to create the crushmap, bail out
		return nil
	}

	logger.Info("creating initial crushmap")
	out, err := client.CreateDefaultCrushMap(c.context, c.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v. output: %s", err, out)
	}

	logger.Info("created initial crushmap")

	// save the fact that we've created the initial crushmap to a configmap
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crushConfigMapName,
			Namespace: c.Namespace,
		},
		Data: map[string]string{crushmapCreatedKey: "1"},
	}

	if !configMapExists {
		if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap); err != nil {
			return fmt.Errorf("failed to create configmap %s: %+v", crushConfigMapName, err)
		}
	} else {
		if _, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(configMap); err != nil {
			return fmt.Errorf("failed to update configmap %s: %+v", crushConfigMapName, err)
		}
	}

	return nil
}

func (c *Cluster) createClientAccess(clusterInfo *cephmon.ClusterInfo) error {
	// create a user for rbd clients
	name := fmt.Sprintf("%s-rook-user", c.Namespace)
	username := fmt.Sprintf("client.%s", name)
	access := []string{"osd", "allow rwx", "mon", "allow r"}

	// get-or-create-key for the user account
	rbdKey, err := client.AuthGetOrCreateKey(c.context, c.Namespace, username, access)
	if err != nil {
		return fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	// store the secret for the rbd user in the default namespace
	secrets := map[string]string{
		"key": rbdKey,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: k8sutil.DefaultNamespace},
		StringData: secrets,
		Type:       k8sutil.RbdType,
	}
	_, err = c.context.Clientset.CoreV1().Secrets(k8sutil.DefaultNamespace).Create(secret)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to save %s secret. %+v", name, err)
		}

		// update the secret in case we have a new cluster
		_, err = c.context.Clientset.CoreV1().Secrets(k8sutil.DefaultNamespace).Update(secret)
		if err != nil {
			return fmt.Errorf("failed to update %s secret. %+v", name, err)
		}
		logger.Infof("updated existing %s secret", name)
	} else {
		logger.Infof("saved %s secret", name)
	}

	return nil
}

func (c *Cluster) GetRookClient() (rookclient.RookRestClient, error) {
	if c.rclient != nil {
		return c.rclient, nil
	}

	// Look up the api service for the given namespace
	logger.Infof("retrieving rook api endpoint for namespace %s", c.Namespace)
	svc, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get(api.DeploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to find the api service. %+v", err)
	}

	httpClient := http.DefaultClient
	httpClient.Timeout = clientTimeout
	endpoint := fmt.Sprintf("%s:%d", svc.Spec.ClusterIP, svc.Spec.Ports[0].Port)
	c.rclient = rookclient.NewRookNetworkRestClient(rookclient.GetRestURL(endpoint), httpClient)
	logger.Infof("rook api endpoint %s for namespace %s", endpoint, c.Namespace)
	return c.rclient, nil
}
