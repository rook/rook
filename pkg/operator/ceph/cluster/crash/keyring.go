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

package crash

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	crashKeyringTemplate = `
[client.crash]
	key = %s
	caps mon = "allow profile crash"
	caps mgr = "allow profile crash"
`
)

// CreateCrashCollectorSecret creates the Kubernetes Crash Collector Secret
func CreateCrashCollectorSecret(context *clusterd.Context, clusterName string, ownerRef *metav1.OwnerReference) error {
	k := keyring.GetSecretStore(context, clusterName, ownerRef)

	// Create CrashCollector Ceph key
	crashCollectorSecretKey, err := createCrashCollectorKeyring(k)
	if err != nil {
		return errors.Wrapf(err, "failed to create %q ceph keyring", crashCollectorKeyringUsername)
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateCrashCollectorSecret(clusterName, crashCollectorSecretKey, k, ownerRef); err != nil {
		return errors.Wrapf(err, "failed to create kubernetes csi secret")
	}

	return nil
}

func cephCrashCollectorKeyringCaps() []string {
	return []string{
		"mon", "allow profile crash",
		"mgr", "allow profile crash",
	}
}

func createCrashCollectorKeyring(s *keyring.SecretStore) (string, error) {
	key, err := s.GenerateKey(crashCollectorKeyringUsername, cephCrashCollectorKeyringCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createOrUpdateCrashCollectorSecret(namespace, crashCollectorSecretKey string, k *keyring.SecretStore, ownerRef *metav1.OwnerReference) error {

	keyring := fmt.Sprintf(crashKeyringTemplate, crashCollectorSecretKey)

	crashCollectorSecret := map[string][]byte{
		"keyring": []byte(keyring),
	}

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crashCollectorSecretName,
			Namespace: namespace,
		},
		Data: crashCollectorSecret,
		Type: k8sutil.RookType,
	}
	k8sutil.SetOwnerRef(&s.ObjectMeta, ownerRef)

	// Create Kubernetes Secret
	err := k.CreateSecret(s)
	if err != nil {
		return errors.Wrapf(err, "failed to create kubernetes secret %q for cluster %q", crashCollectorSecret, namespace)
	}

	logger.Infof("created kubernetes crash collector secret for cluster %q", namespace)
	return nil
}
