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

// Package client to manage a rook client.
package client

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-client")

// ClientResource represents the Client custom resource object
var ClientResource = opkit.CustomResource{
	Name:    "cephclient",
	Plural:  "cephclients",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1.CephClient{}).Name(),
}

// ClientController represents a controller object for client custom resources
type ClientController struct {
	context   *clusterd.Context
	namespace string
}

// NewClientController create controller for watching client custom resources created
func NewClientController(context *clusterd.Context, namespace string) *ClientController {
	return &ClientController{
		context:   context,
		namespace: namespace,
	}
}

// Watch watches for instances of Client custom resources and acts on them
func (c *ClientController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching client resources in namespace %s", c.namespace)
	watcher := opkit.NewWatcher(ClientResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient())
	go watcher.Watch(&cephv1.CephClient{}, stopCh)

	return nil
}

func (c *ClientController) onAdd(obj interface{}) {
	logger.Infof("new client onAdd() called")

	client, err := getClientObject(obj)
	if err != nil {
		logger.Errorf("failed to get client object: %+v", err)
		return
	}

	err = createClient(c, client)
	if err != nil {
		logger.Errorf("failed to create client %s. %+v", client.ObjectMeta.Name, err)
	}
}

// Create the client
func createClient(c *ClientController, p *cephv1.CephClient) error {
	// validate the client settings
	if err := ValidateClient(c.context, p); err != nil {
		return fmt.Errorf("invalid client %s arguments. %+v", p.Name, err)
	}

	// create the client
	logger.Infof("creating client %s in namespace %s", p.Name, p.Namespace)

	clientEntity := fmt.Sprintf("client.%s", p.Name)
	caps := []string{}
	if p.Spec.Caps.Osd != "" {
		caps = append(caps, "osd", p.Spec.Caps.Osd)
	}
	if p.Spec.Caps.Mon != "" {
		caps = append(caps, "mon", p.Spec.Caps.Mon)
	}
	if p.Spec.Caps.Mds != "" {
		caps = append(caps, "mds", p.Spec.Caps.Mds)
	}

	// Example in pkg/operator/ceph/config/keyring/store.go:65
	key, err := ceph.AuthGetOrCreateKey(c.context, p.Namespace, clientEntity, caps)
	if err != nil {
		return fmt.Errorf("failed to create client %s. %+v", p.Name, err)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-client-key", p.Name),
			Namespace: p.Namespace,
		},
		StringData: map[string]string{
			p.Name: key,
		},
		Type: k8sutil.RookType,
	}

	secretName := secret.ObjectMeta.Name
	_, err = c.context.Clientset.CoreV1().Secrets(p.Namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("creating secret for %s", secretName)
			if _, err := c.context.Clientset.CoreV1().Secrets(p.Namespace).Create(secret); err != nil {
				return fmt.Errorf("failed to create secret for %s. %+v", secretName, err)
			}
			return nil
		}
		return fmt.Errorf("failed to get secret for %s. %+v", secretName, err)
	}
	logger.Debugf("updating secret for %s", secretName)
	if _, err := c.context.Clientset.CoreV1().Secrets(p.Namespace).Update(secret); err != nil {
		return fmt.Errorf("failed to update secret for %s. %+v", secretName, err)
	}

	logger.Infof("created client %s", p.Name)
	return nil
}

func updateClient(context *clusterd.Context, p *cephv1.CephClient) error {
	// validate the client settings
	if err := ValidateClient(context, p); err != nil {
		return fmt.Errorf("invalid client %s arguments. %+v", p.Name, err)
	}

	// update the client
	logger.Infof("updating client %s in namespace %s", p.Name, p.Namespace)

	clientEntity := fmt.Sprintf("client.%s", p.Name)
	caps := []string{}
	if p.Spec.Caps.Osd != "" {
		caps = append(caps, "osd", p.Spec.Caps.Osd)
	}
	if p.Spec.Caps.Mon != "" {
		caps = append(caps, "mon", p.Spec.Caps.Mon)
	}
	if p.Spec.Caps.Mds != "" {
		caps = append(caps, "mds", p.Spec.Caps.Mds)
	}

	err := ceph.AuthUpdateCaps(context, p.Namespace, clientEntity, caps)
	if err != nil {
		return fmt.Errorf("failed to update client %s. %+v", p.Name, err)
	}

	logger.Infof("updated client %s", p.Name)
	return nil
}

func (c *ClientController) onUpdate(oldObj, newObj interface{}) {
	oldClient, err := getClientObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old client object: %+v", err)
		return
	}
	client, err := getClientObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new client object: %+v", err)
		return
	}

	if oldClient.Name != client.Name {
		logger.Errorf("failed to update client %s. name update not allowed", client.Name)
		return
	}
	if !clientChanged(oldClient.Spec, client.Spec) {
		logger.Debugf("client %s not changed", client.Name)
		return
	}

	logger.Infof("updating client %s", client.Name)
	if err := updateClient(c.context, client); err != nil {
		logger.Errorf("failed to update client %s. %+v", client.ObjectMeta.Name, err)
	}
}

func (c *ClientController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo) {
	logger.Debugf("No need to update the client after the parent cluster changed")
}

func clientChanged(old, new cephv1.ClientSpec) bool {
	if old.Caps.Mon != new.Caps.Mon {
		logger.Infof("client mon caps changed from '%s' to '%s'", old.Caps.Mon, new.Caps.Mon)
		return true
	}
	if old.Caps.Osd != new.Caps.Osd {
		logger.Infof("client osd caps changed from '%s' to '%s'", old.Caps.Osd, new.Caps.Osd)
		return true
	}
	if old.Caps.Mon != new.Caps.Mon {
		logger.Infof("client mds caps changed from '%s' to '%s'", old.Caps.Mds, new.Caps.Mds)
		return true
	}

	return false
}

func (c *ClientController) onDelete(obj interface{}) {
	client, err := getClientObject(obj)
	if err != nil {
		logger.Errorf("failed to get client object: %+v", err)
		return
	}

	logger.Infof("Going to remove client object %s", client.Name)
	if err := deleteClient(c.context, client); err != nil {
		logger.Errorf("failed to delete client %s. %+v", client.ObjectMeta.Name, err)
	}
	logger.Infof("Removed client %s", client.Name)
}

// Delete the client
func deleteClient(context *clusterd.Context, p *cephv1.CephClient) error {
	clientEntity := fmt.Sprintf("client.%s", p.Name)
	if err := ceph.AuthDelete(context, p.Namespace, clientEntity); err != nil {
		return fmt.Errorf("failed to delete client '%s'. %+v", p.Name, err)
	}
	// TODO Remove corresponding secret as well
	secretName := fmt.Sprintf("%s-client-key", p.Name)
	if err := context.Clientset.CoreV1().Secrets(p.Namespace).Delete(secretName, &metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("failed to remote client %s secret %s", p.Name, secretName)
	}

	return nil
}

// Check if the client exists
func clientExists(context *clusterd.Context, p *cephv1.CephClient) (bool, error) {
	// TODO implement get clients
	clients, err := ceph.GetPools(context, p.Namespace)
	if err != nil {
		return false, err
	}
	for _, client := range clients {
		if client.Name == p.Name {
			return true, nil
		}
	}
	return false, nil
}

// Validate the client arguments
func ValidateClient(context *clusterd.Context, p *cephv1.CephClient) error {
	if p.Name == "" {
		return fmt.Errorf("missing name")
	}
	if p.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if err := ValidateClientSpec(context, p.Namespace, &p.Spec); err != nil {
		return err
	}
	return nil
}

func ValidateClientSpec(context *clusterd.Context, namespace string, p *cephv1.ClientSpec) error {
	if p.Caps.Mon == "" && p.Caps.Osd == "" && p.Caps.Mds == "" {
		return fmt.Errorf("no caps specified")
	}
	return nil
}

func getClientObject(obj interface{}) (client *cephv1.CephClient, err error) {
	var ok bool
	client, ok = obj.(*cephv1.CephClient)
	if ok {
		// the client object is of the latest type, simply return it
		return client.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known client object: %+v", obj)
}
