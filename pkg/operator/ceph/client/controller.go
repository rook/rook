/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	"regexp"

	"github.com/pkg/errors"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const ClientSecretName = "-client-key"

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-client")

// ClientResource represents the Client custom resource object
var ClientResource = k8sutil.CustomResource{
	Name:    "cephclient",
	Plural:  "cephclients",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
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

	logger.Infof("start watching client resources in namespace %q", c.namespace)
	go k8sutil.WatchCR(ClientResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient(), &cephv1.CephClient{}, stopCh)

	return nil
}

func (c *ClientController) onAdd(obj interface{}) {
	logger.Infof("new client onAdd() called")

	client, err := getClientObject(obj)
	if err != nil {
		logger.Errorf("failed to get client object. %v", err)
		return
	}

	err = createClient(c.context, client)
	if err != nil {
		logger.Errorf("failed to create client %q. %v", client.ObjectMeta.Name, err)
	}
}

func genClientEntity(p *cephv1.CephClient, context *clusterd.Context) (clientEntity string, caps []string, err error) {
	clientEntity = fmt.Sprintf("client.%s", p.Name)
	caps = []string{}

	// validate the client settings
	if err := ValidateClient(context, p); err != nil {
		return clientEntity, caps, errors.Wrapf(err, "invalid client %q arguments", p.Name)
	}

	for name, cap := range p.Spec.Caps {
		caps = append(caps, name, cap)
	}

	return clientEntity, caps, nil
}

// Create the client
func createClient(context *clusterd.Context, p *cephv1.CephClient) error {
	logger.Infof("creating client %s in namespace %s", p.Name, p.Namespace)

	clientEntity, caps, err := genClientEntity(p, context)
	if err != nil {
		return errors.Wrapf(err, "failed to generate client entity %q", p.Name)
	}

	// Check if client was created manually, create if necessary or update caps and create secret
	key, err := ceph.AuthGetKey(context, p.Namespace, clientEntity)
	if err != nil {
		// Example in pkg/operator/ceph/config/keyring/store.go:65
		key, err = ceph.AuthGetOrCreateKey(context, p.Namespace, clientEntity, caps)
		if err != nil {
			return errors.Wrapf(err, "failed to create client %q", p.Name)
		}
	} else {
		err = ceph.AuthUpdateCaps(context, p.Namespace, clientEntity, caps)
		if err != nil {
			return errors.Wrapf(err, "client %q exists, failed to update client caps", p.Name)
		}
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s%s", p.Name, ClientSecretName),
			Namespace: p.Namespace,
		},
		StringData: map[string]string{
			p.Name: key,
		},
		Type: k8sutil.RookType,
	}

	secretName := secret.ObjectMeta.Name
	_, err = context.Clientset.CoreV1().Secrets(p.Namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("creating secret for %s", secretName)
			if _, err := context.Clientset.CoreV1().Secrets(p.Namespace).Create(secret); err != nil {
				return errors.Wrapf(err, "failed to create secret for %q", secretName)
			}
			return nil
		}
		return errors.Wrapf(err, "failed to get secret for %q", secretName)
	}
	logger.Debugf("updating secret for %s", secretName)
	if _, err := context.Clientset.CoreV1().Secrets(p.Namespace).Update(secret); err != nil {
		return errors.Wrapf(err, "failed to update secret for %q", secretName)
	}

	logger.Infof("created client %s", p.Name)
	return nil
}

func updateClient(context *clusterd.Context, p *cephv1.CephClient) error {
	logger.Infof("updating client %s in namespace %s", p.Name, p.Namespace)

	clientEntity, caps, err := genClientEntity(p, context)
	if err != nil {
		return errors.Wrapf(err, "failed to generate client entity %q", p.Name)
	}

	err = ceph.AuthUpdateCaps(context, p.Namespace, clientEntity, caps)
	if err != nil {
		return errors.Wrapf(err, "failed to update client %q", p.Name)
	}

	logger.Infof("updated client %s", p.Name)
	return nil
}

func (c *ClientController) onUpdate(oldObj, newObj interface{}) {
	oldClient, err := getClientObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old client object. %v", err)
		return
	}
	client, err := getClientObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new client object. %v", err)
		return
	}

	if oldClient.Name != client.Name {
		logger.Errorf("failed to update client %q. name update not allowed", client.Name)
		return
	}
	if !clientChanged(oldClient.Spec, client.Spec) {
		logger.Debugf("client %q not changed", client.Name)
		return
	}

	logger.Infof("updating client %s", client.Name)
	if err := updateClient(c.context, client); err != nil {
		logger.Errorf("failed to update client %q. %v", client.ObjectMeta.Name, err)
	}
}

func (c *ClientController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo) {
	logger.Debugf("No need to update the client after the parent cluster changed")
}

func clientChanged(old, new cephv1.ClientSpec) bool {
	for name, cap := range new.Caps {
		if cap != old.Caps[name] {
			return true
		}
	}
	return false
}

func (c *ClientController) onDelete(obj interface{}) {
	client, err := getClientObject(obj)
	if err != nil {
		logger.Errorf("failed to get client object. %v", err)
		return
	}

	logger.Infof("Going to remove client object %s", client.Name)
	if err := deleteClient(c.context, client); err != nil {
		logger.Errorf("failed to delete client %q. %v", client.ObjectMeta.Name, err)
	}
	logger.Infof("Removed client %s", client.Name)
}

// Delete the client
func deleteClient(context *clusterd.Context, p *cephv1.CephClient) error {
	clientEntity := fmt.Sprintf("client.%s", p.Name)
	if err := ceph.AuthDelete(context, p.Namespace, clientEntity); err != nil {
		return errors.Wrapf(err, "failed to delete client %q", p.Name)
	}
	secretName := fmt.Sprintf("%s-client-key", p.Name)
	if err := context.Clientset.CoreV1().Secrets(p.Namespace).Delete(secretName, &metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
		return errors.Errorf("failed to remote client %q secret %q", p.Name, secretName)
	}

	return nil
}

// Check if the client exists
func clientExists(context *clusterd.Context, p *cephv1.CephClient) (bool, error) {
	_, err := ceph.AuthGetKey(context, p.Namespace, p.Name)
	if err != nil {
		return false, err
	}
	return true, nil
}

// Validate the client arguments
func ValidateClient(context *clusterd.Context, p *cephv1.CephClient) error {
	if p.Name == "" {
		return errors.New("missing name")
	}
	reservedNames := regexp.MustCompile("^admin$|^rgw.*$|^rbd-mirror$|^osd.[0-9]*$|^bootstrap-(mds|mgr|mon|osd|rgw|^rbd-mirror)$")
	if reservedNames.Match([]byte(p.Name)) {
		return errors.Errorf("ignoring reserved name %q", p.Name)
	}
	if p.Namespace == "" {
		return errors.New("missing namespace")
	}
	if err := ValidateClientSpec(context, p.Namespace, &p.Spec); err != nil {
		return err
	}
	return nil
}

// ValidateClientSpec checks if caps were passed for new or updated client
func ValidateClientSpec(context *clusterd.Context, namespace string, p *cephv1.ClientSpec) error {
	if p.Caps == nil {
		return errors.New("no caps specified")
	}
	for _, cap := range p.Caps {
		if cap == "" {
			return errors.New("no caps specified")
		}
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

	return nil, errors.Errorf("not a known client object: %+v", obj)
}
