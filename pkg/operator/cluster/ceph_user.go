/*
Copyright 2017 The Rook Authors. All rights reserved.

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

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type cephUser struct {
	// secretName is Kubernetes secret to store Ceph token
	secretName string

	// context Rook cluster context
	context *clusterd.Context

	// targetNamespace is a namespace that uses rook resources
	targetNamespace string

	// clusternamespace is a namespace of rook cluster for provisioning resources
	clusterNamespace string

	// key is ceph user token for targetNamespace
	key string
}

func newCephUser(context *clusterd.Context, ns, tns string) *cephUser {
	n := fmt.Sprintf("%s-rook-user", ns)
	return &cephUser{
		secretName:       n,
		targetNamespace:  tns,
		clusterNamespace: ns,
		context:          context,
	}
}

func (cu *cephUser) create() (string, error) {
	var (
		access   = []string{"osd", "allow rwx", "mon", "allow r"}
		username = "client." + cu.targetNamespace
	)

	key, err := client.AuthGetOrCreateKey(cu.context, cu.clusterNamespace, username, access)
	if err != nil {
		return key, fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	cu.key = key

	return key, nil
}

func (cu *cephUser) setKubeSecret() error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cu.secretName,
			Namespace: cu.targetNamespace,
		},
		StringData: map[string]string{"key": cu.key},
		Type:       k8sutil.RbdType,
	}

	_, err := cu.context.Clientset.CoreV1().Secrets(cu.targetNamespace).Create(secret)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to save %s secret. %+v", cu.secretName, err)
		}

		// update the secret in case we have a new cluster
		_, err = cu.context.Clientset.CoreV1().Secrets(cu.targetNamespace).Update(secret)
		if err != nil {
			return fmt.Errorf("failed to update %s secret. %+v", cu.secretName, err)
		}
		logger.Infof("updated existing %s secret", cu.secretName)
	} else {
		logger.Infof("saved %s secret", cu.secretName)
	}

	return nil
}
