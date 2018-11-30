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

// Package spec provides Kubernetes controller/pod/container spec items used for many Ceph daemons
package spec

import (
	"fmt"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KeyringSecretKeyName = "keyring"
)

type KeyringConfig struct {
	Namespace    string
	ResourceName string
	DaemonName   string
	OwnerRef     metav1.OwnerReference
	Username     string
	Access       []string
}

func CreateKeyring(context *clusterd.Context, config KeyringConfig) error {
	_, err := context.Clientset.CoreV1().Secrets(config.Namespace).Get(config.ResourceName, metav1.GetOptions{})
	if err == nil {
		logger.Infof("the keyring %s was already generated", config.ResourceName)
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get secret %s. %+v", config.ResourceName, err)
	}

	// get-or-create-key for the user account
	keyring, err := client.AuthGetOrCreateKey(context, config.Namespace, config.Username, config.Access)
	if err != nil {
		return fmt.Errorf("failed to get or create auth key for %s. %+v", config.Username, err)
	}

	// Store the keyring in a secret
	secrets := map[string]string{
		KeyringSecretKeyName: keyring,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ResourceName,
			Namespace: config.Namespace,
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	k8sutil.SetOwnerRef(context.Clientset, config.Namespace, &secret.ObjectMeta, &config.OwnerRef)

	_, err = context.Clientset.CoreV1().Secrets(config.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save mirroring secret. %+v", err)
	}

	return nil
}
